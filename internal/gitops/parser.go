package gitops

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// parseResource attempts to parse a YAML document as a GitOps resource.
// Returns nil if the document is not a recognized GitOps manifest.
// Returns multiple resources for ApplicationSets (one per generator element).
func parseResource(data []byte, filePath, engineHint string) []Resource {
	// First, extract apiVersion and kind to determine the resource type
	var meta struct {
		APIVersion string `yaml:"apiVersion"`
		Kind       string `yaml:"kind"`
	}
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return nil
	}
	if meta.Kind == "" {
		return nil
	}

	switch {
	case meta.Kind == "HelmRelease" && strings.Contains(meta.APIVersion, "fluxcd"):
		if r := parseFluxHelmRelease(data, filePath, meta.APIVersion); r != nil {
			return []Resource{*r}
		}
	case meta.Kind == "Kustomization" && strings.Contains(meta.APIVersion, "fluxcd"):
		if r := parseFluxKustomization(data, filePath, meta.APIVersion); r != nil {
			return []Resource{*r}
		}
	case meta.Kind == "Application" && strings.Contains(meta.APIVersion, "argoproj"):
		if r := parseArgoApplication(data, filePath, meta.APIVersion); r != nil {
			return []Resource{*r}
		}
	case meta.Kind == "ApplicationSet" && strings.Contains(meta.APIVersion, "argoproj"):
		return parseArgoApplicationSet(data, filePath, meta.APIVersion)
	case meta.Kind == "Kustomization" && (strings.Contains(meta.APIVersion, "kustomize") || engineHint == "fluxcd"):
		// kustomize.config.k8s.io Kustomization (plain kustomize, not Flux)
		if r := parseKustomizeFile(data, filePath); r != nil {
			return []Resource{*r}
		}
	}

	return nil
}

// parseFluxHelmRelease parses a FluxCD HelmRelease resource.
// Supports both v2beta1/v2beta2 (spec.chart.spec) and v2 (spec.chartRef) formats.
func parseFluxHelmRelease(data []byte, filePath, apiVersion string) *Resource {
	var raw struct {
		Metadata struct {
			Name      string            `yaml:"name"`
			Namespace string            `yaml:"namespace"`
			Labels    map[string]string `yaml:"labels"`
		} `yaml:"metadata"`
		Spec struct {
			Suspend bool `yaml:"suspend"`
			// v2beta1/v2beta2 format
			Chart struct {
				Spec struct {
					Chart     string `yaml:"chart"`
					Version   string `yaml:"version"`
					SourceRef struct {
						Kind string `yaml:"kind"`
						Name string `yaml:"name"`
					} `yaml:"sourceRef"`
				} `yaml:"spec"`
			} `yaml:"chart"`
			// v2 format: chartRef
			ChartRef *struct {
				Kind      string `yaml:"kind"`
				Name      string `yaml:"name"`
				Namespace string `yaml:"namespace"`
			} `yaml:"chartRef"`
			Values     map[string]interface{} `yaml:"values"`
			ValuesFrom []struct {
				Kind       string `yaml:"kind"`
				Name       string `yaml:"name"`
				ValuesKey  string `yaml:"valuesKey"`
				TargetPath string `yaml:"targetPath"`
				Optional   bool   `yaml:"optional"`
			} `yaml:"valuesFrom"`
			DependsOn []struct {
				Name      string `yaml:"name"`
				Namespace string `yaml:"namespace"`
			} `yaml:"dependsOn"`
			// Post-build variable substitution
			PostBuild *struct {
				Substitute     map[string]string `yaml:"substitute"`
				SubstituteFrom []struct {
					Kind string `yaml:"kind"`
					Name string `yaml:"name"`
				} `yaml:"substituteFrom"`
			} `yaml:"postBuild"`
		} `yaml:"spec"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil
	}

	r := &Resource{
		Kind:       "HelmRelease",
		APIVersion: apiVersion,
		Name:       raw.Metadata.Name,
		Namespace:  raw.Metadata.Namespace,
		FilePath:   filePath,
		Chart:      raw.Spec.Chart.Spec.Chart,
		Version:    raw.Spec.Chart.Spec.Version,
		Repo:       raw.Spec.Chart.Spec.SourceRef.Name,
		Values:     raw.Spec.Values,
		Labels:     raw.Metadata.Labels,
		RawYAML:    string(data),
		Suspended:  raw.Spec.Suspend,
	}

	// Build repo reference as "kind/name"
	if raw.Spec.Chart.Spec.SourceRef.Kind != "" {
		r.Repo = raw.Spec.Chart.Spec.SourceRef.Kind + "/" + raw.Spec.Chart.Spec.SourceRef.Name
	}

	// Handle v2 chartRef format
	if r.Chart == "" && raw.Spec.ChartRef != nil {
		r.Chart = raw.Spec.ChartRef.Name
		r.ChartRef = &FluxChartRef{
			Kind:      raw.Spec.ChartRef.Kind,
			Name:      raw.Spec.ChartRef.Name,
			Namespace: raw.Spec.ChartRef.Namespace,
		}
		if r.Repo == "" {
			r.Repo = raw.Spec.ChartRef.Kind + "/" + raw.Spec.ChartRef.Name
		}
	}

	for _, vf := range raw.Spec.ValuesFrom {
		r.ValuesFrom = append(r.ValuesFrom, ValuesReference{
			Kind:       vf.Kind,
			Name:       vf.Name,
			ValuesKey:  vf.ValuesKey,
			TargetPath: vf.TargetPath,
			Optional:   vf.Optional,
		})
	}

	for _, dep := range raw.Spec.DependsOn {
		ref := dep.Name
		if dep.Namespace != "" {
			ref = dep.Namespace + "/" + dep.Name
		}
		r.DependsOn = append(r.DependsOn, ref)
	}

	return r
}

// parseFluxKustomization parses a FluxCD Kustomization resource.
func parseFluxKustomization(data []byte, filePath, apiVersion string) *Resource {
	var raw struct {
		Metadata struct {
			Name      string            `yaml:"name"`
			Namespace string            `yaml:"namespace"`
			Labels    map[string]string `yaml:"labels"`
		} `yaml:"metadata"`
		Spec struct {
			Suspend   bool   `yaml:"suspend"`
			Path      string `yaml:"path"`
			SourceRef struct {
				Kind string `yaml:"kind"`
				Name string `yaml:"name"`
			} `yaml:"sourceRef"`
			DependsOn []struct {
				Name      string `yaml:"name"`
				Namespace string `yaml:"namespace"`
			} `yaml:"dependsOn"`
		} `yaml:"spec"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil
	}

	r := &Resource{
		Kind:       "Kustomization",
		APIVersion: apiVersion,
		Name:       raw.Metadata.Name,
		Namespace:  raw.Metadata.Namespace,
		FilePath:   filePath,
		Labels:     raw.Metadata.Labels,
		RawYAML:    string(data),
		Suspended:  raw.Spec.Suspend,
	}

	// Store source ref as the "repo" field
	if raw.Spec.SourceRef.Kind != "" {
		r.Repo = raw.Spec.SourceRef.Kind + "/" + raw.Spec.SourceRef.Name
	}

	for _, dep := range raw.Spec.DependsOn {
		ref := dep.Name
		if dep.Namespace != "" {
			ref = dep.Namespace + "/" + dep.Name
		}
		r.DependsOn = append(r.DependsOn, ref)
	}

	return r
}

