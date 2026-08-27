package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	dockerpkg "github.com/pepa/pepa/internal/docker"
	"github.com/pepa/pepa/pkg/models"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// DiscoveredService represents a service discovered from external sources
type DiscoveredService struct {
	Name          string            `json:"name"`
	Namespace     string            `json:"namespace"`
	Cluster       string            `json:"cluster"` // cluster name
	Source        string            `json:"source"`  // "pepa", "argocd", "fluxcd", "manual"
	Status        string            `json:"status"`  // "running", "deploying", "failed", "unknown"
	Health        string            `json:"health"`  // "healthy", "degraded", "progressing", "unknown"
	Replicas      int               `json:"replicas"`
	ReadyReplicas int               `json:"ready_replicas"`
	Image         string            `json:"image"`
	LastUpdated   time.Time         `json:"last_updated"`
	Labels        map[string]string `json:"labels"`
	SyncStatus    string            `json:"sync_status"` // "synced", "out_of_sync", "unknown"
	URL           string            `json:"url,omitempty"`
}

// In-memory cache for discovery results.
var (
	// Discovery result cache — avoids hitting k8s API on every request
	discoveryCacheMu         sync.RWMutex
	discoveryCache           []DiscoveredService
	discoveryCacheTime       time.Time
	discoveryCacheTTL        = 2 * time.Minute
)

// DiscoveryService handles service discovery from multiple sources
type DiscoveryService struct {
	// ArgoCD client
	ArgoCDEnabled bool
	ArgoCDURL     string
	ArgoCDToken   string

	// FluxCD client
	FluxCDEnabled    bool
	FluxCDKubeconfig string
}

// ArgoCDConfig holds ArgoCD connection details for a cluster
type ArgoCDConfig struct {
	URL   string
	Token string
}

func registerDiscoveryRoutes(r *gin.RouterGroup, deps Dependencies) {
	discovery := r.Group("/discovery")
	{
		// Root endpoint — redirect to /services for convenience
		discovery.GET("", func(c *gin.Context) {
			c.Redirect(http.StatusMovedPermanently, "/api/v1/discovery/services")
		})
		discovery.GET("/services", discoverServices(deps))
		discovery.POST("/sync", syncServices(deps))
		discovery.GET("/sources", getDiscoverySources(deps))
		discovery.GET("/clusters", getDiscoveryClusters(deps))
		discovery.GET("/namespaces", getDiscoveryNamespaces(deps))
		// FluxCD management
		discovery.POST("/fluxcd/:cluster/:namespace/:name/suspend", fluxcdSuspend(deps))
		discovery.POST("/fluxcd/:cluster/:namespace/:name/resume", fluxcdResume(deps))
		discovery.POST("/fluxcd/:cluster/:namespace/:name/reconcile", fluxcdReconcile(deps))
		discovery.DELETE("/fluxcd/:cluster/:namespace/:name", fluxcdDelete(deps))
		// Kubernetes deployment management
		discovery.GET("/k8s/:cluster/:namespace/:name", k8sGetDeployment(deps))
		discovery.PUT("/k8s/:cluster/:namespace/:name", k8sUpdateDeployment(deps))
		discovery.POST("/k8s/:cluster/:namespace/:name/scale", k8sScaleDeployment(deps))
		discovery.POST("/k8s/:cluster/:namespace/:name/restart", k8sRestartDeployment(deps))
		discovery.DELETE("/k8s/:cluster/:namespace/:name", k8sDeleteDeployment(deps))
		discovery.GET("/k8s/:cluster/:namespace/:name/logs", k8sGetLogs(deps))
		discovery.GET("/k8s/:cluster/:namespace/:name/events", k8sGetEvents(deps))
		// Docker container logs (for discovered containers from Docker hosts)
		discovery.GET("/docker-container/:host/:name/logs", dockerContainerLogs(deps))
	}

	// Team workflow config routes
	tw := r.Group("/team-workflows")
	{
		tw.GET("", listTeamWorkflows(deps))
		tw.GET("/:team", getTeamWorkflow(deps))
		tw.PUT("/:team", saveTeamWorkflow(deps))
		tw.DELETE("/:team", deleteTeamWorkflow(deps))
	}
}

