package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/k8s"
	"github.com/pepa/pepa/internal/repository"
)

// DeploymentService handles deployment business logic.
type DeploymentService struct {
	clusterRepo    *repository.ClusterRepository
	deploymentRepo *repository.DeploymentRepository
	helmRepo       *repository.HelmRepository
	executor       *DeploymentExecutor
	eventRecorder  func(deploymentID uuid.UUID, eventType, message string) // optional callback for timeline events
}

// NewDeploymentService creates a new DeploymentService.
func NewDeploymentService(
	clusterRepo *repository.ClusterRepository,
	deploymentRepo *repository.DeploymentRepository,
	helmRepo *repository.HelmRepository,
) *DeploymentService {
	return &DeploymentService{
		clusterRepo:    clusterRepo,
		deploymentRepo: deploymentRepo,
		helmRepo:       helmRepo,
		executor:       NewDeploymentExecutor(clusterRepo, helmRepo),
	}
}

// SetEventRecorder sets an optional callback for recording deployment timeline events.
func (s *DeploymentService) SetEventRecorder(recorder func(deploymentID uuid.UUID, eventType, message string)) {
	s.eventRecorder = recorder
}

// recordEvent records a deployment timeline event if a recorder is configured.
func (s *DeploymentService) recordEvent(deploymentID uuid.UUID, eventType, message string) {
	if s.eventRecorder != nil {
		s.eventRecorder(deploymentID, eventType, message)
	}
}

// DeploymentResult represents the result of a deployment operation.
type DeploymentResult struct {
	Success bool
	Message string
	Logs    string
}

// updateStatusWithLogs updates deployment status with logs.
func (s *DeploymentService) updateStatusWithLogs(deploymentID uuid.UUID, status, logs string) {
	deployment, err := s.deploymentRepo.Get(context.Background(), deploymentID)
	if err != nil {
		slog.Info("ERROR: deployment : get for status update", "id", deploymentID, "error", err)
		return
	}
	deployment.Status = status
	deployment.Logs = logs
	if err := s.deploymentRepo.Update(context.Background(), deployment); err != nil {
		slog.Info("ERROR: deployment : update status", "id", deploymentID, "error", err)
	}
}

// updateStatusWithError updates deployment status with error message and logs.
func (s *DeploymentService) updateStatusWithError(deploymentID uuid.UUID, status, errorMsg, logs string) {
	deployment, err := s.deploymentRepo.Get(context.Background(), deploymentID)
	if err != nil {
		slog.Info("ERROR: deployment : get for status update", "id", deploymentID, "error", err)
		return
	}
	deployment.Status = status
	deployment.ErrorMessage = errorMsg
	deployment.Logs = logs
	if err := s.deploymentRepo.Update(context.Background(), deployment); err != nil {
		slog.Info("ERROR: deployment : update status", "id", deploymentID, "error", err)
	}
}

