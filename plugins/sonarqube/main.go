// PEPA SonarQube Plugin — Code quality and security analysis.
// Implements the Provider interface for SonarQube code quality scanning.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	sdk "github.com/pepa/pepa/internal/plugin/sdk-go"
	"github.com/pepa/pepa/internal/provider"
)

// SonarQubePlugin implements provider.Provider for SonarQube code quality analysis.
type SonarQubePlugin struct {
	url        string
	token      string
	projectKey string
	branch     string
	httpClient *http.Client
}

// QualityGate represents a SonarQube quality gate status.
type QualityGate struct {
	Status    string          `json:"status"` // OK, ERROR, WARN, NONE
	Conditions []GateCondition `json:"conditions"`
}

// GateCondition represents a single quality gate condition.
type GateCondition struct {
	Status        string `json:"status"`
	MetricKey     string `json:"metric_key"`
	Comparator    string `json:"comparator"`
	PeriodIndex   int    `json:"period_index"`
	ErrorThreshold string `json:"error_threshold"`
	ActualValue   string `json:"actual_value"`
}

// Issue represents a SonarQube issue (bug, vulnerability, or code smell).
type Issue struct {
	Key         string `json:"key"`
	Rule        string `json:"rule"`
	Severity    string `json:"severity"`
	Status      string `json:"status"`
	Message     string `json:"message"`
	Component   string `json:"component"`
	Project     string `json:"project"`
	Line        int    `json:"line,omitempty"`
	Type        string `json:"type"`
	Tags        []string `json:"tags,omitempty"`
	CreationAt  string `json:"creation_date"`
	UpdateAt    string `json:"update_date"`
}

// Measures represents project metrics.
type Measures struct {
	Component  string         `json:"component"`
	Branch     string         `json:"branch,omitempty"`
	Metrics    []Metric       `json:"metrics"`
}

// Metric represents a single metric value.
type Metric struct {
	Metric    string `json:"metric"`
	Value     string `json:"value"`
	BestValue bool   `json:"best_value,omitempty"`
}

// ProjectSummary is a comprehensive quality summary.
type ProjectSummary struct {
	ProjectKey   string       `json:"project_key"`
	Branch       string       `json:"branch"`
	QualityGate  *QualityGate `json:"quality_gate"`
	Measures     *Measures    `json:"measures"`
	IssueSummary *IssueSummary `json:"issue_summary"`
	FetchedAt    string       `json:"fetched_at"`
}

// IssueSummary holds issue counts by type and severity.
type IssueSummary struct {
	Bugs          int `json:"bugs"`
	Vulnerabilities int `json:"vulnerabilities"`
	CodeSmells    int `json:"code_smells"`
	BySeverity    map[string]int `json:"by_severity"`
}

// validateSonarQubeURL performs basic safety checks on the SonarQube URL.
// Since the URL is configured by platform admins (not end users) and SonarQube
// is typically deployed on internal networks, private IPs are allowed.
// Only cloud metadata endpoints and empty hosts are blocked.
func validateSonarQubeURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("only http and https schemes are allowed")
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("URL must include a hostname")
	}

	// Block cloud metadata IP explicitly (169.254.169.254)
	ips, err := net.LookupIP(host)
	if err == nil {
		for _, ip := range ips {
			if ip == nil {
				continue
			}
			// Block link-local metadata endpoint (AWS/GCP/Azure metadata)
			if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
				return fmt.Errorf("URL must not target link-local or metadata endpoints")
			}
		}
	}
	return nil
}

