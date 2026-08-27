package gitops

import (
	"fmt"
	"strings"
	"time"
)

// DriftType represents the category of configuration drift.
type DriftType string

const (
	DriftSuspended   DriftType = "suspended"    // suspended in cluster but not in Git
	DriftResumed     DriftType = "resumed"      // active in cluster but suspended in Git
	DriftVersion     DriftType = "version"      // chart version differs
	DriftMissing     DriftType = "missing"      // exists in Git but not in cluster
	DriftOrphaned    DriftType = "orphaned"     // exists in cluster but not in Git
	DriftValues      DriftType = "values"       // values differ
)

// DriftEntry represents a single drift between Git desired state and live cluster state.
type DriftEntry struct {
	// Resource identity
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Cluster   string `json:"cluster,omitempty"`

	// Drift metadata
	DriftType   DriftType `json:"drift_type"`
	Severity    string    `json:"severity"` // critical, warning, info
	Description string    `json:"description"`

	// Desired (Git) values
	GitValue string `json:"git_value,omitempty"`

	// Actual (cluster) values
	ClusterValue string `json:"cluster_value,omitempty"`

	// File path in Git repo
	FilePath string `json:"file_path,omitempty"`

	// When detected
	DetectedAt time.Time `json:"detected_at"`
}

// DriftResult holds the full drift analysis for a GitOps repository vs a cluster.
type DriftResult struct {
	RepoID      string         `json:"repo_id"`
	RepoName    string         `json:"repo_name"`
	ClusterID   string         `json:"cluster_id,omitempty"`
	ClusterName string         `json:"cluster_name,omitempty"`
	Entries     []DriftEntry   `json:"entries"`
	Summary     DriftSummary   `json:"summary"`
	ScannedAt   time.Time      `json:"scanned_at"`
}

// DriftSummary provides aggregate counts.
type DriftSummary struct {
	TotalDrifts   int `json:"total_drifts"`
	Critical      int `json:"critical"`
	Warning       int `json:"warning"`
	Info          int `json:"info"`
	Suspended     int `json:"suspended"`
	Resumed       int `json:"resumed"`
	VersionDrift  int `json:"version_drift"`
	Missing       int `json:"missing"`
	Orphaned      int `json:"orphaned"`
	TotalCompared int `json:"total_compared"`
}

// LiveResource represents a resource from the live cluster, normalized for comparison.
type LiveResource struct {
	Kind      string
	Name      string
	Namespace string
	Suspended bool
	Version   string // chart version
	Revision  string // last applied revision
	Health    string
}

// resourceKey produces a unique matching key.
func resourceKey(kind, name, namespace string) string {
	return strings.ToLower(fmt.Sprintf("%s/%s/%s", kind, namespace, name))
}