// parseArgoApplication parses an ArgoCD Application resource.
// Supports both single-source (spec.source) and multi-source (spec.sources[]).
func parseArgoApplication(data []byte, filePath, apiVersion string) *Resource {
	var raw struct {
		Metadata struct {
			Name      string            `yaml:"name"`
			Namespace string            `yaml:"namespace"`
			Labels    map[string]string `yaml:"labels"`
		} `yaml:"metadata"`
		Spec struct {
			Source *struct {
				RepoURL        string `yaml:"repoURL"`
				Path           string `yaml:"path"`
				TargetRevision string `yaml:"targetRevision"`
				Helm           *struct {
					ValueFiles []string               `yaml:"valueFiles"`
					Values     map[string]interface{} `yaml:"values"`
					Parameters []struct {
						Name  string `yaml:"name"`
						Value string `yaml:"value"`
					} `yaml:"parameters"`
				} `yaml:"helm"`
			} `yaml:"source"`
			Sources []struct {
				RepoURL        string `yaml:"repoURL"`
				Path           string `yaml:"path"`
				TargetRevision string `yaml:"targetRevision"`
				Helm           *struct {
					ValueFiles []string               `yaml:"valueFiles"`
					Values     map[string]interface{} `yaml:"values"`
					Parameters []struct {
						Name  string `yaml:"name"`
						Value string `yaml:"value"`
					} `yaml:"parameters"`
				} `yaml:"helm"`
			} `yaml:"sources"`
			Destination struct {
				Server    string `yaml:"server"`
				Namespace string `yaml:"namespace"`
				Name      string `yaml:"name"`
			} `yaml:"destination"`
		} `yaml:"spec"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil
	}

	r := &Resource{
		Kind:       "Application",
		APIVersion: apiVersion,
		Name:       raw.Metadata.Name,
		Namespace:  raw.Metadata.Namespace,
		FilePath:   filePath,
		Labels:     raw.Metadata.Labels,
		RawYAML:    string(data),
		Dest: &ArgoDest{
			Server:    raw.Spec.Destination.Server,
			Namespace: raw.Spec.Destination.Namespace,
			Name:      raw.Spec.Destination.Name,
		},
	}

	// Parse single-source (spec.source)
	if raw.Spec.Source != nil {
		r.Source = &ArgoSource{
			RepoURL:        raw.Spec.Source.RepoURL,
			Path:           raw.Spec.Source.Path,
			TargetRevision: raw.Spec.Source.TargetRevision,
		}
		if raw.Spec.Source.Helm != nil {
			h := raw.Spec.Source.Helm
			r.Source.Helm = &ArgoHelm{
				ValueFiles: h.ValueFiles,
				Values:     h.Values,
			}
			for _, p := range h.Parameters {
				r.Source.Helm.Parameters = append(r.Source.Helm.Parameters, ArgoHelmParameter{
					Name:  p.Name,
					Value: p.Value,
				})
			}
			if h.Values != nil {
				r.Values = h.Values
			}
		}
		if raw.Spec.Source.Path != "" {
			r.Chart = raw.Spec.Source.Path
		}
		r.Repo = raw.Spec.Source.RepoURL
	}

	// Parse multi-source (spec.sources[])
	for _, src := range raw.Spec.Sources {
		argoSrc := ArgoSource{
			RepoURL:        src.RepoURL,
			Path:           src.Path,
			TargetRevision: src.TargetRevision,
		}
		if src.Helm != nil {
			argoSrc.Helm = &ArgoHelm{
				ValueFiles: src.Helm.ValueFiles,
				Values:     src.Helm.Values,
			}
			for _, p := range src.Helm.Parameters {
				argoSrc.Helm.Parameters = append(argoSrc.Helm.Parameters, ArgoHelmParameter{
					Name:  p.Name,
					Value: p.Value,
				})
			}
		}
		r.Sources = append(r.Sources, argoSrc)
	}

	// If multi-source, use first source for chart/repo fields
	if len(r.Sources) > 0 && r.Chart == "" {
		r.Chart = r.Sources[0].Path
		r.Repo = r.Sources[0].RepoURL
	}

	return r
}

// parseArgoApplicationSet parses an ArgoCD ApplicationSet resource.
// It expands generators into individual Application resources where possible.
func parseArgoApplicationSet(data []byte, filePath, apiVersion string) []Resource {
	var raw struct {
		Metadata struct {
			Name      string            `yaml:"name"`
			Namespace string            `yaml:"namespace"`
			Labels    map[string]string `yaml:"labels"`
		} `yaml:"metadata"`
		Spec struct {
			Generators []map[string]interface{} `yaml:"generators"`
			Template   struct {
				Metadata struct {
					Name   string            `yaml:"name"`
					Labels map[string]string `yaml:"labels"`
				} `yaml:"metadata"`
				Spec struct {
					Source *struct {
						RepoURL        string `yaml:"repoURL"`
						Path           string `yaml:"path"`
						TargetRevision string `yaml:"targetRevision"`
						Helm           *struct {
							ValueFiles []string               `yaml:"valueFiles"`
							Values     map[string]interface{} `yaml:"values"`
						} `yaml:"helm"`
					} `yaml:"source"`
					Destination struct {
						Server    string `yaml:"server"`
						Namespace string `yaml:"namespace"`
						Name      string `yaml:"name"`
					} `yaml:"destination"`
				} `yaml:"spec"`
			} `yaml:"template"`
		} `yaml:"spec"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil
	}

	// Extract generator elements
	elements := extractAppSetElements(raw.Spec.Generators)

	// If no elements could be extracted, create a single resource for the ApplicationSet itself
	if len(elements) == 0 {
		elements = []map[string]string{{}}
	}

	var resources []Resource
	for i, elem := range elements {
		appName := substituteVars(raw.Spec.Template.Metadata.Name, elem)
		if appName == "" {
			appName = raw.Metadata.Name
			if len(elements) > 1 {
				appName = raw.Metadata.Name + "-" + elem["cluster"]
			}
		}

		r := Resource{
			Kind:       "Application",
			APIVersion: apiVersion,
			Name:       appName,
			Namespace:  raw.Metadata.Namespace,
			FilePath:   filePath,
			Labels:     raw.Metadata.Labels,
			RawYAML:    string(data),
		}

		// Apply template with variable substitution
		if raw.Spec.Template.Spec.Source != nil {
			src := raw.Spec.Template.Spec.Source
			r.Source = &ArgoSource{
				RepoURL:        substituteVars(src.RepoURL, elem),
				Path:           substituteVars(src.Path, elem),
				TargetRevision: substituteVars(src.TargetRevision, elem),
			}
			if src.Helm != nil {
				r.Source.Helm = &ArgoHelm{
					ValueFiles: src.Helm.ValueFiles,
					Values:     src.Helm.Values,
				}
			}
			if r.Source.Path != "" {
				r.Chart = r.Source.Path
			}
			r.Repo = r.Source.RepoURL
		}

		r.Dest = &ArgoDest{
			Server:    substituteVars(raw.Spec.Template.Spec.Destination.Server, elem),
			Namespace: substituteVars(raw.Spec.Template.Spec.Destination.Namespace, elem),
			Name:      substituteVars(raw.Spec.Template.Spec.Destination.Name, elem),
		}

		// Set environment from generator element (sub-cluster is refined by path hierarchy detection)
		if cluster, ok := elem["cluster"]; ok {
			r.Environment = cluster
		}
		r.LayoutRole = "app"

		// Mark source for dedup if multiple elements
		if i > 0 || len(elements) > 1 {
			if r.Labels == nil {
				r.Labels = map[string]string{}
			}
			r.Labels["applicationset"] = raw.Metadata.Name
		}

		resources = append(resources, r)
	}

	return resources
}