// NewSonarQubePlugin creates a new SonarQube plugin instance.
func NewSonarQubePlugin(config map[string]string) (*SonarQubePlugin, error) {
	sqURL := config["url"]
	if sqURL == "" {
		return nil, fmt.Errorf("sonarqube plugin requires url")
	}
	token := config["token"]
	if token == "" {
		return nil, fmt.Errorf("sonarqube plugin requires token")
	}
	projectKey := config["project_key"]
	if projectKey == "" {
		return nil, fmt.Errorf("sonarqube plugin requires project_key")
	}

	// Validate URL to prevent SSRF
	if err := validateSonarQubeURL(sqURL); err != nil {
		return nil, fmt.Errorf("invalid SonarQube URL: %w", err)
	}

	branch := config["branch"]
	if branch == "" {
		branch = "main"
	}

	return &SonarQubePlugin{
		url:        strings.TrimRight(sqURL, "/"),
		token:      token,
		projectKey: projectKey,
		branch:     branch,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return fmt.Errorf("too many redirects")
				}
				// Validate redirect target to prevent SSRF via redirect
				if err := validateSonarQubeURL(req.URL.String()); err != nil {
					return fmt.Errorf("redirect to disallowed address: %w", err)
				}
				return nil
			},
		},
	}, nil
}

func (p *SonarQubePlugin) Name() string        { return "sonarqube" }
func (p *SonarQubePlugin) Version() string     { return "0.1.0" }
func (p *SonarQubePlugin) Description() string { return "SonarQube code quality scanner — bugs, vulnerabilities, code smells, and coverage analysis" }
func (p *SonarQubePlugin) PluginType() string  { return "security_scanner" }

func (p *SonarQubePlugin) Actions() []string {
	return []string{
		"analyze",
		"get_quality_gate",
		"get_issues",
		"get_coverage",
		"get_measures",
		"get_project_summary",
	}
}

