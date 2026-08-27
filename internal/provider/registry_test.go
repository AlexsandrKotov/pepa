package provider

import (
	"context"
	"testing"

	pb "github.com/pepa/pepa/internal/plugin/proto"
)

// mockExecutor implements the Executor interface for testing.
type mockExecutor struct {
	executeResp *pb.ExecuteResponse
	executeErr  error
	healthResp  *pb.HealthCheckResponse
	healthErr   error
	infoResp    *pb.InfoResponse
	infoErr     error
}

func (m *mockExecutor) Execute(_ context.Context, _ string, _ []byte, _ string, _ map[string]string) (*pb.ExecuteResponse, error) {
	return m.executeResp, m.executeErr
}
func (m *mockExecutor) HealthCheck(_ context.Context) (*pb.HealthCheckResponse, error) {
	return m.healthResp, m.healthErr
}
func (m *mockExecutor) Info(_ context.Context) (*pb.InfoResponse, error) {
	return m.infoResp, m.infoErr
}

func newTestRegistry() *Registry {
	return NewRegistry()
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := newTestRegistry()
	entry := &PluginEntry{
		Name:     "test-plugin",
		Type:     "git_provider",
		Enabled:  true,
		Executor: &mockExecutor{},
		Info:     &pb.InfoResponse{Name: "test-plugin", Version: "1.0.0"},
	}

	r.Register(entry)

	got, ok := r.Get("test-plugin")
	if !ok {
		t.Fatal("expected plugin to be registered")
	}
	if got.Name != "test-plugin" {
		t.Errorf("Name = %q, want %q", got.Name, "test-plugin")
	}
	if got.Type != "git_provider" {
		t.Errorf("Type = %q, want %q", got.Type, "git_provider")
	}
}

func TestRegistry_GetNotFound(t *testing.T) {
	r := newTestRegistry()
	_, ok := r.Get("nonexistent")
	if ok {
		t.Fatal("expected plugin not found")
	}
}

func TestRegistry_Unregister(t *testing.T) {
	r := newTestRegistry()
	r.Register(&PluginEntry{Name: "temp", Enabled: true, Executor: &mockExecutor{}})

	r.Unregister("temp")

	_, ok := r.Get("temp")
	if ok {
		t.Fatal("expected plugin to be unregistered")
	}
}

func TestRegistry_GetByType(t *testing.T) {
	r := newTestRegistry()
	r.Register(&PluginEntry{Name: "a", Type: "git_provider", Enabled: true, Executor: &mockExecutor{}})
	r.Register(&PluginEntry{Name: "b", Type: "git_provider", Enabled: true, Executor: &mockExecutor{}})
	r.Register(&PluginEntry{Name: "c", Type: "task_tracker", Enabled: true, Executor: &mockExecutor{}})
	r.Register(&PluginEntry{Name: "d", Type: "git_provider", Enabled: false, Executor: &mockExecutor{}})

	result := r.GetByType("git_provider")
	if len(result) != 2 {
		t.Fatalf("expected 2 enabled git_providers, got %d", len(result))
	}
}

func TestRegistry_List(t *testing.T) {
	r := newTestRegistry()
	r.Register(&PluginEntry{Name: "a", Enabled: true, Executor: &mockExecutor{}})
	r.Register(&PluginEntry{Name: "b", Enabled: false, Executor: &mockExecutor{}})

	list := r.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 plugins, got %d", len(list))
	}
}

func TestRegistry_ExecuteAction(t *testing.T) {
	r := newTestRegistry()
	exec := &mockExecutor{
		executeResp: &pb.ExecuteResponse{Success: true, Output: []byte(`{"result":"ok"}`)},
	}
	r.Register(&PluginEntry{Name: "test", Enabled: true, Executor: exec})

	resp, err := r.ExecuteAction(context.Background(), "test", "do_thing", []byte(`{}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Success {
		t.Error("expected success")
	}
}

func TestRegistry_ExecuteAction_NotFound(t *testing.T) {
	r := newTestRegistry()
	_, err := r.ExecuteAction(context.Background(), "nonexistent", "action", nil, nil)
	if err == nil {
		t.Fatal("expected error for nonexistent plugin")
	}
}

func TestRegistry_ExecuteAction_Disabled(t *testing.T) {
	r := newTestRegistry()
	r.Register(&PluginEntry{Name: "test", Enabled: false, Executor: &mockExecutor{}})

	_, err := r.ExecuteAction(context.Background(), "test", "action", nil, nil)
	if err == nil {
		t.Fatal("expected error for disabled plugin")
	}
}

func TestRegistry_ExecuteActionByType(t *testing.T) {
	r := newTestRegistry()
	exec := &mockExecutor{
		executeResp: &pb.ExecuteResponse{Success: true},
	}
	r.Register(&PluginEntry{Name: "a", Type: "cd_engine", Enabled: true, Executor: exec})

	resp, err := r.ExecuteActionByType(context.Background(), "cd_engine", "deploy", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Success {
		t.Error("expected success")
	}
}

func TestRegistry_ExecuteActionByType_NoPlugins(t *testing.T) {
	r := newTestRegistry()
	_, err := r.ExecuteActionByType(context.Background(), "cd_engine", "deploy", nil, nil)
	if err == nil {
		t.Fatal("expected error when no plugins of type")
	}
}

func TestRegistry_SetEnabled(t *testing.T) {
	r := newTestRegistry()
	r.Register(&PluginEntry{Name: "test", Enabled: true, Executor: &mockExecutor{}})

	if err := r.SetEnabled("test", false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entry, _ := r.Get("test")
	if entry.Enabled {
		t.Error("expected plugin to be disabled")
	}
}

func TestRegistry_SetEnabled_NotFound(t *testing.T) {
	r := newTestRegistry()
	err := r.SetEnabled("nonexistent", true)
	if err == nil {
		t.Fatal("expected error for nonexistent plugin")
	}
}

func TestRegistry_Summary(t *testing.T) {
	r := newTestRegistry()
	r.Register(&PluginEntry{
		Name:    "test",
		Type:    "git_provider",
		Enabled: true,
		Info: &pb.InfoResponse{
			Actions: []*pb.ActionInfo{{Name: "list_repos"}, {Name: "get_branches"}},
		},
		Executor: &mockExecutor{},
	})

	summary := r.Summary()
	if len(summary) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(summary))
	}
	actions := summary[0]["actions"].([]string)
	if len(actions) != 2 {
		t.Errorf("expected 2 actions, got %d", len(actions))
	}
}

func TestRegistry_HealthCheck(t *testing.T) {
	r := newTestRegistry()
	exec := &mockExecutor{
		healthResp: &pb.HealthCheckResponse{Status: "ok"},
	}
	r.Register(&PluginEntry{Name: "test", Enabled: true, Executor: exec})

	resp, err := r.HealthCheck(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("status = %q, want %q", resp.Status, "ok")
	}
}