func discoverServices(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		tenantID := auth.GetTenantID(c)

		var allServices []DiscoveredService

		// Check cache first (2min TTL)
		discoveryCacheMu.RLock()
		cacheValid := discoveryCache != nil && time.Since(discoveryCacheTime) < discoveryCacheTTL
		if cacheValid {
			allServices = make([]DiscoveredService, len(discoveryCache))
			copy(allServices, discoveryCache)
		}
		discoveryCacheMu.RUnlock()

		if !cacheValid {
			// 1. Get services from PEPA database
			pepaServices, err := getPEPAServices(ctx, deps)
			if err == nil {
				allServices = append(allServices, pepaServices...)
			}

			// 2. Get Docker services from database
			dockerServices, err := getDockerServices(ctx, deps, tenantID)
			if err == nil && len(dockerServices) > 0 {
				allServices = append(allServices, dockerServices...)
			}

			// 3. Discover raw containers from connected Docker hosts (not deployed via PEPA)
			discoveredContainers, err := discoverDockerContainers(ctx, deps, tenantID)
			if err == nil && len(discoveredContainers) > 0 {
				allServices = append(allServices, discoveredContainers...)
			}

			// 4. Discover from all registered clusters in parallel
			if deps.Repos.Cluster != nil && tenantID != (uuid.UUID{}) {
				clusterList, err := deps.Repos.Cluster.List(ctx, tenantID)
				if err == nil {
					type clusterResult struct {
						services []DiscoveredService
					}
					var wg sync.WaitGroup
					results := make([]clusterResult, len(clusterList))
					for i := range clusterList {
						if !clusterList[i].IsActive {
							log.Printf("discovery: skipping cluster %s (not active)", clusterList[i].Name)
							continue
						}
						wg.Add(1)
						go func(idx int) {
							defer wg.Done()
							cluster := clusterList[idx]
							kubeconfig, err := deps.Repos.Cluster.GetKubeconfig(ctx, cluster.ID, uuid.Nil)
							if err != nil || kubeconfig == "" {
								log.Printf("discovery: skipping cluster %s (kubeconfig unavailable: err=%v, empty=%v)", cluster.Name, err, kubeconfig == "")
								return
							}
							var clusterSvcs []DiscoveredService

							// Discover FluxCD resources — try for all clusters (auto-detect)
							fluxSvcs, err := discoverFromFluxCD(ctx, kubeconfig, cluster.Name)
							if err != nil {
								log.Printf("discovery: fluxcd error for cluster %s: %v", cluster.Name, err)
							} else {
								log.Printf("discovery: fluxcd found %d services for cluster %s", len(fluxSvcs), cluster.Name)
								clusterSvcs = append(clusterSvcs, fluxSvcs...)
							}

							// Discover ArgoCD resources — try for all clusters (auto-detect)
							argoSvcs, err := discoverFromArgoCD(ctx, kubeconfig, cluster.Name)
							if err != nil {
								// ArgoCD CRDs not installed — this is normal, no need to log
							} else {
								log.Printf("discovery: argocd found %d services for cluster %s", len(argoSvcs), cluster.Name)
								clusterSvcs = append(clusterSvcs, argoSvcs...)
							}

							// Discover Kubernetes deployments
							k8sSvcs, err := discoverFromKubernetesCluster(ctx, kubeconfig, cluster.Name)
							if err != nil {
								log.Printf("discovery: k8s error for %s: %v", cluster.Name, err)
							}
							// Filter out already discovered within this cluster
							k8sAdded := 0
							k8sSkipped := 0
							for _, k8sSvc := range k8sSvcs {
								found := false
								for _, existing := range clusterSvcs {
									if existing.Name == k8sSvc.Name && existing.Namespace == k8sSvc.Namespace && existing.Cluster == k8sSvc.Cluster {
										found = true
										break
									}
								}
								if !found {
									clusterSvcs = append(clusterSvcs, k8sSvc)
									k8sAdded++
								} else {
									k8sSkipped++
								}
							}
							log.Printf("discovery: k8s found %d deployments for cluster %s (%d added, %d deduplicated)", len(k8sSvcs), cluster.Name, k8sAdded, k8sSkipped)
							results[idx] = clusterResult{services: clusterSvcs}
						}(i)
					}
					wg.Wait()

					// Merge results from all clusters
					for _, r := range results {
						allServices = append(allServices, r.services...)
					}
				}
			}

			log.Printf("discovery: total services discovered: %d (pepa: %d, argocd: %d, fluxcd: %d, docker: %d, manual: %d)",
				len(allServices),
				countBySource(allServices, "pepa"),
				countBySource(allServices, "argocd"),
				countBySource(allServices, "fluxcd"),
				countBySource(allServices, "docker")+countBySource(allServices, "docker-container"),
				countBySource(allServices, "manual"),
			)

			// Update cache
			discoveryCacheMu.Lock()
			discoveryCache = allServices
			discoveryCacheTime = time.Now()
			discoveryCacheMu.Unlock()
		}

		// Apply server-side filters
		nsFilter := c.Query("namespace")
		sourceFilter := c.Query("source")
		clusterFilter := c.Query("cluster")
		healthFilter := c.Query("health")
		statusFilter := c.Query("status")
		searchFilter := c.Query("search")

		filtered := make([]DiscoveredService, 0, len(allServices))
		for _, svc := range allServices {
			if nsFilter != "" && svc.Namespace != nsFilter {
				continue
			}
			if sourceFilter != "" && svc.Source != sourceFilter {
				continue
			}
			if clusterFilter != "" && svc.Cluster != clusterFilter {
				continue
			}
			if healthFilter != "" && svc.Health != healthFilter {
				continue
			}
			if statusFilter != "" && svc.Status != statusFilter {
				continue
			}
			if searchFilter != "" {
				name := strings.ToLower(svc.Name)
				ns := strings.ToLower(svc.Namespace)
				s := strings.ToLower(searchFilter)
				if !strings.Contains(name, s) && !strings.Contains(ns, s) {
					continue
				}
			}
			filtered = append(filtered, svc)
		}

		// Collect namespaces for filter options
		namespaces := map[string]int{}
		for _, svc := range allServices {
			namespaces[svc.Namespace]++
		}

		// Group services by cluster (for filter dropdown — includes Docker hosts)
		allClusters := map[string]int{}
		for _, svc := range filtered {
			if svc.Cluster != "" {
				allClusters[svc.Cluster]++
			} else {
				allClusters["default"]++
			}
		}

		// Count only real Kubernetes clusters/namespaces for stat cards (exclude Docker sources)
		k8sClusterSet := map[string]struct{}{}
		for _, svc := range filtered {
			if svc.Source == "docker" || svc.Source == "docker-container" {
				continue
			}
			key := svc.Cluster
			if key == "" {
				key = "default"
			}
			k8sClusterSet[key] = struct{}{}
		}

		k8sNamespaceSet := map[string]struct{}{}
		for _, svc := range allServices {
			if svc.Source == "docker" || svc.Source == "docker-container" {
				continue
			}
			k8sNamespaceSet[svc.Namespace] = struct{}{}
		}

		c.JSON(http.StatusOK, gin.H{
			"services":         filtered,
			"total":            len(filtered),
			"total_unfiltered": len(allServices),
			"sources": map[string]int{
				"pepa":             countBySource(allServices, "pepa"),
				"argocd":           countBySource(allServices, "argocd"),
				"fluxcd":           countBySource(allServices, "fluxcd"),
				"docker":           countBySource(allServices, "docker"),
				"docker-container": countBySource(allServices, "docker-container"),
				"manual":           countBySource(allServices, "manual"),
			},
			"clusters":         allClusters,
			"namespaces":       namespaces,
			"k8s_clusters":     len(k8sClusterSet),
			"k8s_namespaces":   len(k8sNamespaceSet),
		})
	}
}

