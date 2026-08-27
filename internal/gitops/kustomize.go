package gitops

import (
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// KustomizeImage represents an image override in a kustomization.yaml.
type KustomizeImage struct {
	Name    string `json:"name" yaml:"name"`
	NewName string `json:"new_name,omitempty" yaml:"newName,omitempty"`
	NewTag  string `json:"new_tag,omitempty" yaml:"newTag,omitempty"`
	Digest  string `json:"digest,omitempty" yaml:"digest,omitempty"`
}

// KustomizePatch represents a patch entry in a kustomization.yaml.
type KustomizePatch struct {
	Path   string `json:"path,omitempty" yaml:"path,omitempty"`
	Target *struct {
		Group              string `json:"group,omitempty" yaml:"group,omitempty"`
		Version            string `json:"version,omitempty" yaml:"version,omitempty"`
		Kind               string `json:"kind,omitempty" yaml:"kind,omitempty"`
		Name               string `json:"name,omitempty" yaml:"name,omitempty"`
		Namespace          string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
		AnnotationSelector string `json:"annotationSelector,omitempty" yaml:"annotationSelector,omitempty"`
		LabelSelector      string `json:"labelSelector,omitempty" yaml:"labelSelector,omitempty"`
	} `json:"target,omitempty" yaml:"target,omitempty"`
}

// KustomizeGenerator represents a configMapGenerator or secretGenerator entry.
type KustomizeGenerator struct {
	Name      string            `json:"name" yaml:"name"`
	Namespace string            `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	Literals  []string          `json:"literals,omitempty" yaml:"literals,omitempty"`
	Files     []string          `json:"files,omitempty" yaml:"files,omitempty"`
	EnvFiles  []string          `json:"envs,omitempty" yaml:"envs,omitempty"`
	Behavior  string            `json:"behavior,omitempty" yaml:"behavior,omitempty"`
	Type      string            `json:"type,omitempty" yaml:"type,omitempty"` // Secret type
	Options   map[string]string `json:"options,omitempty" yaml:"options,omitempty"`
}

// KustomizeInfo holds parsed kustomization.yaml data.
type KustomizeInfo struct {
	FilePath   string               `json:"file_path"`
	Resources  []string             `json:"resources,omitempty"` // base references
	Patches    []KustomizePatch     `json:"patches,omitempty"`   // overlay patches
	Images     []KustomizeImage     `json:"images,omitempty"`    // image overrides
	NamePrefix string               `json:"name_prefix,omitempty"`
	NameSuffix string               `json:"name_suffix,omitempty"`
	Namespace  string               `json:"namespace,omitempty"`
	ConfigMaps []KustomizeGenerator `json:"config_maps,omitempty"` // generated ConfigMap names
	Secrets    []KustomizeGenerator `json:"secrets,omitempty"`     // generated Secret names
	Components []string             `json:"components,omitempty"`  // component references
	Vars       []KustomizeVar       `json:"vars,omitempty"`        // variable substitutions
	IsBase     bool                 `json:"is_base"`               // true if no bases/resources pointing outward
	IsOverlay  bool                 `json:"is_overlay"`            // true if references external base
	BasePath   string               `json:"base_path,omitempty"`   // resolved base path relative to this file
}

// KustomizeVar represents a variable substitution in kustomization.yaml.
type KustomizeVar struct {
	Name   string `json:"name" yaml:"name"`
	ObjRef struct {
		APIVersion string `json:"apiVersion,omitempty" yaml:"apiVersion,omitempty"`
		Kind       string `json:"kind,omitempty" yaml:"kind,omitempty"`
		Name       string `json:"name,omitempty" yaml:"name,omitempty"`
		Namespace  string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	} `json:"objref" yaml:"objref"`
	FieldRef struct {
		FieldPath string `json:"fieldPath,omitempty" yaml:"fieldPath,omitempty"`
	} `json:"fieldref" yaml:"fieldref"`
}

// rawKustomization is the raw YAML structure of a kustomization.yaml file.
type rawKustomization struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`

	// Resource references
	Resources  []string `yaml:"resources"`
	Bases      []string `yaml:"bases"` // deprecated but still used
	Components []string `yaml:"components"`

	// Patches
	Patches               []KustomizePatch `yaml:"patches"`
	PatchesStrategicMerge []string         `yaml:"patchesStrategicMerge"`
	PatchesJson6902       []struct {
		Path   string `yaml:"path"`
		Target struct {
			Group   string `yaml:"group"`
			Version string `yaml:"version"`
			Kind    string `yaml:"kind"`
			Name    string `yaml:"name"`
		} `yaml:"target"`
	} `yaml:"patchesJson6902"`

	// Transformations
	NamePrefix        string            `yaml:"namePrefix"`
	NameSuffix        string            `yaml:"nameSuffix"`
	Namespace         string            `yaml:"namespace"`
	CommonLabels      map[string]string `yaml:"commonLabels"`
	CommonAnnotations map[string]string `yaml:"commonAnnotations"`

	// Images
	Images []KustomizeImage `yaml:"images"`

	// Generators
	ConfigMapGenerator []KustomizeGenerator `yaml:"configMapGenerator"`
	SecretGenerator    []KustomizeGenerator `yaml:"secretGenerator"`

	// Variables
	Vars []KustomizeVar `yaml:"vars"`
}

// ParseKustomization parses a kustomization.yaml file and extracts structure information.
func ParseKustomization(data []byte, filePath string) *KustomizeInfo {
	var raw rawKustomization
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil
	}

	// Must look like a kustomization file
	if raw.Kind != "" && raw.Kind != "Kustomization" {
		return nil
	}
	// If no kind, check if it has kustomize-specific fields
	if raw.Kind == "" && len(raw.Resources) == 0 && len(raw.Bases) == 0 &&
		raw.NamePrefix == "" && raw.NameSuffix == "" && len(raw.Images) == 0 &&
		len(raw.Patches) == 0 && len(raw.ConfigMapGenerator) == 0 {
		return nil
	}

	info := &KustomizeInfo{
		FilePath:   filePath,
		NamePrefix: raw.NamePrefix,
		NameSuffix: raw.NameSuffix,
		Namespace:  raw.Namespace,
	}

	// Combine resources + bases
	info.Resources = append(info.Resources, raw.Resources...)
	info.Resources = append(info.Resources, raw.Bases...)
	info.Components = append(info.Components, raw.Components...)

	// Collect patches
	info.Patches = append(info.Patches, raw.Patches...)
	for _, p := range raw.PatchesStrategicMerge {
		info.Patches = append(info.Patches, KustomizePatch{Path: p})
	}
	for _, p := range raw.PatchesJson6902 {
		info.Patches = append(info.Patches, KustomizePatch{
			Path: p.Path,
			Target: &struct {
				Group              string `json:"group,omitempty" yaml:"group,omitempty"`
				Version            string `json:"version,omitempty" yaml:"version,omitempty"`
				Kind               string `json:"kind,omitempty" yaml:"kind,omitempty"`
				Name               string `json:"name,omitempty" yaml:"name,omitempty"`
				Namespace          string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
				AnnotationSelector string `json:"annotationSelector,omitempty" yaml:"annotationSelector,omitempty"`
				LabelSelector      string `json:"labelSelector,omitempty" yaml:"labelSelector,omitempty"`
			}{
				Group:   p.Target.Group,
				Version: p.Target.Version,
				Kind:    p.Target.Kind,
				Name:    p.Target.Name,
			},
		})
	}

	// Images
	info.Images = append(info.Images, raw.Images...)

	// Generators
	info.ConfigMaps = append(info.ConfigMaps, raw.ConfigMapGenerator...)
	info.Secrets = append(info.Secrets, raw.SecretGenerator...)

	// Variables
	info.Vars = append(info.Vars, raw.Vars...)

	// Determine if this is a base or overlay
	info.IsBase, info.IsOverlay, info.BasePath = classifyKustomizeRole(info, filePath)

	return info
}

