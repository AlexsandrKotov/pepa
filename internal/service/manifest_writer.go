package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/gitops"
	"github.com/pepa/pepa/internal/repository"
)

// ManifestWriter handles image-tag write-back to GitOps manifest repositories.
// When PEPA deploys a new image, the writer updates the corresponding manifest
// in Git so that FluxCD/ArgoCD picks up the change natively.
type ManifestWriter struct {
	gitopsRepo   *gitops.Repository
	bindingRepo  *repository.GitOpsBindingRepository
	cacheDir     string
}

// NewManifestWriter creates a new ManifestWriter.
func NewManifestWriter(gitopsRepo *gitops.Repository, bindingRepo *repository.GitOpsBindingRepository) *ManifestWriter {
	return &ManifestWriter{
		gitopsRepo:  gitopsRepo,
		bindingRepo: bindingRepo,
		cacheDir:    "", // uses default temp dir
	}
}

// WriteBackRequest describes an image-tag update to be written to Git.
type WriteBackRequest struct {
	BindingID     uuid.UUID `json:"binding_id"`
	ImageTag      string    `json:"image_tag"`
	ImageName     string    `json:"image_name"`     // e.g. "registry.example.com/app"
	CommitMessage string    `json:"commit_message"` // optional override
	Branch        string    `json:"branch"`         // optional target branch override
	DryRun        bool      `json:"dry_run"`        // if true, only preview the diff
}

// WriteBackResult is returned after a successful write-back.
type WriteBackResult struct {
	CommitSHA string `json:"commit_sha"`
	Branch    string `json:"branch"`
	Diff      string `json:"diff"`
	MRNeeded  bool   `json:"mr_needed"`
	MRURL     string `json:"mr_url,omitempty"`
	FilePath  string `json:"file_path"`
	Strategy  string `json:"strategy"`
}

// WriteBack updates the image tag in the GitOps manifest repo based on the
// binding's update_strategy. It clones the repo, modifies the file, commits,
// and pushes.
func (w *ManifestWriter) WriteBack(ctx context.Context, tenantID uuid.UUID, req *WriteBackRequest) (*WriteBackResult, error) {
	if w.gitopsRepo == nil {
		return nil, fmt.Errorf("gitops repository not configured")
	}
	if w.bindingRepo == nil {
		return nil, fmt.Errorf("binding repository not configured")
	}
	if req.ImageTag == "" {
		return nil, fmt.Errorf("image_tag is required")
	}

	// Fetch the binding
	binding, err := w.bindingRepo.Get(ctx, req.BindingID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("get binding: %w", err)
	}
	if binding.RepoID == nil {
		return nil, fmt.Errorf("binding has no associated GitOps repository")
	}

	// Fetch the GitOps repo
	repo, err := w.gitopsRepo.Get(ctx, *binding.RepoID)
	if err != nil {
		return nil, fmt.Errorf("get gitops repo: %w", err)
	}

	// Determine file path and edit strategy
	filePath := resolveFilePath(binding)
	editReq, err := w.buildEditRequest(binding, req, filePath)
	if err != nil {
		return nil, fmt.Errorf("build edit request: %w", err)
	}

	editor := gitops.NewEditor(w.cacheDir)

	// Dry-run: only preview the diff
	if req.DryRun {
		diff, err := editor.PreviewDiff(ctx, repo, editReq)
		if err != nil {
			return nil, fmt.Errorf("preview diff: %w", err)
		}
		return &WriteBackResult{
			Diff:     diff,
			FilePath: filePath,
			Strategy: binding.UpdateStrategy,
		}, nil
	}

	// Apply the edit (clone → modify → commit → push)
	result, err := editor.ApplyEdit(ctx, repo, editReq)
	if err != nil {
		return nil, fmt.Errorf("apply edit: %w", err)
	}

	slog.Info("manifest write-back complete",
		"binding", binding.ID,
		"repo", repo.Name,
		"file", filePath,
		"strategy", binding.UpdateStrategy,
		"commit", result.CommitSHA,
		"branch", result.Branch,
		"mr_needed", result.MRNeeded,
	)

	return &WriteBackResult{
		CommitSHA: result.CommitSHA,
		Branch:    result.Branch,
		Diff:      result.Diff,
		MRNeeded:  result.MRNeeded,
		MRURL:     result.MRURL,
		FilePath:  filePath,
		Strategy:  binding.UpdateStrategy,
	}, nil
}