// DetectDrift compares Git desired state against live cluster state.
// gitResources: parsed resources from the Git manifest repository.
// liveResources: resources queried from the live Kubernetes cluster.
func DetectDrift(repo *Repo, gitResources []Resource, liveResources []LiveResource) *DriftResult {
	result := &DriftResult{
		RepoID:    repo.ID.String(),
		RepoName:  repo.Name,
		Entries:   make([]DriftEntry, 0),
		ScannedAt: time.Now(),
	}

	// Build lookup maps
	liveMap := make(map[string]LiveResource, len(liveResources))
	for _, lr := range liveResources {
		key := resourceKey(lr.Kind, lr.Name, lr.Namespace)
		liveMap[key] = lr
	}

	gitMap := make(map[string]Resource, len(gitResources))
	for _, gr := range gitResources {
		// Only compare FluxCD-managed resources
		if !isFluxManaged(gr.Kind) {
			continue
		}
		key := resourceKey(gr.Kind, gr.Name, gr.Namespace)
		gitMap[key] = gr
	}

	compared := 0

	// 1. Check each Git resource against the live cluster
	for key, gr := range gitMap {
		lr, exists := liveMap[key]
		if !exists {
			// Resource defined in Git but not found in cluster
			result.Entries = append(result.Entries, DriftEntry{
				Kind:        gr.Kind,
				Name:        gr.Name,
				Namespace:   gr.Namespace,
				Cluster:     gr.Cluster,
				DriftType:   DriftMissing,
				Severity:    "info",
				Description: fmt.Sprintf("%s %s/%s exists in Git but is not deployed in the cluster", gr.Kind, gr.Namespace, gr.Name),
				GitValue:    "defined",
				ClusterValue: "not found",
				FilePath:    gr.FilePath,
				DetectedAt:  time.Now(),
			})
			compared++
			continue
		}

		compared++

		// Check suspend drift: Git says active, cluster says suspended
		if !gr.Suspended && lr.Suspended {
			result.Entries = append(result.Entries, DriftEntry{
				Kind:        gr.Kind,
				Name:        gr.Name,
				Namespace:   gr.Namespace,
				Cluster:     gr.Cluster,
				DriftType:   DriftSuspended,
				Severity:    "critical",
				Description: fmt.Sprintf("%s %s/%s is suspended in the cluster but active in Git (someone ran suspend via CLI)", gr.Kind, gr.Namespace, gr.Name),
				GitValue:    "active",
				ClusterValue: "suspended",
				FilePath:    gr.FilePath,
				DetectedAt:  time.Now(),
			})
		}

		// Check resume drift: Git says suspended, cluster says active
		if gr.Suspended && !lr.Suspended {
			result.Entries = append(result.Entries, DriftEntry{
				Kind:        gr.Kind,
				Name:        gr.Name,
				Namespace:   gr.Namespace,
				Cluster:     gr.Cluster,
				DriftType:   DriftResumed,
				Severity:    "warning",
				Description: fmt.Sprintf("%s %s/%s is active in the cluster but suspended in Git", gr.Kind, gr.Namespace, gr.Name),
				GitValue:    "suspended",
				ClusterValue: "active",
				FilePath:    gr.FilePath,
				DetectedAt:  time.Now(),
			})
		}

		// Check chart version drift
		if gr.Version != "" && lr.Version != "" && gr.Version != lr.Version {
			result.Entries = append(result.Entries, DriftEntry{
				Kind:        gr.Kind,
				Name:        gr.Name,
				Namespace:   gr.Namespace,
				Cluster:     gr.Cluster,
				DriftType:   DriftVersion,
				Severity:    "warning",
				Description: fmt.Sprintf("%s %s/%s chart version differs: Git=%s, Cluster=%s", gr.Kind, gr.Namespace, gr.Name, gr.Version, lr.Version),
				GitValue:    gr.Version,
				ClusterValue: lr.Version,
				FilePath:    gr.FilePath,
				DetectedAt:  time.Now(),
			})
		}
	}

	// 2. Check for orphaned resources (in cluster but not in Git)
	for key, lr := range liveMap {
		if _, exists := gitMap[key]; !exists {
			result.Entries = append(result.Entries, DriftEntry{
				Kind:        lr.Kind,
				Name:        lr.Name,
				Namespace:   lr.Namespace,
				DriftType:   DriftOrphaned,
				Severity:    "info",
				Description: fmt.Sprintf("%s %s/%s exists in the cluster but is not defined in Git", lr.Kind, lr.Namespace, lr.Name),
				GitValue:    "not defined",
				ClusterValue: fmt.Sprintf("deployed (health: %s)", lr.Health),
				DetectedAt:  time.Now(),
			})
		}
	}

	// Build summary
	result.Summary = buildSummary(result.Entries, compared)
	return result
}

