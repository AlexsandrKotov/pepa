package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/gitops"
	"github.com/pepa/pepa/internal/k8s"
	"github.com/pepa/pepa/internal/repository"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	k8srest "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func registerClusterRoutes(r *gin.RouterGroup, deps Dependencies) {
	clusters := r.Group("/clusters")
	{
		clusters.GET("", listClusters(deps))
		clusters.POST("", createCluster(deps))
		clusters.GET("/:id", getCluster(deps))
		clusters.PUT("/:id", updateCluster(deps))
		clusters.DELETE("/:id", deleteCluster(deps))
		clusters.POST("/:id/kubeconfig", uploadKubeconfig(deps))
		clusters.GET("/:id/health", getClusterHealth(deps))
		clusters.GET("/:id/nodes", getClusterNodes(deps))
		clusters.GET("/:id/namespaces", listNamespaces(deps))
		clusters.GET("/:id/resources", listResources(deps))
		clusters.GET("/:id/flux", listFluxResources(deps))
		clusters.GET("/:id/argo", listArgoResources(deps))
		clusters.GET("/:id/gitops", getGitOpsEngine(deps))
		clusters.POST("/:id/test", testClusterConnection(deps))
		clusters.GET("/:id/topology", clusterTopology(deps))
	}
}

// clusterK8sClient creates a k8s client from a cluster's kubeconfig, applying the API server URL override if set.
func clusterK8sClient(cluster *repository.Cluster, kubeconfig string) (*k8s.Client, error) {
	if cluster.APIServerURL != "" {
		return k8s.NewClientWithServerOverride(kubeconfig, cluster.APIServerURL)
	}
	return k8s.NewClient(kubeconfig)
}

func listClusters(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Cluster == nil {
			c.JSON(http.StatusOK, gin.H{"clusters": []interface{}{}, "total": 0})
			return
		}
		tenantID := auth.GetTenantID(c)
		items, err := deps.Repos.Cluster.List(c.Request.Context(), tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		// Return cached data from DB immediately. Refresh cluster health in background (parallel).
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			type clusterJob struct {
				idx int
			}
			jobs := make([]clusterJob, 0, len(items))
			for i := range items {
				if items[i].HasKubeconfig {
					jobs = append(jobs, clusterJob{idx: i})
				}
			}

			sem := make(chan struct{}, 5) // limit concurrency to 5
			var wg sync.WaitGroup
			for _, job := range jobs {
				wg.Add(1)
				sem <- struct{}{}
				go func(j clusterJob) {
					defer wg.Done()
					defer func() { <-sem }()
					item := items[j.idx]
					kubeconfig, err := deps.Repos.Cluster.GetKubeconfig(bgCtx, item.ID, tenantID)
					if err != nil || kubeconfig == "" {
						item.Status = "disconnected"
						_ = deps.Repos.Cluster.Update(context.Background(), &item)
						return
					}
					ctx, cancel := context.WithTimeout(bgCtx, 10*time.Second)
					defer cancel()
					client, err := clusterK8sClient(&item, kubeconfig)
					if err != nil {
						item.Status = "disconnected"
						_ = deps.Repos.Cluster.Update(context.Background(), &item)
						return
					}
					nodeCount, k8sVersion, err := client.GetClusterInfo(ctx)
					if err == nil {
						item.NodeCount = nodeCount
						item.KubernetesVersion = k8sVersion
						item.Status = "connected"
					} else {
						item.Status = "disconnected"
					}
					// Detect GitOps engines (FluxCD/ArgoCD)
					if engine, engineErr := client.DetectGitOpsEngine(ctx); engineErr == nil {
						if engine.FluxCD != item.FluxInstalled {
							item.FluxInstalled = engine.FluxCD
						}
						if engine.ArgoCD {
							if item.Labels == nil {
								item.Labels = make(map[string]string)
							}
							item.Labels["argocd_detected"] = "true"
						}
					}
					_ = deps.Repos.Cluster.Update(context.Background(), &item)
				}(job)
			}
			wg.Wait()
		}()

		c.JSON(http.StatusOK, gin.H{"clusters": items, "total": len(items)})
	}
}

