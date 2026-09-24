package security

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/config"
	"github.com/pepa/pepa/internal/repository"
)

func sonarTarget(config map[string]any) *repository.ScanTarget {
	return &repository.ScanTarget{
		ID:          uuid.New(),
		TenantID:    uuid.New(),
		Name:        "checkout quality",
		ScannerType: "sonarqube",
		TargetType:  "sonarqube_project",
		TargetRef:   "checkout-service",
		ScanConfig:  config,
	}
}

// The two products name severities differently; the report keeps the Trivy scale
// so severity bars, filters and exports stay shared between scanners.
func TestSonarSeverityToTrivy(t *testing.T) {
	cases := map[string]string{
		"BLOCKER":  "critical",
		"CRITICAL": "high",
		"MAJOR":    "medium",
		"MINOR":    "low",
		"INFO":     "unknown",
		"info":     "unknown",
		" TRIVIAL": "unknown",
		"":         "unknown",
	}
	for severity, want := range cases {
		if got := sonarSeverityToTrivy(severity); got != want {
			t.Errorf("sonarSeverityToTrivy(%q) = %q, want %q", severity, got, want)
		}
	}
}

// target_ref is a git URL or a path for every non-SonarQube target, so it must
// never be mistaken for a project key (bug B2).
func TestSonarProjectKeyResolution(t *testing.T) {
	if got := SonarProjectKey(sonarTarget(map[string]any{"project_key": "explicit-key"})); got != "explicit-key" {
		t.Errorf("scan_config.project_key must win over target_ref, got %q", got)
	}
	if got := SonarProjectKey(sonarTarget(nil)); got != "checkout-service" {
		t.Errorf("a sonarqube_project target_ref is the project key, got %q", got)
	}

	git := sonarTarget(nil)
	git.TargetType = "git_repo"
	git.TargetRef = "https://gitlab.example.com/team/app.git"
	if got := SonarProjectKey(git); got != "" {
		t.Errorf("a git URL must not be used as a project key, got %q", got)
	}

	// A key configured on a non-project target still works, which is how a
	// leftover target keeps scanning instead of failing every run.
	if got := SonarProjectKey(&repository.ScanTarget{TargetType: "git_repo", TargetRef: "https://x/y.git", ScanConfig: map[string]any{"project_key": " legacy "}}); got != "legacy" {
		t.Errorf("project_key must be trimmed, got %q", got)
	}
}

func TestValidateSonarProjectKey(t *testing.T) {
	if err := ValidateSonarProjectKey(sonarTarget(nil)); err != nil {
		t.Errorf("a valid sonarqube target must pass: %v", err)
	}
	if err := ValidateSonarProjectKey(&repository.ScanTarget{TargetType: "git_repo", TargetRef: "https://x/y.git"}); err == nil {
		t.Error("a target that can never resolve a project key must be rejected while saving")
	}
	conflict := sonarTarget(map[string]any{"project_key": "other-key"})
	if err := ValidateSonarProjectKey(conflict); err == nil {
		t.Error("scan_config.project_key conflicting with target_ref must be rejected")
	} else if !strings.Contains(err.Error(), "conflicts") {
		t.Errorf("expected a conflict message, got %v", err)
	}
	// Identical values in both places are the shape the UI writes.
	same := sonarTarget(map[string]any{"project_key": "checkout-service"})
	if err := ValidateSonarProjectKey(same); err != nil {
		t.Errorf("matching key and ref must pass: %v", err)
	}
}

// Credentials used to live in scan_config, where they are stored as plain JSONB.
func TestValidateScanConfigRejectsCredentials(t *testing.T) {
	for _, key := range []string{"url", "token"} {
		err := ValidateScanConfig("sonarqube", map[string]any{key: "value"})
		if err == nil {
			t.Fatalf("scan_config.%s must be rejected for sonarqube targets", key)
		}
		if !strings.Contains(err.Error(), "Connection") {
			t.Errorf("the error must point at Connections, got %v", err)
		}
	}
	if err := ValidateScanConfig("trivy", map[string]any{"token": "value"}); err == nil {
		t.Error("trivy targets must not accept credentials either")
	}
	if err := ValidateScanConfig("sonarqube", map[string]any{
		"project_key": "k", "branch": "main", "severity": "MAJOR",
		"stale_after_hours": 24, "source_ci_url": "https://ci/example",
	}); err != nil {
		t.Errorf("supported keys must pass: %v", err)
	}
	if err := ValidateScanConfig("sonarqube", map[string]any{"nope": 1}); err == nil {
		t.Error("unknown keys must be rejected")
	}
	// A "both" target needs the union of the two scanners' keys.
	if err := ValidateScanConfig("both", map[string]any{"scan_type": "fs", "project_key": "k"}); err != nil {
		t.Errorf("both must accept each scanner's keys: %v", err)
	}
}

