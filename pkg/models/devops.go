package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ============================================================
// DeploymentWindow — controls when deployments are allowed/blocked
// ============================================================

type DeploymentWindow struct {
	ID            uuid.UUID   `json:"id" db:"id"`
	TenantID      uuid.UUID   `json:"tenant_id" db:"tenant_id"`
	Name          string      `json:"name" db:"name"`
	Description   string      `json:"description,omitempty" db:"description"`
	WindowType    string      `json:"window_type" db:"window_type"` // 'allowed' | 'blocked' | 'freeze'
	CronExpression string     `json:"cron_expression,omitempty" db:"cron_expression"`
	StartAt       *time.Time  `json:"start_at,omitempty" db:"start_at"`
	EndAt         *time.Time  `json:"end_at,omitempty" db:"end_at"`
	Timezone      string      `json:"timezone" db:"timezone"`
	Environments  []string    `json:"environments" db:"environments"`
	ServiceIDs    []uuid.UUID `json:"service_ids" db:"service_ids"`
	Enabled       bool        `json:"enabled" db:"enabled"`
	Priority      int         `json:"priority" db:"priority"`
	Reason        string      `json:"reason,omitempty" db:"reason"`
	OverrideRoles []string    `json:"override_roles" db:"override_roles"`
	CreatedBy     *uuid.UUID  `json:"created_by,omitempty" db:"created_by"`
	CreatedAt     time.Time   `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at" db:"updated_at"`
}

type CreateDeploymentWindowRequest struct {
	Name           string      `json:"name" binding:"required"`
	Description    string      `json:"description,omitempty"`
	WindowType     string      `json:"window_type" binding:"required,oneof=allowed blocked freeze"`
	CronExpression string      `json:"cron_expression,omitempty"`
	StartAt        *time.Time  `json:"start_at,omitempty"`
	EndAt          *time.Time  `json:"end_at,omitempty"`
	Timezone       string      `json:"timezone,omitempty"`
	Environments   []string    `json:"environments,omitempty"`
	ServiceIDs     []uuid.UUID `json:"service_ids,omitempty"`
	Enabled        *bool       `json:"enabled,omitempty"`
	Priority       *int        `json:"priority,omitempty"`
	Reason         string      `json:"reason,omitempty"`
	OverrideRoles  []string    `json:"override_roles,omitempty"`
}

type UpdateDeploymentWindowRequest struct {
	Name           *string     `json:"name,omitempty"`
	Description    *string     `json:"description,omitempty"`
	WindowType     *string     `json:"window_type,omitempty"`
	CronExpression *string     `json:"cron_expression,omitempty"`
	StartAt        *time.Time  `json:"start_at,omitempty"`
	EndAt          *time.Time  `json:"end_at,omitempty"`
	Timezone       *string     `json:"timezone,omitempty"`
	Environments   []string    `json:"environments,omitempty"`
	ServiceIDs     []uuid.UUID `json:"service_ids,omitempty"`
	Enabled        *bool       `json:"enabled,omitempty"`
	Priority       *int        `json:"priority,omitempty"`
	Reason         *string     `json:"reason,omitempty"`
	OverrideRoles  []string    `json:"override_roles,omitempty"`
}

// WindowCheckResult represents the result of checking if deployment is allowed
type WindowCheckResult struct {
	Allowed       bool              `json:"allowed"`
	Reason        string            `json:"reason,omitempty"`
	BlockingWindow *DeploymentWindow `json:"blocking_window,omitempty"`
	ActiveWindows  []DeploymentWindow `json:"active_windows,omitempty"`
}

// ============================================================
// BatchOperation — mass restart/rollback/scale during incidents
// ============================================================

type BatchOperation struct {
	ID             uuid.UUID       `json:"id" db:"id"`
	TenantID       uuid.UUID       `json:"tenant_id" db:"tenant_id"`
	Name           string          `json:"name" db:"name"`
	OperationType  string          `json:"operation_type" db:"operation_type"` // 'restart' | 'rollback' | 'scale' | 'custom'
	Status         string          `json:"status" db:"status"`                 // 'pending' | 'running' | 'completed' | 'failed' | 'cancelled'
	ServiceIDs     []uuid.UUID     `json:"service_ids" db:"service_ids"`
	Parameters     json.RawMessage `json:"parameters,omitempty" db:"parameters"`
	Results        json.RawMessage `json:"results,omitempty" db:"results"`
	TotalCount     int             `json:"total_count" db:"total_count"`
	CompletedCount int             `json:"completed_count" db:"completed_count"`
	FailedCount    int             `json:"failed_count" db:"failed_count"`
	InitiatedBy    *uuid.UUID      `json:"initiated_by,omitempty" db:"initiated_by"`
	Reason         string          `json:"reason,omitempty" db:"reason"`
	IncidentID     string          `json:"incident_id,omitempty" db:"incident_id"`
	TimeoutSeconds int             `json:"timeout_seconds" db:"timeout_seconds"`
	StartedAt      *time.Time      `json:"started_at,omitempty" db:"started_at"`
	CompletedAt    *time.Time      `json:"completed_at,omitempty" db:"completed_at"`
	ErrorMessage   string          `json:"error_message,omitempty" db:"error_message"`
	CreatedAt      time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at" db:"updated_at"`
}

