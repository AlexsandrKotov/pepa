// PEPA ArgoCD Plugin — implements CDEngineProvider.
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	sdk "github.com/pepa/pepa/internal/plugin/sdk-go"
	"github.com/pepa/pepa/internal/provider"
)

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
	if serverURL == "" || authToken == "" {
		return nil, fmt.Errorf("argocd plugin requires server_url and auth_token")
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
func (p *ArgoCDPlugin) Version() string     { return "1.0.0" }
func (p *ArgoCDPlugin) Description() string { return "ArgoCD integration — deploy, sync, rollback" }
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

func (p *ArgoCDPlugin) Execute(ctx context.Context, action string, params []byte, config map[string]string) ([]byte, error) {
	if config != nil && config["auth_token"] != "" {
		plugin, err := NewArgoCDPlugin(config)
		if err != nil {
			return nil, err
		}
		return plugin.Execute(ctx, action, params, nil)
	}

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

func (p *ArgoCDPlugin) HealthCheck(ctx context.Context) (*provider.HealthStatus, error) {
	if p.client == nil {
		return &provider.HealthStatus{Status: "unhealthy", Message: "argocd not configured"}, nil
	}
	start := time.Now()
	resp, err := p.doRequest(ctx, "GET", "/api/v1/session/userinfo", nil)
	latency := time.Since(start)
	if err != nil {
		return &provider.HealthStatus{Status: "unhealthy", Message: fmt.Sprintf("argocd unreachable: %v", err), LatencyMs: latency.Milliseconds()}, nil
	}
	if resp["username"] == nil {
		return &provider.HealthStatus{Status: "degraded", Message: "argocd auth issue", LatencyMs: latency.Milliseconds()}, nil
	}
	return &provider.HealthStatus{Status: "healthy", Message: "connected to argocd", LatencyMs: latency.Milliseconds()}, nil
}

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
	defer resp.Body.Close()

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
	log.Println("[argocd-plugin] starting ArgoCD plugin v1.0.0")
	plugin := &ArgoCDPlugin{}
	sdk.Serve(plugin)
}
