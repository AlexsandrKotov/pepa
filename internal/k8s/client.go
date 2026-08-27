// Package k8s provides a Kubernetes client for interacting with real clusters.
package k8s

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Client wraps a Kubernetes client for a specific cluster.
type Client struct {
	clientset      *kubernetes.Clientset
	kubeconfig     string
	serverOverride string // if set, overrides the API server URL in kubeconfig for Helm CLI
}

// NodeInfo represents information about a Kubernetes node.
type NodeInfo struct {
	Name              string `json:"name"`
	Status            string `json:"status"`
	Roles             string `json:"roles"`
	KubernetesVersion string `json:"kubernetes_version"`
	CPUCapacity       string `json:"cpu_capacity"`
	MemoryCapacity    string `json:"memory_capacity"`
	PodCapacity       int    `json:"pod_capacity"`
	CPUUsage          string `json:"cpu_usage"`
	MemoryUsage       string `json:"memory_usage"`
	OSImage           string `json:"os_image"`
	ContainerRuntime  string `json:"container_runtime"`
}

// NamespaceInfo represents information about a Kubernetes namespace.
type NamespaceInfo struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Pods   int    `json:"pods"`
}

// ResourceInfo represents information about a Kubernetes resource.
type ResourceInfo struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

// NewClientWithServerOverride creates a Kubernetes client from a kubeconfig but overrides the API server URL.
// This is useful when the kubeconfig contains an internal IP that is not reachable from PEPA.
func NewClientWithServerOverride(kubeconfig, serverURL string) (*Client, error) {
	if kubeconfig == "" {
		return nil, fmt.Errorf("kubeconfig is empty")
	}

	config, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubeconfig))
	if err != nil {
		return nil, fmt.Errorf("failed to parse kubeconfig: %w", err)
	}

	// Override the API server URL
	if serverURL != "" {
		config.Host = serverURL
	}

	// Set timeout to avoid hanging on unreachable clusters
	config.Timeout = 10 * time.Second

	// Allow self-signed certificates (common for k3s/kind/minikube)
	config.Insecure = true
	config.CAData = nil
	config.CAFile = ""

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return &Client{
		clientset:      clientset,
		kubeconfig:     kubeconfig,
		serverOverride: serverURL,
	}, nil
}

// NewClient creates a new Kubernetes client from a kubeconfig.
func NewClient(kubeconfig string) (*Client, error) {
	if kubeconfig == "" {
		return nil, fmt.Errorf("kubeconfig is empty")
	}

	config, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubeconfig))
	if err != nil {
		return nil, fmt.Errorf("failed to parse kubeconfig: %w", err)
	}

	// Set timeout to avoid hanging on unreachable clusters
	config.Timeout = 10 * time.Second

	// Allow self-signed certificates (common for k3s/kind/minikube)
	config.Insecure = true
	config.CAData = nil
	config.CAFile = ""

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return &Client{
		clientset:  clientset,
		kubeconfig: kubeconfig,
	}, nil
}

// ListNodes returns a list of nodes in the cluster.
func (c *Client) ListNodes(ctx context.Context) ([]NodeInfo, error) {
	nodes, err := c.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	var result []NodeInfo
	for _, node := range nodes.Items {
		info := NodeInfo{
			Name:              node.Name,
			CPUCapacity:       node.Status.Capacity.Cpu().String(),
			MemoryCapacity:    formatMemory(node.Status.Capacity.Memory().Value()),
			PodCapacity:       int(node.Status.Capacity.Pods().Value()),
			OSImage:           node.Status.NodeInfo.OSImage,
			ContainerRuntime:  node.Status.NodeInfo.ContainerRuntimeVersion,
			KubernetesVersion: node.Status.NodeInfo.KubeletVersion,
		}

		// Extract roles
		info.Roles = extractNodeRoles(node)

		// Extract status
		info.Status = extractNodeStatus(node)

		// Calculate allocatable resources as usage proxy
		info.CPUUsage = calcCPUUsage(node)
		info.MemoryUsage = calcMemoryUsage(node)

		result = append(result, info)
	}

	return result, nil
}