type CreateBatchOperationRequest struct {
	Name           string          `json:"name" binding:"required"`
	OperationType  string          `json:"operation_type" binding:"required,oneof=restart rollback scale custom"`
	ServiceIDs     []uuid.UUID     `json:"service_ids" binding:"required,min=1"`
	Parameters     json.RawMessage `json:"parameters,omitempty"`
	Reason         string          `json:"reason,omitempty"`
	IncidentID     string          `json:"incident_id,omitempty"`
	TimeoutSeconds *int            `json:"timeout_seconds,omitempty"`
}

type BatchOperationResult struct {
	ServiceID     uuid.UUID `json:"service_id"`
	ServiceName   string    `json:"service_name"`
	Status        string    `json:"status"` // 'success' | 'failed' | 'skipped'
	DeploymentID  *uuid.UUID `json:"deployment_id,omitempty"`
	ErrorMessage  string    `json:"error_message,omitempty"`
	DurationMs    int       `json:"duration_ms,omitempty"`
}

// ============================================================
// CompliancePolicy — policy-as-code for deployment validation
// ============================================================

type CompliancePolicy struct {
	ID                 uuid.UUID       `json:"id" db:"id"`
	TenantID           uuid.UUID       `json:"tenant_id" db:"tenant_id"`
	Name               string          `json:"name" db:"name"`
	Description        string          `json:"description,omitempty" db:"description"`
	PolicyType         string          `json:"policy_type" db:"policy_type"` // 'resource_limits' | 'security_scan' | 'approval' | 'custom'
	PolicySpec         json.RawMessage `json:"policy_spec" db:"policy_spec"`
	Severity           string          `json:"severity" db:"severity"` // 'block' | 'warn' | 'info'
	Blocking           bool            `json:"blocking" db:"blocking"`
	Environments       []string        `json:"environments" db:"environments"`
	ServiceIDs         []uuid.UUID     `json:"service_ids" db:"service_ids"`
	Enabled            bool            `json:"enabled" db:"enabled"`
	LastEvaluatedAt    *time.Time      `json:"last_evaluated_at,omitempty" db:"last_evaluated_at"`
	LastViolationCount int             `json:"last_violation_count" db:"last_violation_count"`
	CreatedBy          *uuid.UUID      `json:"created_by,omitempty" db:"created_by"`
	CreatedAt          time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at" db:"updated_at"`
}

type CreateCompliancePolicyRequest struct {
	Name         string          `json:"name" binding:"required"`
	Description  string          `json:"description,omitempty"`
	PolicyType   string          `json:"policy_type" binding:"required,oneof=resource_limits security_scan approval custom"`
	PolicySpec   json.RawMessage `json:"policy_spec" binding:"required"`
	Severity     string          `json:"severity" binding:"required,oneof=block warn info"`
	Blocking     *bool           `json:"blocking,omitempty"`
	Environments []string        `json:"environments,omitempty"`
	ServiceIDs   []uuid.UUID     `json:"service_ids,omitempty"`
	Enabled      *bool           `json:"enabled,omitempty"`
}

type UpdateCompliancePolicyRequest struct {
	Name         *string         `json:"name,omitempty"`
	Description  *string         `json:"description,omitempty"`
	PolicyType   *string         `json:"policy_type,omitempty"`
	PolicySpec   json.RawMessage `json:"policy_spec,omitempty"`
	Severity     *string         `json:"severity,omitempty"`
	Blocking     *bool           `json:"blocking,omitempty"`
	Environments []string        `json:"environments,omitempty"`
	ServiceIDs   []uuid.UUID     `json:"service_ids,omitempty"`
	Enabled      *bool           `json:"enabled,omitempty"`
}

