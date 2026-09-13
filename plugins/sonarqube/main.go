// PEPA SonarQube Plugin — Code quality and security analysis.
// Implements the Provider interface for SonarQube code quality scanning.
package main

import (
	"context"
	"crypto/tls"
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
// The plugin is a pure REST client: SonarQube is an external application and the
// analysis itself is produced there (CI or manually). PEPA never executes a scanner.
type SonarQubePlugin struct {
	url        string
	token      string
	projectKey string
	branch     string
	httpClient *http.Client
}

// Paging limits for issue fetching. SonarQube caps deep paging, so we stop at a
// hard ceiling and report the result as truncated instead of looping forever.
const (
	issuesPageSize        = 500
	issuesMaxFetch        = 5000
	projectsPageSize      = 50
	projectsMaxFetch      = 500
	validIssueTransitions = "falsepositive,wontfix,reopen,accept,confirm"
)

// QualityGate represents a SonarQube quality gate status.
type QualityGate struct {
	Status     string          `json:"status"` // OK, ERROR, WARN, NONE
	Conditions []GateCondition `json:"conditions"`
}

// GateCondition represents a single quality gate condition.
type GateCondition struct {
	Status         string `json:"status"`
	MetricKey      string `json:"metric_key"`
	Comparator     string `json:"comparator"`
	PeriodIndex    int    `json:"period_index"`
	ErrorThreshold string `json:"error_threshold"`
	ActualValue    string `json:"actual_value"`
}

// Issue represents a SonarQube issue (bug, vulnerability, or code smell).
type Issue struct {
	Key           string   `json:"key"`
	Rule          string   `json:"rule"`
	Severity      string   `json:"severity"`
	Status        string   `json:"status"`
	Message       string   `json:"message"`
	Component     string   `json:"component"`
	ComponentPath string   `json:"component_path,omitempty"`
	Project       string   `json:"project"`
	Line          int      `json:"line,omitempty"`
	Type          string   `json:"type"`
	Tags          []string `json:"tags,omitempty"`
	Effort        string   `json:"effort,omitempty"`
	Debt          string   `json:"debt,omitempty"`
	URL           string   `json:"url,omitempty"`
	CreationAt    string   `json:"creation_date"`
	UpdateAt      string   `json:"update_date"`
}

// Measures represents project metrics.
type Measures struct {
	Component string   `json:"component"`
	Branch    string   `json:"branch,omitempty"`
	Metrics   []Metric `json:"metrics"`
}

// Metric represents a single metric value.
type Metric struct {
	Metric    string `json:"metric"`
	Value     string `json:"value"`
	BestValue bool   `json:"best_value,omitempty"`
}

// ProjectSummary is a comprehensive quality summary.
type ProjectSummary struct {
	ProjectKey   string        `json:"project_key"`
	Branch       string        `json:"branch"`
	QualityGate  *QualityGate  `json:"quality_gate"`
	Measures     *Measures     `json:"measures"`
	IssueSummary *IssueSummary `json:"issue_summary"`
	// AnalysisDate is when SonarQube last computed this project. PEPA does not
	// trigger analyses, so consumers use this to judge report freshness.
	AnalysisDate string `json:"analysis_date,omitempty"`
	FetchedAt    string `json:"fetched_at"`
	// Warnings collects non-fatal collection problems (e.g. measures endpoint
	// unavailable while the quality gate is fine). The report stays valid.
	Warnings []string `json:"warnings,omitempty"`
}

// IssueSummary holds issue counts by type and severity.
type IssueSummary struct {
	Total           int            `json:"total"`
	Bugs            int            `json:"bugs"`
	Vulnerabilities int            `json:"vulnerabilities"`
	CodeSmells      int            `json:"code_smells"`
	BySeverity      map[string]int `json:"by_severity"`
}

// issueFacet is one facet group returned by /api/issues/search.
type issueFacet struct {
	Property string            `json:"property"`
	Values   []issueFacetValue `json:"values"`
}

// issueFacetValue is a single bucket inside a facet group.
type issueFacetValue struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

// rawIssue mirrors the issue object as returned by the SonarQube API, using
// SonarQube's own camelCase field names (additionalFields adds the rest).
type rawIssue struct {
	Key           string   `json:"key"`
	Rule          string   `json:"rule"`
	Severity      string   `json:"severity"`
	Status        string   `json:"status"`
	Message       string   `json:"message"`
	Component     string   `json:"component"`
	ComponentPath string   `json:"componentPath"`
	Project       string   `json:"project"`
	Line          int      `json:"line"`
	Type          string   `json:"type"`
	Tags          []string `json:"tags"`
	Effort        string   `json:"effort"`
	Debt          string   `json:"debt"`
	URL           string   `json:"url"`
	CreationDate  string   `json:"creationDate"`
	UpdateDate    string   `json:"updateDate"`
}

// toIssue maps the API shape onto the plugin's public Issue type.
func (r rawIssue) toIssue() Issue {
	return Issue{
		Key:           r.Key,
		Rule:          r.Rule,
		Severity:      r.Severity,
		Status:        r.Status,
		Message:       r.Message,
		Component:     r.Component,
		ComponentPath: r.ComponentPath,
		Project:       r.Project,
		Line:          r.Line,
		Type:          r.Type,
		Tags:          r.Tags,
		Effort:        r.Effort,
		Debt:          r.Debt,
		URL:           r.URL,
		CreationAt:    r.CreationDate,
		UpdateAt:      r.UpdateDate,
	}
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
	// Use a timeout to prevent slow DNS lookups from blocking plugin operations
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resolver := net.Resolver{}
	ips, err := resolver.LookupIPAddr(ctx, host)
	if err == nil {
		for _, ip := range ips {
			// Block link-local metadata endpoint (AWS/GCP/Azure metadata)
			if ip.IP.IsLinkLocalUnicast() || ip.IP.IsLinkLocalMulticast() {
				return fmt.Errorf("URL must not target link-local or metadata endpoints")
			}
		}
	}
	// DNS lookup failure is not fatal — the URL may still work if the hostname
	// resolves at request time. Only block if we positively detect a metadata IP.
	return nil
}

// NewSonarQubePlugin creates a new SonarQube plugin instance.
// Only url and token are mandatory: the project key is per-target, not
// per-connection, and is normally supplied with each action's params.
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

	// Validate URL to prevent SSRF
	if err := validateSonarQubeURL(sqURL); err != nil {
		return nil, fmt.Errorf("invalid SonarQube URL: %w", err)
	}

	// Branch stays empty unless explicitly configured: omitting the branch
	// parameter makes SonarQube use the project's main branch, whereas a wrong
	// guess such as "main" on a "master"-based project fails the request.
	branch := config["branch"]

	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Internal SonarQube instances frequently use self-signed certificates. The
	// endpoint and this opt-in flag are both provided by a platform admin through
	// a Connection, never by an end user, so skipping verification is a deliberate
	// deployment choice rather than user-controlled input.
	if config["insecure"] == "true" {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // #nosec G402: admin-provided SonarQube endpoint
	}

	return &SonarQubePlugin{
		url:        strings.TrimRight(sqURL, "/"),
		token:      token,
		projectKey: projectKey,
		branch:     branch,
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
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

func (p *SonarQubePlugin) Name() string    { return "sonarqube" }
func (p *SonarQubePlugin) Version() string { return "0.1.0" }
func (p *SonarQubePlugin) Description() string {
	return "SonarQube code quality scanner — bugs, vulnerabilities, code smells, and coverage analysis"
}
func (p *SonarQubePlugin) PluginType() string { return "security_scanner" }

func (p *SonarQubePlugin) Actions() []string {
	return []string{
		"analyze",
		"get_quality_gate",
		"get_issues",
		"get_coverage",
		"get_measures",
		"get_project_summary",
		"list_projects",
		"get_analysis_status",
		"transition_issue",
	}
}

func (p *SonarQubePlugin) Execute(ctx context.Context, action string, params []byte, config map[string]string) ([]byte, error) {
	// Allow per-request config override. Credentials come from the linked
	// Connection (url + token); project_key is optional here because most
	// actions pass it through params.
	if config != nil && config["url"] != "" && config["token"] != "" {
		plugin, err := NewSonarQubePlugin(config)
		if err != nil {
			return nil, err
		}
		return plugin.Execute(ctx, action, params, nil)
	}

	// The plugin is served with a zero-value struct when unconfigured, so a
	// missing HTTP client means nobody supplied url/token — fail explicitly
	// instead of dereferencing a nil client.
	if p.httpClient == nil {
		return nil, fmt.Errorf("sonarqube plugin not configured: url and token are required (link a SonarQube connection)")
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
	case "list_projects":
		return p.listProjects(ctx, paramMap)
	case "get_analysis_status":
		return p.getAnalysisStatus(ctx, paramMap)
	case "transition_issue":
		return p.transitionIssue(ctx, paramMap)
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

// requireProjectKey fails fast instead of sending an empty componentKeys value,
// which SonarQube answers with an unrelated "resource not found" error.
func (p *SonarQubePlugin) requireProjectKey(params map[string]interface{}) (string, error) {
	key := p.resolveProjectKey(params)
	if key == "" {
		return "", fmt.Errorf("project_key is required — set the SonarQube project key on the scan target")
	}
	return key, nil
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
		// Empty values are skipped: SonarQube rejects parameters like "branch="
		// rather than treating them as absent.
		if v == "" {
			continue
		}
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
	defer func() { _ = resp.Body.Close() }()

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

// apiPost performs an authenticated form-encoded POST request to the SonarQube API.
// SonarQube's write endpoints accept parameters as application/x-www-form-urlencoded.
func (p *SonarQubePlugin) apiPost(ctx context.Context, path string, form url.Values) ([]byte, error) {
	u, err := url.Parse(p.url + path)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.SetBasicAuth(p.token, "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sonarqube API request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("authentication failed (401) — check SonarQube token")
	}
	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("permission denied (403) — the SonarQube token cannot perform this operation")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sonarqube API returned status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// parseIssueFacets extracts a counts map from a single facet property of an
// /api/issues/search response (e.g. "severities" or "types").
func parseIssueFacets(facets []issueFacet, property string) map[string]int {
	out := map[string]int{}
	for _, f := range facets {
		if f.Property != property {
			continue
		}
		for _, v := range f.Values {
			out[v.Value] = v.Count
		}
	}
	return out
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
	projectKey, err := p.requireProjectKey(params)
	if err != nil {
		return nil, err
	}
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
				Status         string `json:"status"`
				MetricKey      string `json:"metricKey"`
				Comparator     string `json:"comparator"`
				PeriodIndex    int    `json:"periodIndex"`
				ErrorThreshold string `json:"errorThreshold"`
				ActualValue    string `json:"actualValue"`
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
			Status:         c.Status,
			MetricKey:      c.MetricKey,
			Comparator:     c.Comparator,
			PeriodIndex:    c.PeriodIndex,
			ErrorThreshold: c.ErrorThreshold,
			ActualValue:    c.ActualValue,
		})
	}

	return json.Marshal(gate)
}

// buildIssueSearchParams assembles the shared query for /api/issues/search.
func (p *SonarQubePlugin) buildIssueSearchParams(params map[string]interface{}) map[string]string {
	qp := map[string]string{
		"componentKeys":    p.resolveProjectKey(params),
		"ps":               strconv.Itoa(issuesPageSize),
		"additionalFields": "_all",
		"facets":           "severities,types",
	}
	if branch := p.resolveBranch(params); branch != "" {
		qp["branch"] = branch
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
	if ps, ok := params["page_size"].(float64); ok && ps > 0 && int(ps) <= issuesPageSize {
		qp["ps"] = strconv.Itoa(int(ps))
	}
	return qp
}

// fetchIssuePages walks /api/issues/search page by page up to issuesMaxFetch.
func (p *SonarQubePlugin) fetchIssuePages(ctx context.Context, qp map[string]string) ([]Issue, int, []issueFacet, error) {
	pageSize, _ := strconv.Atoi(qp["ps"])
	if pageSize <= 0 {
		pageSize = issuesPageSize
	}

	var (
		issues []Issue
		total  int
		facets []issueFacet
	)

	for page := 1; ; page++ {
		qp["page"] = strconv.Itoa(page)
		data, err := p.apiGet(ctx, "/api/issues/search", qp)
		if err != nil {
			return nil, 0, nil, err
		}

		var result struct {
			Total  int          `json:"total"`
			Facets []issueFacet `json:"facets"`
			Issues []rawIssue   `json:"issues"`
		}
		if err := json.Unmarshal(data, &result); err != nil {
			return nil, 0, nil, fmt.Errorf("failed to parse issues: %w", err)
		}

		total = result.Total
		if len(facets) == 0 {
			facets = result.Facets
		}
		for _, i := range result.Issues {
			issues = append(issues, i.toIssue())
		}

		if len(result.Issues) == 0 || len(issues) >= total || len(issues) >= issuesMaxFetch {
			break
		}
	}

	return issues, total, facets, nil
}

// getIssues fetches code issues across pages.
func (p *SonarQubePlugin) getIssues(ctx context.Context, params map[string]interface{}) ([]byte, error) {
	// Without a project key the query would span every project the token can
	// see, so refuse instead of sending componentKeys empty.
	if _, err := p.requireProjectKey(params); err != nil {
		return nil, err
	}
	qp := p.buildIssueSearchParams(params)
	issues, total, facets, err := p.fetchIssuePages(ctx, qp)
	if err != nil {
		return nil, err
	}
	if issues == nil {
		issues = []Issue{}
	}

	return json.Marshal(map[string]interface{}{
		"issues":      issues,
		"total":       total,
		"fetched":     len(issues),
		"truncated":   total > len(issues),
		"by_severity": parseIssueFacets(facets, "severities"),
		"by_type":     parseIssueFacets(facets, "types"),
	})
}

// listProjects returns the analysable projects of the SonarQube instance so the
// UI can offer project keys instead of making the user guess them.
func (p *SonarQubePlugin) listProjects(ctx context.Context, params map[string]interface{}) ([]byte, error) {
	q := ""
	if s, ok := params["query"].(string); ok {
		q = s
	}

	qp := map[string]string{
		"qualifiers": "TRK",
		"ps":         strconv.Itoa(projectsPageSize),
	}
	if q != "" {
		qp["q"] = q
	}

	projects := make([]map[string]string, 0, projectsPageSize)
	for page := 1; ; page++ {
		qp["page"] = strconv.Itoa(page)
		data, err := p.apiGet(ctx, "/api/projects/component_suggestions", qp)
		if err != nil {
			return nil, err
		}

		var result struct {
			Components []struct {
				Key       string `json:"key"`
				Name      string `json:"name"`
				Qualifier string `json:"qualifier"`
			} `json:"components"`
		}
		if err := json.Unmarshal(data, &result); err != nil {
			return nil, fmt.Errorf("failed to parse projects: %w", err)
		}
		for _, c := range result.Components {
			projects = append(projects, map[string]string{
				"key":       c.Key,
				"name":      c.Name,
				"qualifier": c.Qualifier,
			})
		}

		// A short page is the last one: instances that ignore the page parameter
		// would otherwise be walked until the ceiling.
		if len(result.Components) == 0 || len(result.Components) < projectsPageSize || len(projects) >= projectsMaxFetch {
			break
		}
	}

	return json.Marshal(map[string]interface{}{
		"projects":  projects,
		"truncated": len(projects) >= projectsMaxFetch,
	})
}

// getAnalysisStatus reports whether SonarQube has ever analysed the project.
// A missing endpoint (older or restricted instances) is reported as unknown
// rather than an error, so the report can still be collected.
func (p *SonarQubePlugin) getAnalysisStatus(ctx context.Context, params map[string]interface{}) ([]byte, error) {
	projectKey, err := p.requireProjectKey(params)
	if err != nil {
		return nil, err
	}

	data, err := p.apiGet(ctx, "/api/analysis_reports/has_been_analyzed", map[string]string{
		"project": projectKey,
	})
	if err != nil {
		return json.Marshal(map[string]interface{}{
			"has_been_analyzed": nil,
			"warning":           err.Error(),
		})
	}

	// The endpoint returns a bare JSON boolean.
	var analyzed bool
	if err := json.Unmarshal(data, &analyzed); err != nil {
		return json.Marshal(map[string]interface{}{
			"has_been_analyzed": nil,
			"warning":           "unexpected response from has_been_analyzed",
		})
	}

	return json.Marshal(map[string]interface{}{"has_been_analyzed": analyzed})
}

// transitionIssue applies an official transition to an issue (e.g. mark as false
// positive). Transitions are whitelisted — the value is passed straight to the API.
func (p *SonarQubePlugin) transitionIssue(ctx context.Context, params map[string]interface{}) ([]byte, error) {
	issueKey, _ := params["issue_key"].(string)
	transition, _ := params["transition"].(string)
	if issueKey == "" {
		return nil, fmt.Errorf("issue_key is required")
	}

	allowed := false
	for _, t := range strings.Split(validIssueTransitions, ",") {
		if t == transition {
			allowed = true
			break
		}
	}
	if !allowed {
		return nil, fmt.Errorf("invalid transition %q (must be one of: %s)", transition, validIssueTransitions)
	}

	data, err := p.apiPost(ctx, "/api/issues/do_transition", url.Values{
		"issue":      []string{issueKey},
		"transition": []string{transition},
	})
	if err != nil {
		return nil, err
	}

	var result struct {
		Issue struct {
			Key    string `json:"key"`
			Status string `json:"status"`
		} `json:"issue"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		// A successful transition with an unparseable body is still a success;
		// report the echo we have rather than failing the user's action.
		return json.Marshal(map[string]interface{}{"ok": true})
	}

	return json.Marshal(map[string]interface{}{
		"ok":     true,
		"key":    result.Issue.Key,
		"status": result.Issue.Status,
	})
}

// getCoverage fetches code coverage metrics.
func (p *SonarQubePlugin) getCoverage(ctx context.Context, params map[string]interface{}) ([]byte, error) {
	projectKey, err := p.requireProjectKey(params)
	if err != nil {
		return nil, err
	}
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
	projectKey, err := p.requireProjectKey(params)
	if err != nil {
		return nil, err
	}

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
	projectKey, err := p.requireProjectKey(params)
	if err != nil {
		return nil, err
	}
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
					Status         string `json:"status"`
					MetricKey      string `json:"metricKey"`
					Comparator     string `json:"comparator"`
					PeriodIndex    int    `json:"periodIndex"`
					ErrorThreshold string `json:"errorThreshold"`
					ActualValue    string `json:"actualValue"`
				} `json:"conditions"`
			} `json:"projectStatus"`
		}
		if json.Unmarshal(qgData, &qgResult) == nil {
			gate := &QualityGate{Status: qgResult.ProjectStatus.Status}
			for _, c := range qgResult.ProjectStatus.Conditions {
				gate.Conditions = append(gate.Conditions, GateCondition{
					Status:         c.Status,
					MetricKey:      c.MetricKey,
					Comparator:     c.Comparator,
					PeriodIndex:    c.PeriodIndex,
					ErrorThreshold: c.ErrorThreshold,
					ActualValue:    c.ActualValue,
				})
			}
			summary.QualityGate = gate
		}
	} else {
		summary.Warnings = append(summary.Warnings, fmt.Sprintf("quality gate unavailable: %v", err))
	}

	// Fetch measures. analysis_date tells the caller how fresh the report is,
	// since PEPA collects analyses rather than producing them.
	measuresData, err := p.apiGet(ctx, "/api/measures/component", map[string]string{
		"component":  projectKey,
		"metricKeys": "ncloc,bugs,vulnerabilities,code_smells,coverage,line_coverage,branch_coverage,duplicated_lines_density,complexity,violations,sq_debt,reopened_issues,new_vulnerabilities,analysis_date",
		"branch":     branch,
	})
	if err != nil {
		summary.Warnings = append(summary.Warnings, fmt.Sprintf("measures unavailable: %v", err))
	} else {
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
			m := &Measures{Component: mResult.Component.Key, Branch: branch}
			for _, met := range mResult.Component.Measures {
				if met.Metric == "analysis_date" {
					summary.AnalysisDate = met.Value
					continue
				}
				m.Metrics = append(m.Metrics, Metric{
					Metric:    met.Metric,
					Value:     met.Value,
					BestValue: met.BestValue,
				})
			}
			summary.Measures = m
		} else {
			summary.Warnings = append(summary.Warnings, "measures response could not be parsed")
		}
	}

	// Issue summary — one facet request replaces the previous nine.
	issueData, err := p.apiGet(ctx, "/api/issues/search", map[string]string{
		"componentKeys": projectKey,
		"branch":        branch,
		"ps":            "1",
		"facets":        "severities,types",
		"statuses":      "OPEN,CONFIRMED,REOPENED",
	})
	if err != nil {
		summary.Warnings = append(summary.Warnings, fmt.Sprintf("issues unavailable: %v", err))
		return summary, nil
	}

	var iResult struct {
		Total  int          `json:"total"`
		Facets []issueFacet `json:"facets"`
	}
	if err := json.Unmarshal(issueData, &iResult); err != nil {
		summary.Warnings = append(summary.Warnings, "issues response could not be parsed")
		return summary, nil
	}

	byType := parseIssueFacets(iResult.Facets, "types")
	bySeverity := parseIssueFacets(iResult.Facets, "severities")
	issueSummary := &IssueSummary{
		Total:           iResult.Total,
		Bugs:            byType["BUG"],
		Vulnerabilities: byType["VULNERABILITY"],
		CodeSmells:      byType["CODE_SMELL"],
		BySeverity:      make(map[string]int, len(bySeverity)),
	}
	for sev, count := range bySeverity {
		issueSummary.BySeverity[strings.ToLower(sev)] = count
	}
	summary.IssueSummary = issueSummary

	return summary, nil
}

func (p *SonarQubePlugin) HealthCheck(ctx context.Context) (*provider.HealthStatus, error) {
	// If the plugin was not configured (served with zero-value struct),
	// httpClient will be nil — report unhealthy instead of panicking.
	if p.httpClient == nil || p.url == "" {
		return &provider.HealthStatus{
			Status:  "unhealthy",
			Message: "sonarqube plugin not configured — set url and token (project_key comes per target)",
		}, nil
	}

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