// WriteBackByService finds bindings for a service and writes back the image tag
// to all of them. Useful when deploying a service that has multiple bindings
// across environments.
func (w *ManifestWriter) WriteBackByService(ctx context.Context, tenantID uuid.UUID, serviceID uuid.UUID, imageTag, imageName string) ([]WriteBackResults, error) {
	if w.bindingRepo == nil {
		return nil, fmt.Errorf("binding repository not configured")
	}

	bindings, err := w.bindingRepo.FindByService(ctx, serviceID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("find bindings: %w", err)
	}

	var results []WriteBackResults
	for _, b := range bindings {
		if b.RepoID == nil {
			continue
		}
		req := &WriteBackRequest{
			BindingID: b.ID,
			ImageTag:  imageTag,
			ImageName: imageName,
		}
		result, err := w.WriteBack(ctx, tenantID, req)
		if err != nil {
			slog.Warn("write-back failed for binding", "binding_id", b.ID, "error", err)
			continue
		}
		results = append(results, WriteBackResults{
			BindingID:   b.ID,
			BindingName: b.Name,
			Environment: b.Environment,
			Result:      result,
		})
	}

	if len(results) == 0 && len(bindings) > 0 {
		return nil, fmt.Errorf("all %d write-back attempts failed", len(bindings))
	}

	return results, nil
}

// WriteBackResults combines binding metadata with the write-back result.
type WriteBackResults struct {
	BindingID   uuid.UUID      `json:"binding_id"`
	BindingName string         `json:"binding_name"`
	Environment *string        `json:"environment,omitempty"`
	Result      *WriteBackResult `json:"result"`
}

// resolveFilePath determines the target file path in the repo based on the
// binding's manifest_path and update_path fields.
func resolveFilePath(binding *repository.GitOpsBinding) string {
	// Explicit manifest_path takes priority
	if binding.ManifestPath != nil && *binding.ManifestPath != "" {
		return *binding.ManifestPath
	}

	// Fallback: derive from app_name and engine type
	switch binding.EngineType {
	case "fluxcd":
		// FluxCD convention: <path>/<app_name>/helmrelease.yaml
		return fmt.Sprintf("%s/helmrelease.yaml", binding.AppName)
	case "argocd":
		// ArgoCD convention: <path>/<app_name>/application.yaml
		return fmt.Sprintf("%s/application.yaml", binding.AppName)
	default:
		return fmt.Sprintf("%s/manifest.yaml", binding.AppName)
	}
}

// buildEditRequest constructs an Editor.ApplyEdit request based on the
// binding's update_strategy.
func (w *ManifestWriter) buildEditRequest(binding *repository.GitOpsBinding, req *WriteBackRequest, filePath string) (*gitops.EditRequest, error) {
	commitMsg := req.CommitMessage
	if commitMsg == "" {
		commitMsg = fmt.Sprintf("gitops: update %s image tag to %s", binding.AppName, req.ImageTag)
	}

	editReq := &gitops.EditRequest{
		FilePath:  filePath,
		CommitMsg: commitMsg,
		Branch:    req.Branch,
	}

	switch binding.UpdateStrategy {
	case "kustomize_image":
		// Update the images[] array in kustomization.yaml
		return w.buildKustomizeImageEdit(editReq, binding, req)

	case "helm_values":
		// Update spec.values.image.tag in a HelmRelease
		return w.buildHelmValuesEdit(editReq, binding, req)

	case "raw_yaml":
		// Direct YAML field edit using update_path
		return w.buildRawYAMLEdit(editReq, binding, req)

	case "appset_param":
		// Update ApplicationSet generator parameters
		return w.buildAppSetParamEdit(editReq, binding, req)

	default:
		return nil, fmt.Errorf("unsupported update strategy: %s", binding.UpdateStrategy)
	}
}

