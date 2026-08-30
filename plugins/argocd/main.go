// PEPA ArgoCD Plugin — implements CDEngineProvider.
// Supports two connection modes:
//  1. REST API mode: server_url + auth_token → ArgoCD HTTP API
//  2. CRD mode: kubeconfig → Kubernetes API (ArgoCD CRDs)
//
// If both are available, REST API takes priority for sync/rollback operations.
// CRD mode works without ArgoCD server exposure — only needs cluster access.
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	sdk "github.com/pepa/pepa/internal/plugin/sdk-go"
	"github.com/pepa/pepa/internal/provider"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

// ArgoCD Application GVR for CRD mode.
var argoAppGVR = schema.GroupVersionResource{
	Group:    "argoproj.io",
	Version:  "v1alpha1",
	Resource: "applications",
}

// ArgoCDPlugin implements provider.Provider for ArgoCD.
type ArgoCDPlugin struct {
	serverURL string
	authToken string
	insecure  bool
	client    *http.Client
}

func NewArgoCDPlugin(config map[string]string) (*ArgoCDPlugin, error) {
	serverURL := config["server_url"]
	authToken := config["auth_token"]

	// REST API mode: both server_url and auth_token required
	if serverURL == "" && authToken == "" {
		return nil, fmt.Errorf("argocd plugin: neither REST API (server_url+auth_token) nor kubeconfig provided")
	}
	if (serverURL != "" && authToken == "") || (serverURL == "" && authToken != "") {
		return nil, fmt.Errorf("argocd plugin: server_url and auth_token must be provided together")
	}

	insecure := config["insecure"] == "true"
	transport := &http.Transport{}
	if insecure {
		// Dev/self-signed certs only — never enable in production
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}

	return &ArgoCDPlugin{
		serverURL: serverURL,
		authToken: authToken,
		insecure:  insecure,
		client:    &http.Client{Transport: transport, Timeout: 30 * time.Second},
	}, nil
}

func (p *ArgoCDPlugin) Name() string        { return "argocd" }
func (p *ArgoCDPlugin) Version() string     { return "1.1.0" }
func (p *ArgoCDPlugin) Description() string { return "ArgoCD integration — REST API or Kubernetes CRD mode" }
func (p *ArgoCDPlugin) PluginType() string  { return "cd_engine" }

func (p *ArgoCDPlugin) Actions() []string {
	return []string{
		"list_applications",
		"get_application",
		"sync",
		"rollback",
		"get_status",
		"get_health",
	}
}

