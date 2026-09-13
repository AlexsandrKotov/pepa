package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeSonar starts a stand-in SonarQube and returns a plugin pointed at it.
// Every test drives the plugin through Execute, i.e. exactly the path the PEPA
// scanner uses, so a broken request shape is caught here rather than in a scan.
func fakeSonar(t *testing.T, handler http.HandlerFunc) *SonarQubePlugin {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	plugin, err := NewSonarQubePlugin(map[string]string{"url": srv.URL, "token": "sqp-test-token"})
	if err != nil {
		t.Fatalf("NewSonarQubePlugin: %v", err)
	}
	return plugin
}

func writeJSON(t *testing.T, w http.ResponseWriter, payload any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Errorf("encode fixture: %v", err)
	}
}

// TestExecuteWithoutCredentialsFailsFast is the B1 regression: the plugin is
// served as a zero-value struct, so an incomplete config used to reach a nil
// HTTP client and panic inside the plugin process.
func TestExecuteWithoutCredentialsFailsFast(t *testing.T) {
	plugin := &SonarQubePlugin{}

	cases := []struct {
		name   string
		config map[string]string
	}{
		{name: "no config at all"},
		{name: "url only (scanner used to pass this)", config: map[string]string{"url": "http://sonar.internal"}},
		{name: "token only", config: map[string]string{"token": "sqp-x"}},
		{name: "empty config", config: map[string]string{"url": "", "token": ""}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Execute panicked instead of returning an error: %v", r)
				}
			}()
			_, err := plugin.Execute(context.Background(), "get_issues", []byte(`{"project_key":"proj"}`), tc.config)
			if err == nil {
				t.Fatal("expected an error for an unconfigured plugin, got nil")
			}
			if !strings.Contains(err.Error(), "url and token are required") {
				t.Errorf("error should explain the missing credentials, got %q", err.Error())
			}
		})
	}
}

func TestNewSonarQubePluginRequiresCredentials(t *testing.T) {
	if _, err := NewSonarQubePlugin(map[string]string{"token": "t"}); err == nil {
		t.Error("expected an error when url is missing")
	}
	if _, err := NewSonarQubePlugin(map[string]string{"url": "http://sonar.internal"}); err == nil {
		t.Error("expected an error when token is missing")
	}
	// project_key is deliberately optional: it belongs to the scan target, not
	// to the connection, and most actions pass it through params.
	plugin, err := NewSonarQubePlugin(map[string]string{"url": "http://sonar.internal", "token": "t"})
	if err != nil {
		t.Fatalf("url+token must be enough: %v", err)
	}
	if plugin.projectKey != "" {
		t.Errorf("project key must stay empty when unset, got %q", plugin.projectKey)
	}
}

func TestValidateSonarQubeURL(t *testing.T) {
	if err := validateSonarQubeURL("http://169.254.169.254"); err == nil {
		t.Error("cloud metadata endpoint must be rejected")
	}
	if err := validateSonarQubeURL("ftp://sonar.internal"); err == nil {
		t.Error("non-http scheme must be rejected")
	}
	if err := validateSonarQubeURL("http://10.0.0.5:9000"); err != nil {
		t.Errorf("private networks are legal for an internal SonarQube: %v", err)
	}
}

func TestInsecureFlagSkipsTLSVerification(t *testing.T) {
	plugin, err := NewSonarQubePlugin(map[string]string{
		"url": "https://sonar.internal", "token": "t", "insecure": "true",
	})
	if err != nil {
		t.Fatalf("NewSonarQubePlugin: %v", err)
	}
	transport, ok := plugin.httpClient.Transport.(*http.Transport)
	if !ok || transport.TLSClientConfig == nil || !transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("insecure=true must build a client that skips TLS verification")
	}

	safe, err := NewSonarQubePlugin(map[string]string{"url": "https://sonar.internal", "token": "t"})
	if err != nil {
		t.Fatalf("NewSonarQubePlugin: %v", err)
	}
	transport, ok = safe.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatal("expected an http.Transport")
	}
	if transport.TLSClientConfig != nil && transport.TLSClientConfig.InsecureSkipVerify {
		t.Error("insecure must stay opt-in")
	}
}

