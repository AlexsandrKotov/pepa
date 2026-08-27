package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPipelineSourceType_Constants(t *testing.T) {
	if PipelineSourceGitLabCI != "gitlab_ci" {
		t.Errorf("PipelineSourceGitLabCI = %q, want gitlab_ci", PipelineSourceGitLabCI)
	}
	if PipelineSourceAnsible != "ansible" {
		t.Errorf("PipelineSourceAnsible = %q, want ansible", PipelineSourceAnsible)
	}
	if PipelineSourceTerraform != "terraform" {
		t.Errorf("PipelineSourceTerraform = %q, want terraform", PipelineSourceTerraform)
	}
}

func TestPipelineRunStatus_Constants(t *testing.T) {
	statuses := map[PipelineRunStatus]string{
		PipelineRunPending:   "pending",
		PipelineRunRunning:   "running",
		PipelineRunSuccess:   "success",
		PipelineRunFailed:    "failed",
		PipelineRunCancelled: "cancelled",
		PipelineRunTimeout:   "timeout",
		PipelineRunError:     "error",
	}
	for status, want := range statuses {
		if string(status) != want {
			t.Errorf("status = %q, want %q", status, want)
		}
	}
}

func TestPipelineSource_JSON(t *testing.T) {
	id := uuid.New()
	tenantID := uuid.New()
	ps := PipelineSource{
		ID:         id,
		TenantID:   tenantID,
		Name:       "My Pipeline",
		SourceType: "gitlab_ci",
		Config:     json.RawMessage(`{"project_id":"123"}`),
		Status:     "active",
		CreatedAt:  time.Now().Truncate(time.Second),
	}

	data, err := json.Marshal(ps)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded PipelineSource
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.ID != id {
		t.Errorf("ID mismatch: %v vs %v", decoded.ID, id)
	}
	if decoded.Name != "My Pipeline" {
		t.Errorf("Name = %q, want My Pipeline", decoded.Name)
	}
	if decoded.SourceType != "gitlab_ci" {
		t.Errorf("SourceType = %q, want gitlab_ci", decoded.SourceType)
	}
}

func TestPipelineRun_JSON(t *testing.T) {
	id := uuid.New()
	sourceID := uuid.New()
	now := time.Now().Truncate(time.Second)
	dur := 5000

	run := PipelineRun{
		ID:         id,
		SourceID:   sourceID,
		Status:     PipelineRunRunning,
		DurationMs: &dur,
		StartedAt:  &now,
	}

	data, err := json.Marshal(run)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded PipelineRun
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Status != PipelineRunRunning {
		t.Errorf("Status = %q, want running", decoded.Status)
	}
	if *decoded.DurationMs != 5000 {
		t.Errorf("DurationMs = %d, want 5000", *decoded.DurationMs)
	}
}

func TestExecutionStatus_Constants(t *testing.T) {
	statuses := map[ExecutionStatus]string{
		ExecutionPending:    "pending",
		ExecutionRunning:    "running",
		ExecutionWaiting:    "waiting",
		ExecutionSuccess:    "success",
		ExecutionFailed:     "failed",
		ExecutionCancelled:  "cancelled",
		ExecutionRolling:    "rolling_back",
		ExecutionRolledBack: "rolled_back",
	}
	for status, want := range statuses {
		if string(status) != want {
			t.Errorf("status = %q, want %q", status, want)
		}
	}
}

func TestWorkflowSpec_JSON(t *testing.T) {
	spec := WorkflowSpec{
		Triggers: []TriggerSpec{
			{Type: "webhook", Config: json.RawMessage(`{"url":"https://example.com"}`)},
			{Type: "manual"},
		},
		Steps: []StepSpec{
			{Name: "build", Action: "compile"},
			{Name: "test", DependsOn: []string{"build"}, Condition: "steps.build == success"},
			{Name: "deploy", DependsOn: []string{"test"}, RunWhen: "always"},
		},
		Settings: SettingsSpec{
			Timeout:     "30m",
			Concurrency: 1,
			OnConflict:  "reject",
		},
	}

	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded WorkflowSpec
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(decoded.Triggers) != 2 {
		t.Errorf("triggers = %d, want 2", len(decoded.Triggers))
	}
	if len(decoded.Steps) != 3 {
		t.Errorf("steps = %d, want 3", len(decoded.Steps))
	}
	if decoded.Steps[1].DependsOn[0] != "build" {
		t.Errorf("step[1] depends_on = %v, want [build]", decoded.Steps[1].DependsOn)
	}
	if decoded.Settings.Timeout != "30m" {
		t.Errorf("timeout = %s, want 30m", decoded.Settings.Timeout)
	}
}

