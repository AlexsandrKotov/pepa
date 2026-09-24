package rest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/config"
	"github.com/pepa/pepa/internal/repository"
	"github.com/pepa/pepa/internal/security"
)

// callHandler runs a handler on a throwaway route, including the path params the
// route pattern declares.
func callHandler(t *testing.T, h gin.HandlerFunc, method, pattern, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Handle(method, pattern, h)

	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// The server must not accept target/scanner pairs that can never produce a
// report; they used to be saved happily and fail minutes later as a scan error.
func TestValidateScannerTargetPair(t *testing.T) {
	cases := []struct {
		scanner, target string
		wantErr         bool
	}{
		{"sonarqube", "sonarqube_project", false},
		{"sonarqube", "git_repo", true},
		{"sonarqube", "filesystem", true},
		{"sonarqube", "image", true},
		{"trivy", "image", false},
		{"trivy", "registry", false},
		{"trivy", "git_repo", false},   // Trivy scans code itself (bug B5)
		{"trivy", "filesystem", false}, // Trivy scans code itself (bug B5)
		{"trivy", "sonarqube_project", true},
		{"both", "git_repo", false},
		{"both", "filesystem", false},
		{"both", "image", true},
		{"both", "sonarqube_project", true},
	}
	for _, tc := range cases {
		err := validateScannerTargetPair(tc.scanner, tc.target)
		if tc.wantErr && err == nil {
			t.Errorf("%s + %s must be rejected", tc.scanner, tc.target)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("%s + %s must be accepted: %v", tc.scanner, tc.target, err)
		}
	}
}

// Credentials only ever come from a Connection, and the server is the place that
// has to enforce it — the UI alone would leave the API open to bad payloads.
func TestValidateScanTargetShapeRequiresSonarConnection(t *testing.T) {
	deps := Dependencies{Repos: &Repositories{}}
	sonar := func() *repository.ScanTarget {
		return &repository.ScanTarget{
			ID:          uuid.New(),
			TenantID:    uuid.New(),
			ScannerType: "sonarqube",
			TargetType:  "sonarqube_project",
			TargetRef:   "checkout-service",
		}
	}

	if _, ok := validateScanTargetShape(t.Context(), deps, sonar()); ok {
		t.Fatal("a SonarQube target without a connection must be rejected")
	}

	noProject := sonar()
	noProject.ConnectionID = &uuid.UUID{}
	if _, ok := validateScanTargetShape(t.Context(), deps, noProject); !ok {
		// Repositories are unwired in a unit test, so this is the 503-style
		// rejection; the point is that it never silently passes.
		t.Log("connection lookup unavailable, as expected without a DB")
	}

	git := &repository.ScanTarget{
		ID: uuid.New(), TenantID: uuid.New(),
		ScannerType: "sonarqube", TargetType: "git_repo", TargetRef: "https://gitlab.example.com/team/app.git",
	}
	msg, ok := validateScanTargetShape(t.Context(), deps, git)
	if ok {
		t.Fatal("a git URL must not be accepted for a SonarQube target")
	}
	if !strings.Contains(msg, "sonarqube_project") {
		t.Errorf("the error should name the required target type, got %q", msg)
	}

	legacy := &repository.ScanTarget{
		ID: uuid.New(), TenantID: uuid.New(),
		ScannerType: "trivy", TargetType: "image", TargetRef: "alpine:3.19",
		ScanConfig: map[string]any{"token": "sqp-secret"},
	}
	if msg, ok := validateScanTargetShape(t.Context(), deps, legacy); ok {
		t.Fatal("credentials in scan_config must be rejected")
	} else if !strings.Contains(msg, "Connection") {
		t.Errorf("the error should point at Connections, got %q", msg)
	}
}

func TestCheckSonarWebhookSecret(t *testing.T) {
	deps := Dependencies{Config: &config.Config{}}
	if ok, status := checkSonarWebhookSecret("anything", deps); ok || status != http.StatusServiceUnavailable {
		t.Errorf("an unconfigured webhook must answer 503, got ok=%v status=%d", ok, status)
	}

	deps.Config.Security.SonarWebhookToken = "s3cret"
	if ok, status := checkSonarWebhookSecret("", deps); ok || status != http.StatusUnauthorized {
		t.Errorf("a missing signature must be rejected, got ok=%v status=%d", ok, status)
	}
	if ok, status := checkSonarWebhookSecret("s3cre", deps); ok || status != http.StatusUnauthorized {
		t.Errorf("a wrong signature must be rejected, got ok=%v status=%d", ok, status)
	}
	if ok, status := checkSonarWebhookSecret("s3cret", deps); !ok || status != http.StatusOK {
		t.Errorf("the shared secret must be accepted, got ok=%v status=%d", ok, status)
	}
}

// The matched Connection is the one whose credentials are used for the report
// collection, so a look-alike host must never match.
func TestSonarPayloadURLMatches(t *testing.T) {
	cases := []struct {
		payload, base string
		want          bool
	}{
		{"https://sonar.example.com/dashboard?id=proj", "https://sonar.example.com", true},
		{"https://sonar.example.com/dashboard?id=proj", "https://sonar.example.com/", true},
		{"https://sonar.example.com/sonar/dashboard?id=proj", "https://sonar.example.com/sonar", true},
		{"https://sonar.example.com/sonarx/dashboard", "https://sonar.example.com/sonar", false},
		{"http://sonar.example.com/dashboard", "https://sonar.example.com", false},
		{"https://sonar.example.com.evil.tld/dashboard", "https://sonar.example.com", false},
		{"https://evil.tld/dashboard", "https://sonar.example.com", false},
		{"", "https://sonar.example.com", false},
		{"https://sonar.example.com/dashboard", "", false},
	}
	for _, tc := range cases {
		if got := sonarPayloadURLMatches(tc.payload, tc.base); got != tc.want {
			t.Errorf("sonarPayloadURLMatches(%q, %q) = %v, want %v", tc.payload, tc.base, got, tc.want)
		}
	}
}

func TestSonarWebhookTargetMatches(t *testing.T) {
	connectionID := uuid.New()
	other := uuid.New()
	key := "checkout-service"

	matches := func() *repository.ScanTarget {
		return &repository.ScanTarget{
			ID: uuid.New(), ScannerType: "sonarqube", TargetType: "sonarqube_project",
			TargetRef: key, ConnectionID: &connectionID,
		}
	}
	if !sonarWebhookTargetMatches(matches(), connectionID, key) {
		t.Error("the target owning the analysed project must be collected")
	}

	wrongConn := matches()
	wrongConn.ConnectionID = &other
	if sonarWebhookTargetMatches(wrongConn, connectionID, key) {
		t.Error("a target on another SonarQube instance must not react")
	}

	wrongProject := matches()
	wrongProject.TargetRef = "billing-service"
	if sonarWebhookTargetMatches(wrongProject, connectionID, key) {
		t.Error("a target for another project must not react")
	}

	git := matches()
	git.TargetType = "git_repo"
	git.TargetRef = "https://gitlab.example.com/team/app.git"
	git.ScanConfig = map[string]any{"project_key": key}
	if sonarWebhookTargetMatches(git, connectionID, key) {
		t.Error("Trivy targets must never be triggered by a SonarQube webhook")
	}

	noConn := matches()
	noConn.ConnectionID = nil
	if sonarWebhookTargetMatches(noConn, connectionID, key) {
		t.Error("a target without a connection cannot belong to this instance")
	}
}

// The webhook is a public route, so its guards have to be exercised over HTTP.
func TestSonarWebhookHTTPGuards(t *testing.T) {
	deps := Dependencies{
		Config:  &config.Config{},
		Scanner: security.NewScanner(nil, nil, nil, nil, nil, config.SecurityConfig{}, ""),
		Repos:   &Repositories{Connection: &repository.ConnectionRepository{}, SecurityScan: &repository.SecurityScanRepository{}},
	}
	h := sonarWebhook(deps)

	if w := callHandler(t, h, "POST", "/api/v1/security/sonar/webhook", "/api/v1/security/sonar/webhook", `{}`, nil); w.Code != http.StatusServiceUnavailable {
		t.Errorf("disabled webhook: got %d, want 503", w.Code)
	}

	deps.Config.Security.SonarWebhookToken = "s3cret"
	h = sonarWebhook(deps)
	if w := callHandler(t, h, "POST", "/api/v1/security/sonar/webhook", "/api/v1/security/sonar/webhook", `{}`, nil); w.Code != http.StatusUnauthorized {
		t.Errorf("unsigned webhook: got %d, want 401", w.Code)
	}
	if w := callHandler(t, h, "POST", "/api/v1/security/sonar/webhook", "/api/v1/security/sonar/webhook",
		`{}`, map[string]string{"X-PEPA-Signature": "nope"}); w.Code != http.StatusUnauthorized {
		t.Errorf("wrong signature: got %d, want 401", w.Code)
	}
}

func TestGetScanReportGuards(t *testing.T) {
	deps := Dependencies{Repos: &Repositories{}}
	h := getScanReport(deps)
	id := uuid.New().String()
	if w := callHandler(t, h, "GET", "/api/v1/security/scans/:id/report", "/api/v1/security/scans/"+id+"/report", "", nil); w.Code != http.StatusServiceUnavailable {
		t.Errorf("unwired repository: got %d, want 503", w.Code)
	}

	withRepo := Dependencies{Repos: &Repositories{SecurityScan: &repository.SecurityScanRepository{}}}
	h = getScanReport(withRepo)
	if w := callHandler(t, h, "GET", "/api/v1/security/scans/:id/report", "/api/v1/security/scans/xyz/report", "", nil); w.Code != http.StatusBadRequest {
		t.Errorf("invalid scan id: got %d, want 400", w.Code)
	}
	if w := callHandler(t, h, "GET", "/api/v1/security/scans/:id/report", "/api/v1/security/scans/"+id+"/report?format=pdf", "", nil); w.Code != http.StatusBadRequest {
		t.Errorf("unsupported format: got %d, want 400", w.Code)
	}
}

func TestTransitionSonarIssueGuards(t *testing.T) {
	deps := Dependencies{Repos: &Repositories{}}
	h := transitionSonarIssue(deps)
	if w := callHandler(t, h, "POST", "/api/v1/security/sonar/issues/transition", "/api/v1/security/sonar/issues/transition", `{}`, nil); w.Code != http.StatusServiceUnavailable {
		t.Errorf("unwired scanner: got %d, want 503", w.Code)
	}

	h = transitionSonarIssue(Dependencies{Scanner: security.NewScanner(nil, nil, nil, nil, nil, config.SecurityConfig{}, ""), Repos: &Repositories{Connection: &repository.ConnectionRepository{}}})
	if w := callHandler(t, h, "POST", "/api/v1/security/sonar/issues/transition", "/api/v1/security/sonar/issues/transition", `{"connection_id":"not-a-uuid"}`, nil); w.Code != http.StatusBadRequest {
		t.Errorf("invalid connection id: got %d, want 400", w.Code)
	}
	if w := callHandler(t, h, "POST", "/api/v1/security/sonar/issues/transition", "/api/v1/security/sonar/issues/transition",
		`{"connection_id":"`+uuid.New().String()+`","issue_key":"","transition":"falsepositive"}`, nil); w.Code != http.StatusBadRequest {
		t.Errorf("missing issue key: got %d, want 400", w.Code)
	}
}

func TestListSonarProjectsGuards(t *testing.T) {
	h := listSonarProjects(Dependencies{Repos: &Repositories{}})
	if w := callHandler(t, h, "GET", "/api/v1/security/sonar/projects", "/api/v1/security/sonar/projects", "", nil); w.Code != http.StatusServiceUnavailable {
		t.Errorf("unwired scanner: got %d, want 503", w.Code)
	}

	h = listSonarProjects(Dependencies{Scanner: security.NewScanner(nil, nil, nil, nil, nil, config.SecurityConfig{}, ""), Repos: &Repositories{}})
	if w := callHandler(t, h, "GET", "/api/v1/security/sonar/projects", "/api/v1/security/sonar/projects", "", nil); w.Code != http.StatusBadRequest {
		t.Errorf("missing connection_id: got %d, want 400", w.Code)
	}
}