// Execute dispatches to REST API or CRD mode based on config.
func (p *ArgoCDPlugin) Execute(ctx context.Context, action string, params []byte, config map[string]string) ([]byte, error) {
	// Determine mode: check if kubeconfig is provided and REST API is not configured
	kubeconfig := ""
	if config != nil {
		kubeconfig = config["kubeconfig"]
	}

	// If config provides kubeconfig but no REST API credentials, use CRD mode
	if kubeconfig != "" && p.serverURL == "" {
		return p.executeCRDMode(ctx, action, params, kubeconfig)
	}

	// If per-request config overrides REST API credentials, create a temporary plugin
	if config != nil && config["auth_token"] != "" && config["server_url"] != "" {
		plugin, err := NewArgoCDPlugin(config)
		if err != nil {
			return nil, err
		}
		return plugin.Execute(ctx, action, params, nil)
	}

	// Use REST API mode (default)
	if p.client == nil && p.serverURL != "" {
		plugin, err := NewArgoCDPlugin(map[string]string{
			"server_url": p.serverURL,
			"auth_token": p.authToken,
			"insecure":   fmt.Sprintf("%v", p.insecure),
		})
		if err != nil {
			return nil, err
		}
		p.client = plugin.client
	}

	switch action {
	case "list_applications":
		return p.listApplications(ctx)
	case "get_application":
		return p.getApplication(ctx, params)
	case "sync":
		return p.sync(ctx, params)
	case "rollback":
		return p.rollback(ctx, params)
	case "get_status":
		return p.getStatus(ctx, params)
	case "get_health":
		return p.getHealth(ctx, params)
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

// HealthCheck reports plugin readiness.
// Unlike v1.0, this no longer requires REST API — CRD mode is also valid.
func (p *ArgoCDPlugin) HealthCheck(ctx context.Context) (*provider.HealthStatus, error) {
	if p.client == nil && p.serverURL == "" {
		return &provider.HealthStatus{
			Status:  "healthy",
			Message: "argocd plugin ready — requires kubeconfig or server_url+auth_token",
		}, nil
	}
	if p.client == nil {
		start := time.Now()
		resp, err := p.doRequest(ctx, "GET", "/api/v1/session/userinfo", nil)
		latency := time.Since(start)
		if err != nil {
			return &provider.HealthStatus{Status: "unhealthy", Message: fmt.Sprintf("argocd unreachable: %v", err), LatencyMs: latency.Milliseconds()}, nil
		}
		if resp["username"] == nil {
			return &provider.HealthStatus{Status: "degraded", Message: "argocd auth issue", LatencyMs: latency.Milliseconds()}, nil
		}
		return &provider.HealthStatus{Status: "healthy", Message: "connected to argocd (REST API)", LatencyMs: latency.Milliseconds()}, nil
	}
	return &provider.HealthStatus{Status: "healthy", Message: "argocd plugin ready (REST API configured)"}, nil
}

// =============================================================================
// CRD Mode — operates via Kubernetes API directly (no ArgoCD REST API needed)
// =============================================================================

func (p *ArgoCDPlugin) executeCRDMode(ctx context.Context, action string, params []byte, kubeconfig string) ([]byte, error) {
	dc, err := newCRDClient([]byte(kubeconfig))
	if err != nil {
		return nil, fmt.Errorf("argocd CRD mode: %w", err)
	}

	switch action {
	case "list_applications":
		return crdListApplications(ctx, dc)
	case "get_application":
		return crdGetApplication(ctx, dc, params)
	case "sync":
		return crdSync(ctx, dc, params)
	case "rollback":
		return crdRollback(ctx, dc, params)
	case "get_status":
		return crdGetStatus(ctx, dc, params)
	case "get_health":
		return crdGetStatus(ctx, dc, params) // same as get_status for CRD mode
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

// crdClient wraps the dynamic Kubernetes client for ArgoCD CRD operations.
type crdClient struct {
	client dynamic.Interface
}

func newCRDClient(kubeconfig []byte) (*crdClient, error) {
	config, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("parse kubeconfig: %w", err)
	}
	dc, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create dynamic client: %w", err)
	}
	return &crdClient{client: dc}, nil
}

// crdListApplications lists all ArgoCD Applications across all namespaces.
func crdListApplications(ctx context.Context, dc *crdClient) ([]byte, error) {
	list, err := dc.client.Resource(argoAppGVR).Namespace("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list argocd applications: %w", err)
	}

	apps := make([]provider.CDApplication, 0, len(list.Items))
	for _, item := range list.Items {
		health, _ := getNestedString(item.Object, "status", "health", "status")
		syncStatus, _ := getNestedString(item.Object, "status", "sync", "status")
		revision, _ := getNestedString(item.Object, "status", "sync", "revision")

		apps = append(apps, provider.CDApplication{
			Name:       item.GetName(),
			Namespace:  item.GetNamespace(),
			Health:     health,
			SyncStatus: syncStatus,
			Revision:   revision,
		})
	}
	return sdk.ActionOutput(apps)
}

// crdGetApplication returns a single ArgoCD Application by name.
func crdGetApplication(ctx context.Context, dc *crdClient, params []byte) ([]byte, error) {
	var input struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, err
	}
	if input.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if input.Namespace == "" {
		input.Namespace = "argocd"
	}

	obj, err := dc.client.Resource(argoAppGVR).Namespace(input.Namespace).Get(ctx, input.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get application %s/%s: %w", input.Namespace, input.Name, err)
	}

	return sdk.ActionOutput(flattenArgoResource(obj))
}

// crdSync triggers a sync by annotating the Application for refresh.
// ArgoCD controller picks up the annotation and reconciles.
func crdSync(ctx context.Context, dc *crdClient, params []byte) ([]byte, error) {
	var input struct {
		Application string `json:"application"`
		AppName     string `json:"app_name"`
		Namespace   string `json:"namespace"`
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, err
	}
	appName := input.Application
	if appName == "" {
		appName = input.AppName
	}
	if appName == "" {
		return nil, fmt.Errorf("application name is required")
	}
	if input.Namespace == "" {
		input.Namespace = "argocd"
	}

	obj, err := dc.client.Resource(argoAppGVR).Namespace(input.Namespace).Get(ctx, appName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get application %s/%s: %w", input.Namespace, appName, err)
	}

	// Trigger sync via annotation (ArgoCD watches for this)
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations["argocd.argoproj.io/refresh-type"] = "Normal"
	annotations["argocd.argoproj.io/refresh-requested-at"] = time.Now().Format(time.RFC3339Nano)
	obj.SetAnnotations(annotations)

	_, err = dc.client.Resource(argoAppGVR).Namespace(input.Namespace).Update(ctx, obj, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("trigger sync for %s/%s: %w", input.Namespace, appName, err)
	}

	return sdk.ActionOutput(map[string]string{
		"status":    "success",
		"message":   fmt.Sprintf("sync triggered for %s/%s", input.Namespace, appName),
		"mode":      "crd",
		"application": appName,
	})
}