func TestSonarConfigInt(t *testing.T) {
	cfg := map[string]any{"as_int": 12, "as_float": float64(7), "as_string": " 24 ", "bad": "x"}
	for key, want := range map[string]int{"as_int": 12, "as_float": 7, "as_string": 24, "bad": 0, "missing": 0} {
		if got := sonarConfigInt(cfg, key); got != want {
			t.Errorf("sonarConfigInt(%q) = %d, want %d", key, got, want)
		}
	}
	if got := sonarConfigInt(nil, "as_int"); got != 0 {
		t.Errorf("nil config must yield 0, got %d", got)
	}
}

func TestSonarReportURLs(t *testing.T) {
	dash := sonarDashboardURL("https://sonar.example.com/", "grp/proj")
	if !strings.HasPrefix(dash, "https://sonar.example.com/dashboard?id=grp%2Fproj") {
		t.Errorf("unexpected dashboard URL: %s", dash)
	}
	issue := sonarIssueURL("https://sonar.example.com", "proj", "AKK 1")
	if !strings.Contains(issue, "id=proj") || !strings.Contains(issue, "open=AKK+1") {
		t.Errorf("unexpected issue URL: %s", issue)
	}
}

func sonarFixtureSnapshot() sonarSnapshot {
	return sonarSnapshot{
		ProjectKey:   "checkout-service",
		Branch:       "main",
		AnalysisDate: "2026-02-05T11:22:33+0000",
		QualityGate: &sonarQualityGate{
			Status: "ERROR",
			Conditions: []sonarGateCondition{
				{Status: "OK", MetricKey: "bugs", Comparator: "GT", ErrorThreshold: "0", ActualValue: "0"},
				{Status: "ERROR", MetricKey: "coverage", Comparator: "LT", ErrorThreshold: "80", ActualValue: "62.5"},
			},
		},
		Measures: &sonarMeasures{Metrics: []sonarMetric{
			{Metric: "bugs", Value: "9"},
			{Metric: "coverage", Value: "62.5"},
			{Metric: "duplicated_lines_density", Value: "3.1"},
			{Metric: "sq_debt", Value: "12min"},
		}},
		Warnings: []string{"issues partially unavailable"},
	}
}

