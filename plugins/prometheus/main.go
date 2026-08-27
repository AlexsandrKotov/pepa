// PEPA Prometheus Plugin — implements MonitoringProvider.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	sdk "github.com/pepa/pepa/internal/plugin/sdk-go"
	"github.com/pepa/pepa/internal/provider"
)

// PrometheusPlugin implements provider.Provider for Prometheus integration.
type PrometheusPlugin struct{}

var _ provider.Provider = (*PrometheusPlugin)(nil)

func (p *PrometheusPlugin) Name() string    { return "prometheus" }
func (p *PrometheusPlugin) Version() string { return "0.1.0" }
func (p *PrometheusPlugin) Description() string {
	return "Prometheus monitoring integration — PromQL queries, alerts, targets, and rules."
}
func (p *PrometheusPlugin) PluginType() string { return "monitoring" }

func (p *PrometheusPlugin) Actions() []string {
	return []string{
		"query",
		"query_range",
		"list_alerts",
		"list_rules",
		"list_targets",
		"labels",
		"label_values",
		"alertmanagers",
	}
}

func (p *PrometheusPlugin) Execute(ctx context.Context, action string, params []byte, config map[string]string) ([]byte, error) {
	baseURL := strings.TrimRight(config["url"], "/")
	token := config["token"]
	if baseURL == "" {
		return nil, fmt.Errorf("prometheus plugin requires 'url' in config")
	}

	switch action {
	case "query":
		return p.query(ctx, baseURL, token, params)
	case "query_range":
		return p.queryRange(ctx, baseURL, token, params)
	case "list_alerts":
		return p.listAlerts(ctx, baseURL, token)
	case "list_rules":
		return p.listRules(ctx, baseURL, token)
	case "list_targets":
		return p.listTargets(ctx, baseURL, token)
	case "labels":
		return p.labels(ctx, baseURL, token)
	case "label_values":
		return p.labelValues(ctx, baseURL, token, params)
	case "alertmanagers":
		return p.alertmanagers(ctx, baseURL, token)
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

func (p *PrometheusPlugin) HealthCheck(ctx context.Context) (*provider.HealthStatus, error) {
	return &provider.HealthStatus{
		Status:  "healthy",
		Message: "Prometheus plugin ready — requires connection config (url)",
	}, nil
}

// ── Helpers ────────────────────────────────────────────────────

func promRequest(ctx context.Context, endpoint, token string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("prometheus returned status %d: %s", resp.StatusCode, string(body))
	}
	return io.ReadAll(resp.Body)
}

func promRequestWithParams(ctx context.Context, endpoint, token string, params url.Values) ([]byte, error) {
	fullURL := endpoint
	if len(params) > 0 {
		fullURL += "?" + params.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("prometheus returned status %d: %s", resp.StatusCode, string(body))
	}
	return io.ReadAll(resp.Body)
}

// ── Actions ────────────────────────────────────────────────────

func (p *PrometheusPlugin) query(ctx context.Context, baseURL, token string, params []byte) ([]byte, error) {
	var req struct {
		Query string `json:"query"`
		Time  string `json:"time,omitempty"` // RFC3339 or unix timestamp
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("parse params: %w", err)
	}
	if req.Query == "" {
		return nil, fmt.Errorf("query is required")
	}

	qp := url.Values{}
	qp.Set("query", req.Query)
	if req.Time != "" {
		qp.Set("time", req.Time)
	}

	data, err := promRequestWithParams(ctx, baseURL+"/api/v1/query", token, qp)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (p *PrometheusPlugin) queryRange(ctx context.Context, baseURL, token string, params []byte) ([]byte, error) {
	var req struct {
		Query string `json:"query"`
		Start string `json:"start"` // RFC3339
		End   string `json:"end"`   // RFC3339
		Step  string `json:"step"`  // e.g. "15s", "1m", "5m"
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("parse params: %w", err)
	}
	if req.Query == "" {
		return nil, fmt.Errorf("query is required")
	}
	if req.Start == "" || req.End == "" {
		return nil, fmt.Errorf("start and end are required")
	}
	if req.Step == "" {
		req.Step = "1m"
	}

	qp := url.Values{}
	qp.Set("query", req.Query)
	qp.Set("start", req.Start)
	qp.Set("end", req.End)
	qp.Set("step", req.Step)

	data, err := promRequestWithParams(ctx, baseURL+"/api/v1/query_range", token, qp)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (p *PrometheusPlugin) listAlerts(ctx context.Context, baseURL, token string) ([]byte, error) {
	// Try Alertmanager first (common setup), fall back to Prometheus /api/v1/alerts
	alertmanagerURL := baseURL
	// Prometheus has /api/v1/alerts for its own alerting rules
	data, err := promRequest(ctx, baseURL+"/api/v1/alerts", token)
	if err != nil {
		return nil, err
	}
	_ = alertmanagerURL
	return data, nil
}

func (p *PrometheusPlugin) listRules(ctx context.Context, baseURL, token string) ([]byte, error) {
	data, err := promRequest(ctx, baseURL+"/api/v1/rules", token)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (p *PrometheusPlugin) listTargets(ctx context.Context, baseURL, token string) ([]byte, error) {
	data, err := promRequest(ctx, baseURL+"/api/v1/targets", token)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (p *PrometheusPlugin) labels(ctx context.Context, baseURL, token string) ([]byte, error) {
	data, err := promRequest(ctx, baseURL+"/api/v1/labels", token)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (p *PrometheusPlugin) labelValues(ctx context.Context, baseURL, token string, params []byte) ([]byte, error) {
	var req struct {
		Label string `json:"label"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("parse params: %w", err)
	}
	if req.Label == "" {
		return nil, fmt.Errorf("label is required")
	}

	data, err := promRequest(ctx, baseURL+"/api/v1/label/"+url.PathEscape(req.Label)+"/values", token)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (p *PrometheusPlugin) alertmanagers(ctx context.Context, baseURL, token string) ([]byte, error) {
	data, err := promRequest(ctx, baseURL+"/api/v1/alertmanagers", token)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func main() {
	sdk.Serve(&PrometheusPlugin{})
}
