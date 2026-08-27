package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
)

// mockProvider is a test implementation of Provider
type mockProvider struct {
	name string
}

func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) ResolveSchema(_ context.Context, _ json.RawMessage) (*ParameterSchema, error) {
	return &ParameterSchema{
		Type: "object",
		Properties: map[string]PropertyDef{
			"env": {Type: "string", Description: "Environment", Default: "staging"},
		},
	}, nil
}
func (m *mockProvider) Trigger(_ context.Context, _ json.RawMessage, params map[string]any) (*TriggerResult, error) {
	return &TriggerResult{
		ExternalRunID: "run-123",
		ExternalURL:   "https://ci.example.com/run/123",
		Status:        "pending",
	}, nil
}
func (m *mockProvider) Status(_ context.Context, _ json.RawMessage, runID string) (*RunStatus, error) {
	return &RunStatus{
		ExternalRunID: runID,
		Status:        "running",
	}, nil
}
func (m *mockProvider) Jobs(_ context.Context, _ json.RawMessage, runID string) ([]JobInfo, error) {
	return []JobInfo{
		{ExternalJobID: "job-1", Name: "build", Status: "success"},
		{ExternalJobID: "job-2", Name: "test", Status: "running"},
	}, nil
}
func (m *mockProvider) Logs(_ context.Context, _ json.RawMessage, runID string, jobID string) (string, error) {
	return fmt.Sprintf("logs for run=%s job=%s", runID, jobID), nil
}
func (m *mockProvider) Cancel(_ context.Context, _ json.RawMessage, runID string) error {
	return nil
}

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry() returned nil")
	}
	types := r.List()
	if len(types) != 0 {
		t.Errorf("expected empty registry, got %d entries", len(types))
	}
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	gitlab := &mockProvider{name: "gitlab_ci"}
	ansible := &mockProvider{name: "ansible"}

	r.Register("gitlab_ci", gitlab)
	r.Register("ansible", ansible)

	got, err := r.Get("gitlab_ci")
	if err != nil {
		t.Fatalf("Get(gitlab_ci) error: %v", err)
	}
	if got.Name() != "gitlab_ci" {
		t.Errorf("expected name gitlab_ci, got %s", got.Name())
	}

	got, err = r.Get("ansible")
	if err != nil {
		t.Fatalf("Get(ansible) error: %v", err)
	}
	if got.Name() != "ansible" {
		t.Errorf("expected name ansible, got %s", got.Name())
	}
}

func TestRegistry_GetUnknown(t *testing.T) {
	r := NewRegistry()
	_, err := r.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
	expected := "no pipeline provider registered for type: nonexistent"
	if err.Error() != expected {
		t.Errorf("error = %q, want %q", err.Error(), expected)
	}
}

func TestRegistry_List(t *testing.T) {
	r := NewRegistry()
	r.Register("gitlab_ci", &mockProvider{name: "gitlab_ci"})
	r.Register("ansible", &mockProvider{name: "ansible"})
	r.Register("terraform", &mockProvider{name: "terraform"})

	types := r.List()
	if len(types) != 3 {
		t.Fatalf("expected 3 types, got %d", len(types))
	}

	found := make(map[string]bool)
	for _, typ := range types {
		found[typ] = true
	}
	for _, expected := range []string{"gitlab_ci", "ansible", "terraform"} {
		if !found[expected] {
			t.Errorf("missing type %q in List()", expected)
		}
	}
}

func TestRegistry_OverwriteProvider(t *testing.T) {
	r := NewRegistry()
	r.Register("gitlab_ci", &mockProvider{name: "old"})
	r.Register("gitlab_ci", &mockProvider{name: "new"})

	p, err := r.Get("gitlab_ci")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if p.Name() != "new" {
		t.Errorf("expected overwritten provider name 'new', got %s", p.Name())
	}
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	r := NewRegistry()
	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			name := fmt.Sprintf("provider_%d", idx)
			r.Register(name, &mockProvider{name: name})
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			name := fmt.Sprintf("provider_%d", idx)
			r.Get(name) // may or may not find it
			r.List()
		}(i)
	}

	wg.Wait()

	types := r.List()
	if len(types) != 20 {
		t.Errorf("expected 20 providers, got %d", len(types))
	}
}