func syncServices(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		tenantID := auth.GetTenantID(c)

		var synced int
		var errors []string

		if deps.Repos.Cluster != nil && tenantID != (uuid.UUID{}) {
			clusterList, err := deps.Repos.Cluster.List(ctx, tenantID)
			if err == nil {
				for _, cluster := range clusterList {
					if !cluster.IsActive {
						continue
					}
					kubeconfig, err := deps.Repos.Cluster.GetKubeconfig(ctx, cluster.ID, uuid.Nil)
					if err != nil || kubeconfig == "" {
						continue
					}

					if cluster.FluxInstalled {
						_, err = discoverFromFluxCD(ctx, kubeconfig, cluster.Name)
						if err != nil {
							errors = append(errors, fmt.Sprintf("%s/fluxcd: %v", cluster.Name, err))
						} else {
							synced++
						}
					}

					_, err = discoverFromKubernetesCluster(ctx, kubeconfig, cluster.Name)
					if err != nil {
						errors = append(errors, fmt.Sprintf("%s: %v", cluster.Name, err))
					} else {
						synced++
					}
				}
			}
		}

		if len(errors) > 0 {
			// Invalidate cache after sync
			discoveryCacheMu.Lock()
			discoveryCacheTime = time.Time{}
			discoveryCacheMu.Unlock()
			logAudit(deps, c, "sync", "discovery", "all", nil, gin.H{"synced": synced, "errors": len(errors)})
			c.JSON(http.StatusPartialContent, gin.H{"synced": synced, "errors": errors})
			return
		}
		// Invalidate cache after sync
		discoveryCacheMu.Lock()
		discoveryCacheTime = time.Time{}
		discoveryCacheMu.Unlock()
		logAudit(deps, c, "sync", "discovery", "all", nil, gin.H{"synced": synced})
		c.JSON(http.StatusOK, gin.H{"message": "Sync completed successfully", "synced": synced})
	}
}

func getDiscoverySources(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		sources := []map[string]interface{}{
			{
				"name":    "PEPA",
				"type":    "internal",
				"enabled": true,
				"status":  "active",
			},
			{
				"name":    "ArgoCD",
				"type":    "gitops",
				"enabled": true,
				"status":  "connected",
			},
			{
				"name":    "FluxCD",
				"type":    "gitops",
				"enabled": true,
				"status":  "connected",
			},
			{
				"name":    "Kubernetes",
				"type":    "cluster",
				"enabled": true,
				"status":  "active",
			},
			{
				"name":    "Docker Hosts",
				"type":    "container",
				"enabled": true,
				"status":  "active",
			},
		}

		c.JSON(http.StatusOK, gin.H{
			"sources": sources,
			"total":   len(sources),
		})
	}
}

// Helper functions

// ── Real Kubernetes Discovery ──────────────────────────────

// buildRestConfig parses kubeconfig and returns a Kubernetes REST config
func buildRestConfig(kubeconfig string) (*rest.Config, error) {
	config, err := clientcmd.NewClientConfigFromBytes([]byte(kubeconfig))
	if err != nil {
		return nil, fmt.Errorf("parse kubeconfig: %w", err)
	}
	restConfig, err := config.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("build rest config: %w", err)
	}
	// Allow self-signed certificates (common for k3s/kind/minikube)
	restConfig.TLSClientConfig.Insecure = true
	restConfig.TLSClientConfig.CAData = nil
	restConfig.TLSClientConfig.CAFile = ""
	return restConfig, nil
}

// k8sRequest makes an authenticated request to the Kubernetes API
func k8sRequest(restConfig *rest.Config, path string) ([]byte, error) {
	transport, err := rest.TransportFor(restConfig)
	if err != nil {
		return nil, fmt.Errorf("create transport: %w", err)
	}
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}
	url := restConfig.Host + path
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("k8s API request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("k8s API returned %d for %s", resp.StatusCode, path)
	}
	var buf []byte
	buf = make([]byte, 0, 64*1024)
	tmp := make([]byte, 4096)
	for {
		n, readErr := resp.Body.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if readErr != nil {
			break
		}
	}
	return buf, nil
}

// k8sRequestWithBody makes an authenticated Kubernetes API request with custom method and body
func k8sRequestWithBody(ctx context.Context, restConfig *rest.Config, method, path string, body string) ([]byte, error) {
	transport, err := rest.TransportFor(restConfig)
	if err != nil {
		return nil, fmt.Errorf("create transport: %w", err)
	}
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second}

	url := strings.TrimSuffix(restConfig.Host, "/") + path
	var req *http.Request
	if body != "" {
		req, err = http.NewRequestWithContext(ctx, method, url, strings.NewReader(body))
	} else {
		req, err = http.NewRequestWithContext(ctx, method, url, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != "" {
		req.Header.Set("Content-Type", "application/merge-patch+json")
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("k8s API request: %w", err)
	}
	defer resp.Body.Close()

	var buf []byte
	buf = make([]byte, 0, 64*1024)
	tmp := make([]byte, 4096)
	for {
		n, readErr := resp.Body.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if readErr != nil {
			break
		}
	}

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("k8s API returned %d: %s", resp.StatusCode, string(buf))
	}
	return buf, nil
}