// classifyKustomizeRole determines whether a kustomization.yaml is a base or overlay.
func classifyKustomizeRole(info *KustomizeInfo, filePath string) (isBase, isOverlay bool, basePath string) {
	dir := filepath.Dir(filePath)

	// Check if any resource/base reference points outside this directory
	for _, ref := range info.Resources {
		// Skip non-file references (URLs, inline resources)
		if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") || strings.HasPrefix(ref, "github.com/") {
			continue
		}
		// Skip if it's a relative path pointing to a file in the same directory
		resolved := filepath.Clean(filepath.Join(dir, ref))
		rel, err := filepath.Rel(dir, resolved)
		if err != nil {
			continue
		}
		// If the reference goes up (starts with ..), this is an overlay referencing a base
		if strings.HasPrefix(rel, "..") {
			return false, true, ref
		}
	}

	// If directory is named "base" or is inside a "base" directory, it's a base
	dirName := filepath.Base(dir)
	if strings.EqualFold(dirName, "base") {
		return true, false, ""
	}

	// If directory is inside "overlays" or "overlay", it's an overlay
	parentDir := filepath.Base(filepath.Dir(dir))
	if strings.EqualFold(parentDir, "overlays") || strings.EqualFold(parentDir, "overlay") {
		// Try to find the base path
		basePath = findBasePath(dir, info.Resources)
		return false, true, basePath
	}

	// If it has patches but no external references, it might still be an overlay
	// if it references resources in subdirectories
	if len(info.Patches) > 0 && len(info.Resources) > 0 {
		return false, true, info.Resources[0]
	}

	// Default: treat as base if it has resources but no patches
	if len(info.Resources) > 0 && len(info.Patches) == 0 {
		return true, false, ""
	}

	// If it has no resources and no bases, it might be a standalone kustomization
	return false, false, ""
}

// findBasePath attempts to resolve the base path from an overlay directory.
func findBasePath(overlayDir string, resources []string) string {
	for _, ref := range resources {
		if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") || strings.HasPrefix(ref, "github.com/") {
			continue
		}
		resolved := filepath.Clean(filepath.Join(overlayDir, ref))
		rel, err := filepath.Rel(overlayDir, resolved)
		if err != nil {
			continue
		}
		if strings.HasPrefix(rel, "..") {
			return ref
		}
	}
	return ""
}