func TestEntity_JSON(t *testing.T) {
	id := uuid.New()
	typeID := uuid.New()
	tenantID := uuid.New()
	orgID := uuid.New()

	e := Entity{
		ID:             id,
		TypeID:         typeID,
		TypeKey:        "service",
		Name:           "my-service",
		TenantID:       tenantID,
		OrganizationID: orgID,
		Status:         "active",
		Metadata:       json.RawMessage(`{"version":"1.0"}`),
	}

	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Entity
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.TypeKey != "service" {
		t.Errorf("TypeKey = %q, want service", decoded.TypeKey)
	}
	if decoded.Name != "my-service" {
		t.Errorf("Name = %q, want my-service", decoded.Name)
	}
}

func TestService_JSON(t *testing.T) {
	id := uuid.New()
	svc := Service{
		ID:        id,
		Name:      "web-app",
		Slug:      "web-app",
		Namespace: "production",
		Status:    "running",
	}

	data, err := json.Marshal(svc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Service
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Slug != "web-app" {
		t.Errorf("Slug = %q, want web-app", decoded.Slug)
	}
}

func TestCreateServiceRequest_Required(t *testing.T) {
	req := CreateServiceRequest{
		Name: "test-service",
	}
	if req.Name != "test-service" {
		t.Errorf("Name = %q, want test-service", req.Name)
	}
}

func TestServiceFilter_Defaults(t *testing.T) {
	f := ServiceFilter{
		Page:    1,
		PerPage: 20,
	}
	if f.Page != 1 {
		t.Errorf("Page = %d, want 1", f.Page)
	}
	if f.PerPage != 20 {
		t.Errorf("PerPage = %d, want 20", f.PerPage)
	}
}

func TestRetrySpec_JSON(t *testing.T) {
	rs := RetrySpec{
		MaxRetries:      3,
		Backoff:         "exponential",
		InitialInterval: "1s",
		MaxInterval:     "30s",
		Multiplier:      2.0,
	}
	data, err := json.Marshal(rs)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded RetrySpec
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", decoded.MaxRetries)
	}
	if decoded.Multiplier != 2.0 {
		t.Errorf("Multiplier = %f, want 2.0", decoded.Multiplier)
	}
}

func TestGraphResult_Structure(t *testing.T) {
	gr := GraphResult{
		Nodes: []GraphNode{
			{Entity: Entity{Name: "node1"}, Depth: 0, Path: []string{"node1"}},
			{Entity: Entity{Name: "node2"}, Depth: 1, Path: []string{"node1", "node2"}},
		},
		Edges: []GraphEdge{
			{
				Source:    Entity{Name: "node1"},
				Target:    Entity{Name: "node2"},
				Direction: "outgoing",
			},
		},
	}
	if len(gr.Nodes) != 2 {
		t.Errorf("nodes = %d, want 2", len(gr.Nodes))
	}
	if len(gr.Edges) != 1 {
		t.Errorf("edges = %d, want 1", len(gr.Edges))
	}
	if gr.Edges[0].Direction != "outgoing" {
		t.Errorf("direction = %q, want outgoing", gr.Edges[0].Direction)
	}
}

func TestEntityType_JSON(t *testing.T) {
	et := EntityType{
		TypeKey:     "service",
		DisplayName: "Service",
		Category:    "application",
		IsSystem:    true,
		IsEnabled:   true,
	}
	data, err := json.Marshal(et)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded EntityType
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.TypeKey != "service" {
		t.Errorf("TypeKey = %q, want service", decoded.TypeKey)
	}
	if !decoded.IsSystem {
		t.Error("expected IsSystem=true")
	}
}

func TestServiceDeployment_JSON(t *testing.T) {
	sd := ServiceDeployment{
		Environment: "production",
		Status:      "deployed",
		PodsReady:   3,
		PodsTotal:   3,
		FluxSynced:  true,
	}
	data, err := json.Marshal(sd)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded ServiceDeployment
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.PodsReady != 3 || decoded.PodsTotal != 3 {
		t.Errorf("pods = %d/%d, want 3/3", decoded.PodsReady, decoded.PodsTotal)
	}
	if !decoded.FluxSynced {
		t.Error("expected FluxSynced=true")
	}
}