// discoverFromFluxCD discovers services managed by FluxCD HelmReleases
func discoverFromFluxCD(ctx context.Context, kubeconfig string, clusterName string) ([]DiscoveredService, error) {
	restConfig, err := buildRestConfig(kubeconfig)
	if err != nil {
		return nil, err
	}

	var services []DiscoveredService

	// List FluxCD HelmReleases — try newest API version first
	data, err := k8sRequest(restConfig, "/apis/helm.toolkit.fluxcd.io/v2/helmreleases")
	if err != nil {
		// Try v2beta2
		data, err = k8sRequest(restConfig, "/apis/helm.toolkit.fluxcd.io/v2beta2/helmreleases")
		if err != nil {
			// Try v2beta1
			data, err = k8sRequest(restConfig, "/apis/helm.toolkit.fluxcd.io/v2beta1/helmreleases")
			if err != nil {
				return nil, fmt.Errorf("list helmreleases: %w", err)
			}
		}
	}

	var list struct {
		Items []struct {
			Metadata struct {
				Name              string            `json:"name"`
				Namespace         string            `json:"namespace"`
				Labels            map[string]string `json:"labels"`
				CreationTimestamp string            `json:"creationTimestamp"`
			} `json:"metadata"`
			Status struct {
				Conditions []struct {
					Type   string `json:"type"`
					Status string `json:"status"`
					Reason string `json:"reason"`
				} `json:"conditions"`
				HelmChart           string `json:"helmChart"`
				LastAppliedRevision string `json:"lastAppliedRevision"`
			} `json:"status"`
			Spec struct {
				// v2beta1/v2beta2 format: spec.chart.spec
				Chart struct {
					Spec struct {
						Chart   string `json:"chart"`
						Version string `json:"version"`
					} `json:"spec"`
				} `json:"chart"`
				// v2 format: spec.chartRef
				ChartRef *struct {
					Name      string `json:"name"`
					Namespace string `json:"namespace"`
					Kind      string `json:"kind"`
				} `json:"chartRef"`
			} `json:"spec"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("parse helmreleases: %w", err)
	}

	for _, item := range list.Items {
		health := "unknown"
		syncStatus := "unknown"
		for _, cond := range item.Status.Conditions {
			if cond.Type == "Ready" {
				if cond.Status == "True" {
					health = "healthy"
					syncStatus = "synced"
				} else {
					health = "degraded"
					syncStatus = "out_of_sync"
				}
				break
			}
		}

		status := "running"
		if health == "degraded" {
			status = "failed"
		}

		ts, _ := time.Parse(time.RFC3339, item.Metadata.CreationTimestamp)

		// Extract chart/image info from either v2 chartRef or older chart.spec
		image := item.Spec.Chart.Spec.Chart
		if item.Spec.Chart.Spec.Version != "" {
			image += ":" + item.Spec.Chart.Spec.Version
		}
		if image == "" && item.Spec.ChartRef != nil {
			image = item.Spec.ChartRef.Name
			if item.Spec.ChartRef.Kind != "" {
				image = item.Spec.ChartRef.Kind + "/" + image
			}
		}

		services = append(services, DiscoveredService{
			Name:          item.Metadata.Name,
			Namespace:     item.Metadata.Namespace,
			Cluster:       clusterName,
			Source:        "fluxcd",
			Status:        status,
			Health:        health,
			Replicas:      1,
			ReadyReplicas: 1,
			Image:         image,
			LastUpdated:   ts,
			Labels:        item.Metadata.Labels,
			SyncStatus:    syncStatus,
		})
	}

	return services, nil
}

// discoverFromArgoCD discovers services from ArgoCD Applications deployed in a cluster.
// It tries to find the ArgoCD server running in the cluster (argocd-server service)
// and queries its API for Applications.
func discoverFromArgoCD(ctx context.Context, kubeconfig string, clusterName string) ([]DiscoveredService, error) {
	restConfig, err := buildRestConfig(kubeconfig)
	if err != nil {
		return nil, err
	}

	// Try to discover ArgoCD Applications via the Kubernetes API (CRDs)
	// This works without needing the ArgoCD server URL or token
	data, err := k8sRequest(restConfig, "/apis/argoproj.io/v1alpha1/applications")
	if err != nil {
		return nil, fmt.Errorf("list argocd applications: %w", err)
	}

	var list struct {
		Items []struct {
			Metadata struct {
				Name              string            `json:"name"`
				Namespace         string            `json:"namespace"`
				Labels            map[string]string `json:"labels"`
				Annotations       map[string]string `json:"annotations"`
				CreationTimestamp string            `json:"creationTimestamp"`
			} `json:"metadata"`
			Spec struct {
				Source struct {
					RepoURL        string `json:"repoURL"`
					Path           string `json:"path"`
					Chart          string `json:"chart"`
					TargetRevision string `json:"targetRevision"`
				} `json:"source"`
				Sources []struct {
					RepoURL        string `json:"repoURL"`
					Chart          string `json:"chart"`
					TargetRevision string `json:"targetRevision"`
				} `json:"sources"`
				Destination struct {
					Server    string `json:"server"`
					Namespace string `json:"namespace"`
					Name      string `json:"name"`
				} `json:"destination"`
			} `json:"spec"`
			Status struct {
				Sync struct {
					Status   string `json:"status"`
					Revision string `json:"revision"`
				} `json:"sync"`
				Health struct {
					Status  string `json:"status"`
					Message string `json:"message"`
				} `json:"health"`
				Summary struct {
					Images []string `json:"images"`
				} `json:"summary"`
				OperationState *struct {
					Phase string `json:"phase"`
				} `json:"operationState"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("parse argocd applications: %w", err)
	}

	var services []DiscoveredService
	for _, item := range list.Items {
		// Map ArgoCD health status to our health model
		health := "unknown"
		switch strings.ToLower(item.Status.Health.Status) {
		case "healthy":
			health = "healthy"
		case "degraded":
			health = "degraded"
		case "progressing", "missing":
			health = "progressing"
		case "suspended":
			health = "unknown"
		}

		// Map sync status
		syncStatus := "unknown"
		switch strings.ToLower(item.Status.Sync.Status) {
		case "synced":
			syncStatus = "synced"
		case "outsynced", "out_of_sync":
			syncStatus = "out_of_sync"
		}

		// Determine overall status
		status := "running"
		if health == "degraded" {
			status = "failed"
		} else if health == "progressing" {
			status = "deploying"
		} else if item.Status.OperationState != nil && item.Status.OperationState.Phase == "Running" {
			status = "deploying"
		}

		// Extract image info
		image := ""
		if len(item.Status.Summary.Images) > 0 {
			image = item.Status.Summary.Images[0]
		} else if item.Spec.Source.Chart != "" {
			image = item.Spec.Source.Chart + ":" + item.Spec.Source.TargetRevision
		}

		ts, _ := time.Parse(time.RFC3339, item.Metadata.CreationTimestamp)

		// Use destination namespace if available, otherwise metadata namespace
		ns := item.Spec.Destination.Namespace
		if ns == "" {
			ns = item.Metadata.Namespace
		}

		services = append(services, DiscoveredService{
			Name:          item.Metadata.Name,
			Namespace:     ns,
			Cluster:       clusterName,
			Source:        "argocd",
			Status:        status,
			Health:        health,
			Replicas:      1,
			ReadyReplicas: 1,
			Image:         image,
			LastUpdated:   ts,
			Labels:        item.Metadata.Labels,
			SyncStatus:    syncStatus,
		})
	}

	return services, nil
}

// discoverFromKubernetesCluster lists deployments from a real cluster
func discoverFromKubernetesCluster(ctx context.Context, kubeconfig string, clusterName string) ([]DiscoveredService, error) {
	restConfig, err := buildRestConfig(kubeconfig)
	if err != nil {
		return nil, err
	}

	data, err := k8sRequest(restConfig, "/apis/apps/v1/deployments")
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}

	var list struct {
		Items []struct {
			Metadata struct {
				Name              string            `json:"name"`
				Namespace         string            `json:"namespace"`
				Labels            map[string]string `json:"labels"`
				Annotations       map[string]string `json:"annotations"`
				CreationTimestamp string            `json:"creationTimestamp"`
			} `json:"metadata"`
			Spec struct {
				Replicas int `json:"replicas"`
				Template struct {
					Spec struct {
						Containers []struct {
							Name  string `json:"name"`
							Image string `json:"image"`
						} `json:"containers"`
					} `json:"spec"`
				} `json:"template"`
			} `json:"spec"`
			Status struct {
				Replicas          int `json:"replicas"`
				ReadyReplicas     int `json:"readyReplicas"`
				AvailableReplicas int `json:"availableReplicas"`
				UpdatedReplicas   int `json:"updatedReplicas"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("parse deployments: %w", err)
	}

	// Skip system namespaces
	skipNS := map[string]bool{
		"kube-system": true, "kube-public": true, "kube-node-lease": true,
		"flux-system": true, "calico-system": true, "tigera-operator": true,
		"argocd": true,
	}

	var services []DiscoveredService
	for _, item := range list.Items {
		if skipNS[item.Metadata.Namespace] {
			continue
		}

		health := "unknown"
		if item.Status.ReadyReplicas >= item.Spec.Replicas && item.Spec.Replicas > 0 {
			health = "healthy"
		} else if item.Status.ReadyReplicas > 0 {
			health = "progressing"
		} else if item.Spec.Replicas > 0 {
			health = "degraded"
		}

		status := "running"
		if health == "degraded" {
			status = "failed"
		} else if health == "progressing" {
			status = "deploying"
		}

		image := ""
		if len(item.Spec.Template.Spec.Containers) > 0 {
			image = item.Spec.Template.Spec.Containers[0].Image
		}

		ts, _ := time.Parse(time.RFC3339, item.Metadata.CreationTimestamp)

		// Detect actual source from labels/annotations
		source := detectDeploymentSource(item.Metadata.Labels, item.Metadata.Annotations)

		services = append(services, DiscoveredService{
			Name:          item.Metadata.Name,
			Namespace:     item.Metadata.Namespace,
			Cluster:       clusterName,
			Source:        source,
			Status:        status,
			Health:        health,
			Replicas:      item.Status.Replicas,
			ReadyReplicas: item.Status.ReadyReplicas,
			Image:         image,
			LastUpdated:   ts,
			Labels:        item.Metadata.Labels,
			SyncStatus:    "synced",
		})
	}

	return services, nil
}

// detectDeploymentSource determines the GitOps tool that manages a deployment
// by inspecting its labels and annotations.
func detectDeploymentSource(labels, annotations map[string]string) string {
	if labels == nil {
		labels = map[string]string{}
	}
	if annotations == nil {
		annotations = map[string]string{}
	}

	// ArgoCD markers
	if labels["app.kubernetes.io/managed-by"] == "argocd" ||
		annotations["argocd.argoproj.io/tracking-id"] != "" ||
		labels["argocd.argoproj.io/instance"] != "" {
		return "argocd"
	}

	// FluxCD markers
	if labels["helm.toolkit.fluxcd.io/name"] != "" ||
		labels["kustomize.toolkit.fluxcd.io/name"] != "" {
		return "fluxcd"
	}

	return "manual"
}

func countBySource(services []DiscoveredService, source string) int {
	count := 0
	for _, svc := range services {
		if svc.Source == source {
			count++
		}
	}
	return count
}

// getPEPAServices returns services managed by PEPA from the database
func getPEPAServices(ctx context.Context, deps Dependencies) ([]DiscoveredService, error) {
	if deps.Repos.Service == nil {
		return []DiscoveredService{}, nil
	}

	result, err := deps.Repos.Service.List(ctx, models.ServiceFilter{PerPage: 10000})
	if err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}

	// Build cluster ID -> name map
	var clusterNames map[string]string
	if deps.Repos.Cluster != nil {
		// We need all clusters for name resolution; use a broad tenant-agnostic query
		// RLS will scope to the current tenant automatically
		clusterList, err := deps.Repos.Cluster.List(ctx, uuid.UUID{})
		if err != nil {
			log.Printf("discovery: failed to list clusters for PEPA services: %v", err)
		} else {
			clusterNames = make(map[string]string, len(clusterList))
			for _, c := range clusterList {
				clusterNames[c.ID.String()] = c.Name
			}
		}
	}

	var services []DiscoveredService
	for _, s := range result.Items {
		health := "unknown"
		switch s.Status {
		case "active", "running":
			health = "healthy"
		case "error", "failed":
			health = "failed"
		case "deploying":
			health = "progressing"
		case "configured":
			health = "healthy"
		}

		status := s.Status
		if status == "" {
			status = "configured"
		}

		// Resolve cluster name from target_clusters
		clusterName := "default"
		if len(s.TargetClusters) > 0 && clusterNames != nil {
			if name, ok := clusterNames[s.TargetClusters[0].String()]; ok {
				clusterName = name
			}
		}

		ns := s.Namespace
		if ns == "" {
			ns = "default"
		}

		image := s.ImageRepository
		if image == "" {
			image = "-"
		}

		services = append(services, DiscoveredService{
			Name:        s.Name,
			Namespace:   ns,
			Cluster:     clusterName,
			Source:      "pepa",
			Status:      status,
			Health:      health,
			Replicas:    0,
			Image:       image,
			LastUpdated: s.UpdatedAt,
			Labels:      map[string]string{},
			SyncStatus:  "synced",
		})
	}

	log.Printf("discovery: found %d PEPA services in database", len(services))
	return services, nil
}

// getDockerServices returns Docker Compose services deployed on Docker hosts
func getDockerServices(ctx context.Context, deps Dependencies, tenantID uuid.UUID) ([]DiscoveredService, error) {
	if deps.Repos.DockerHost == nil {
		return []DiscoveredService{}, nil
	}

	// Get all Docker hosts for name resolution
	hosts, err := deps.Repos.DockerHost.ListHosts(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list docker hosts: %w", err)
	}
	hostNames := make(map[string]string, len(hosts))
	for _, h := range hosts {
		hostNames[h.ID.String()] = h.Name
	}

	// Get all Docker services
	dockerSvcs, err := deps.Repos.DockerHost.ListServices(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list docker services: %w", err)
	}

	var services []DiscoveredService
	for _, ds := range dockerSvcs {
		hostName := hostNames[ds.DockerHostID.String()]
		if hostName == "" {
			hostName = "docker-host"
		}

		var health, status string
		switch ds.Status {
		case "running":
			health = "healthy"
			status = "running"
		case "stopped":
			health = "degraded"
			status = "stopped"
		case "deploying":
			health = "progressing"
			status = "deploying"
		case "error", "failed":
			health = "failed"
			status = "failed"
		default:
			health = "unknown"
			status = ds.Status
		}

		// Count containers as replicas
		replicas := 0
		if ds.Containers != nil {
			var containers []json.RawMessage
			if err := json.Unmarshal(ds.Containers, &containers); err == nil {
				replicas = len(containers)
			}
		}

		// Use first container image as representative
		image := ""
		if ds.Containers != nil {
			var containers []struct {
				Image string `json:"image"`
			}
			if err := json.Unmarshal(ds.Containers, &containers); err == nil && len(containers) > 0 {
				image = containers[0].Image
			}
		}

		services = append(services, DiscoveredService{
			Name:          ds.Name,
			Namespace:     "docker",
			Cluster:       hostName,
			Source:        "docker",
			Status:        status,
			Health:        health,
			Replicas:      replicas,
			ReadyReplicas: replicas,
			Image:         image,
			LastUpdated:   ds.UpdatedAt,
			Labels:        map[string]string{},
			SyncStatus:    "synced",
		})
	}

	return services, nil
}

// discoverDockerContainers discovers running containers from connected Docker hosts
// that are not managed through PEPA's docker_services table.
func discoverDockerContainers(ctx context.Context, deps Dependencies, tenantID uuid.UUID) ([]DiscoveredService, error) {
	if deps.Repos.DockerHost == nil {
		return nil, nil
	}

	hosts, err := deps.Repos.DockerHost.ListHosts(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list docker hosts: %w", err)
	}

	// Get already-tracked compose project names from docker_services
	trackedServices, err := deps.Repos.DockerHost.ListServices(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list docker services: %w", err)
	}
	trackedComposeProjects := make(map[string]bool)
	for _, svc := range trackedServices {
		trackedComposeProjects[svc.Name] = true
	}

	hostNames := make(map[string]string, len(hosts))
	for _, h := range hosts {
		hostNames[h.ID.String()] = h.Name
	}

	var services []DiscoveredService
	for _, host := range hosts {
		if host.Status != "connected" {
			continue
		}

		// Get decrypted credentials for Docker CLI connection
		decrypted, err := deps.Repos.DockerHost.GetHostDecrypted(ctx, host.ID, tenantID)
		if err != nil {
			log.Printf("discovery: failed to decrypt host %s: %v", host.Name, err)
			continue
		}

		cfg := dockerpkg.HostConfig{
			HostType:    decrypted.HostType,
			HostAddress: decrypted.HostAddress,
			TLSCACert:   decrypted.TLSCACert,
			TLSCert:     decrypted.TLSCert,
			TLSKey:      decrypted.TLSKey,
			SSHKey:      decrypted.SSHKey,
		}
		client := dockerpkg.NewClient(cfg)

		discCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		containers, err := client.ListContainers(discCtx, false)
		cancel()
		if err != nil {
			log.Printf("discovery: docker ps error for %s: %v", host.Name, err)
			continue
		}

		for _, c := range containers {
			// Skip containers belonging to already-tracked compose projects
			if c.ComposeProject != "" && trackedComposeProjects[c.ComposeProject] {
				continue
			}

			var health, status string
			switch strings.ToLower(c.State) {
			case "running":
				health = "healthy"
				status = "running"
			case "exited", "stopped":
				health = "degraded"
				status = "stopped"
			case "restarting":
				health = "progressing"
				status = "deploying"
			default:
				health = "unknown"
				status = c.State
			}

			services = append(services, DiscoveredService{
				Name:          c.Name,
				Namespace:     "docker",
				Cluster:       hostNames[host.ID.String()],
				Source:        "docker-container",
				Status:        status,
				Health:        health,
				Replicas:      1,
				ReadyReplicas: 1,
				Image:         c.Image,
				LastUpdated:   time.Now(),
				Labels:        c.Labels,
				SyncStatus:    "live",
			})
		}
	}

	return services, nil
}

// getDiscoveryClusters returns actual clusters from the database
func getDiscoveryClusters(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := auth.GetTenantID(c)
		if deps.Repos.Cluster == nil || tenantID == (uuid.UUID{}) {
			c.JSON(http.StatusOK, gin.H{"clusters": []interface{}{}, "total": 0})
			return
		}

		clusterList, err := deps.Repos.Cluster.List(c.Request.Context(), tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		clusters := make([]map[string]interface{}, 0, len(clusterList))
		for _, cl := range clusterList {
			clusters = append(clusters, map[string]interface{}{
				"name":               cl.Name,
				"provider":           "kubernetes",
				"region":             "",
				"status":             cl.Status,
				"nodes":              cl.NodeCount,
				"kubernetes_version": cl.KubernetesVersion,
				"flux_installed":     cl.FluxInstalled,
				"environment":        cl.Environment,
				"is_active":          cl.IsActive,
			})
		}

		c.JSON(http.StatusOK, gin.H{"clusters": clusters, "total": len(clusters)})
	}
}

// =============================================================================
// FluxCD Management
// =============================================================================

func getDiscoveryNamespaces(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		tenantID := auth.GetTenantID(c)

		namespaces := map[string]int{}

		if deps.Repos.Cluster != nil && tenantID != (uuid.UUID{}) {
			clusterList, err := deps.Repos.Cluster.List(ctx, tenantID)
			if err == nil {
				for _, cluster := range clusterList {
					if !cluster.IsActive {
						continue
					}
					kubeconfig, err := deps.Repos.Cluster.GetKubeconfig(ctx, cluster.ID, uuid.Nil)
					if err != nil || kubeconfig == "" {
						continue
					}
					ns, err := getNamespacesFromCluster(ctx, kubeconfig)
					if err != nil {
						log.Printf("discovery: namespaces error for %s: %v", cluster.Name, err)
						continue
					}
					for _, n := range ns {
						namespaces[n]++
					}
				}
			}
		}

		// Sort and return
		nsList := make([]string, 0, len(namespaces))
		for ns := range namespaces {
			nsList = append(nsList, ns)
		}
		sort.Strings(nsList)

		c.JSON(http.StatusOK, gin.H{
			"namespaces": nsList,
			"total":      len(nsList),
		})
	}
}

func getNamespacesFromCluster(ctx context.Context, kubeconfig string) ([]string, error) {
	restConfig, err := buildRestConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("build rest config: %w", err)
	}

	body, err := k8sRequestWithBody(ctx, restConfig, "GET", "/api/v1/namespaces", "")
	if err != nil {
		return nil, err
	}

	var result struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("unmarshal namespaces: %w", err)
	}

	var namespaces []string
	for _, ns := range result.Items {
		if ns.Metadata.Name != "" {
			namespaces = append(namespaces, ns.Metadata.Name)
		}
	}
	return namespaces, nil
}

// fluxcdSuspend suspends a FluxCD HelmRelease reconciliation
func fluxcdSuspend(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		clusterName := c.Param("cluster")
		namespace := c.Param("namespace")
		name := c.Param("name")

		ctx := c.Request.Context()
		tenantID := auth.GetTenantID(c)

		kubeconfig, err := getClusterKubeconfig(ctx, deps, tenantID, clusterName)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		err = fluxcdSetSuspend(ctx, kubeconfig, namespace, name, true)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "suspend", "fluxcd_helmrelease", fmt.Sprintf("%s/%s/%s", clusterName, namespace, name), nil, gin.H{"cluster": clusterName})

		c.JSON(http.StatusOK, gin.H{
			"message":   fmt.Sprintf("HelmRelease %s/%s suspended", namespace, name),
			"name":      name,
			"namespace": namespace,
			"cluster":   clusterName,
		})
	}
}

// fluxcdResume resumes a FluxCD HelmRelease reconciliation
func fluxcdResume(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		clusterName := c.Param("cluster")
		namespace := c.Param("namespace")
		name := c.Param("name")

		ctx := c.Request.Context()
		tenantID := auth.GetTenantID(c)

		kubeconfig, err := getClusterKubeconfig(ctx, deps, tenantID, clusterName)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		err = fluxcdSetSuspend(ctx, kubeconfig, namespace, name, false)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "resume", "fluxcd_helmrelease", fmt.Sprintf("%s/%s/%s", clusterName, namespace, name), nil, gin.H{"cluster": clusterName})

		c.JSON(http.StatusOK, gin.H{
			"message":   fmt.Sprintf("HelmRelease %s/%s resumed", namespace, name),
			"name":      name,
			"namespace": namespace,
			"cluster":   clusterName,
		})
	}
}

// fluxcdReconcile forces reconciliation of a FluxCD HelmRelease
func fluxcdReconcile(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		clusterName := c.Param("cluster")
		namespace := c.Param("namespace")
		name := c.Param("name")

		ctx := c.Request.Context()
		tenantID := auth.GetTenantID(c)

		kubeconfig, err := getClusterKubeconfig(ctx, deps, tenantID, clusterName)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		err = fluxcdForceReconcile(ctx, kubeconfig, namespace, name)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "reconcile", "fluxcd_helmrelease", fmt.Sprintf("%s/%s/%s", clusterName, namespace, name), nil, gin.H{"cluster": clusterName})

		c.JSON(http.StatusOK, gin.H{
			"message":   fmt.Sprintf("HelmRelease %s/%s reconciliation triggered", namespace, name),
			"name":      name,
			"namespace": namespace,
			"cluster":   clusterName,
		})
	}
}

// fluxcdDelete deletes a FluxCD HelmRelease
func fluxcdDelete(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		clusterName := c.Param("cluster")
		namespace := c.Param("namespace")
		name := c.Param("name")

		ctx := c.Request.Context()
		tenantID := auth.GetTenantID(c)

		kubeconfig, err := getClusterKubeconfig(ctx, deps, tenantID, clusterName)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		err = fluxcdDeleteHelmRelease(ctx, kubeconfig, namespace, name)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "delete", "fluxcd_helmrelease", fmt.Sprintf("%s/%s/%s", clusterName, namespace, name), nil, gin.H{"cluster": clusterName})

		c.JSON(http.StatusOK, gin.H{
			"message":   fmt.Sprintf("HelmRelease %s/%s deleted", namespace, name),
			"name":      name,
			"namespace": namespace,
			"cluster":   clusterName,
		})
	}
}

// getClusterKubeconfig retrieves kubeconfig for a cluster by name
func getClusterKubeconfig(ctx context.Context, deps Dependencies, tenantID uuid.UUID, clusterName string) (string, error) {
	if deps.Repos.Cluster == nil {
		return "", fmt.Errorf("cluster repository not available")
	}
	if tenantID == (uuid.UUID{}) {
		return "", fmt.Errorf("tenant not found")
	}

	clusterList, err := deps.Repos.Cluster.List(ctx, tenantID)
	if err != nil {
		return "", fmt.Errorf("list clusters: %w", err)
	}

	for _, cluster := range clusterList {
		if cluster.Name == clusterName {
			kubeconfig, err := deps.Repos.Cluster.GetKubeconfig(ctx, cluster.ID, uuid.Nil)
			if err != nil {
				return "", fmt.Errorf("get kubeconfig: %w", err)
			}
			if kubeconfig == "" {
				return "", fmt.Errorf("kubeconfig not found for cluster %s", clusterName)
			}
			return kubeconfig, nil
		}
	}
	return "", fmt.Errorf("cluster %s not found", clusterName)
}

// fluxcdSetSuspend sets the suspend field on a HelmRelease
func fluxcdSetSuspend(ctx context.Context, kubeconfig, namespace, name string, suspend bool) error {
	restConfig, err := buildRestConfig(kubeconfig)
	if err != nil {
		return fmt.Errorf("build rest config: %w", err)
	}

	// First get the current HelmRelease
	apiPath := fmt.Sprintf("/apis/helm.toolkit.fluxcd.io/v2/namespaces/%s/helmreleases/%s", namespace, name)
	body, err := k8sRequestWithBody(ctx, restConfig, "GET", apiPath, "")
	if err != nil {
		// Try v2beta2
		apiPath = fmt.Sprintf("/apis/helm.toolkit.fluxcd.io/v2beta2/namespaces/%s/helmreleases/%s", namespace, name)
		body, err = k8sRequestWithBody(ctx, restConfig, "GET", apiPath, "")
		if err != nil {
			// Try v2beta1
			apiPath = fmt.Sprintf("/apis/helm.toolkit.fluxcd.io/v2beta1/namespaces/%s/helmreleases/%s", namespace, name)
			body, err = k8sRequestWithBody(ctx, restConfig, "GET", apiPath, "")
			if err != nil {
				return fmt.Errorf("get helmrelease: %w", err)
			}
		}
	}

	// Parse and modify
	var hr map[string]interface{}
	if err := json.Unmarshal(body, &hr); err != nil {
		return fmt.Errorf("unmarshal helmrelease: %w", err)
	}

	spec, ok := hr["spec"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid helmrelease spec")
	}
	spec["suspend"] = suspend

	// PATCH back
	patchData, err := json.Marshal(map[string]interface{}{
		"spec": spec,
	})
	if err != nil {
		return fmt.Errorf("marshal patch: %w", err)
	}

	_, err = k8sRequestWithBody(ctx, restConfig, "PATCH", apiPath, string(patchData))
	if err != nil {
		return fmt.Errorf("patch helmrelease: %w", err)
	}

	return nil
}

// fluxcdForceReconcile triggers reconciliation by annotating the HelmRelease
func fluxcdForceReconcile(ctx context.Context, kubeconfig, namespace, name string) error {
	restConfig, err := buildRestConfig(kubeconfig)
	if err != nil {
		return fmt.Errorf("build rest config: %w", err)
	}

	apiPath := fmt.Sprintf("/apis/helm.toolkit.fluxcd.io/v2/namespaces/%s/helmreleases/%s", namespace, name)

	// Get current to preserve spec
	body, err := k8sRequestWithBody(ctx, restConfig, "GET", apiPath, "")
	if err != nil {
		apiPath = fmt.Sprintf("/apis/helm.toolkit.fluxcd.io/v2beta2/namespaces/%s/helmreleases/%s", namespace, name)
		body, err = k8sRequestWithBody(ctx, restConfig, "GET", apiPath, "")
		if err != nil {
			apiPath = fmt.Sprintf("/apis/helm.toolkit.fluxcd.io/v2beta1/namespaces/%s/helmreleases/%s", namespace, name)
			body, err = k8sRequestWithBody(ctx, restConfig, "GET", apiPath, "")
			if err != nil {
				return fmt.Errorf("get helmrelease: %w", err)
			}
		}
	}

	var hr map[string]interface{}
	if err := json.Unmarshal(body, &hr); err != nil {
		return fmt.Errorf("unmarshal helmrelease: %w", err)
	}

	// Add reconcile annotation
	metadata, _ := hr["metadata"].(map[string]interface{})
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	annotations, _ := metadata["annotations"].(map[string]interface{})
	if annotations == nil {
		annotations = map[string]interface{}{}
	}
	annotations["reconcile.fluxcd.io/requestAt"] = time.Now().UTC().Format(time.RFC3339)
	metadata["annotations"] = annotations

	patchData, err := json.Marshal(map[string]interface{}{
		"metadata": metadata,
	})
	if err != nil {
		return fmt.Errorf("marshal patch: %w", err)
	}

	_, err = k8sRequestWithBody(ctx, restConfig, "PATCH", apiPath, string(patchData))
	if err != nil {
		return fmt.Errorf("patch helmrelease: %w", err)
	}

	return nil
}

// fluxcdDeleteHelmRelease deletes a HelmRelease resource
func fluxcdDeleteHelmRelease(ctx context.Context, kubeconfig, namespace, name string) error {
	restConfig, err := buildRestConfig(kubeconfig)
	if err != nil {
		return fmt.Errorf("build rest config: %w", err)
	}

	// Try v2 first, then fallback to older versions
	apiPath := fmt.Sprintf("/apis/helm.toolkit.fluxcd.io/v2/namespaces/%s/helmreleases/%s", namespace, name)
	_, err = k8sRequestWithBody(ctx, restConfig, "DELETE", apiPath, "")
	if err != nil {
		apiPath = fmt.Sprintf("/apis/helm.toolkit.fluxcd.io/v2beta2/namespaces/%s/helmreleases/%s", namespace, name)
		_, err = k8sRequestWithBody(ctx, restConfig, "DELETE", apiPath, "")
		if err != nil {
			apiPath = fmt.Sprintf("/apis/helm.toolkit.fluxcd.io/v2beta1/namespaces/%s/helmreleases/%s", namespace, name)
			_, err = k8sRequestWithBody(ctx, restConfig, "DELETE", apiPath, "")
			if err != nil {
				return fmt.Errorf("delete helmrelease: %w", err)
			}
		}
	}

	return nil
}

// dockerContainerLogs returns logs from a Docker container discovered via a Docker host.
func dockerContainerLogs(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.DockerHost == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "docker host repository not available"})
			return
		}

		hostName := c.Param("host")
		containerName := c.Param("name")
		tailLines := 200
		if t := c.Query("tail"); t != "" {
			if parsed, err := parseDockerTailInt(t); err == nil {
				tailLines = parsed
			}
		}

		ctx := c.Request.Context()

		// Look up the Docker host by name
		host, err := deps.Repos.DockerHost.GetHostByName(ctx, hostName)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("docker host %q not found", hostName)})
			return
		}

		// Get decrypted credentials
		hostDecrypted, err := deps.Repos.DockerHost.GetHostDecrypted(ctx, host.ID, host.TenantID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to decrypt docker host credentials"})
			return
		}

		cfg := dockerpkg.HostConfig{
			HostType:    hostDecrypted.HostType,
			HostAddress: hostDecrypted.HostAddress,
			TLSCACert:   hostDecrypted.TLSCACert,
			TLSCert:     hostDecrypted.TLSCert,
			TLSKey:      hostDecrypted.TLSKey,
			SSHKey:      hostDecrypted.SSHKey,
		}
		client := dockerpkg.NewClient(cfg)

		logsCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		logs, err := client.ContainerLogs(logsCtx, containerName, tailLines)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"logs":    logs,
			"name":    containerName,
			"cluster": hostName,
		})
	}
}

func parseDockerTailInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}