// ComplianceEvaluation represents a single policy evaluation result
type ComplianceEvaluation struct {
	ID           uuid.UUID       `json:"id" db:"id"`
	TenantID     uuid.UUID       `json:"tenant_id" db:"tenant_id"`
	PolicyID     uuid.UUID       `json:"policy_id" db:"policy_id"`
	ServiceID    *uuid.UUID      `json:"service_id,omitempty" db:"service_id"`
	DeploymentID *uuid.UUID      `json:"deployment_id,omitempty" db:"deployment_id"`
	Result       string          `json:"result" db:"result"` // 'pass' | 'fail' | 'warn' | 'skip'
	Violations   json.RawMessage `json:"violations,omitempty" db:"violations"`
	Context      json.RawMessage `json:"context,omitempty" db:"context"`
	EvaluatedAt  time.Time       `json:"evaluated_at" db:"evaluated_at"`
	// Joined fields
	PolicyName string `json:"policy_name,omitempty" db:"policy_name"`
}

type ComplianceViolation struct {
	Field       string `json:"field"`
	Message     string `json:"message"`
	Expected    string `json:"expected,omitempty"`
	Actual      string `json:"actual,omitempty"`
	Severity    string `json:"severity"`
}

// ComplianceCheckResult is the result of checking all policies for a deployment
type ComplianceCheckResult struct {
	Passed       bool                  `json:"passed"`
	Blocked      bool                  `json:"blocked"`
	Evaluations  []ComplianceEvaluation `json:"evaluations"`
	Violations   []ComplianceViolation  `json:"violations,omitempty"`
}

// ============================================================
// SecurityFinding — aggregated security findings from scans
// ============================================================

type SecurityFinding struct {
	ID              uuid.UUID  `json:"id" db:"id"`
	TenantID        uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	ScanRunID       *uuid.UUID `json:"scan_run_id,omitempty" db:"scan_run_id"`
	FindingType     string     `json:"finding_type" db:"finding_type"` // 'vulnerability' | 'misconfiguration' | 'secret' | 'license'
	Severity        string     `json:"severity" db:"severity"`         // 'critical' | 'high' | 'medium' | 'low' | 'info'
	Title           string     `json:"title" db:"title"`
	Description     string     `json:"description,omitempty" db:"description"`
	Identifier      string     `json:"identifier,omitempty" db:"identifier"` // CVE ID etc.
	ResourceType    string     `json:"resource_type,omitempty" db:"resource_type"`
	ResourceName    string     `json:"resource_name,omitempty" db:"resource_name"`
	FixAvailable    bool       `json:"fix_available" db:"fix_available"`
	FixVersion      string     `json:"fix_version,omitempty" db:"fix_version"`
	FixInstructions string     `json:"fix_instructions,omitempty" db:"fix_instructions"`
	Status          string     `json:"status" db:"status"` // 'open' | 'acknowledged' | 'fixed' | 'false_positive' | 'risk_accepted'
	ResolvedBy      *uuid.UUID `json:"resolved_by,omitempty" db:"resolved_by"`
	ResolvedAt      *time.Time `json:"resolved_at,omitempty" db:"resolved_at"`
	ResolutionNotes string     `json:"resolution_notes,omitempty" db:"resolution_notes"`
	FirstSeenAt     time.Time  `json:"first_seen_at" db:"first_seen_at"`
	LastSeenAt      time.Time  `json:"last_seen_at" db:"last_seen_at"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
}

type CreateSecurityFindingRequest struct {
	ScanRunID       *uuid.UUID `json:"scan_run_id,omitempty"`
	FindingType     string     `json:"finding_type" binding:"required,oneof=vulnerability misconfiguration secret license"`
	Severity        string     `json:"severity" binding:"required,oneof=critical high medium low info"`
	Title           string     `json:"title" binding:"required"`
	Description     string     `json:"description,omitempty"`
	Identifier      string     `json:"identifier,omitempty"`
	ResourceType    string     `json:"resource_type,omitempty"`
	ResourceName    string     `json:"resource_name,omitempty"`
	FixAvailable    *bool      `json:"fix_available,omitempty"`
	FixVersion      string     `json:"fix_version,omitempty"`
	FixInstructions string     `json:"fix_instructions,omitempty"`
}

type UpdateSecurityFindingRequest struct {
	Status          *string `json:"status,omitempty"`
	ResolutionNotes *string `json:"resolution_notes,omitempty"`
}

type SecurityFindingFilter struct {
	Severity     string `form:"severity"`
	Status       string `form:"status"`
	FindingType  string `form:"finding_type"`
	ServiceID    string `form:"service_id"`
	Search       string `form:"search"`
	Page         int    `form:"page,default=1"`
	PerPage      int    `form:"per_page,default=50"`
}

type SecurityFindingSummary struct {
	Total       int            `json:"total"`
	BySeverity  map[string]int `json:"by_severity"`
	ByStatus    map[string]int `json:"by_status"`
	ByType      map[string]int `json:"by_type"`
	OpenCount   int            `json:"open_count"`
	CriticalCount int          `json:"critical_count"`
}

// ============================================================
// SecretRotation — track secret rotation schedules
// ============================================================

type SecretRotation struct {
	ID                 uuid.UUID   `json:"id" db:"id"`
	TenantID           uuid.UUID   `json:"tenant_id" db:"tenant_id"`
	Name               string      `json:"name" db:"name"`
	Description        string      `json:"description,omitempty" db:"description"`
	SecretPath         string      `json:"secret_path" db:"secret_path"`
	RotationType       string      `json:"rotation_type" db:"rotation_type"` // 'scheduled' | 'on_demand' | 'on_expiry'
	CronExpression     string      `json:"cron_expression,omitempty" db:"cron_expression"`
	RotationIntervalDays int       `json:"rotation_interval_days,omitempty" db:"rotation_interval_days"`
	LastRotatedAt      *time.Time  `json:"last_rotated_at,omitempty" db:"last_rotated_at"`
	LastRotatedBy      *uuid.UUID  `json:"last_rotated_by,omitempty" db:"last_rotated_by"`
	NextRotationAt     *time.Time  `json:"next_rotation_at,omitempty" db:"next_rotation_at"`
	ExpiresAt          *time.Time  `json:"expires_at,omitempty" db:"expires_at"`
	Status             string      `json:"status" db:"status"` // 'active' | 'paused' | 'expired' | 'failed'
	RotationCount      int         `json:"rotation_count" db:"rotation_count"`
	ServiceIDs         []uuid.UUID `json:"service_ids" db:"service_ids"`
	Enabled            bool        `json:"enabled" db:"enabled"`
	LastError          string      `json:"last_error,omitempty" db:"last_error"`
	LastErrorAt        *time.Time  `json:"last_error_at,omitempty" db:"last_error_at"`
	CreatedBy          *uuid.UUID  `json:"created_by,omitempty" db:"created_by"`
	CreatedAt          time.Time   `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time   `json:"updated_at" db:"updated_at"`
}

