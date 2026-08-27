package workflow

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/pepa/pepa/pkg/models"
)

func TestBuildDAG_Linear(t *testing.T) {
	steps := []models.StepSpec{
		{Name: "a"},
		{Name: "b", DependsOn: []string{"a"}},
		{Name: "c", DependsOn: []string{"b"}},
	}
	levels, err := buildDAG(steps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(levels) != 3 {
		t.Fatalf("expected 3 levels, got %d: %v", len(levels), levels)
	}
	// Level 0: [a], Level 1: [b], Level 2: [c]
	for i, level := range levels {
		if len(level) != 1 {
			t.Errorf("level %d: expected 1 step, got %d: %v", i, len(level), level)
		}
	}
}

func TestBuildDAG_Parallel(t *testing.T) {
	steps := []models.StepSpec{
		{Name: "root"},
		{Name: "x", DependsOn: []string{"root"}},
		{Name: "y", DependsOn: []string{"root"}},
		{Name: "z", DependsOn: []string{"root"}},
		{Name: "final", DependsOn: []string{"x", "y", "z"}},
	}
	levels, err := buildDAG(steps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(levels) != 3 {
		t.Fatalf("expected 3 levels, got %d: %v", len(levels), levels)
	}
	// Level 0: [root], Level 1: [x, y, z], Level 2: [final]
	if len(levels[0]) != 1 || levels[0][0] != "root" {
		t.Errorf("level 0: expected [root], got %v", levels[0])
	}
	if len(levels[1]) != 3 {
		t.Errorf("level 1: expected 3 parallel steps, got %d: %v", len(levels[1]), levels[1])
	}
	if len(levels[2]) != 1 || levels[2][0] != "final" {
		t.Errorf("level 2: expected [final], got %v", levels[2])
	}
}

func TestBuildDAG_Diamond(t *testing.T) {
	// Diamond: a -> b, a -> c, b -> d, c -> d
	steps := []models.StepSpec{
		{Name: "a"},
		{Name: "b", DependsOn: []string{"a"}},
		{Name: "c", DependsOn: []string{"a"}},
		{Name: "d", DependsOn: []string{"b", "c"}},
	}
	levels, err := buildDAG(steps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(levels) != 3 {
		t.Fatalf("expected 3 levels, got %d", len(levels))
	}
	if len(levels[1]) != 2 {
		t.Errorf("level 1: expected 2 parallel steps, got %v", levels[1])
	}
}

func TestBuildDAG_CircularDependency(t *testing.T) {
	steps := []models.StepSpec{
		{Name: "a", DependsOn: []string{"c"}},
		{Name: "b", DependsOn: []string{"a"}},
		{Name: "c", DependsOn: []string{"b"}},
	}
	_, err := buildDAG(steps)
	if err == nil {
		t.Fatal("expected circular dependency error, got nil")
	}
	if err.Error() != "circular dependency detected in workflow steps" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBuildDAG_DuplicateName(t *testing.T) {
	steps := []models.StepSpec{
		{Name: "a"},
		{Name: "a"},
	}
	_, err := buildDAG(steps)
	if err == nil {
		t.Fatal("expected duplicate step name error")
	}
}

func TestBuildDAG_EmptyName(t *testing.T) {
	steps := []models.StepSpec{
		{Name: ""},
	}
	_, err := buildDAG(steps)
	if err == nil {
		t.Fatal("expected empty name error")
	}
}

func TestBuildDAG_UnknownDependency(t *testing.T) {
	steps := []models.StepSpec{
		{Name: "a", DependsOn: []string{"nonexistent"}},
	}
	_, err := buildDAG(steps)
	if err == nil {
		t.Fatal("expected unknown dependency error")
	}
}

func TestBuildDAG_NoSteps(t *testing.T) {
	levels, err := buildDAG(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(levels) != 0 {
		t.Errorf("expected 0 levels, got %d", len(levels))
	}
}

func TestHasFailed(t *testing.T) {
	tests := []struct {
		name    string
		results map[string]*stepResult
		want    bool
	}{
		{"empty", map[string]*stepResult{}, false},
		{"all success", map[string]*stepResult{
			"a": {status: "success"},
			"b": {status: "success"},
		}, false},
		{"one failed", map[string]*stepResult{
			"a": {status: "success"},
			"b": {status: "failed"},
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasFailed(tt.results); got != tt.want {
				t.Errorf("hasFailed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasFailedDeps(t *testing.T) {
	results := map[string]*stepResult{
		"a": {status: "success"},
		"b": {status: "failed"},
		"c": {status: "pending"},
	}

	tests := []struct {
		deps []string
		want bool
	}{
		{[]string{"a"}, false},
		{[]string{"b"}, true},
		{[]string{"a", "b"}, true},
		{[]string{"a", "c"}, false},
		{[]string{"nonexistent"}, false},
		{nil, false},
	}
	for _, tt := range tests {
		got := hasFailedDeps(tt.deps, results)
		if got != tt.want {
			t.Errorf("hasFailedDeps(%v) = %v, want %v", tt.deps, got, tt.want)
		}
	}
}

func TestEvaluateCondition(t *testing.T) {
	results := map[string]*stepResult{
		"build": {status: "success"},
		"test":  {status: "failed"},
	}

	tests := []struct {
		cond string
		want bool
	}{
		{"", true},
		{"entity.status == active", false}, // simple string compare: "entity.status" != "active"
		{"steps.build == success", true},   // checks result of step "build" == "success"
		{"steps.test == success", false},   // test step is "failed"
		{"hello == hello", true},           // simple equality
		{"hello == world", false},          // simple inequality
	}
	for _, tt := range tests {
		got := evaluateCondition(tt.cond, results, nil)
		if got != tt.want {
			t.Errorf("evaluateCondition(%q) = %v, want %v", tt.cond, got, tt.want)
		}
	}
}

func TestStepIndex(t *testing.T) {
	steps := []models.StepSpec{
		{Name: "a"},
		{Name: "b"},
		{Name: "c"},
	}
	if idx := stepIndex(steps, "b"); idx != 1 {
		t.Errorf("stepIndex(b) = %d, want 1", idx)
	}
	if idx := stepIndex(steps, "z"); idx != -1 {
		t.Errorf("stepIndex(z) = %d, want -1", idx)
	}
}

func TestStepResultsToMap(t *testing.T) {
	results := map[string]*stepResult{
		"a": {status: "success", output: json.RawMessage(`{"ok":true}`)},
		"b": {status: "failed", err: fmt.Errorf("something broke")},
	}
	m := stepResultsToMap(results)
	if len(m) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(m))
	}
	aEntry := m["a"].(map[string]interface{})
	if aEntry["status"] != "success" {
		t.Errorf("a.status = %v, want success", aEntry["status"])
	}
	bEntry := m["b"].(map[string]interface{})
	if bEntry["error"] != "something broke" {
		t.Errorf("b.error = %v, want 'something broke'", bEntry["error"])
	}
}