func createCluster(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Cluster == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "cluster repository not available"})
			return
		}
		var req struct {
			Name              string            `json:"name" binding:"required"`
			Description       string            `json:"description"`
			Environment       string            `json:"environment"`
			APIServerURL      string            `json:"api_server_url"`
			KubernetesVersion string            `json:"kubernetes_version"`
			Labels            map[string]string `json:"labels"`
			Notes             string            `json:"notes"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if req.Environment == "" {
			req.Environment = "dev"
		}
		cluster := &repository.Cluster{
			TenantID:          auth.GetTenantID(c),
			Name:              req.Name,
			Description:       req.Description,
			Environment:       req.Environment,
			APIServerURL:      req.APIServerURL,
			KubernetesVersion: req.KubernetesVersion,
			Labels:            req.Labels,
			Notes:             req.Notes,
			Status:            "connected",
			IsActive:          true,
		}
		if err := deps.Repos.Cluster.Create(c.Request.Context(), cluster); err != nil {
			respondInternalError(c, err)
			return
		}
		logAudit(deps, c, "create", "cluster", cluster.ID.String(), nil, gin.H{"name": cluster.Name, "environment": cluster.Environment})
		c.JSON(http.StatusCreated, cluster)
	}
}

func getCluster(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Cluster == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "cluster repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cluster ID"})
			return
		}
		cluster, err := deps.Repos.Cluster.Get(c.Request.Context(), id, auth.GetTenantID(c))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		// If cluster has kubeconfig, fetch real cluster info
		if cluster.HasKubeconfig {
			kubeconfig, err := deps.Repos.Cluster.GetKubeconfig(c.Request.Context(), id, auth.GetTenantID(c))
			if err == nil && kubeconfig != "" {
				client, err := clusterK8sClient(cluster, kubeconfig)
				if err == nil {
					nodeCount, k8sVersion, err := client.GetClusterInfo(c.Request.Context())
					if err == nil {
						cluster.NodeCount = nodeCount
						cluster.KubernetesVersion = k8sVersion
						// Update cluster in DB with real info
						_ = deps.Repos.Cluster.Update(c.Request.Context(), cluster)
					}
				}
			}
		}

		c.JSON(http.StatusOK, cluster)
	}
}

func updateCluster(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Cluster == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "cluster repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cluster ID"})
			return
		}
		cluster, err := deps.Repos.Cluster.Get(c.Request.Context(), id, auth.GetTenantID(c))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		var req struct {
			Name              string            `json:"name"`
			Description       string            `json:"description"`
			Environment       string            `json:"environment"`
			APIServerURL      string            `json:"api_server_url"`
			FluxInstalled     *bool             `json:"flux_installed"`
			Status            string            `json:"status"`
			NodeCount         *int              `json:"node_count"`
			KubernetesVersion string            `json:"kubernetes_version"`
			IsActive          *bool             `json:"is_active"`
			Labels            map[string]string `json:"labels"`
			Notes             string            `json:"notes"`
			ConnectionID      *string           `json:"connection_id"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if req.Name != "" {
			cluster.Name = req.Name
		}
		cluster.Description = req.Description
		if req.Environment != "" {
			cluster.Environment = req.Environment
		}
		if req.APIServerURL != "" {
			cluster.APIServerURL = req.APIServerURL
		}
		if req.FluxInstalled != nil {
			cluster.FluxInstalled = *req.FluxInstalled
		}
		if req.Status != "" {
			cluster.Status = req.Status
		}
		if req.NodeCount != nil {
			cluster.NodeCount = *req.NodeCount
		}
		if req.KubernetesVersion != "" {
			cluster.KubernetesVersion = req.KubernetesVersion
		}
		if req.IsActive != nil {
			cluster.IsActive = *req.IsActive
		}
		if req.Labels != nil {
			cluster.Labels = req.Labels
		}
		cluster.Notes = req.Notes
		if req.ConnectionID != nil {
			if *req.ConnectionID == "" {
				cluster.ConnectionID = nil
			} else {
				parsed, err := uuid.Parse(*req.ConnectionID)
				if err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid connection_id"})
					return
				}
				cluster.ConnectionID = &parsed
			}
		}
		if err := deps.Repos.Cluster.Update(c.Request.Context(), cluster); err != nil {
			respondInternalError(c, err)
			return
		}
		logAudit(deps, c, "update", "cluster", cluster.ID.String(), nil, gin.H{"name": cluster.Name, "status": cluster.Status})
		c.JSON(http.StatusOK, cluster)
	}
}