type CreateSecretRotationRequest struct {
	Name                 string      `json:"name" binding:"required"`
	Description          string      `json:"description,omitempty"`
	SecretPath           string      `json:"secret_path" binding:"required"`
	RotationType         string      `json:"rotation_type" binding:"required,oneof=scheduled on_demand on_expiry"`
	CronExpression       string      `json:"cron_expression,omitempty"`
	RotationIntervalDays *int        `json:"rotation_interval_days,omitempty"`
	ExpiresAt            *time.Time  `json:"expires_at,omitempty"`
	ServiceIDs           []uuid.UUID `json:"service_ids,omitempty"`
	Enabled              *bool       `json:"enabled,omitempty"`
}

type UpdateSecretRotationRequest struct {
	Name                 *string     `json:"name,omitempty"`
	Description          *string     `json:"description,omitempty"`
	SecretPath           *string     `json:"secret_path,omitempty"`
	RotationType         *string     `json:"rotation_type,omitempty"`
	CronExpression       *string     `json:"cron_expression,omitempty"`
	RotationIntervalDays *int        `json:"rotation_interval_days,omitempty"`
	ExpiresAt            *time.Time  `json:"expires_at,omitempty"`
	ServiceIDs           []uuid.UUID `json:"service_ids,omitempty"`
	Enabled              *bool       `json:"enabled,omitempty"`
	Status               *string     `json:"status,omitempty"`
}

type SecretRotationLog struct {
	ID           uuid.UUID       `json:"id" db:"id"`
	TenantID     uuid.UUID       `json:"tenant_id" db:"tenant_id"`
	RotationID   uuid.UUID       `json:"rotation_id" db:"rotation_id"`
	Status       string          `json:"status" db:"status"` // 'success' | 'failed' | 'skipped'
	Details      json.RawMessage `json:"details,omitempty" db:"details"`
	ErrorMessage string          `json:"error_message,omitempty" db:"error_message"`
	TriggeredBy  *uuid.UUID      `json:"triggered_by,omitempty" db:"triggered_by"`
	TriggerType  string          `json:"trigger_type" db:"trigger_type"` // 'scheduled' | 'manual' | 'on_expiry'
	ExecutedAt   time.Time       `json:"executed_at" db:"executed_at"`
}