// TestGetIssuesPaginates guards the paging loop: SonarQube caps a page at 500
// issues, so a report is only complete once every page has been walked.
func TestGetIssuesPaginates(t *testing.T) {
	var pages []string
	plugin := fakeSonar(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/issues/search" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if user, _, ok := r.BasicAuth(); !ok || user != "sqp-test-token" {
			t.Error("the token must be sent as basic auth username")
		}
		page := r.URL.Query().Get("page")
		pages = append(pages, page)
		if r.URL.Query().Get("statuses") == "" {
			t.Error("only open issues should be reported")
		}
		switch page {
		case "1":
			writeJSON(t, w, map[string]any{"total": 3, "issues": []rawIssue{{Key: "A1", Severity: "BLOCKER", ComponentPath: "src/a.go"}}})
		case "2":
			writeJSON(t, w, map[string]any{"total": 3, "issues": []rawIssue{{Key: "A2", Severity: "MAJOR", ComponentPath: "src/b.go"}}})
		default:
			writeJSON(t, w, map[string]any{"total": 3, "issues": []rawIssue{{Key: "A3", Severity: "INFO", ComponentPath: "src/c.go"}}})
		}
	})

	out, err := plugin.Execute(context.Background(), "get_issues", []byte(`{"project_key":"proj","page_size":1}`), nil)
	if err != nil {
		t.Fatalf("get_issues: %v", err)
	}
	var page struct {
		Issues    []Issue `json:"issues"`
		Total     int     `json:"total"`
		Fetched   int     `json:"fetched"`
		Truncated bool    `json:"truncated"`
	}
	if err := json.Unmarshal(out, &page); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if page.Fetched != 3 || len(page.Issues) != 3 {
		t.Errorf("expected all 3 paged issues, got %d (fetched %d)", len(page.Issues), page.Fetched)
	}
	if page.Truncated {
		t.Error("a complete walk must not be reported as truncated")
	}
	if len(pages) != 3 {
		t.Errorf("expected 3 requests, got %d", len(pages))
	}
	if page.Issues[0].ComponentPath != "src/a.go" {
		t.Errorf("componentPath must be surfaced for grouping, got %q", page.Issues[0].ComponentPath)
	}
}

func TestGetIssuesReportsTruncation(t *testing.T) {
	plugin := fakeSonar(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "1" {
			writeJSON(t, w, map[string]any{"total": 10, "issues": []rawIssue{{Key: "A1", Severity: "MAJOR"}}})
			return
		}
		writeJSON(t, w, map[string]any{"total": 10, "issues": []rawIssue{}})
	})

	out, err := plugin.Execute(context.Background(), "get_issues", []byte(`{"project_key":"proj"}`), nil)
	if err != nil {
		t.Fatalf("get_issues: %v", err)
	}
	var page struct {
		Fetched   int  `json:"fetched"`
		Total     int  `json:"total"`
		Truncated bool `json:"truncated"`
	}
	if err := json.Unmarshal(out, &page); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if !page.Truncated || page.Total != 10 || page.Fetched != 1 {
		t.Errorf("expected truncated report (10 total, 1 fetched), got %+v", page)
	}
}

func TestGetIssuesNeedsProjectKey(t *testing.T) {
	plugin := fakeSonar(t, func(w http.ResponseWriter, _ *http.Request) {
		t.Error("no request must be sent without a project key")
	})
	if _, err := plugin.Execute(context.Background(), "get_issues", []byte(`{}`), nil); err == nil {
		t.Fatal("an empty project key must be rejected instead of asking for every issue")
	}
}