func deleteCluster(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Cluster == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "cluster repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cluster ID"})
			return
		}
		if err := deps.Repos.Cluster.Delete(c.Request.Context(), id); err != nil {
			respondInternalError(c, err)
			return
		}
		logAudit(deps, c, "delete", "cluster", id.String(), nil, nil)
		c.JSON(http.StatusOK, gin.H{"message": "cluster deleted"})
	}
}

func listFluxResources(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Cluster == nil {
			c.JSON(http.StatusOK, gin.H{"resources": []interface{}{}})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cluster ID"})
			return
		}
		// Check cluster exists and has kubeconfig
		cluster, err := deps.Repos.Cluster.Get(c.Request.Context(), id, auth.GetTenantID(c))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "cluster not found"})
			return
		}
		if !cluster.HasKubeconfig {
			c.JSON(http.StatusOK, gin.H{"resources": []interface{}{}})
			return
		}

		// Get kubeconfig and create k8s client
		kubeconfig, err := deps.Repos.Cluster.GetKubeconfig(c.Request.Context(), id, auth.GetTenantID(c))
		if err != nil || kubeconfig == "" {
			c.JSON(http.StatusOK, gin.H{"resources": []interface{}{}})
			return
		}

		client, err := clusterK8sClient(cluster, kubeconfig)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"resources": []interface{}{}})
			return
		}

		resources, err := client.ListFluxResources(c.Request.Context())
		if err != nil {
			// If FluxCD CRDs are not installed, return empty list
			c.JSON(http.StatusOK, gin.H{"resources": []interface{}{}})
			return
		}

		c.JSON(http.StatusOK, gin.H{"resources": resources})
	}
}

func listArgoResources(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Cluster == nil {
			c.JSON(http.StatusOK, gin.H{"resources": []interface{}{}})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cluster ID"})
			return
		}
		cluster, err := deps.Repos.Cluster.Get(c.Request.Context(), id, auth.GetTenantID(c))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "cluster not found"})
			return
		}
		if !cluster.HasKubeconfig {
			c.JSON(http.StatusOK, gin.H{"resources": []interface{}{}})
			return
		}

		kubeconfig, err := deps.Repos.Cluster.GetKubeconfig(c.Request.Context(), id, auth.GetTenantID(c))
		if err != nil || kubeconfig == "" {
			c.JSON(http.StatusOK, gin.H{"resources": []interface{}{}})
			return
		}

		client, err := clusterK8sClient(cluster, kubeconfig)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"resources": []interface{}{}})
			return
		}

		resources, err := client.ListArgoResources(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"resources": []interface{}{}})
			return
		}

		c.JSON(http.StatusOK, gin.H{"resources": resources})
	}
}