// buildSonarReport is the contract that lets every downstream consumer — finding
// list, ignores, exports, dashboards — stay scanner-agnostic.
func TestBuildSonarReport(t *testing.T) {
	page := sonarIssuePage{
		Total:     10,
		Fetched:   5,
		Truncated: true,
		Issues: []sonarIssue{
			{Key: "AKK1", Rule: "java:S2259", Severity: "BLOCKER", Status: "OPEN", Message: "NPE", ComponentPath: "src/main/A.java", Line: 42, Type: "BUG", Effort: "30min", Debt: "30min"},
			{Key: "AKK2", Rule: "java:S1135", Severity: "MAJOR", Status: "OPEN", Message: "TODO tag", ComponentPath: "src/main/A.java", Line: 7, Type: "CODE_SMELL"},
			{Key: "AKK3", Rule: "go:S100", Severity: "CRITICAL", Status: "CONFIRMED", Message: "naming", ComponentPath: "internal/b.go", Line: 3, Type: "CODE_SMELL"},
			{Key: "AKK4", Rule: "py:S1481", Severity: "MINOR", Status: "OPEN", Message: "unused", Component: "proj:legacy/c.py", Type: "CODE_SMELL"},
			{Key: "AKK5", Rule: "js:S3457", Severity: "INFO", Status: "OPEN", Message: "format", ComponentPath: "web/index.js", Type: "BUG"},
		},
	}
	// One issue by key, one whole rule suppressed.
	ignores := map[string]bool{"AKK2": true, "rule:go:S100": true}

	summary, full := buildSonarReport(sonarFixtureSnapshot(), page, "https://sonar.example.com", "checkout-service", ignores)

	// AKK1 (critical), AKK4 (low), AKK5 (unknown) survive; the two ignored ones
	// must not be counted at all.
	for key, want := range map[string]any{
		"critical": 1, "high": 0, "medium": 0, "low": 1, "unknown": 1, "total": 3,
	} {
		if summary[key] != want {
			t.Errorf("summary[%q] = %v, want %v", key, summary[key], want)
		}
	}
	for key, want := range map[string]any{
		"quality_gate_status": "ERROR",
		"project_key":         "checkout-service",
		"branch":              "main",
		"last_analysis_at":    "2026-02-05T11:22:33+0000",
		"truncated":           true,
		"issue_total":         10,
		"bugs":                9, // measures win unless the issue summary overrides them
		"technical_debt":      "12min",
	} {
		if summary[key] != want {
			t.Errorf("summary[%q] = %v, want %v", key, summary[key], want)
		}
	}
	if coverage, ok := summary["coverage"].(float64); !ok || coverage != 62.5 {
		t.Errorf("coverage must be a number for the metric tiles, got %T %v", summary["coverage"], summary["coverage"])
	}
	if duplicated, ok := summary["duplicated_lines_density"].(float64); !ok || duplicated != 3.1 {
		t.Errorf("duplicated_lines_density must be a number, got %T %v", summary["duplicated_lines_density"], summary["duplicated_lines_density"])
	}

	results, ok := full["Results"].([]map[string]any)
	if !ok {
		t.Fatalf("Results must be a list, got %T", full["Results"])
	}
	// Grouped per component path and issue type, sorted by path — this is what
	// renders as the collapsible list Trivy users already know.
	if len(results) != 3 {
		t.Fatalf("expected 3 groups, got %d: %+v", len(results), results)
	}
	if results[0]["Target"] != "proj:legacy/c.py" || results[1]["Target"] != "src/main/A.java" || results[2]["Target"] != "web/index.js" {
		t.Errorf("unexpected group order: %v %v %v", results[0]["Target"], results[1]["Target"], results[2]["Target"])
	}
	if results[0]["Class"] != "sonar" {
		t.Errorf("groups must be labelled as sonar findings, got %v", results[0]["Class"])
	}
	if results[2]["Type"] != "BUG" {
		t.Errorf("group type must come from the issue type, got %v", results[2]["Type"])
	}

	firstGroup := results[1]["Vulnerabilities"].([]map[string]any)
	if len(firstGroup) != 1 {
		t.Fatalf("the ignored issue must be gone, got %+v", firstGroup)
	}
	finding := firstGroup[0]
	if finding["VulnerabilityID"] != "AKK1" || finding["PkgName"] != "java:S2259" || finding["Title"] != "NPE" {
		t.Errorf("finding not normalised as documented: %+v", finding)
	}
	if finding["Severity"] != "CRITICAL" || finding["SonarSeverity"] != "BLOCKER" {
		t.Errorf("the mapped bucket and the original severity must both be kept: %+v", finding)
	}
	if finding["InstalledVersion"] != "src/main/A.java:42" {
		t.Errorf("location must reuse the Trivy field, got %v", finding["InstalledVersion"])
	}
	if finding["Effort"] != "30min" || finding["TechnicalDebt"] != "30min" || finding["Status"] != "OPEN" {
		t.Errorf("remediation fields must survive normalisation: %+v", finding)
	}
	link, _ := finding["PrimaryURL"].(string)
	if !strings.HasPrefix(link, "https://sonar.example.com/project/issues?id=checkout-service&open=AKK1") {
		t.Errorf("findings must link back to SonarQube, got %q", link)
	}

	// A component without a path falls back to the component key itself.
	noPath := results[0]["Vulnerabilities"].([]map[string]any)[0]
	if noPath["InstalledVersion"] != "proj:legacy/c.py" {
		t.Errorf("expected the component as location, got %v", noPath["InstalledVersion"])
	}

	warnings, ok := full["sonar_warnings"].([]string)
	if !ok || len(warnings) != 1 {
		t.Errorf("partial failures must be reported, not swallowed: %v", full["sonar_warnings"])
	}
	if full["sonar"] == nil {
		t.Error("the raw snapshot is kept for the quality-gate detail table")
	}
}

// An issue summary from the plugin's facets wins over the raw measure.
func TestBuildSonarReportPrefersIssueSummary(t *testing.T) {
	snapshot := sonarFixtureSnapshot()
	snapshot.IssueSummary = &sonarIssueSummary{Total: 4, Bugs: 1, Vulnerabilities: 2, CodeSmells: 1}

	summary, _ := buildSonarReport(snapshot, sonarIssuePage{}, "https://sonar.example.com", "checkout-service", nil)
	if summary["bugs"] != 1 || summary["vulnerabilities"] != 2 || summary["code_smells"] != 1 {
		t.Errorf("facet counts must be used when present: %v %v %v", summary["bugs"], summary["vulnerabilities"], summary["code_smells"])
	}
	if summary["total"] != 0 {
		t.Error("with no findings collected, the severity total is 0")
	}
	if summary["quality_gate_status"] != "ERROR" {
		t.Errorf("the gate verdict comes from the snapshot, got %v", summary["quality_gate_status"])
	}
	if _, exists := summary["sonar_warnings"]; exists {
		t.Error("sonar_warnings is only written when something degraded")
	}
}