func TestMockProvider_ResolveSchema(t *testing.T) {
	p := &mockProvider{name: "test"}
	schema, err := p.ResolveSchema(context.Background(), nil)
	if err != nil {
		t.Fatalf("ResolveSchema error: %v", err)
	}
	if schema.Type != "object" {
		t.Errorf("schema type = %s, want object", schema.Type)
	}
	if _, ok := schema.Properties["env"]; !ok {
		t.Error("expected 'env' property in schema")
	}
}

func TestMockProvider_Trigger(t *testing.T) {
	p := &mockProvider{name: "test"}
	result, err := p.Trigger(context.Background(), nil, map[string]any{"env": "prod"})
	if err != nil {
		t.Fatalf("Trigger error: %v", err)
	}
	if result.ExternalRunID != "run-123" {
		t.Errorf("run ID = %s, want run-123", result.ExternalRunID)
	}
	if result.Status != "pending" {
		t.Errorf("status = %s, want pending", result.Status)
	}
}

func TestMockProvider_Status(t *testing.T) {
	p := &mockProvider{name: "test"}
	status, err := p.Status(context.Background(), nil, "run-456")
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if status.ExternalRunID != "run-456" {
		t.Errorf("run ID = %s, want run-456", status.ExternalRunID)
	}
	if status.Status != "running" {
		t.Errorf("status = %s, want running", status.Status)
	}
}

func TestMockProvider_Jobs(t *testing.T) {
	p := &mockProvider{name: "test"}
	jobs, err := p.Jobs(context.Background(), nil, "run-789")
	if err != nil {
		t.Fatalf("Jobs error: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobs))
	}
	if jobs[0].Name != "build" {
		t.Errorf("job[0].Name = %s, want build", jobs[0].Name)
	}
}

func TestMockProvider_Logs(t *testing.T) {
	p := &mockProvider{name: "test"}
	logs, err := p.Logs(context.Background(), nil, "run-1", "job-1")
	if err != nil {
		t.Fatalf("Logs error: %v", err)
	}
	expected := "logs for run=run-1 job=job-1"
	if logs != expected {
		t.Errorf("logs = %q, want %q", logs, expected)
	}
}

func TestMockProvider_Cancel(t *testing.T) {
	p := &mockProvider{name: "test"}
	err := p.Cancel(context.Background(), nil, "run-1")
	if err != nil {
		t.Fatalf("Cancel error: %v", err)
	}
}

func TestParameterSchema_JSON(t *testing.T) {
	s := ParameterSchema{
		Type: "object",
		Properties: map[string]PropertyDef{
			"env": {
				Type:        "enum",
				Description: "Target environment",
				Enum:        []string{"dev", "staging", "prod"},
				Default:     "staging",
			},
		},
		Required: []string{"env"},
	}

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded ParameterSchema
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Type != "object" {
		t.Errorf("type = %s, want object", decoded.Type)
	}
	if len(decoded.Required) != 1 || decoded.Required[0] != "env" {
		t.Errorf("required = %v, want [env]", decoded.Required)
	}
	prop := decoded.Properties["env"]
	if len(prop.Enum) != 3 {
		t.Errorf("enum length = %d, want 3", len(prop.Enum))
	}
}

func TestTriggerResult_JSON(t *testing.T) {
	tr := TriggerResult{
		ExternalRunID: "run-1",
		ExternalURL:   "https://ci.example.com/1",
		Status:        "success",
	}
	data, err := json.Marshal(tr)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded TriggerResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.ExternalRunID != "run-1" {
		t.Errorf("ExternalRunID = %s, want run-1", decoded.ExternalRunID)
	}
}

func TestRunStatus_JSON(t *testing.T) {
	dur := 5000
	rs := RunStatus{
		ExternalRunID: "run-1",
		Status:        "success",
		DurationMs:    &dur,
		Logs:          "build ok",
	}
	data, err := json.Marshal(rs)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded RunStatus
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if *decoded.DurationMs != 5000 {
		t.Errorf("DurationMs = %d, want 5000", *decoded.DurationMs)
	}
}
