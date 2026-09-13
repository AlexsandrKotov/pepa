package service

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestContainsHelmChart(t *testing.T) {
	tests := []struct {
		name string
		spec json.RawMessage
		want bool
	}{
		{
			name: "helm chart with source_type",
			spec: json.RawMessage(`{"chart":{"source_type":"helm_repo","chart_name":"nginx","chart_version":"1.0.0"}}`),
			want: true,
		},
		{
			name: "container source_type is not helm",
			spec: json.RawMessage(`{"chart":{"source_type":"container","image":"nginx:latest"}}`),
			want: false,
		},
		{
			name: "empty source_type",
			spec: json.RawMessage(`{"chart":{"source_type":""}}`),
			want: false,
		},
		{
			name: "no chart field",
			spec: json.RawMessage(`{"containers":[{"name":"app","image":"nginx"}]}`),
			want: false,
		},
		{
			name: "null chart",
			spec: json.RawMessage(`{"chart":null}`),
			want: false,
		},
		{
			name: "invalid JSON",
			spec: json.RawMessage(`{invalid`),
			want: false,
		},
		{
			name: "empty JSON",
			spec: json.RawMessage(`{}`),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsHelmChart(tt.spec)
			if got != tt.want {
				t.Errorf("containsHelmChart(%s) = %v, want %v", string(tt.spec), got, tt.want)
			}
		})
	}
}

func TestNewDeploymentService(t *testing.T) {
	// Verify constructor doesn't panic with nil repos (used in tests)
	svc := NewDeploymentService(nil, nil, nil)
	if svc == nil {
		t.Fatal("NewDeploymentService returned nil")
	}
	if svc.executor == nil {
		t.Fatal("executor should be initialised")
	}
}

func TestDeploymentService_SetEventRecorder(t *testing.T) {
	svc := NewDeploymentService(nil, nil, nil)

	// Initially nil
	svc.recordEvent(uuid.Nil, "test", "msg") // should not panic

	// Set recorder
	called := false
	svc.SetEventRecorder(func(_ uuid.UUID, eventType, message string) {
		called = true
		if eventType != "deployed" {
			t.Errorf("expected eventType 'deployed', got %q", eventType)
		}
		if message != "test message" {
			t.Errorf("expected message 'test message', got %q", message)
		}
	})
	svc.recordEvent(uuid.Nil, "deployed", "test message")
	if !called {
		t.Error("event recorder was not called")
	}
}

func TestNewConnectionService(t *testing.T) {
	svc := NewConnectionService()
	if svc == nil {
		t.Fatal("NewConnectionService returned nil")
	}
	if svc.httpClient == nil {
		t.Fatal("httpClient should be initialised")
	}
}

func TestNewServiceDeploymentService(t *testing.T) {
	svc := NewServiceDeploymentService(nil, nil, nil)
	if svc == nil {
		t.Fatal("NewServiceDeploymentService returned nil")
	}
}

func TestNewEntitySyncService(t *testing.T) {
	svc := NewEntitySyncService(nil)
	if svc == nil {
		t.Fatal("NewEntitySyncService returned nil")
	}
}

func TestNewScorecardEvalService(t *testing.T) {
	svc := NewScorecardEvalService(nil, nil)
	if svc == nil {
		t.Fatal("NewScorecardEvalService returned nil")
	}
}