func uploadKubeconfig(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cluster id"})
			return
		}
		var req struct {
			Kubeconfig string `json:"kubeconfig"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.Kubeconfig == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "kubeconfig is required"})
			return
		}
		if err := deps.Repos.Cluster.SaveKubeconfig(c.Request.Context(), id, req.Kubeconfig); err != nil {
			respondInternalError(c, err)
			return
		}

		// Detect GitOps engines (FluxCD/ArgoCD) after saving kubeconfig
		cluster, clusterErr := deps.Repos.Cluster.Get(c.Request.Context(), id, auth.GetTenantID(c))
		if clusterErr == nil && cluster != nil {
			detectCtx, detectCancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
			defer detectCancel()
			if client, clientErr := clusterK8sClient(cluster, req.Kubeconfig); clientErr == nil {
				if engine, engineErr := client.DetectGitOpsEngine(detectCtx); engineErr == nil {
					updated := false
					if engine.FluxCD != cluster.FluxInstalled {
						cluster.FluxInstalled = engine.FluxCD
						updated = true
					}
					if engine.ArgoCD {
						if cluster.Labels == nil {
							cluster.Labels = make(map[string]string)
						}
						if cluster.Labels["argocd_detected"] != "true" {
							cluster.Labels["argocd_detected"] = "true"
							updated = true
						}
					}
					if updated {
						_ = deps.Repos.Cluster.Update(c.Request.Context(), cluster)
					}
				}
			}
		}

		c.JSON(http.StatusOK, gin.H{"status": "connected", "message": "Kubeconfig saved successfully"})
	}
}

func listNamespaces(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cluster id"})
			return
		}
		// Check cluster exists and has kubeconfig
		cluster, err := deps.Repos.Cluster.Get(c.Request.Context(), id, auth.GetTenantID(c))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "cluster not found"})
			return
		}
		if !cluster.HasKubeconfig {
			c.JSON(http.StatusBadRequest, gin.H{"error": "kubeconfig not uploaded"})
			return
		}

		// Get kubeconfig and create k8s client
		kubeconfig, err := deps.Repos.Cluster.GetKubeconfig(c.Request.Context(), id, auth.GetTenantID(c))
		if err != nil || kubeconfig == "" {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve kubeconfig"})
			return
		}

		client, err := clusterK8sClient(cluster, kubeconfig)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to connect to cluster"})
			return
		}

		namespaces, err := client.ListNamespaces(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list namespaces"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"namespaces": namespaces})
	}
}

func listResources(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cluster id"})
			return
		}
		namespace := c.Query("namespace")
		// Check cluster exists and has kubeconfig
		cluster, err := deps.Repos.Cluster.Get(c.Request.Context(), id, auth.GetTenantID(c))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "cluster not found"})
			return
		}
		if !cluster.HasKubeconfig {
			c.JSON(http.StatusBadRequest, gin.H{"error": "kubeconfig not uploaded"})
			return
		}

		// Get kubeconfig and create k8s client
		kubeconfig, err := deps.Repos.Cluster.GetKubeconfig(c.Request.Context(), id, auth.GetTenantID(c))
		if err != nil || kubeconfig == "" {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve kubeconfig"})
			return
		}

		client, err := clusterK8sClient(cluster, kubeconfig)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to connect to cluster"})
			return
		}

		resources, err := client.ListResources(c.Request.Context(), namespace)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list resources"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"resources": resources})
	}
}

func getClusterHealth(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Cluster == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "cluster repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cluster ID"})
			return
		}
		cluster, err := deps.Repos.Cluster.Get(c.Request.Context(), id, auth.GetTenantID(c))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		// Determine health status
		healthStatus := "healthy"
		if !cluster.IsActive {
			healthStatus = "unhealthy"
		} else if cluster.Status != "connected" {
			healthStatus = "degraded"
		}

		// Default usage values
		cpuUsage := "N/A"
		memUsage := "N/A"
		podUsage := "N/A"

		// If cluster has kubeconfig, try to get real metrics
		if cluster.HasKubeconfig {
			kubeconfig, err := deps.Repos.Cluster.GetKubeconfig(c.Request.Context(), id, auth.GetTenantID(c))
			if err == nil && kubeconfig != "" {
				client, err := clusterK8sClient(cluster, kubeconfig)
				if err == nil {
					// Calculate cluster-wide usage from nodes
					nodes, err := client.ListNodes(c.Request.Context())
					if err == nil && len(nodes) > 0 {
						// Average CPU/Memory usage across nodes
						var totalCPU, totalMem int
						count := 0
						for _, n := range nodes {
							if cpu, ok := parsePercent(n.CPUUsage); ok {
								totalCPU += cpu
								count++
							}
							if mem, ok := parsePercent(n.MemoryUsage); ok {
								totalMem += mem
							}
						}
						if count > 0 {
							cpuUsage = fmt.Sprintf("%d%%", totalCPU/count)
							memUsage = fmt.Sprintf("%d%%", totalMem/count)
						}
						// Pod usage: count total pods vs total capacity
						nsList, err := client.ListNamespaces(c.Request.Context())
						if err == nil {
							totalPods := 0
							for _, ns := range nsList {
								totalPods += ns.Pods
							}
							totalCapacity := len(nodes) * 110 // default pod capacity per node
							if totalCapacity > 0 {
								podUsage = fmt.Sprintf("%d%%", totalPods*100/totalCapacity)
							}
						}
					}
				}
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"cluster_id":         id.String(),
			"cluster_name":       cluster.Name,
			"status":             healthStatus,
			"api_server":         cluster.APIServerURL,
			"node_count":         cluster.NodeCount,
			"kubernetes_version": cluster.KubernetesVersion,
			"cpu_usage":          cpuUsage,
			"memory_usage":       memUsage,
			"pod_usage":          podUsage,
		})
	}
}

// clusterTopology builds a dependency graph from live FluxCD resources in the cluster.
func clusterTopology(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Cluster == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "cluster repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cluster ID"})
			return
		}

		ctx := c.Request.Context()
		cluster, err := deps.Repos.Cluster.Get(ctx, id, auth.GetTenantID(c))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "cluster not found"})
			return
		}
		if !cluster.HasKubeconfig {
			c.JSON(http.StatusOK, &gitops.TopologyGraph{
				Nodes: []gitops.TopologyNode{},
				Edges: []gitops.TopologyEdge{},
			})
			return
		}

		kubeconfig, err := deps.Repos.Cluster.GetKubeconfig(ctx, id, auth.GetTenantID(c))
		if err != nil || kubeconfig == "" {
			c.JSON(http.StatusOK, &gitops.TopologyGraph{
				Nodes: []gitops.TopologyNode{},
				Edges: []gitops.TopologyEdge{},
			})
			return
		}

		restConfig, err := buildRestConfig(kubeconfig)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		var resources []gitops.Resource

		// Query FluxCD HelmReleases
		hrResources := queryFluxHelmReleases(ctx, restConfig)
		resources = append(resources, hrResources...)

		// Query FluxCD Kustomizations
		ksResources := queryFluxKustomizations(ctx, restConfig)
		resources = append(resources, ksResources...)

		// Build topology graph
		graph := gitops.BuildTopology(ctx, nil, resources)
		c.JSON(http.StatusOK, graph)
	}
}

// queryFluxHelmReleases fetches FluxCD HelmReleases from the cluster.
func queryFluxHelmReleases(ctx context.Context, restConfig *k8srest.Config) []gitops.Resource {
	var resources []gitops.Resource

	// Try multiple API versions
	apiPaths := []string{
		"/apis/helm.toolkit.fluxcd.io/v2/helmreleases",
		"/apis/helm.toolkit.fluxcd.io/v2beta2/helmreleases",
		"/apis/helm.toolkit.fluxcd.io/v2beta1/helmreleases",
	}

	var data []byte
	var err error
	for _, path := range apiPaths {
		data, err = k8sRequest(restConfig, path)
		if err == nil {
			break
		}
	}
	if err != nil {
		return resources
	}

	var list struct {
		Items []struct {
			Metadata struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
			Status struct {
				Conditions []struct {
					Type   string `json:"type"`
					Status string `json:"status"`
				} `json:"conditions"`
			} `json:"status"`
			Spec struct {
				Suspend bool `json:"suspend"`
				// v2beta1/v2beta2 format
				Chart struct {
					Spec struct {
						Chart     string `json:"chart"`
						Version   string `json:"version"`
						SourceRef struct {
							Kind string `json:"kind"`
							Name string `json:"name"`
						} `json:"sourceRef"`
					} `json:"spec"`
				} `json:"chart"`
				// v2 format: chartRef
				ChartRef *struct {
					Kind      string `json:"kind"`
					Name      string `json:"name"`
					Namespace string `json:"namespace"`
				} `json:"chartRef"`
				ValuesFrom []struct {
					Kind      string `json:"kind"`
					Name      string `json:"name"`
					ValuesKey string `json:"valuesKey"`
				} `json:"valuesFrom"`
				DependsOn []struct {
					Name      string `json:"name"`
					Namespace string `json:"namespace"`
				} `json:"dependsOn"`
			} `json:"spec"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &list); err != nil {
		return resources
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

		r := gitops.Resource{
			Kind:       "HelmRelease",
			APIVersion: "helm.toolkit.fluxcd.io/v2",
			Name:       item.Metadata.Name,
			Namespace:  item.Metadata.Namespace,
			Chart:      item.Spec.Chart.Spec.Chart,
			Version:    item.Spec.Chart.Spec.Version,
			Repo:       item.Spec.Chart.Spec.SourceRef.Name,
			Suspended:  item.Spec.Suspend,
		}

		// Handle v2 chartRef format
		if r.Chart == "" && item.Spec.ChartRef != nil {
			r.Chart = item.Spec.ChartRef.Name
			r.ChartRef = &gitops.FluxChartRef{
				Kind:      item.Spec.ChartRef.Kind,
				Name:      item.Spec.ChartRef.Name,
				Namespace: item.Spec.ChartRef.Namespace,
			}
			if r.Repo == "" {
				r.Repo = item.Spec.ChartRef.Name
			}
		}

		for _, vf := range item.Spec.ValuesFrom {
			r.ValuesFrom = append(r.ValuesFrom, gitops.ValuesReference{
				Kind:      vf.Kind,
				Name:      vf.Name,
				ValuesKey: vf.ValuesKey,
			})
		}

		for _, dep := range item.Spec.DependsOn {
			ref := dep.Name
			if dep.Namespace != "" {
				ref = dep.Namespace + "/" + dep.Name
			}
			r.DependsOn = append(r.DependsOn, ref)
		}

		// Store health in labels for topology display
		if r.Labels == nil {
			r.Labels = map[string]string{}
		}
		r.Labels["health"] = health
		r.Labels["sync_status"] = syncStatus

		resources = append(resources, r)
	}

	return resources
}