// crdRollback updates the targetRevision to a previous revision.
func crdRollback(ctx context.Context, dc *crdClient, params []byte) ([]byte, error) {
	var input struct {
		Application string `json:"application"`
		AppName     string `json:"app_name"`
		Namespace   string `json:"namespace"`
		Revision    string `json:"revision"`
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, err
	}
	appName := input.Application
	if appName == "" {
		appName = input.AppName
	}
	if appName == "" {
		return nil, fmt.Errorf("application name is required")
	}
	if input.Revision == "" {
		return nil, fmt.Errorf("revision is required for rollback")
	}
	if input.Namespace == "" {
		input.Namespace = "argocd"
	}

	obj, err := dc.client.Resource(argoAppGVR).Namespace(input.Namespace).Get(ctx, appName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get application %s/%s: %w", input.Namespace, appName, err)
	}

	// Update source.targetRevision
	spec, ok := obj.Object["spec"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("application %s/%s has no spec", input.Namespace, appName)
	}
	source, ok := spec["source"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("application %s/%s has no spec.source", input.Namespace, appName)
	}
	source["targetRevision"] = input.Revision

	_, err = dc.client.Resource(argoAppGVR).Namespace(input.Namespace).Update(ctx, obj, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("rollback %s/%s to %s: %w", input.Namespace, appName, input.Revision, err)
	}

	return sdk.ActionOutput(map[string]string{
		"status":    "success",
		"message":   fmt.Sprintf("rollback %s/%s to revision %s", input.Namespace, appName, input.Revision),
		"mode":      "crd",
		"application": appName,
		"revision":  input.Revision,
	})
}

// crdGetStatus returns the health and sync status of an Application.
func crdGetStatus(ctx context.Context, dc *crdClient, params []byte) ([]byte, error) {
	var input struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, err
	}
	if input.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if input.Namespace == "" {
		input.Namespace = "argocd"
	}

	obj, err := dc.client.Resource(argoAppGVR).Namespace(input.Namespace).Get(ctx, input.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get application %s/%s: %w", input.Namespace, input.Name, err)
	}

	health, _ := getNestedString(obj.Object, "status", "health", "status")
	syncStatus, _ := getNestedString(obj.Object, "status", "sync", "status")
	revision, _ := getNestedString(obj.Object, "status", "sync", "revision")

	return sdk.ActionOutput(provider.DeployStatus{
		Application: input.Name,
		Status:      syncStatus,
		Revision:    revision,
		Synced:      syncStatus == "Synced",
		Health:      health,
	})
}

// flattenArgoResource extracts key fields from an ArgoCD Application for clean output.
func flattenArgoResource(obj *unstructured.Unstructured) map[string]interface{} {
	result := map[string]interface{}{
		"name":            obj.GetName(),
		"namespace":       obj.GetNamespace(),
		"uid":             string(obj.GetUID()),
		"resourceVersion": obj.GetResourceVersion(),
		"created":         obj.GetCreationTimestamp().Time.Format(time.RFC3339),
	}

	if spec, ok := obj.Object["spec"].(map[string]interface{}); ok {
		result["project"] = spec["project"]
		if source, ok := spec["source"].(map[string]interface{}); ok {
			result["source"] = map[string]interface{}{
				"repoURL":        source["repoURL"],
				"path":           source["path"],
				"targetRevision": source["targetRevision"],
			}
		}
		if dest, ok := spec["destination"].(map[string]interface{}); ok {
			result["destination"] = map[string]interface{}{
				"server":    dest["server"],
				"namespace": dest["namespace"],
			}
		}
	}

	if status, ok := obj.Object["status"].(map[string]interface{}); ok {
		health, _ := getNestedString(obj.Object, "status", "health", "status")
		syncStatus, _ := getNestedString(obj.Object, "status", "sync", "status")
		revision, _ := getNestedString(obj.Object, "status", "sync", "revision")
		result["status"] = map[string]interface{}{
			"health":     health,
			"syncStatus": syncStatus,
			"revision":   revision,
			"conditions": status["conditions"],
		}
	}

	return result
}