// ListNamespaces returns a list of namespaces in the cluster.
func (c *Client) ListNamespaces(ctx context.Context) ([]NamespaceInfo, error) {
	namespaces, err := c.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %w", err)
	}

	var result []NamespaceInfo
	for _, ns := range namespaces.Items {
		info := NamespaceInfo{
			Name:   ns.Name,
			Status: string(ns.Status.Phase),
		}

		// Count pods in namespace
		pods, err := c.clientset.CoreV1().Pods(ns.Name).List(ctx, metav1.ListOptions{})
		if err == nil {
			info.Pods = len(pods.Items)
		}

		result = append(result, info)
	}

	return result, nil
}

// ListResources returns a list of common resources in the cluster.
func (c *Client) ListResources(ctx context.Context, namespace string) ([]ResourceInfo, error) {
	var result []ResourceInfo

	// List Deployments
	deployments, err := c.clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, d := range deployments.Items {
			status := "Unknown"
			if d.Status.ReadyReplicas == *d.Spec.Replicas {
				status = "Ready"
			} else if d.Status.ReadyReplicas > 0 {
				status = "Progressing"
			} else if d.Status.UnavailableReplicas > 0 {
				status = "Unavailable"
			}
			result = append(result, ResourceInfo{
				Kind:      "Deployment",
				Name:      d.Name,
				Namespace: d.Namespace,
				Status:    status,
				CreatedAt: d.CreationTimestamp.Format(time.RFC3339),
			})
		}
	}

	// List Services
	services, err := c.clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, s := range services.Items {
			result = append(result, ResourceInfo{
				Kind:      "Service",
				Name:      s.Name,
				Namespace: s.Namespace,
				Status:    string(s.Spec.Type),
				CreatedAt: s.CreationTimestamp.Format(time.RFC3339),
			})
		}
	}

	// List ConfigMaps
	configMaps, err := c.clientset.CoreV1().ConfigMaps(namespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, cm := range configMaps.Items {
			result = append(result, ResourceInfo{
				Kind:      "ConfigMap",
				Name:      cm.Name,
				Namespace: cm.Namespace,
				Status:    "Active",
				CreatedAt: cm.CreationTimestamp.Format(time.RFC3339),
			})
		}
	}

	// List Secrets
	secrets, err := c.clientset.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, s := range secrets.Items {
			result = append(result, ResourceInfo{
				Kind:      "Secret",
				Name:      s.Name,
				Namespace: s.Namespace,
				Status:    string(s.Type),
				CreatedAt: s.CreationTimestamp.Format(time.RFC3339),
			})
		}
	}

	// List StatefulSets
	statefulSets, err := c.clientset.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, ss := range statefulSets.Items {
			status := "Unknown"
			if ss.Status.ReadyReplicas == *ss.Spec.Replicas {
				status = "Ready"
			} else if ss.Status.ReadyReplicas > 0 {
				status = "Progressing"
			}
			result = append(result, ResourceInfo{
				Kind:      "StatefulSet",
				Name:      ss.Name,
				Namespace: ss.Namespace,
				Status:    status,
				CreatedAt: ss.CreationTimestamp.Format(time.RFC3339),
			})
		}
	}

	// List DaemonSets
	daemonSets, err := c.clientset.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, ds := range daemonSets.Items {
			status := "Unknown"
			if ds.Status.NumberReady == ds.Status.DesiredNumberScheduled {
				status = "Ready"
			} else if ds.Status.NumberReady > 0 {
				status = "Progressing"
			}
			result = append(result, ResourceInfo{
				Kind:      "DaemonSet",
				Name:      ds.Name,
				Namespace: ds.Namespace,
				Status:    status,
				CreatedAt: ds.CreationTimestamp.Format(time.RFC3339),
			})
		}
	}

	return result, nil
}

// GetClusterInfo returns basic cluster information.
func (c *Client) GetClusterInfo(ctx context.Context) (nodeCount int, k8sVersion string, err error) {
	// Get server version
	version, err := c.clientset.Discovery().ServerVersion()
	if err != nil {
		return 0, "", fmt.Errorf("failed to get server version: %w", err)
	}
	k8sVersion = version.GitVersion

	// Get node count
	nodes, err := c.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return 0, k8sVersion, fmt.Errorf("failed to list nodes: %w", err)
	}
	nodeCount = len(nodes.Items)

	return nodeCount, k8sVersion, nil
}