// queryFluxKustomizations fetches FluxCD Kustomizations from the cluster.
func queryFluxKustomizations(ctx context.Context, restConfig *k8srest.Config) []gitops.Resource {
	var resources []gitops.Resource

	apiPaths := []string{
		"/apis/kustomize.toolkit.fluxcd.io/v1/kustomizations",
		"/apis/kustomize.toolkit.fluxcd.io/v1beta2/kustomizations",
		"/apis/kustomize.toolkit.fluxcd.io/v1beta1/kustomizations",
	}

	var data []byte
	var err error
	for _, path := range apiPaths {
		data, err = k8sRequest(restConfig, path)
		if err == nil {
			break
		}
	}
	if err != nil {
		return resources
	}

	var list struct {
		Items []struct {
			Metadata struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
			Status struct {
				Conditions []struct {
					Type   string `json:"type"`
					Status string `json:"status"`
				} `json:"conditions"`
			} `json:"status"`
			Spec struct {
				Suspend   bool   `json:"suspend"`
				Path      string `json:"path"`
				SourceRef struct {
					Kind string `json:"kind"`
					Name string `json:"name"`
				} `json:"sourceRef"`
				DependsOn []struct {
					Name      string `json:"name"`
					Namespace string `json:"namespace"`
				} `json:"dependsOn"`
			} `json:"spec"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &list); err != nil {
		return resources
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

		r := gitops.Resource{
			Kind:       "Kustomization",
			APIVersion: "kustomize.toolkit.fluxcd.io/v1",
			Name:       item.Metadata.Name,
			Namespace:  item.Metadata.Namespace,
			Suspended:  item.Spec.Suspend,
		}

		for _, dep := range item.Spec.DependsOn {
			ref := dep.Name
			if dep.Namespace != "" {
				ref = dep.Namespace + "/" + dep.Name
			}
			r.DependsOn = append(r.DependsOn, ref)
		}

		if r.Labels == nil {
			r.Labels = map[string]string{}
		}
		r.Labels["health"] = health
		r.Labels["sync_status"] = syncStatus

		resources = append(resources, r)
	}

	return resources
}

func parsePercent(s string) (int, bool) {
	var val int
	if _, err := fmt.Sscanf(s, "%d%%", &val); err == nil {
		return val, true
	}
	return 0, false
}

func getClusterNodes(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Cluster == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "cluster repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cluster ID"})
			return
		}
		cluster, err := deps.Repos.Cluster.Get(c.Request.Context(), id, auth.GetTenantID(c))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		if !cluster.HasKubeconfig {
			c.JSON(http.StatusBadRequest, gin.H{"error": "kubeconfig not uploaded"})
			return
		}

		// Get kubeconfig and create k8s client
		kubeconfig, err := deps.Repos.Cluster.GetKubeconfig(c.Request.Context(), id, auth.GetTenantID(c))
		if err != nil || kubeconfig == "" {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve kubeconfig"})
			return
		}

		client, err := clusterK8sClient(cluster, kubeconfig)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to connect to cluster"})
			return
		}

		nodes, err := client.ListNodes(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list nodes"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"nodes": nodes, "total": len(nodes)})
	}
}

func getGitOpsEngine(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cluster id"})
			return
		}
		cluster, err := deps.Repos.Cluster.Get(c.Request.Context(), id, auth.GetTenantID(c))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "cluster not found"})
			return
		}
		if !cluster.HasKubeconfig {
			c.JSON(http.StatusOK, gin.H{"fluxcd": false, "argocd": false, "message": "no kubeconfig"})
			return
		}
		kubeconfig, err := deps.Repos.Cluster.GetKubeconfig(c.Request.Context(), id, auth.GetTenantID(c))
		if err != nil || kubeconfig == "" {
			c.JSON(http.StatusOK, gin.H{"fluxcd": false, "argocd": false, "message": "kubeconfig not available"})
			return
		}
		client, err := clusterK8sClient(cluster, kubeconfig)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to connect to cluster"})
			return
		}
		engine, err := client.DetectGitOpsEngine(c.Request.Context())
		if err != nil {
			slog.Info("getGitOpsEngine: gitops engine detection failed", "error", err)
			c.JSON(http.StatusOK, gin.H{"fluxcd": false, "argocd": false, "error": "gitops engine detection failed"})
			return
		}
		c.JSON(http.StatusOK, engine)
	}
}

func testClusterConnection(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Cluster == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "cluster repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cluster ID"})
			return
		}
		cluster, err := deps.Repos.Cluster.Get(c.Request.Context(), id, auth.GetTenantID(c))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "cluster not found"})
			return
		}
		if !cluster.HasKubeconfig {
			c.JSON(http.StatusBadRequest, gin.H{"error": "kubeconfig not uploaded"})
			return
		}

		// Parse optional API server URL override from request body
		var req struct {
			APIServerURL string `json:"api_server_url"`
		}
		_ = c.ShouldBindJSON(&req)

		kubeconfig, err := deps.Repos.Cluster.GetKubeconfig(c.Request.Context(), id, auth.GetTenantID(c))
		if err != nil || kubeconfig == "" {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve kubeconfig"})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()

		// Determine the server URL to use
		serverURL := req.APIServerURL
		if serverURL == "" {
			serverURL = cluster.APIServerURL
		}

		// Build REST config, optionally overriding the server URL
		var clientset *kubernetes.Clientset
		if serverURL != "" {
			config, parseErr := clientcmd.RESTConfigFromKubeConfig([]byte(kubeconfig))
			if parseErr != nil {
				c.JSON(http.StatusOK, gin.H{
					"status":  "error",
					"message": fmt.Sprintf("Invalid kubeconfig: %v", parseErr),
				})
				return
			}
			config.Host = serverURL
			config.Timeout = 10 * time.Second
			// Allow self-signed certificates (common for k3s/kind/minikube)
			config.Insecure = true
			config.CAData = nil
			config.CAFile = ""
			clientset, err = kubernetes.NewForConfig(config)
		} else {
			// Use kubeconfig as-is
			client, clientErr := k8s.NewClient(kubeconfig)
			if clientErr != nil {
				c.JSON(http.StatusOK, gin.H{
					"status":  "error",
					"message": fmt.Sprintf("Failed to create client: %v", clientErr),
				})
				return
			}
			// Extract version to test connectivity
			nodeCount, k8sVersion, infoErr := client.GetClusterInfo(ctx)
			if infoErr != nil {
				// Update cluster status
				cluster.Status = "disconnected"
				_ = deps.Repos.Cluster.Update(c.Request.Context(), cluster)
				c.JSON(http.StatusOK, gin.H{
					"status":  "error",
					"message": fmt.Sprintf("Cannot reach cluster: %v. Check if the API server is accessible from PEPA.", infoErr),
				})
				return
			}
			// Success
			cluster.Status = "connected"
			cluster.NodeCount = nodeCount
			cluster.KubernetesVersion = k8sVersion
			// Detect GitOps engines (FluxCD/ArgoCD)
			if engine, engineErr := client.DetectGitOpsEngine(ctx); engineErr == nil {
				cluster.FluxInstalled = engine.FluxCD
				if engine.ArgoCD {
					if cluster.Labels == nil {
						cluster.Labels = make(map[string]string)
					}
					cluster.Labels["argocd_detected"] = "true"
				}
			}
			_ = deps.Repos.Cluster.Update(c.Request.Context(), cluster)
			c.JSON(http.StatusOK, gin.H{
				"status":             "connected",
				"message":            fmt.Sprintf("Connection test passed. Kubernetes %s, %d nodes.", k8sVersion, nodeCount),
				"kubernetes_version": k8sVersion,
				"node_count":         nodeCount,
				"flux_installed":     cluster.FluxInstalled,
			})
			return
		}

		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"status":  "error",
				"message": fmt.Sprintf("Failed to create client: %v", err),
			})
			return
		}

		// Test connectivity with overridden URL
		version, err := clientset.Discovery().ServerVersion()
		if err != nil {
			cluster.Status = "disconnected"
			_ = deps.Repos.Cluster.Update(c.Request.Context(), cluster)
			c.JSON(http.StatusOK, gin.H{
				"status":  "error",
				"message": fmt.Sprintf("Cannot reach API Server at %s: %v. Check network connectivity and firewall rules.", serverURL, err),
			})
			return
		}

		// Get node count
		nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		nodeCount := 0
		if err == nil {
			nodeCount = len(nodes.Items)
		}

		// Update cluster status
		cluster.Status = "connected"
		cluster.NodeCount = nodeCount
		cluster.KubernetesVersion = version.GitVersion
		if serverURL != "" {
			cluster.APIServerURL = serverURL
		}
		// Detect GitOps engines (FluxCD/ArgoCD)
		if k8sClient, clientErr := clusterK8sClient(cluster, kubeconfig); clientErr == nil {
			if engine, engineErr := k8sClient.DetectGitOpsEngine(ctx); engineErr == nil {
				cluster.FluxInstalled = engine.FluxCD
				if engine.ArgoCD {
					if cluster.Labels == nil {
						cluster.Labels = make(map[string]string)
					}
					cluster.Labels["argocd_detected"] = "true"
				}
			}
		}
		_ = deps.Repos.Cluster.Update(c.Request.Context(), cluster)

		c.JSON(http.StatusOK, gin.H{
			"status":             "connected",
			"message":            fmt.Sprintf("Connection test passed. Kubernetes %s, %d nodes.", version.GitVersion, nodeCount),
			"kubernetes_version": version.GitVersion,
			"node_count":         nodeCount,
			"api_server_url":     serverURL,
			"flux_installed":     cluster.FluxInstalled,
		})
	}
}