// ============================================================
// DeploymentAuditLog — detailed audit for deployments
// ============================================================

type DeploymentAuditLog struct {
	ID                  uuid.UUID       `json:"id" db:"id"`
	TenantID            uuid.UUID       `json:"tenant_id" db:"tenant_id"`
	DeploymentID        *uuid.UUID      `json:"deployment_id,omitempty" db:"deployment_id"`
	ServiceID           *uuid.UUID      `json:"service_id,omitempty" db:"service_id"`
	Action              string          `json:"action" db:"action"` // 'deploy' | 'rollback' | 'promote' | 'scale' | 'restart' | 'cancel' | 'verify'
	ActorType           string          `json:"actor_type" db:"actor_type"` // 'user' | 'system' | 'workflow' | 'api_key'
	ActorID             *uuid.UUID      `json:"actor_id,omitempty" db:"actor_id"`
	ActorName           string          `json:"actor_name,omitempty" db:"actor_name"`
	Environment         string          `json:"environment,omitempty" db:"environment"`
	ClusterID           *uuid.UUID      `json:"cluster_id,omitempty" db:"cluster_id"`
	Namespace           string          `json:"namespace,omitempty" db:"namespace"`
	ImageTag            string          `json:"image_tag,omitempty" db:"image_tag"`
	PreviousState       json.RawMessage `json:"previous_state,omitempty" db:"previous_state"`
	NewState            json.RawMessage `json:"new_state,omitempty" db:"new_state"`
	ComplianceResults   json.RawMessage `json:"compliance_results,omitempty" db:"compliance_results"`
	SecurityGateResults json.RawMessage `json:"security_gate_results,omitempty" db:"security_gate_results"`
	RiskScore           *int            `json:"risk_score,omitempty" db:"risk_score"`
	RiskFactors         json.RawMessage `json:"risk_factors,omitempty" db:"risk_factors"`
	Status              string          `json:"status" db:"status"` // 'success' | 'failed' | 'blocked' | 'rolled_back'
	ErrorMessage        string          `json:"error_message,omitempty" db:"error_message"`
	Metadata            json.RawMessage `json:"metadata,omitempty" db:"metadata"`
	IPAddress           string          `json:"ip_address,omitempty" db:"ip_address"`
	UserAgent           string          `json:"user_agent,omitempty" db:"user_agent"`
	RequestID           string          `json:"request_id,omitempty" db:"request_id"`
	CreatedAt           time.Time       `json:"created_at" db:"created_at"`
	// Joined fields
	ServiceName string `json:"service_name,omitempty" db:"service_name"`
}

type DeploymentAuditFilter struct {
	Action      string `form:"action"`
	ServiceID   string `form:"service_id"`
	Environment string `form:"environment"`
	Status      string `form:"status"`
	ActorType   string `form:"actor_type"`
	StartDate   string `form:"start_date"`
	EndDate     string `form:"end_date"`
	Page        int    `form:"page,default=1"`
	PerPage     int    `form:"per_page,default=50"`
}

type DeploymentAuditListResponse struct {
	Items      []DeploymentAuditLog `json:"items"`
	Total      int64                `json:"total"`
	Page       int                  `json:"page"`
	PerPage    int                  `json:"per_page"`
	TotalPages int                  `json:"total_pages"`
}

// ============================================================
// Pre-Deploy Gate — combined security + compliance check
// ============================================================

type PreDeployGateRequest struct {
	ServiceID   uuid.UUID       `json:"service_id" binding:"required"`
	Environment string          `json:"environment" binding:"required"`
	ImageTag    string          `json:"image_tag,omitempty"`
	ClusterID   *uuid.UUID      `json:"cluster_id,omitempty"`
	Config      json.RawMessage `json:"config,omitempty"` // Resource config, etc.
}

type PreDeployGateResult struct {
	Allowed          bool                   `json:"allowed"`
	WindowCheck      *WindowCheckResult     `json:"window_check,omitempty"`
	ComplianceCheck  *ComplianceCheckResult `json:"compliance_check,omitempty"`
	SecurityCheck    *SecurityCheckResult   `json:"security_check,omitempty"`
	BlockedReasons   []string               `json:"blocked_reasons,omitempty"`
	Warnings         []string               `json:"warnings,omitempty"`
}

type SecurityCheckResult struct {
	Passed          bool              `json:"passed"`
	OpenFindings    int               `json:"open_findings"`
	CriticalCount   int               `json:"critical_count"`
	HighCount       int               `json:"high_count"`
	BlockingIssues  []string          `json:"blocking_issues,omitempty"`
}