// Helper functions

func extractNodeRoles(node corev1.Node) string {
	var roles []string
	for label := range node.Labels {
		if label == "node-role.kubernetes.io/control-plane" || label == "node-role.kubernetes.io/master" {
			roles = append(roles, "control-plane")
		} else if label == "node-role.kubernetes.io/worker" {
			roles = append(roles, "worker")
		} else if label == "node-role.kubernetes.io/infra" {
			roles = append(roles, "infra")
		}
	}
	// If no explicit role labels, check if it's not a master/infra node -> worker
	if len(roles) == 0 {
		isMaster := false
		for label := range node.Labels {
			if label == "node-role.kubernetes.io/control-plane" || label == "node-role.kubernetes.io/master" || label == "node-role.kubernetes.io/infra" {
				isMaster = true
				break
			}
		}
		if !isMaster {
			roles = append(roles, "worker")
		}
	}
	// Remove duplicates
	seen := make(map[string]bool)
	var unique []string
	for _, r := range roles {
		if !seen[r] {
			seen[r] = true
			unique = append(unique, r)
		}
	}
	return joinStrings(unique, ",")
}

func extractNodeStatus(node corev1.Node) string {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			if condition.Status == corev1.ConditionTrue {
				return "Ready"
			}
			return "NotReady"
		}
	}
	return "Unknown"
}

func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}

// FluxResource represents a FluxCD resource.
type FluxResource struct {
	Kind             string `json:"kind"`
	Name             string `json:"name"`
	Namespace        string `json:"namespace"`
	Status           string `json:"status"`
	Revision         string `json:"revision,omitempty"`
	Message          string `json:"message,omitempty"`
	LastReconciledAt string `json:"last_reconciled_at,omitempty"`
	Suspended        bool   `json:"suspended"`
}