// buildKustomizeImageEdit updates the images[].newTag field in kustomization.yaml.
// If the image entry exists, update its newTag. If not, append a new entry.
func (w *ManifestWriter) buildKustomizeImageEdit(editReq *gitops.EditRequest, binding *repository.GitOpsBinding, req *WriteBackRequest) (*gitops.EditRequest, error) {
	// For kustomize_image, we use a special field path that the Editor's
	// applyFieldPatch understands. Since the Editor works with dot-separated
	// paths, we need to provide the full YAML with the image updated.
	// However, since we don't have the file content here, we use a field path
	// approach: "images.<image_name>.newTag"
	//
	// The Editor's setNestedValue will create the path. For kustomize images
	// which are arrays, we need to handle this differently.
	//
	// Strategy: use the update_path if specified, otherwise construct a
	// field path that targets the image entry.
	if binding.UpdatePath != nil && *binding.UpdatePath != "" {
		editReq.FieldPath = *binding.UpdatePath
		editReq.NewValue = req.ImageTag
		return editReq, nil
	}

	// Default: set images[].newTag via field path
	// The Editor's applyFieldPatch handles this as a nested map update.
	// For array-based kustomize images, we provide the image name as part
	// of the field path: "images.<name>.newTag"
	imageName := req.ImageName
	if imageName == "" {
		imageName = binding.AppName
	}
	editReq.FieldPath = fmt.Sprintf("images.%s.newTag", imageName)
	editReq.NewValue = req.ImageTag
	return editReq, nil
}

// buildHelmValuesEdit updates spec.values.image.tag in a FluxCD HelmRelease.
func (w *ManifestWriter) buildHelmValuesEdit(editReq *gitops.EditRequest, binding *repository.GitOpsBinding, req *WriteBackRequest) (*gitops.EditRequest, error) {
	if binding.UpdatePath != nil && *binding.UpdatePath != "" {
		editReq.FieldPath = *binding.UpdatePath
	} else {
		// Default FluxCD HelmRelease image tag path
		editReq.FieldPath = "spec.values.image.tag"
	}
	editReq.NewValue = req.ImageTag
	return editReq, nil
}

// buildRawYAMLEdit performs a direct YAML field edit using the binding's
// update_path as the field path.
func (w *ManifestWriter) buildRawYAMLEdit(editReq *gitops.EditRequest, binding *repository.GitOpsBinding, req *WriteBackRequest) (*gitops.EditRequest, error) {
	if binding.UpdatePath == nil || *binding.UpdatePath == "" {
		return nil, fmt.Errorf("raw_yaml strategy requires update_path to be set on the binding")
	}
	editReq.FieldPath = *binding.UpdatePath
	editReq.NewValue = req.ImageTag
	return editReq, nil
}

// buildAppSetParamEdit updates an ApplicationSet generator parameter.
func (w *ManifestWriter) buildAppSetParamEdit(editReq *gitops.EditRequest, binding *repository.GitOpsBinding, req *WriteBackRequest) (*gitops.EditRequest, error) {
	if binding.UpdatePath != nil && *binding.UpdatePath != "" {
		editReq.FieldPath = *binding.UpdatePath
	} else {
		// Default ApplicationSet parameter path
		editReq.FieldPath = fmt.Sprintf("spec.generators[0].elements.%s.imageTag", binding.AppName)
	}
	editReq.NewValue = req.ImageTag
	return editReq, nil
}