// DetectDriftMultiCluster runs drift detection for a set of Git resources against
// multiple clusters. liveByCluster maps cluster name -> live resources.
func DetectDriftMultiCluster(repo *Repo, gitResources []Resource, liveByCluster map[string][]LiveResource) *DriftResult {
	result := &DriftResult{
		RepoID:    repo.ID.String(),
		RepoName:  repo.Name,
		Entries:   make([]DriftEntry, 0),
		ScannedAt: time.Now(),
	}

	compared := 0

	for clusterName, liveResources := range liveByCluster {
		// Filter git resources that target this cluster
		clusterGitResources := filterResourcesByCluster(gitResources, clusterName)

		liveMap := make(map[string]LiveResource, len(liveResources))
		for _, lr := range liveResources {
			key := resourceKey(lr.Kind, lr.Name, lr.Namespace)
			liveMap[key] = lr
		}

		gitMap := make(map[string]Resource, len(clusterGitResources))
		for _, gr := range clusterGitResources {
			if !isFluxManaged(gr.Kind) {
				continue
			}
			key := resourceKey(gr.Kind, gr.Name, gr.Namespace)
			gitMap[key] = gr
		}

		for key, gr := range gitMap {
			lr, exists := liveMap[key]
			if !exists {
				result.Entries = append(result.Entries, DriftEntry{
					Kind:         gr.Kind,
					Name:         gr.Name,
					Namespace:    gr.Namespace,
					Cluster:      clusterName,
					DriftType:    DriftMissing,
					Severity:     "info",
					Description:  fmt.Sprintf("%s %s/%s exists in Git but is not deployed in cluster %s", gr.Kind, gr.Namespace, gr.Name, clusterName),
					GitValue:     "defined",
					ClusterValue: "not found",
					FilePath:     gr.FilePath,
					DetectedAt:   time.Now(),
				})
				compared++
				continue
			}

			compared++

			if !gr.Suspended && lr.Suspended {
				result.Entries = append(result.Entries, DriftEntry{
					Kind:         gr.Kind,
					Name:         gr.Name,
					Namespace:    gr.Namespace,
					Cluster:      clusterName,
					DriftType:    DriftSuspended,
					Severity:     "critical",
					Description:  fmt.Sprintf("%s %s/%s is suspended in cluster %s but active in Git", gr.Kind, gr.Namespace, gr.Name, clusterName),
					GitValue:     "active",
					ClusterValue: "suspended",
					FilePath:     gr.FilePath,
					DetectedAt:   time.Now(),
				})
			}

			if gr.Suspended && !lr.Suspended {
				result.Entries = append(result.Entries, DriftEntry{
					Kind:         gr.Kind,
					Name:         gr.Name,
					Namespace:    gr.Namespace,
					Cluster:      clusterName,
					DriftType:    DriftResumed,
					Severity:     "warning",
					Description:  fmt.Sprintf("%s %s/%s is active in cluster %s but suspended in Git", gr.Kind, gr.Namespace, gr.Name, clusterName),
					GitValue:     "suspended",
					ClusterValue: "active",
					FilePath:     gr.FilePath,
					DetectedAt:   time.Now(),
				})
			}

			if gr.Version != "" && lr.Version != "" && gr.Version != lr.Version {
				result.Entries = append(result.Entries, DriftEntry{
					Kind:         gr.Kind,
					Name:         gr.Name,
					Namespace:    gr.Namespace,
					Cluster:      clusterName,
					DriftType:    DriftVersion,
					Severity:     "warning",
					Description:  fmt.Sprintf("%s %s/%s version drift in cluster %s: Git=%s, Cluster=%s", gr.Kind, gr.Namespace, gr.Name, clusterName, gr.Version, lr.Version),
					GitValue:     gr.Version,
					ClusterValue: lr.Version,
					FilePath:     gr.FilePath,
					DetectedAt:   time.Now(),
				})
			}
		}

		// Orphaned in this cluster
		for key, lr := range liveMap {
			if _, exists := gitMap[key]; !exists {
				result.Entries = append(result.Entries, DriftEntry{
					Kind:         lr.Kind,
					Name:         lr.Name,
					Namespace:    lr.Namespace,
					Cluster:      clusterName,
					DriftType:    DriftOrphaned,
					Severity:     "info",
					Description:  fmt.Sprintf("%s %s/%s in cluster %s is not defined in Git", lr.Kind, lr.Namespace, lr.Name, clusterName),
					GitValue:     "not defined",
					ClusterValue: fmt.Sprintf("deployed (health: %s)", lr.Health),
					DetectedAt:   time.Now(),
				})
			}
		}
	}

	result.Summary = buildSummary(result.Entries, compared)
	return result
}

// isFluxManaged returns true if the resource kind is managed by FluxCD.
func isFluxManaged(kind string) bool {
	switch kind {
	case "HelmRelease", "Kustomization":
		return true
	}
	return false
}

// filterResourcesByCluster returns resources that match the given cluster name.
// If the resource has no cluster set, it is included (assumed global).
func filterResourcesByCluster(resources []Resource, clusterName string) []Resource {
	var result []Resource
	for _, r := range resources {
		if r.Cluster == "" || strings.EqualFold(r.Cluster, clusterName) {
			result = append(result, r)
		}
	}
	return result
}

// buildSummary computes aggregate drift counts.
func buildSummary(entries []DriftEntry, compared int) DriftSummary {
	s := DriftSummary{
		TotalDrifts:   len(entries),
		TotalCompared: compared,
	}
	for _, e := range entries {
		switch e.Severity {
		case "critical":
			s.Critical++
		case "warning":
			s.Warning++
		case "info":
			s.Info++
		}
		switch e.DriftType {
		case DriftSuspended:
			s.Suspended++
		case DriftResumed:
			s.Resumed++
		case DriftVersion:
			s.VersionDrift++
		case DriftMissing:
			s.Missing++
		case DriftOrphaned:
			s.Orphaned++
		}
	}
	return s
}