// PerformDeployment executes a deployment to a Kubernetes cluster.
// This is the main business logic extracted from the HTTP handler.
func (s *DeploymentService) PerformDeployment(
	ctx context.Context,
	deploymentID, clusterID uuid.UUID,
	namespace, releaseName string,
	replicas int32,
	specJSON []byte,
	timeoutSeconds int,
) *DeploymentResult {
	// Use a timeout context to prevent hanging on unreachable clusters
	if timeoutSeconds <= 0 {
		timeoutSeconds = 300
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	var logsBuilder strings.Builder

	// Helper to update deployment status
	updateStatus := func(status, errorMsg, logs string) {
		if errorMsg != "" {
			s.updateStatusWithError(deploymentID, status, errorMsg, logs)
		} else {
			s.updateStatusWithLogs(deploymentID, status, logs)
		}
	}

	// Helper to check if deployment was cancelled
	isCancelled := func() bool {
		d, err := s.deploymentRepo.Get(context.Background(), deploymentID)
		return err == nil && d.Status == "cancelled"
	}

	// Mark as syncing to show progress
	updateStatus("syncing", "", "Starting deployment...\n")
	s.recordEvent(deploymentID, "created", "Deployment started")

	// Get kubeconfig for the target cluster (via executor)
	logsBuilder.WriteString(fmt.Sprintf("Getting kubeconfig for cluster %s...\n", clusterID))
	kubeconfig, err := s.executor.ResolveKubeconfig(ctx, clusterID)
	if err != nil {
		slog.Info("ERROR: deployment : get kubeconfig", "id", deploymentID, "error", err)
		updateStatus("failed", fmt.Sprintf("Failed to get kubeconfig: %v", err), logsBuilder.String())
		s.recordEvent(deploymentID, "failed", fmt.Sprintf("Failed to get kubeconfig: %v", err))
		return &DeploymentResult{Success: false, Message: err.Error(), Logs: logsBuilder.String()}
	}

	logsBuilder.WriteString("Kubeconfig obtained.\n")
	s.recordEvent(deploymentID, "kubeconfig_loaded", fmt.Sprintf("Kubeconfig obtained for cluster %s", clusterID))
	logsBuilder.WriteString("Creating Kubernetes client...\n")

	// Check for cancellation before proceeding
	if isCancelled() {
		logsBuilder.WriteString("Deployment cancelled by user.\n")
		updateStatus("cancelled", "", logsBuilder.String())
		return &DeploymentResult{Success: false, Message: "Deployment cancelled by user", Logs: logsBuilder.String()}
	}

	// Create k8s client (via executor)
	client, err := s.executor.CreateK8sClient(ctx, kubeconfig, clusterID)
	if err != nil {
		slog.Info("ERROR: deployment : create k8s client", "id", deploymentID, "error", err)
		updateStatus("failed", fmt.Sprintf("Failed to create k8s client: %v", err), logsBuilder.String())
		return &DeploymentResult{Success: false, Message: err.Error(), Logs: logsBuilder.String()}
	}

	logsBuilder.WriteString("Kubernetes client created. Parsing deployment spec...\n")

	// Parse the deployment spec
	if releaseName == "" {
		releaseName = "pepa-release"
	}
	if namespace == "" {
		namespace = "default"
	}
	deploySpec, err := k8s.ParseDeploySpec(specJSON, releaseName, namespace, replicas)
	if err != nil {
		slog.Info("ERROR: deployment : parse spec", "id", deploymentID, "error", err)
		updateStatus("failed", fmt.Sprintf("Failed to parse deployment spec: %v", err), logsBuilder.String())
		return &DeploymentResult{Success: false, Message: err.Error(), Logs: logsBuilder.String()}
	}

	// Check for cancellation before deploying
	if isCancelled() {
		logsBuilder.WriteString("Deployment cancelled by user.\n")
		updateStatus("cancelled", "", logsBuilder.String())
		return &DeploymentResult{Success: false, Message: "Deployment cancelled by user", Logs: logsBuilder.String()}
	}

	// Execute deploy via executor (handles Helm vs Raw routing + credentials)
	isHelm := deploySpec.Chart != nil && deploySpec.Chart.SourceType != "" && deploySpec.Chart.SourceType != "container"
	if isHelm {
		logsBuilder.WriteString(fmt.Sprintf("Deploying Helm chart: %s/%s (version: %s)\n", deploySpec.Chart.ChartURL, deploySpec.Chart.ChartName, deploySpec.Chart.ChartVersion))
		logsBuilder.WriteString(fmt.Sprintf("Release: %s, Namespace: %s, Timeout: %ds\n", releaseName, namespace, timeoutSeconds))
	} else {
		logsBuilder.WriteString(fmt.Sprintf("Deploying container to namespace %s (timeout: %ds)\n", namespace, timeoutSeconds))
	}

	logsBuilder.WriteString("Deploy initiated...\n")
	deployResult, err := s.executor.ExecuteDeploy(ctx, client, deploySpec, releaseName, namespace, replicas, timeoutSeconds)
	if err != nil {
		errMsg := fmt.Sprintf("Deploy failed: %v", err)
		slog.Info("ERROR: deployment", "id", deploymentID, "error", errMsg)
		logsBuilder.WriteString(fmt.Sprintf("ERROR: %s\n", errMsg))
		updateStatus("failed", errMsg, logsBuilder.String())
		s.recordEvent(deploymentID, "failed", errMsg)
		return &DeploymentResult{Success: false, Message: errMsg, Logs: logsBuilder.String()}
	}

	slog.Info("Deployment applied, running health check", "id", deploymentID)
	logsBuilder.WriteString("Resources applied. Running post-deploy health check...\n")
	s.recordEvent(deploymentID, "resources_applied", "Resources applied to cluster")

	// Post-deploy health check: use remaining context time (context already has deploy timeout)
	if err := client.WaitForReady(ctx, namespace, releaseName, replicas, 0); err != nil {
		errMsg := fmt.Sprintf("Health check failed: %v", err)
		slog.Info("ERROR: deployment health check", "id", deploymentID, "error", errMsg)
		logsBuilder.WriteString(fmt.Sprintf("ERROR: %s\n", errMsg))
		updateStatus("failed", errMsg, logsBuilder.String())
		s.recordEvent(deploymentID, "failed", errMsg)
		return &DeploymentResult{Success: false, Message: errMsg, Logs: logsBuilder.String()}
	}

	slog.Info("Deployment succeeded", "id", deploymentID, "message", deployResult.Message)
	logsBuilder.WriteString("Health check passed.\n")
	s.recordEvent(deploymentID, "pods_ready", "All pods are ready")
	logsBuilder.WriteString(fmt.Sprintf("SUCCESS: %s\n", deployResult.Message))
	updateStatus("deployed", "", logsBuilder.String())
	s.recordEvent(deploymentID, "deployed", "Deployment completed successfully")
	return &DeploymentResult{Success: true, Message: deployResult.Message, Logs: logsBuilder.String()}
}

// PerformRollback executes a real rollback against the Kubernetes cluster.
// It finds the previous successful deployment for the same project/namespace,
// rolls back the cluster resources, and updates the database accordingly.
func (s *DeploymentService) PerformRollback(
	ctx context.Context,
	deploymentID uuid.UUID,
	rolledBackBy string,
) *DeploymentResult {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	var logsBuilder strings.Builder

	updateStatus := func(status, errorMsg, logs string) {
		if errorMsg != "" {
			s.updateStatusWithError(deploymentID, status, errorMsg, logs)
		} else {
			s.updateStatusWithLogs(deploymentID, status, logs)
		}
	}

	// Get the current deployment
	current, err := s.deploymentRepo.Get(ctx, deploymentID)
	if err != nil {
		return &DeploymentResult{Success: false, Message: fmt.Sprintf("get deployment: %v", err)}
	}

	logsBuilder.WriteString(fmt.Sprintf("Starting rollback of deployment %s...\n", deploymentID))

	// Validate status allows rollback
	if current.Status != "deployed" && current.Status != "promoted" {
		return &DeploymentResult{Success: false, Message: fmt.Sprintf("deployment cannot be rolled back: must be in 'deployed' or 'promoted' status, current: %s", current.Status)}
	}

	// Find the previous successful deployment for the same project/namespace
	history, err := s.deploymentRepo.History(ctx, current.TenantID, current.GitlabProjectName, current.TargetNamespace, 20)
	if err != nil {
		logsBuilder.WriteString(fmt.Sprintf("Warning: could not get history: %v\n", err))
	}

	var previous *repository.Deployment
	for i := range history {
		h := &history[i]
		if h.ID != deploymentID && (h.Status == "deployed" || h.Status == "promoted") {
			previous = h
			break
		}
	}

	// If no target cluster, just do a DB-only rollback
	if current.TargetClusterID == nil {
		logsBuilder.WriteString("No target cluster — performing DB-only rollback.\n")
		if err := s.deploymentRepo.Rollback(ctx, deploymentID, rolledBackBy); err != nil {
			return &DeploymentResult{Success: false, Message: err.Error(), Logs: logsBuilder.String()}
		}
		logsBuilder.WriteString("DB rollback completed.\n")
		updateStatus("rolled_back", "", logsBuilder.String())
		return &DeploymentResult{Success: true, Message: "Rollback completed (DB only)", Logs: logsBuilder.String()}
	}

	// Get kubeconfig for the target cluster
	logsBuilder.WriteString(fmt.Sprintf("Getting kubeconfig for cluster %s...\n", *current.TargetClusterID))
	kubeconfig, err := s.executor.ResolveKubeconfig(ctx, *current.TargetClusterID)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to get kubeconfig: %v", err)
		logsBuilder.WriteString(errMsg + "\n")
		updateStatus("failed", errMsg, logsBuilder.String())
		return &DeploymentResult{Success: false, Message: errMsg, Logs: logsBuilder.String()}
	}

	// Create k8s client
	client, err := s.executor.CreateK8sClient(ctx, kubeconfig, *current.TargetClusterID)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to create k8s client: %v", err)
		logsBuilder.WriteString(errMsg + "\n")
		updateStatus("failed", errMsg, logsBuilder.String())
		return &DeploymentResult{Success: false, Message: errMsg, Logs: logsBuilder.String()}
	}

	releaseName := current.GitlabProjectName
	if releaseName == "" {
		releaseName = "pepa-release"
	}
	namespace := current.TargetNamespace
	if namespace == "" {
		namespace = "default"
	}

	// Determine deploy type and perform the appropriate rollback
	isHelm := current.DeployType == "helm" || (current.Spec != nil && containsHelmChart(current.Spec))

	if isHelm {
		logsBuilder.WriteString(fmt.Sprintf("Performing Helm rollback of release %s...\n", releaseName))
		// For Helm, rollback to previous revision (revision 0 means "previous")
		revision := 0
		if previous != nil {
			logsBuilder.WriteString(fmt.Sprintf("Previous deployment found: %s (image: %s)\n", previous.ID, previous.ImageTag))
		}
		if err := client.HelmRollback(ctx, namespace, releaseName, revision); err != nil {
			errMsg := fmt.Sprintf("Helm rollback failed: %v", err)
			logsBuilder.WriteString(errMsg + "\n")
			updateStatus("failed", errMsg, logsBuilder.String())
			return &DeploymentResult{Success: false, Message: errMsg, Logs: logsBuilder.String()}
		}
		logsBuilder.WriteString("Helm rollback succeeded.\n")
	} else {
		logsBuilder.WriteString(fmt.Sprintf("Performing rollout undo for deployment %s/%s...\n", namespace, releaseName))
		if err := client.UndoRollout(ctx, namespace, releaseName); err != nil {
			errMsg := fmt.Sprintf("Rollout undo failed: %v", err)
			logsBuilder.WriteString(errMsg + "\n")
			updateStatus("failed", errMsg, logsBuilder.String())
			return &DeploymentResult{Success: false, Message: errMsg, Logs: logsBuilder.String()}
		}
		logsBuilder.WriteString("Rollout undo succeeded.\n")
	}

	// Update current deployment status to rolled_back
	if err := s.deploymentRepo.Rollback(ctx, deploymentID, rolledBackBy); err != nil {
		logsBuilder.WriteString(fmt.Sprintf("Warning: DB rollback failed: %v\n", err))
	}

	// Create a new deployment record with the previous version's spec (if available)
	if previous != nil && previous.Spec != nil {
		newDeploy := &repository.Deployment{
			TenantID:          current.TenantID,
			GitlabProjectName: current.GitlabProjectName,
			ImageTag:          previous.ImageTag,
			ImageRepository:   previous.ImageRepository,
			TargetClusterID:   current.TargetClusterID,
			TargetNamespace:   current.TargetNamespace,
			DeployType:        current.DeployType,
			Replicas:          previous.Replicas,
			Strategy:          previous.Strategy,
			Spec:              previous.Spec,
			Status:            "deployed",
			CreatedBy:         rolledBackBy,
			TimeoutSeconds:    current.TimeoutSeconds,
			TeamName:          current.TeamName,
			Stage:             current.Stage,
		}
		if err := s.deploymentRepo.Create(ctx, newDeploy); err != nil {
			logsBuilder.WriteString(fmt.Sprintf("Warning: could not create rollback record: %v\n", err))
		} else {
			logsBuilder.WriteString(fmt.Sprintf("Rollback deployment record created: %s\n", newDeploy.ID))
		}
	}

	logsBuilder.WriteString("Rollback completed successfully.\n")
	updateStatus("rolled_back", "", logsBuilder.String())
	return &DeploymentResult{Success: true, Message: "Rollback completed", Logs: logsBuilder.String()}
}