// ListFluxResources returns FluxCD resources (Kustomizations, HelmReleases, etc.) from the cluster.
func (c *Client) ListFluxResources(ctx context.Context) ([]FluxResource, error) {
	// Use dynamic client to query FluxCD CRDs
	config, err := clientcmd.RESTConfigFromKubeConfig([]byte(c.kubeconfig))
	if err != nil {
		return nil, fmt.Errorf("failed to parse kubeconfig: %w", err)
	}
	config.Timeout = 10 * time.Second
	// Allow self-signed certificates (common for k3s/kind/minikube)
	config.Insecure = true
	config.CAData = nil
	config.CAFile = ""

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	var result []FluxResource

	// FluxCD CRD GVRs
	fluxGVRs := []struct {
		gvr  schema.GroupVersionResource
		kind string
	}{
		{schema.GroupVersionResource{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"}, "Kustomization"},
		{schema.GroupVersionResource{Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases"}, "HelmRelease"},
		{schema.GroupVersionResource{Group: "helm.toolkit.fluxcd.io", Version: "v2beta2", Resource: "helmreleases"}, "HelmRelease"},
		{schema.GroupVersionResource{Group: "helm.toolkit.fluxcd.io", Version: "v2beta1", Resource: "helmreleases"}, "HelmRelease"},
		{schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "gitrepositories"}, "GitRepository"},
		{schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: "v1beta2", Resource: "helmrepositories"}, "HelmRepository"},
		{schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: "v1beta2", Resource: "helmcharts"}, "HelmChart"},
	}

	for _, gvrInfo := range fluxGVRs {
		resources, err := dynamicClient.Resource(gvrInfo.gvr).Namespace("").List(ctx, metav1.ListOptions{})
		if err != nil {
			// CRD might not be installed, skip silently
			continue
		}

		for _, r := range resources.Items {
			fr := FluxResource{
				Kind:      gvrInfo.kind,
				Name:      r.GetName(),
				Namespace: r.GetNamespace(),
			}

			// Extract status from unstructured
			if status, ok := r.Object["status"].(map[string]interface{}); ok {
				if conditions, ok := status["conditions"].([]interface{}); ok && len(conditions) > 0 {
					if cond, ok := conditions[0].(map[string]interface{}); ok {
						if condType, ok := cond["type"].(string); ok {
							if condStatus, ok := cond["status"].(string); ok {
								if condType == "Ready" {
									if condStatus == "True" {
										fr.Status = "Ready"
									} else {
										fr.Status = "NotReady"
									}
								}
							}
						}
						if msg, ok := cond["message"].(string); ok {
							fr.Message = msg
						}
						if lastTrans, ok := cond["lastTransitionTime"].(string); ok {
							fr.LastReconciledAt = lastTrans
						}
					}
				}
			}

			// Extract revision
			if spec, ok := r.Object["status"].(map[string]interface{}); ok {
				if rev, ok := spec["lastAppliedRevision"].(string); ok {
					fr.Revision = rev
				} else if rev, ok := spec["lastAttemptedRevision"].(string); ok {
					fr.Revision = rev
				}
			}

			// Check if suspended
			if spec, ok := r.Object["spec"].(map[string]interface{}); ok {
				if susp, ok := spec["suspend"].(bool); ok {
					fr.Suspended = susp
				}
			}

			result = append(result, fr)
		}
	}

	return result, nil
}

// GitOpsEngine represents the detected GitOps engine in a cluster.
type GitOpsEngine struct {
	FluxCD    bool `json:"fluxcd"`
	ArgoCD    bool `json:"argocd"`
	FluxCount int  `json:"flux_count"`
	ArgoCount int  `json:"argo_count"`
}

// DetectGitOpsEngine checks the cluster for FluxCD and ArgoCD installations.
func (c *Client) DetectGitOpsEngine(ctx context.Context) (*GitOpsEngine, error) {
	config, err := clientcmd.RESTConfigFromKubeConfig([]byte(c.kubeconfig))
	if err != nil {
		return nil, fmt.Errorf("parse kubeconfig: %w", err)
	}
	config.Timeout = 5 * time.Second
	// Allow self-signed certificates (common for k3s/kind/minikube)
	config.Insecure = true
	config.CAData = nil
	config.CAFile = ""

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create dynamic client: %w", err)
	}

	engine := &GitOpsEngine{}

	// Check FluxCD — detect by CRD existence (list succeeds) and count resources
	fluxGVRs := []schema.GroupVersionResource{
		{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"},
		{Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases"},
		{Group: "helm.toolkit.fluxcd.io", Version: "v2beta2", Resource: "helmreleases"},
		{Group: "helm.toolkit.fluxcd.io", Version: "v2beta1", Resource: "helmreleases"},
	}
	for _, gvr := range fluxGVRs {
		list, err := dynamicClient.Resource(gvr).Namespace("").List(ctx, metav1.ListOptions{})
		if err == nil {
			engine.FluxCD = true
			engine.FluxCount += len(list.Items)
		}
	}

	// Check ArgoCD — detect by CRD existence and count resources
	argoGVRs := []schema.GroupVersionResource{
		{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"},
		{Group: "argoproj.io", Version: "v1alpha1", Resource: "appprojects"},
	}
	for _, gvr := range argoGVRs {
		list, err := dynamicClient.Resource(gvr).Namespace("").List(ctx, metav1.ListOptions{})
		if err == nil {
			engine.ArgoCD = true
			engine.ArgoCount += len(list.Items)
		}
	}

	return engine, nil
}

// ArgoResource represents an ArgoCD resource.
type ArgoResource struct {
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Status      string `json:"status"`
	Health      string `json:"health"`
	SyncStatus  string `json:"sync_status"`
	RepoURL     string `json:"repo_url,omitempty"`
	TargetRev   string `json:"target_revision,omitempty"`
	Destination string `json:"destination,omitempty"`
	Message     string `json:"message,omitempty"`
	LastUpdated string `json:"last_updated,omitempty"`
}

// ListArgoResources returns ArgoCD resources (Applications, AppProjects) from the cluster.
func (c *Client) ListArgoResources(ctx context.Context) ([]ArgoResource, error) {
	config, err := clientcmd.RESTConfigFromKubeConfig([]byte(c.kubeconfig))
	if err != nil {
		return nil, fmt.Errorf("failed to parse kubeconfig: %w", err)
	}
	config.Timeout = 10 * time.Second
	// Allow self-signed certificates (common for k3s/kind/minikube)
	config.Insecure = true
	config.CAData = nil
	config.CAFile = ""

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	var result []ArgoResource

	argoGVRs := []struct {
		gvr  schema.GroupVersionResource
		kind string
	}{
		{schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}, "Application"},
		{schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "appprojects"}, "AppProject"},
	}

	for _, gvrInfo := range argoGVRs {
		resources, err := dynamicClient.Resource(gvrInfo.gvr).Namespace("").List(ctx, metav1.ListOptions{})
		if err != nil {
			// CRD might not be installed, skip silently
			continue
		}

		for _, r := range resources.Items {
			ar := ArgoResource{
				Kind:      gvrInfo.kind,
				Name:      r.GetName(),
				Namespace: r.GetNamespace(),
			}

			// Extract health status
			if status, ok := r.Object["status"].(map[string]interface{}); ok {
				if health, ok := status["health"].(map[string]interface{}); ok {
					if s, ok := health["status"].(string); ok {
						ar.Health = s
					}
					if msg, ok := health["message"].(string); ok {
						ar.Message = msg
					}
				}
				// Extract sync status
				if sync, ok := status["sync"].(map[string]interface{}); ok {
					if s, ok := sync["status"].(string); ok {
						ar.SyncStatus = s
					}
				}
				// Extract operation state phase as overall status
				if opState, ok := status["operationState"].(map[string]interface{}); ok {
					if phase, ok := opState["phase"].(string); ok {
						ar.Status = phase
					}
				}
			}

			// Derive overall status from health
			if ar.Status == "" {
				switch strings.ToLower(ar.Health) {
				case "healthy":
					ar.Status = "Healthy"
				case "degraded":
					ar.Status = "Degraded"
				case "progressing", "missing":
					ar.Status = "Progressing"
				case "suspended":
					ar.Status = "Suspended"
				default:
					ar.Status = "Unknown"
				}
			}

			// Extract source info
			if spec, ok := r.Object["spec"].(map[string]interface{}); ok {
				if source, ok := spec["source"].(map[string]interface{}); ok {
					if repoURL, ok := source["repoURL"].(string); ok {
						ar.RepoURL = repoURL
					}
					if rev, ok := source["targetRevision"].(string); ok {
						ar.TargetRev = rev
					}
				}
				if dest, ok := spec["destination"].(map[string]interface{}); ok {
					ns, _ := dest["namespace"].(string)
					server, _ := dest["server"].(string)
					if ns != "" {
						ar.Destination = ns
					} else if server != "" {
						ar.Destination = server
					}
				}
			}

			// Extract creation timestamp
			if ts, ok := r.Object["metadata"].(map[string]interface{}); ok {
				if ct, ok := ts["creationTimestamp"].(string); ok {
					ar.LastUpdated = ct
				}
			}

			result = append(result, ar)
		}
	}

	return result, nil
}

// formatMemory converts bytes to human-readable format.
func formatMemory(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1fGi", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1fMi", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1fKi", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d", bytes)
	}
}

// calcCPUUsage calculates approximate CPU usage from allocatable vs capacity.
func calcCPUUsage(node corev1.Node) string {
	capacityMillicores := node.Status.Capacity.Cpu().MilliValue()
	allocatableMillicores := node.Status.Allocatable.Cpu().MilliValue()
	if capacityMillicores <= 0 {
		return "0%"
	}
	// Usage = (capacity - allocatable) / capacity * 100
	// This represents system-reserved + kubelet-reserved as a proxy
	usedPct := float64(capacityMillicores-allocatableMillicores) / float64(capacityMillicores) * 100
	if usedPct < 0 {
		usedPct = 0
	}
	return fmt.Sprintf("%.0f%%", usedPct)
}

// calcMemoryUsage calculates approximate memory usage from allocatable vs capacity.
func calcMemoryUsage(node corev1.Node) string {
	capacityBytes := node.Status.Capacity.Memory().Value()
	allocatableBytes := node.Status.Allocatable.Memory().Value()
	if capacityBytes <= 0 {
		return "0%"
	}
	usedPct := float64(capacityBytes-allocatableBytes) / float64(capacityBytes) * 100
	if usedPct < 0 {
		usedPct = 0
	}
	return fmt.Sprintf("%.0f%%", usedPct)
}