// A "both" target must add the shared counters up, not overwrite them.
func TestMergeScanSummariesAddsSharedCounters(t *testing.T) {
	trivy := map[string]any{"critical": 2, "high": 5, "total": 7, "image_ref": "alpine:3.19"}
	sonar := map[string]any{"critical": 1, "high": 3, "medium": 4, "total": 8, "quality_gate_status": "WARN"}

	merged := mergeScanSummaries(trivy, sonar)
	if merged["critical"] != 3 || merged["high"] != 8 || merged["total"] != 15 {
		t.Errorf("shared counters must add up, got %+v", merged)
	}
	if merged["medium"] != 4 {
		t.Errorf("unique keys must be carried over, got %v", merged["medium"])
	}
	if merged["image_ref"] != "alpine:3.19" || merged["quality_gate_status"] != "WARN" {
		t.Errorf("scanner-specific keys must survive: %+v", merged)
	}
}

// mergeMaps keeps the two full reports apart, so a combined run can still render
// each scanner's findings.
func TestMergeMapsKeepsBothReports(t *testing.T) {
	merged := mergeMaps(map[string]any{"Results": []any{}}, map[string]any{"Results": []any{}}, "trivy", "sonar")
	if merged["trivy"] == nil || merged["sonar"] == nil {
		t.Errorf("both reports must be present, got %+v", merged)
	}
}

// The report is stored as JSONB, so everything it carries must encode.
func TestSonarReportIsJSONSafe(t *testing.T) {
	summary, full := buildSonarReport(sonarFixtureSnapshot(), sonarIssuePage{
		Issues: []sonarIssue{{Key: "AKK1", Severity: "MAJOR", Type: "BUG", ComponentPath: "a.go"}},
	}, "https://sonar.example.com", "checkout-service", nil)
	summary["stale_after_hours"] = 24
	summary["connection_id"] = uuid.New().String()

	if _, err := json.Marshal(summary); err != nil {
		t.Fatalf("summary must be serialisable: %v", err)
	}
	if _, err := json.Marshal(full); err != nil {
		t.Fatalf("full report must be serialisable: %v", err)
	}
}

func TestSonarIssueIgnored(t *testing.T) {
	ignores := map[string]bool{"AKK1": true, "rule:java:S1135": true}
	issue := sonarIssue{Key: "AKK1", Rule: "java:S1135"}
	if !sonarIssueIgnored(ignores, issue) {
		t.Error("an ignored issue key must be suppressed")
	}
	if !sonarIssueIgnored(ignores, sonarIssue{Key: "AKK9", Rule: "java:S1135"}) {
		t.Error("a rule ignore must suppress every finding of that rule")
	}
	if sonarIssueIgnored(ignores, sonarIssue{Key: "AKK9", Rule: "java:S2259"}) {
		t.Error("unrelated findings must stay visible")
	}
	if sonarIssueIgnored(nil, issue) {
		t.Error("no ignores means nothing is suppressed")
	}
}

// A collection is a handful of REST calls, not an analysis: it needs its own
// short budget, and a typo in the environment must not disable it.
func TestSonarScanEnvironmentConfiguration(t *testing.T) {
	s := NewScanner(nil, nil, nil, nil, nil, config.SecurityConfig{}, "")
	if s.sonarTimeout != 2*time.Minute {
		t.Errorf("default sonar timeout = %v, want 2m", s.sonarTimeout)
	}
	if s.sonarDefaultStaleHours != 24 {
		t.Errorf("default staleness budget = %d, want 24", s.sonarDefaultStaleHours)
	}

	s = NewScanner(nil, nil, nil, nil, nil, config.SecurityConfig{SonarScanTimeout: "45s", SonarDefaultStaleHours: "6"}, "")
	if s.sonarTimeout != 45*time.Second || s.sonarDefaultStaleHours != 6 {
		t.Errorf("environment values must be honoured: %v / %d", s.sonarTimeout, s.sonarDefaultStaleHours)
	}

	s = NewScanner(nil, nil, nil, nil, nil, config.SecurityConfig{SonarScanTimeout: "nonsense", SonarDefaultStaleHours: "-1"}, "")
	if s.sonarTimeout != 2*time.Minute || s.sonarDefaultStaleHours != 24 {
		t.Errorf("invalid values must fall back to the defaults, got %v / %d", s.sonarTimeout, s.sonarDefaultStaleHours)
	}
}

func TestSanitizeHTTPURL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"https://gitlab.example.com/org/repo/-/pipelines/1234", "https://gitlab.example.com/org/repo/-/pipelines/1234"},
		{"http://ci.internal/job/42", "http://ci.internal/job/42"},
		{"javascript:alert(1)", ""},
		{"data:text/html,<script>alert(1)</script>", ""},
		{"ftp://example.com/file", ""},
		{"", ""},
		{"   ", ""},
		{"not-a-url", ""},
		{"https://", ""},
	}
	for _, tc := range cases {
		got := sanitizeHTTPURL(tc.in)
		if got != tc.want {
			t.Errorf("sanitizeHTTPURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