// containsHelmChart checks if a spec JSON contains Helm chart information.
func containsHelmChart(specJSON json.RawMessage) bool {
	var raw struct {
		Chart *struct {
			SourceType string `json:"source_type"`
		} `json:"chart"`
	}
	if err := json.Unmarshal(specJSON, &raw); err != nil {
		return false
	}
	return raw.Chart != nil && raw.Chart.SourceType != "" && raw.Chart.SourceType != "container"
}

// DryRunResult represents the result of a dry-run deployment preview.
type DryRunResult struct {
	Resources  string `json:"resources"`
	Manifests  string `json:"manifests,omitempty"`
	DeployType string `json:"deploy_type"`
	ReleaseName string `json:"release_name"`
	Namespace  string `json:"namespace"`
}

// PerformDryRun executes a dry-run deployment preview without creating a record
// or applying changes. For Helm, it runs `helm template`. For raw K8s, it
// validates the spec and returns a summary of resources that would be created.
func (s *DeploymentService) PerformDryRun(
	ctx context.Context,
	clusterID uuid.UUID,
	namespace, releaseName string,
	replicas int32,
	specJSON []byte,
	timeoutSeconds int,
) (*DryRunResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	if releaseName == "" {
		releaseName = "pepa-release"
	}
	if namespace == "" {
		namespace = "default"
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 300
	}

	// Parse the spec
	deploySpec, err := k8s.ParseDeploySpec(specJSON, releaseName, namespace, replicas)
	if err != nil {
		return nil, fmt.Errorf("parse deployment spec: %w", err)
	}

	isHelm := deploySpec.Chart != nil && deploySpec.Chart.SourceType != "" && deploySpec.Chart.SourceType != "container"

	// If no cluster, return a local preview
	if clusterID == uuid.Nil || s.clusterRepo == nil {
		if isHelm {
			return &DryRunResult{
				DeployType:  "helm",
				ReleaseName: releaseName,
				Namespace:   namespace,
				Resources:   fmt.Sprintf("Helm chart: %s/%s v%s\nRelease: %s\nNamespace: %s",
					deploySpec.Chart.ChartURL, deploySpec.Chart.ChartName, deploySpec.Chart.ChartVersion,
					releaseName, namespace),
			}, nil
		}
		// Raw K8s local preview
		var resources string
		resources = fmt.Sprintf("Deployment: %s/%s (replicas: %d)\n", namespace, releaseName, replicas)
		for _, c := range deploySpec.Containers {
			resources += fmt.Sprintf("  Container: %s (image: %s)\n", c.Name, c.Image)
		}
		if deploySpec.Service != nil {
			resources += fmt.Sprintf("Service: %s/%s (port: %d)\n", namespace, releaseName, deploySpec.Service.Port)
		}
		return &DryRunResult{
			DeployType:  "raw",
			ReleaseName: releaseName,
			Namespace:   namespace,
			Resources:   resources,
		}, nil
	}

	// Get kubeconfig and create client
	kubeconfig, err := s.executor.ResolveKubeconfig(ctx, clusterID)
	if err != nil {
		return nil, fmt.Errorf("get kubeconfig: %w", err)
	}
	client, err := s.executor.CreateK8sClient(ctx, kubeconfig, clusterID)
	if err != nil {
		return nil, fmt.Errorf("create k8s client: %w", err)
	}

	if isHelm {
		helmSpec := k8s.HelmSpec{
			SourceType:     deploySpec.Chart.SourceType,
			ChartURL:       deploySpec.Chart.ChartURL,
			ChartName:      deploySpec.Chart.ChartName,
			ChartVersion:   deploySpec.Chart.ChartVersion,
			ValuesYAML:     deploySpec.ValuesYAML,
			ReleaseName:    releaseName,
			Namespace:      namespace,
			TimeoutSeconds: timeoutSeconds,
		}
		username, password, token := s.executor.ResolveHelmCredentials(ctx, deploySpec.Chart.ChartURL)
		helmSpec.Username = username
		helmSpec.Password = password
		helmSpec.Token = token

		manifests, err := client.HelmTemplate(ctx, helmSpec)
		if err != nil {
			return nil, fmt.Errorf("helm template (dry-run) failed: %w", err)
		}
		return &DryRunResult{
			DeployType:  "helm",
			ReleaseName: releaseName,
			Namespace:   namespace,
			Manifests:   manifests,
			Resources:   fmt.Sprintf("Helm chart: %s/%s v%s\nRelease: %s\nNamespace: %s\n\nRendered manifests (%d bytes)",
				deploySpec.Chart.ChartURL, deploySpec.Chart.ChartName, deploySpec.Chart.ChartVersion,
				releaseName, namespace, len(manifests)),
		}, nil
	}

	// Raw K8s dry-run
	summary, err := client.SummarizeDryRun(ctx, deploySpec)
	if err != nil {
		return nil, fmt.Errorf("dry-run apply failed: %w", err)
	}
	return &DryRunResult{
		DeployType:  "raw",
		ReleaseName: releaseName,
		Namespace:   namespace,
		Resources:   summary,
	}, nil
}