// getNestedString safely extracts a string from nested maps.
func getNestedString(obj map[string]interface{}, fields ...string) (string, bool) {
	var val interface{} = obj
	for _, field := range fields {
		if m, ok := val.(map[string]interface{}); ok {
			val = m[field]
		} else {
			return "", false
		}
	}
	if s, ok := val.(string); ok {
		return s, true
	}
	return "", false
}

// =============================================================================
// REST API Mode — operates via ArgoCD HTTP API
// =============================================================================

// ── ArgoCD API helpers ───────────────────────────────────────

func (p *ArgoCDPlugin) doRequest(ctx context.Context, method, path string, body interface{}) (map[string]interface{}, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(data)
	}

	url := p.serverURL + path
	// SSRF guard: only allow the configured server URL
	if p.serverURL == "" {
		return nil, fmt.Errorf("argocd server_url not configured")
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.authToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("argocd API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if len(respBody) > 0 {
		if err := json.Unmarshal(respBody, &result); err != nil {
			return nil, fmt.Errorf("parse response: %w", err)
		}
	}
	return result, nil
}

func (p *ArgoCDPlugin) listApplications(ctx context.Context) ([]byte, error) {
	result, err := p.doRequest(ctx, "GET", "/api/v1/applications", nil)
	if err != nil {
		return nil, err
	}

	items, _ := result["items"].([]interface{})
	apps := make([]provider.CDApplication, 0, len(items))
	for _, item := range items {
		app, _ := item.(map[string]interface{})
		meta, _ := app["metadata"].(map[string]interface{})
		status, _ := app["status"].(map[string]interface{})
		health, _ := status["health"].(map[string]interface{})
		source, _ := status["sync"].(map[string]interface{})

		apps = append(apps, provider.CDApplication{
			Name:       getString(meta, "name"),
			Namespace:  getString(meta, "namespace"),
			Status:     getString(status, "status"),
			Health:     getString(health, "status"),
			SyncStatus: getString(source, "status"),
			Revision:   getString(source, "revision"),
		})
	}
	return sdk.ActionOutput(apps)
}

func (p *ArgoCDPlugin) getApplication(ctx context.Context, params []byte) ([]byte, error) {
	var input struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, err
	}

	result, err := p.doRequest(ctx, "GET", "/api/v1/applications/"+input.Name, nil)
	if err != nil {
		return nil, err
	}
	return sdk.ActionOutput(result)
}

func (p *ArgoCDPlugin) sync(ctx context.Context, params []byte) ([]byte, error) {
	var input provider.SyncRequest
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, err
	}

	body := map[string]interface{}{
		"prune":  input.Prune,
		"dryRun": input.DryRun,
	}
	if input.Revision != "" {
		body["revision"] = input.Revision
	}

	result, err := p.doRequest(ctx, "POST", fmt.Sprintf("/api/v1/applications/%s/sync", input.Application), body)
	if err != nil {
		return nil, fmt.Errorf("sync failed: %w", err)
	}
	return sdk.ActionOutput(result)
}

func (p *ArgoCDPlugin) rollback(ctx context.Context, params []byte) ([]byte, error) {
	var input provider.RollbackRequest
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, err
	}

	body := map[string]interface{}{
		"revision": input.Revision,
	}
	result, err := p.doRequest(ctx, "POST", fmt.Sprintf("/api/v1/applications/%s/rollback", input.Application), body)
	if err != nil {
		return nil, fmt.Errorf("rollback failed: %w", err)
	}
	return sdk.ActionOutput(result)
}

func (p *ArgoCDPlugin) getStatus(ctx context.Context, params []byte) ([]byte, error) {
	var input struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, err
	}

	result, err := p.doRequest(ctx, "GET", "/api/v1/applications/"+input.Name, nil)
	if err != nil {
		return nil, err
	}

	status, _ := result["status"].(map[string]interface{})
	health, _ := status["health"].(map[string]interface{})
	sync, _ := status["sync"].(map[string]interface{})

	return sdk.ActionOutput(provider.DeployStatus{
		Application: input.Name,
		Status:      getString(status, "status"),
		Revision:    getString(sync, "revision"),
		Synced:      getString(sync, "status") == "Synced",
		Health:      getString(health, "status"),
	})
}

func (p *ArgoCDPlugin) getHealth(ctx context.Context, params []byte) ([]byte, error) {
	return p.getStatus(ctx, params)
}

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func main() {
	slog.Info("[argocd-plugin] starting ArgoCD plugin v1.1.0 (dual-mode: REST API + CRD)")
	plugin := &ArgoCDPlugin{}
	sdk.Serve(plugin)
}