func TestListProjects(t *testing.T) {
	var gotQuery, gotQualifiers string
	requests := 0
	plugin := fakeSonar(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/projects/component_suggestions" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		requests++
		gotQuery = r.URL.Query().Get("q")
		gotQualifiers = r.URL.Query().Get("qualifiers")
		writeJSON(t, w, map[string]any{"components": []map[string]string{
			{"key": "checkout-service", "name": "Checkout Service", "qualifier": "TRK"},
		}})
	})

	out, err := plugin.Execute(context.Background(), "list_projects", []byte(`{"query":"checkout"}`), nil)
	if err != nil {
		t.Fatalf("list_projects: %v", err)
	}
	var result struct {
		Projects  []map[string]string `json:"projects"`
		Truncated bool                `json:"truncated"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if gotQuery != "checkout" || gotQualifiers != "TRK" {
		t.Errorf("query=%q qualifiers=%q, want the search term forwarded to TRK projects", gotQuery, gotQualifiers)
	}
	if len(result.Projects) != 1 || result.Projects[0]["key"] != "checkout-service" {
		t.Fatalf("expected one project suggestion, got %+v", result.Projects)
	}
	if result.Truncated {
		t.Error("a short list is not truncated")
	}
	// A page smaller than the requested size is the last one; without this the
	// walk ran until the 500 project ceiling.
	if requests != 1 {
		t.Errorf("expected paging to stop after a short page, got %d requests", requests)
	}
}

// TestGetProjectSummary checks the single-pass snapshot the scanner relies on:
// quality gate with conditions, measures (analysis_date is what tells a report
// apart from a fresh analysis), and issue counts derived from facets.
func TestGetProjectSummary(t *testing.T) {
	plugin := fakeSonar(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/qualitygates/project_status":
			writeJSON(t, w, map[string]any{"projectStatus": map[string]any{
				"status": "ERROR",
				"conditions": []map[string]any{
					{"status": "OK", "metricKey": "bugs", "comparator": "GT", "errorThreshold": "0", "actualValue": "0"},
					{"status": "ERROR", "metricKey": "coverage", "comparator": "LT", "errorThreshold": "80", "actualValue": "62.5"},
				},
			}})
		case "/api/measures/component":
			writeJSON(t, w, map[string]any{"component": map[string]any{
				"key": "proj",
				"measures": []map[string]any{
					{"metric": "bugs", "value": "0"},
					{"metric": "coverage", "value": "62.5"},
					{"metric": "sq_debt", "value": "12min"},
					{"metric": "analysis_date", "value": "2026-02-05T11:22:33+0000"},
				},
			}})
		case "/api/issues/search":
			writeJSON(t, w, map[string]any{
				"total": 7,
				"facets": []issueFacet{
					{Property: "types", Values: []issueFacetValue{{Value: "BUG", Count: 1}, {Value: "CODE_SMELL", Count: 6}}},
					{Property: "severities", Values: []issueFacetValue{{Value: "MAJOR", Count: 4}, {Value: "BLOCKER", Count: 3}}},
				},
			})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})

	out, err := plugin.Execute(context.Background(), "get_project_summary", []byte(`{"project_key":"proj"}`), nil)
	if err != nil {
		t.Fatalf("get_project_summary: %v", err)
	}
	var summary ProjectSummary
	if err := json.Unmarshal(out, &summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if summary.QualityGate == nil || summary.QualityGate.Status != "ERROR" || len(summary.QualityGate.Conditions) != 2 {
		t.Fatalf("quality gate not parsed: %+v", summary.QualityGate)
	}
	if summary.QualityGate.Conditions[1].MetricKey != "coverage" || summary.QualityGate.Conditions[1].ActualValue != "62.5" {
		t.Errorf("gate conditions lost their fields: %+v", summary.QualityGate.Conditions)
	}
	if summary.AnalysisDate != "2026-02-05T11:22:33+0000" {
		t.Errorf("analysis_date must be lifted out of the measures, got %q", summary.AnalysisDate)
	}
	if summary.Measures == nil || summary.sonarValue("sq_debt") != "12min" {
		t.Errorf("measures not parsed: %+v", summary.Measures)
	}
	for _, m := range summary.Measures.Metrics {
		if m.Metric == "analysis_date" {
			t.Error("analysis_date belongs to the snapshot, not to the metric list")
		}
	}
	if summary.IssueSummary == nil {
		t.Fatal("issue summary missing")
	}
	if summary.IssueSummary.Bugs != 1 || summary.IssueSummary.CodeSmells != 6 || summary.IssueSummary.Total != 7 {
		t.Errorf("facet counts not mapped: %+v", summary.IssueSummary)
	}
	if summary.IssueSummary.BySeverity["major"] != 4 {
		t.Errorf("severity facets must be lower-cased for the report contract: %+v", summary.IssueSummary.BySeverity)
	}
	if len(summary.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", summary.Warnings)
	}
}

// sonarValue is a test helper mirroring the scanner's measure lookup.
func (s *ProjectSummary) sonarValue(metric string) string {
	if s == nil || s.Measures == nil {
		return ""
	}
	for _, m := range s.Measures.Metrics {
		if m.Metric == metric {
			return m.Value
		}
	}
	return ""
}

func TestProjectSummaryDegradesToWarnings(t *testing.T) {
	plugin := fakeSonar(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/qualitygates/project_status":
			writeJSON(t, w, map[string]any{"projectStatus": map[string]any{"status": "OK"}})
		default:
			// Measures and issues are unavailable on this instance.
			w.WriteHeader(http.StatusNotFound)
		}
	})

	out, err := plugin.Execute(context.Background(), "get_project_summary", []byte(`{"project_key":"proj"}`), nil)
	if err != nil {
		t.Fatalf("a partial snapshot must not fail the run: %v", err)
	}
	var summary ProjectSummary
	if err := json.Unmarshal(out, &summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if summary.QualityGate == nil || summary.QualityGate.Status != "OK" {
		t.Error("the quality gate is still a valid part of the report")
	}
	if len(summary.Warnings) != 2 {
		t.Errorf("each unavailable endpoint should leave one warning, got %v", summary.Warnings)
	}
}

func TestTransitionIssue(t *testing.T) {
	var gotIssue, gotTransition string
	plugin := fakeSonar(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/issues/do_transition" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		gotIssue, gotTransition = r.Form.Get("issue"), r.Form.Get("transition")
		writeJSON(t, w, map[string]any{"issue": map[string]any{"key": "AKK4", "status": "FALSE_POSITIVE"}})
	})

	out, err := plugin.Execute(context.Background(), "transition_issue", []byte(`{"issue_key":"AKK4","transition":"falsepositive"}`), nil)
	if err != nil {
		t.Fatalf("transition_issue: %v", err)
	}
	if gotIssue != "AKK4" || gotTransition != "falsepositive" {
		t.Errorf("transition not forwarded: issue=%q transition=%q", gotIssue, gotTransition)
	}
	var result map[string]any
	if err := json.Unmarshal(out, &result); err != nil || result["status"] != "FALSE_POSITIVE" {
		t.Errorf("expected the new issue status back, got %s", out)
	}

	// Anything outside the whitelist never reaches the API.
	if _, err := plugin.Execute(context.Background(), "transition_issue", []byte(`{"issue_key":"AKK4","transition":"delete"}`), nil); err == nil {
		t.Error("unlisted transitions must be rejected locally")
	}
	if _, err := plugin.Execute(context.Background(), "transition_issue", []byte(`{"transition":"falsepositive"}`), nil); err == nil {
		t.Error("issue_key is required")
	}
}

func TestAPIErrorsNameTheirCause(t *testing.T) {
	notFound := fakeSonar(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	if _, err := notFound.Execute(context.Background(), "get_quality_gate", []byte(`{"project_key":"proj"}`), nil); err == nil {
		t.Fatal("a 404 project must be an error")
	} else if !strings.Contains(err.Error(), "project_key") {
		t.Errorf("404 must name the likely cause (project key), got %q", err.Error())
	}

	unauthorized := fakeSonar(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	if _, err := unauthorized.Execute(context.Background(), "get_quality_gate", []byte(`{"project_key":"proj"}`), nil); err == nil {
		t.Fatal("a rejected token must be an error")
	} else if !strings.Contains(err.Error(), "token") {
		t.Errorf("401 must name the likely cause (token), got %q", err.Error())
	}
}