// extractAppSetElements extracts parameter maps from ApplicationSet generators.
func extractAppSetElements(generators []map[string]interface{}) []map[string]string {
	var elements []map[string]string

	for _, gen := range generators {
		// List generator: { list: { elements: [{cluster: prod}, ...] } }
		if list, ok := gen["list"]; ok {
			if listMap, ok := list.(map[string]interface{}); ok {
				if elems, ok := listMap["elements"].([]interface{}); ok {
					for _, e := range elems {
						if elemMap, ok := e.(map[string]interface{}); ok {
							params := make(map[string]string)
							for k, v := range elemMap {
								params[k] = toString(v)
							}
							elements = append(elements, params)
						}
					}
				}
			}
		}

		// Clusters generator: { clusters: { selector: { matchLabels: {...} } } }
		if clusters, ok := gen["clusters"]; ok {
			if clusterMap, ok := clusters.(map[string]interface{}); ok {
				if selector, ok := clusterMap["selector"]; ok {
					if selMap, ok := selector.(map[string]interface{}); ok {
						if matchLabels, ok := selMap["matchLabels"].(map[string]interface{}); ok {
							params := make(map[string]string)
							for k, v := range matchLabels {
								params[k] = toString(v)
							}
							// Use the first label value as cluster name hint
							if env, ok := params["env"]; ok {
								params["cluster"] = env
							}
							elements = append(elements, params)
						}
					}
				}
			}
		}

		// Git file generator: { git: { file: { path: "...", repository: "..." } } }
		if git, ok := gen["git"]; ok {
			if gitMap, ok := git.(map[string]interface{}); ok {
				if file, ok := gitMap["file"]; ok {
					if fileMap, ok := file.(map[string]interface{}); ok {
						if path, ok := fileMap["path"].(string); ok {
							params := map[string]string{
								"git_file_path": path,
							}
							elements = append(elements, params)
						}
					}
				}
			}
		}
	}

	return elements
}

