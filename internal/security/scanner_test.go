package security

import (
	"testing"
)

func TestValidateScanConfig_TrivyValid(t *testing.T) {
	cfg := map[string]any{
		"scan_type":      "filesystem",
		"severity":       "HIGH,CRITICAL",
		"ignore_unfixed": true,
	}
	if err := ValidateScanConfig("trivy", cfg); err != nil {
		t.Errorf("expected no error for valid trivy config, got: %v", err)
	}
}

func TestValidateScanConfig_TrivyRejectsCredential(t *testing.T) {
	cfg := map[string]any{
		"url": "https://trivy.example.com",
	}
	err := ValidateScanConfig("trivy", cfg)
	if err == nil {
		t.Fatal("expected error for credential key 'url'")
	}
	if got := err.Error(); got == "" || !containsStr(got, "url") {
		t.Errorf("expected error to mention 'url', got: %v", err)
	}
}

func TestValidateScanConfig_SonarqubeRejectsToken(t *testing.T) {
	cfg := map[string]any{
		"token": "secret-token",
	}
	err := ValidateScanConfig("sonarqube", cfg)
	if err == nil {
		t.Fatal("expected error for credential key 'token'")
	}
}

func TestValidateScanConfig_SonarqubeValid(t *testing.T) {
	cfg := map[string]any{
		"project_key":       "my-project",
		"branch":            "main",
		"stale_after_hours": 48,
	}
	if err := ValidateScanConfig("sonarqube", cfg); err != nil {
		t.Errorf("expected no error for valid sonarqube config, got: %v", err)
	}
}

func TestValidateScanConfig_UnknownScanner(t *testing.T) {
	cfg := map[string]any{
		"anything": "goes",
	}
	// Unknown scanner types are not validated
	if err := ValidateScanConfig("unknown_scanner", cfg); err != nil {
		t.Errorf("expected nil for unknown scanner, got: %v", err)
	}
}

func TestValidateScanConfig_BothValid(t *testing.T) {
	cfg := map[string]any{
		"scan_type":   "filesystem",
		"severity":    "HIGH",
		"project_key": "proj",
	}
	if err := ValidateScanConfig("both", cfg); err != nil {
		t.Errorf("expected no error for valid 'both' config, got: %v", err)
	}
}

func TestValidSeverities(t *testing.T) {
	expected := []string{"UNKNOWN", "LOW", "MEDIUM", "HIGH", "CRITICAL"}
	for _, sev := range expected {
		if !validSeverities[sev] {
			t.Errorf("expected severity %q to be valid", sev)
		}
	}
	if validSeverities["INVALID"] {
		t.Error("INVALID should not be a valid severity")
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