func (p *SonarQubePlugin) Execute(ctx context.Context, action string, params []byte, config map[string]string) ([]byte, error) {
	// Allow per-request config override
	if config != nil && config["url"] != "" && config["token"] != "" && config["project_key"] != "" {
		plugin, err := NewSonarQubePlugin(config)
		if err != nil {
			return nil, err
		}
		return plugin.Execute(ctx, action, params, nil)
	}

	var paramMap map[string]interface{}
	if len(params) > 0 {
		if err := json.Unmarshal(params, &paramMap); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}
	}

	switch action {
	case "analyze":
		return p.analyze(ctx, paramMap)
	case "get_quality_gate":
		return p.getQualityGate(ctx, paramMap)
	case "get_issues":
		return p.getIssues(ctx, paramMap)
	case "get_coverage":
		return p.getCoverage(ctx, paramMap)
	case "get_measures":
		return p.getMeasures(ctx, paramMap)
	case "get_project_summary":
		return p.getProjectSummary(ctx, paramMap)
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

// resolveProjectKey returns the override or default project key.
func (p *SonarQubePlugin) resolveProjectKey(params map[string]interface{}) string {
	if pk, ok := params["project_key"].(string); ok && pk != "" {
		return pk
	}
	return p.projectKey
}

// resolveBranch returns the override or default branch.
func (p *SonarQubePlugin) resolveBranch(params map[string]interface{}) string {
	if b, ok := params["branch"].(string); ok && b != "" {
		return b
	}
	return p.branch
}

// apiGet performs an authenticated GET request to the SonarQube API.
func (p *SonarQubePlugin) apiGet(ctx context.Context, path string, queryParams map[string]string) ([]byte, error) {
	u, err := url.Parse(p.url + path)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	q := u.Query()
	for k, v := range queryParams {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.SetBasicAuth(p.token, "")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sonarqube API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("resource not found (404) — check project_key and SonarQube URL")
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("authentication failed (401) — check SonarQube token")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sonarqube API returned status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// analyze fetches the project summary as the main analysis action.
func (p *SonarQubePlugin) analyze(ctx context.Context, params map[string]interface{}) ([]byte, error) {
	summary, err := p.fetchProjectSummary(ctx, params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(summary)
}

// getQualityGate fetches the quality gate status.
func (p *SonarQubePlugin) getQualityGate(ctx context.Context, params map[string]interface{}) ([]byte, error) {
	projectKey := p.resolveProjectKey(params)
	branch := p.resolveBranch(params)

	qp := map[string]string{
		"projectKey": projectKey,
	}
	if branch != "" {
		qp["branch"] = branch
	}

	data, err := p.apiGet(ctx, "/api/qualitygates/project_status", qp)
	if err != nil {
		return nil, err
	}

	var result struct {
		ProjectStatus struct {
			Status     string `json:"status"`
			Conditions []struct {
				Status        string `json:"status"`
				MetricKey     string `json:"metricKey"`
				Comparator    string `json:"comparator"`
				PeriodIndex   int    `json:"periodIndex"`
				ErrorThreshold string `json:"errorThreshold"`
				ActualValue   string `json:"actualValue"`
			} `json:"conditions"`
		} `json:"projectStatus"`
	}

	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse quality gate: %w", err)
	}

	gate := &QualityGate{
		Status: result.ProjectStatus.Status,
	}
	for _, c := range result.ProjectStatus.Conditions {
		gate.Conditions = append(gate.Conditions, GateCondition{
			Status:        c.Status,
			MetricKey:     c.MetricKey,
			Comparator:    c.Comparator,
			PeriodIndex:   c.PeriodIndex,
			ErrorThreshold: c.ErrorThreshold,
			ActualValue:   c.ActualValue,
		})
	}

	return json.Marshal(gate)
}

// getIssues fetches code issues.
func (p *SonarQubePlugin) getIssues(ctx context.Context, params map[string]interface{}) ([]byte, error) {
	projectKey := p.resolveProjectKey(params)

	qp := map[string]string{
		"componentKeys": projectKey,
		"ps":            "100",
	}

	if t, ok := params["types"].(string); ok && t != "" {
		qp["types"] = t
	}
	if s, ok := params["severities"].(string); ok && s != "" {
		qp["severities"] = s
	}
	if st, ok := params["statuses"].(string); ok && st != "" {
		qp["statuses"] = st
	} else {
		qp["statuses"] = "OPEN,CONFIRMED,REOPENED"
	}
	if ps, ok := params["page_size"].(float64); ok && ps > 0 {
		qp["ps"] = strconv.Itoa(int(ps))
	}

	data, err := p.apiGet(ctx, "/api/issues/search", qp)
	if err != nil {
		return nil, err
	}

	var result struct {
		Total  int `json:"total"`
		Paging struct {
			PageIndex int `json:"pageIndex"`
			Total     int `json:"total"`
		} `json:"paging"`
		Issues []struct {
			Key        string   `json:"key"`
			Rule       string   `json:"rule"`
			Severity   string   `json:"severity"`
			Status     string   `json:"status"`
			Message    string   `json:"message"`
			Component  string   `json:"component"`
			Project    string   `json:"project"`
			Line       int      `json:"line"`
			Type       string   `json:"type"`
			Tags       []string `json:"tags"`
			CreationAt string   `json:"creationDate"`
			UpdateAt   string   `json:"updateDate"`
		} `json:"issues"`
	}

	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse issues: %w", err)
	}

	issues := make([]Issue, 0, len(result.Issues))
	for _, i := range result.Issues {
		issues = append(issues, Issue{
			Key:        i.Key,
			Rule:       i.Rule,
			Severity:   i.Severity,
			Status:     i.Status,
			Message:    i.Message,
			Component:  i.Component,
			Project:    i.Project,
			Line:       i.Line,
			Type:       i.Type,
			Tags:       i.Tags,
			CreationAt: i.CreationAt,
			UpdateAt:   i.UpdateAt,
		})
	}

	return json.Marshal(map[string]interface{}{
		"issues": issues,
		"total":  result.Total,
	})
}

// getCoverage fetches code coverage metrics.
func (p *SonarQubePlugin) getCoverage(ctx context.Context, params map[string]interface{}) ([]byte, error) {
	projectKey := p.resolveProjectKey(params)
	branch := p.resolveBranch(params)

	qp := map[string]string{
		"component":  projectKey,
		"metricKeys": "coverage,line_coverage,branch_coverage,lines_to_cover,lines_covered",
	}
	if branch != "" {
		qp["branch"] = branch
	}

	data, err := p.apiGet(ctx, "/api/measures/component", qp)
	if err != nil {
		return nil, err
	}

	var result struct {
		Component struct {
			Key      string `json:"key"`
			Name     string `json:"name"`
			Branch   string `json:"branch"`
			Measures []struct {
				Metric    string `json:"metric"`
				Value     string `json:"value"`
				BestValue bool   `json:"bestValue"`
			} `json:"measures"`
		} `json:"component"`
	}

	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse coverage: %w", err)
	}

	measures := &Measures{
		Component: result.Component.Key,
		Branch:    result.Component.Branch,
	}
	for _, m := range result.Component.Measures {
		measures.Metrics = append(measures.Metrics, Metric{
			Metric:    m.Metric,
			Value:     m.Value,
			BestValue: m.BestValue,
		})
	}

	return json.Marshal(measures)
}

// getMeasures fetches project metrics.
func (p *SonarQubePlugin) getMeasures(ctx context.Context, params map[string]interface{}) ([]byte, error) {
	projectKey := p.resolveProjectKey(params)

	metricKeys := "ncloc,complexity,violations,bugs,vulnerabilities,code_smells,coverage,duplicated_lines_density"
	if mk, ok := params["metric_keys"].(string); ok && mk != "" {
		metricKeys = mk
	}

	qp := map[string]string{
		"component":  projectKey,
		"metricKeys": metricKeys,
	}

	data, err := p.apiGet(ctx, "/api/measures/component", qp)
	if err != nil {
		return nil, err
	}

	var result struct {
		Component struct {
			Key      string `json:"key"`
			Name     string `json:"name"`
			Measures []struct {
				Metric    string `json:"metric"`
				Value     string `json:"value"`
				BestValue bool   `json:"bestValue"`
			} `json:"measures"`
		} `json:"component"`
	}

	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse measures: %w", err)
	}

	measures := &Measures{
		Component: result.Component.Key,
	}
	for _, m := range result.Component.Measures {
		measures.Metrics = append(measures.Metrics, Metric{
			Metric:    m.Metric,
			Value:     m.Value,
			BestValue: m.BestValue,
		})
	}

	return json.Marshal(measures)
}

