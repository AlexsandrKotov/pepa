package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/repository"
	"github.com/pepa/pepa/internal/security"
)

// SecurityScanAdapter implements Provider for security scan orchestration.
// It allows security scans to be used as pipeline steps in CI/CD workflows.
type SecurityScanAdapter struct {
	scanner *security.Scanner
	repo    *repository.SecurityScanRepository

	mu   sync.RWMutex
	runs map[string]*securityRun
}

type securityRun struct {
	targetID uuid.UUID
	tenantID uuid.UUID
	runID    uuid.UUID
	status   string
	result   map[string]any
}

// NewSecurityScanAdapter creates a new security scan adapter.
func NewSecurityScanAdapter(scanner *security.Scanner, repo *repository.SecurityScanRepository) *SecurityScanAdapter {
	return &SecurityScanAdapter{
		scanner: scanner,
		repo:    repo,
		runs:    make(map[string]*securityRun),
	}
}

func (a *SecurityScanAdapter) Name() string { return "security_scan" }

// ResolveSchema returns the parameter schema for security scan sources.
func (a *SecurityScanAdapter) ResolveSchema(ctx context.Context, config json.RawMessage) (*ParameterSchema, error) {
	props := map[string]PropertyDef{
		"target_id": {
			Type:        "string",
			Description: "UUID of the scan target to scan",
		},
		"fail_on_critical": {
			Type:        "boolean",
			Description: "Fail the pipeline if critical vulnerabilities are found",
			Default:     "true",
		},
		"fail_on_high": {
			Type:        "boolean",
			Description: "Fail the pipeline if high vulnerabilities are found",
			Default:     "false",
		},
		"max_critical": {
			Type:        "number",
			Description: "Maximum number of critical vulnerabilities allowed",
			Default:     "0",
		},
		"max_high": {
			Type:        "number",
			Description: "Maximum number of high vulnerabilities allowed",
			Default:     "10",
		},
	}

	return &ParameterSchema{
		Type:       "object",
		Properties: props,
		Required:   []string{"target_id"},
	}, nil
}

// Trigger starts a security scan for the specified target.
func (a *SecurityScanAdapter) Trigger(ctx context.Context, config json.RawMessage, params map[string]any) (*TriggerResult, error) {
	targetIDStr, ok := params["target_id"].(string)
	if !ok || targetIDStr == "" {
		return nil, fmt.Errorf("target_id is required")
	}

	targetID, err := uuid.Parse(targetIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid target_id: %w", err)
	}

	// Get tenant_id from params or use a default
	tenantIDStr, _ := params["tenant_id"].(string)
	tenantID := uuid.Nil
	if tenantIDStr != "" {
		tenantID, err = uuid.Parse(tenantIDStr)
		if err != nil {
			return nil, fmt.Errorf("invalid tenant_id: %w", err)
		}
	}

	// Run the scan
	run, err := a.scanner.RunScan(ctx, targetID, tenantID, "pipeline")
	if err != nil {
		return nil, fmt.Errorf("security scan failed: %w", err)
	}

	// Track the run
	runID := run.ID.String()
	a.mu.Lock()
	a.runs[runID] = &securityRun{
		targetID: targetID,
		tenantID: tenantID,
		runID:    run.ID,
		status:   run.Status,
		result:   run.ResultSummary,
	}
	a.mu.Unlock()

	// Check quality gate
	failOnCritical, _ := params["fail_on_critical"].(bool)
	if !failOnCritical {
		failOnCritical = true // default
	}
	maxCritical := 0
	if mc, ok := params["max_critical"].(float64); ok {
		maxCritical = int(mc)
	}

	if failOnCritical && run.ResultSummary != nil {
		if critical, ok := run.ResultSummary["critical"].(int); ok && critical > maxCritical {
			return &TriggerResult{
				ExternalRunID: runID,
				Status:        "failed",
				ExternalURL:   fmt.Sprintf("/security/scans/%s", runID),
			}, fmt.Errorf("quality gate failed: %d critical vulnerabilities (max: %d)", critical, maxCritical)
		}
	}

	return &TriggerResult{
		ExternalRunID: runID,
		Status:        run.Status,
		ExternalURL:   fmt.Sprintf("/security/scans/%s", runID),
	}, nil
}

// Status returns the current status of a security scan.
func (a *SecurityScanAdapter) Status(ctx context.Context, config json.RawMessage, externalRunID string) (*RunStatus, error) {
	a.mu.RLock()
	run, ok := a.runs[externalRunID]
	a.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("scan run not found: %s", externalRunID)
	}

	status := "success"
	if run.status == "failed" {
		status = "failed"
	} else if run.status == "running" || run.status == "pending" {
		status = "running"
	}

	return &RunStatus{
		ExternalRunID: externalRunID,
		Status:        status,
		ExternalURL:   fmt.Sprintf("/security/scans/%s", externalRunID),
	}, nil
}

// Jobs returns the jobs for a security scan (single job representing the scan).
func (a *SecurityScanAdapter) Jobs(ctx context.Context, config json.RawMessage, externalRunID string) ([]JobInfo, error) {
	a.mu.RLock()
	run, ok := a.runs[externalRunID]
	a.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("scan run not found: %s", externalRunID)
	}

	jobStatus := "success"
	if run.status == "failed" {
		jobStatus = "failed"
	} else if run.status == "running" || run.status == "pending" {
		jobStatus = "running"
	}

	return []JobInfo{
		{
			ExternalJobID: externalRunID,
			Name:          "Security Scan",
			Stage:         "security",
			Status:        jobStatus,
		},
	}, nil
}

// Logs returns the logs for a security scan (returns JSON summary as logs).
func (a *SecurityScanAdapter) Logs(ctx context.Context, config json.RawMessage, externalRunID string, jobID string) (string, error) {
	a.mu.RLock()
	run, ok := a.runs[externalRunID]
	a.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("scan run not found: %s", externalRunID)
	}

	// Return the result summary as logs
	if run.result != nil {
		logs, err := json.MarshalIndent(run.result, "", "  ")
		if err != nil {
			return "", err
		}
		return string(logs), nil
	}

	return "Security scan completed. No results available.", nil
}

// Cancel is not supported for security scans (they run to completion).
func (a *SecurityScanAdapter) Cancel(ctx context.Context, config json.RawMessage, externalRunID string) error {
	return fmt.Errorf("cancel is not supported for security scans")
}