// substituteVars replaces {{key}} placeholders in a string with values from params.
func substituteVars(s string, params map[string]string) string {
	for k, v := range params {
		s = strings.ReplaceAll(s, "{{"+k+"}}", v)
		s = strings.ReplaceAll(s, "{{ "+k+" }}", v)
	}
	return s
}

// toString converts an interface{} to string.
func toString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	default:
		return ""
	}
}

// parseKustomizeFile parses a plain Kustomize kustomization.yaml (not FluxCD).
// Uses the enhanced KustomizeInfo to extract base/overlay relationships.
func parseKustomizeFile(data []byte, filePath string) *Resource {
	kInfo := ParseKustomization(data, filePath)
	if kInfo == nil {
		return nil
	}

	r := &Resource{
		Kind:     "Kustomize",
		FilePath: filePath,
		Name:     filePath,
		RawYAML:  string(data),
		Images:   kInfo.Images,
	}

	// Set layout role based on analysis
	if kInfo.IsBase {
		r.LayoutRole = "base"
	} else if kInfo.IsOverlay {
		r.LayoutRole = "overlay"
		r.BasePath = kInfo.BasePath
	}

	// Combine resources + bases as dependencies
	for _, ref := range kInfo.Resources {
		r.DependsOn = append(r.DependsOn, ref)
	}
	for _, ref := range kInfo.Components {
		r.DependsOn = append(r.DependsOn, ref)
	}

	// Set namespace override
	if kInfo.Namespace != "" {
		r.Namespace = kInfo.Namespace
	}

	return r
}