// getProjectSummary fetches a comprehensive quality summary.
func (p *SonarQubePlugin) getProjectSummary(ctx context.Context, params map[string]interface{}) ([]byte, error) {
	summary, err := p.fetchProjectSummary(ctx, params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(summary)
}

// fetchProjectSummary gathers quality gate, measures, and issue counts.
func (p *SonarQubePlugin) fetchProjectSummary(ctx context.Context, params map[string]interface{}) (*ProjectSummary, error) {
	projectKey := p.resolveProjectKey(params)
	branch := p.resolveBranch(params)

	summary := &ProjectSummary{
		ProjectKey: projectKey,
		Branch:     branch,
		FetchedAt:  time.Now().UTC().Format(time.RFC3339),
	}

	// Fetch quality gate
	qgData, err := p.apiGet(ctx, "/api/qualitygates/project_status", map[string]string{
		"projectKey": projectKey,
		"branch":     branch,
	})
	if err == nil {
		var qgResult struct {
			ProjectStatus struct {
				Status     string `json:"status"`
				Conditions []struct {
					Status        string `json:"status"`
					MetricKey     string `json:"metricKey"`
					Comparator    string `json:"comparator"`
					PeriodIndex   int    `json:"periodIndex"`
					ErrorThreshold string `json:"errorThreshold"`
					ActualValue   string `json:"actualValue"`
				} `json:"conditions"`
			} `json:"projectStatus"`
		}
		if json.Unmarshal(qgData, &qgResult) == nil {
			gate := &QualityGate{Status: qgResult.ProjectStatus.Status}
			for _, c := range qgResult.ProjectStatus.Conditions {
				gate.Conditions = append(gate.Conditions, GateCondition{
					Status:        c.Status,
					MetricKey:     c.MetricKey,
					Comparator:    c.Comparator,
					PeriodIndex:   c.PeriodIndex,
					ErrorThreshold: c.ErrorThreshold,
					ActualValue:   c.ActualValue,
				})
			}
			summary.QualityGate = gate
		}
	}

	// Fetch measures
	measuresData, err := p.apiGet(ctx, "/api/measures/component", map[string]string{
		"component":  projectKey,
		"metricKeys": "ncloc,bugs,vulnerabilities,code_smells,coverage,duplicated_lines_density,complexity,violations",
		"branch":     branch,
	})
	if err == nil {
		var mResult struct {
			Component struct {
				Key      string `json:"key"`
				Measures []struct {
					Metric    string `json:"metric"`
					Value     string `json:"value"`
					BestValue bool   `json:"bestValue"`
				} `json:"measures"`
			} `json:"component"`
		}
		if json.Unmarshal(measuresData, &mResult) == nil {
			m := &Measures{Component: mResult.Component.Key}
			for _, met := range mResult.Component.Measures {
				m.Metrics = append(m.Metrics, Metric{
					Metric:    met.Metric,
					Value:     met.Value,
					BestValue: met.BestValue,
				})
			}
			summary.Measures = m
		}
	}

	// Fetch issue summary
	issueData, err := p.apiGet(ctx, "/api/issues/search", map[string]string{
		"componentKeys": projectKey,
		"ps":            "1",
		"statuses":      "OPEN,CONFIRMED,REOPENED",
	})
	if err == nil {
		var iResult struct {
			Total int `json:"total"`
		}
		if json.Unmarshal(issueData, &iResult) == nil {
			issueSummary := &IssueSummary{
				BySeverity: make(map[string]int),
			}

			// Fetch counts by type
			for _, issueType := range []string{"BUG", "VULNERABILITY", "CODE_SMELL"} {
				typeData, err := p.apiGet(ctx, "/api/issues/search", map[string]string{
					"componentKeys": projectKey,
					"types":         issueType,
					"ps":            "1",
					"statuses":      "OPEN,CONFIRMED,REOPENED",
				})
				if err == nil {
					var tResult struct {
						Total int `json:"total"`
					}
					if json.Unmarshal(typeData, &tResult) == nil {
						switch issueType {
						case "BUG":
							issueSummary.Bugs = tResult.Total
						case "VULNERABILITY":
							issueSummary.Vulnerabilities = tResult.Total
						case "CODE_SMELL":
							issueSummary.CodeSmells = tResult.Total
						}
					}
				}
			}

			// Fetch counts by severity
			for _, sev := range []string{"BLOCKER", "CRITICAL", "MAJOR", "MINOR", "INFO"} {
				sevData, err := p.apiGet(ctx, "/api/issues/search", map[string]string{
					"componentKeys": projectKey,
					"severities":    sev,
					"ps":            "1",
					"statuses":      "OPEN,CONFIRMED,REOPENED",
				})
				if err == nil {
					var sResult struct {
						Total int `json:"total"`
					}
					if json.Unmarshal(sevData, &sResult) == nil {
						issueSummary.BySeverity[strings.ToLower(sev)] = sResult.Total
					}
				}
			}

			summary.IssueSummary = issueSummary
		}
	}

	return summary, nil
}

func (p *SonarQubePlugin) HealthCheck(ctx context.Context) (*provider.HealthStatus, error) {
	// Check SonarQube server connectivity
	data, err := p.apiGet(ctx, "/api/system/status", nil)
	if err != nil {
		return &provider.HealthStatus{
			Status:  "unhealthy",
			Message: fmt.Sprintf("cannot reach SonarQube at %s: %v", p.url, err),
		}, nil
	}

	var status struct {
		ID     string `json:"id"`
		Status string `json:"status"` // UP, DOWN, STARTING
	}
	if err := json.Unmarshal(data, &status); err != nil {
		return &provider.HealthStatus{
			Status:  "unhealthy",
			Message: "invalid response from SonarQube",
		}, nil
	}

	if status.Status != "UP" {
		return &provider.HealthStatus{
			Status:  "unhealthy",
			Message: fmt.Sprintf("SonarQube status: %s", status.Status),
		}, nil
	}

	return &provider.HealthStatus{
		Status:  "healthy",
		Message: fmt.Sprintf("Connected to SonarQube at %s", p.url),
	}, nil
}

func main() {
	sdk.Serve(&SonarQubePlugin{})
}
