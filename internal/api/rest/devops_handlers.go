package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/repository"
	"github.com/pepa/pepa/pkg/models"
)

// DevOpsHandlers handles DevOps/DevSecOps HTTP requests.
type DevOpsHandlers struct {
	repo *repository.DevOpsRepository
	deps Dependencies
}

// NewDevOpsHandlers creates new DevOps handlers.
func NewDevOpsHandlers(repo *repository.DevOpsRepository, deps Dependencies) *DevOpsHandlers {
	return &DevOpsHandlers{repo: repo, deps: deps}
}

// registerDevOpsRoutes registers all DevOps/DevSecOps API routes.
func registerDevOpsRoutes(v1 *gin.RouterGroup, deps Dependencies) {
	if deps.Repos == nil || deps.Repos.DevOps == nil {
		return
	}
	h := NewDevOpsHandlers(deps.Repos.DevOps, deps)

	// ── Deployment Windows ──────────────────────────────────────
	windows := v1.Group("/deployment-windows")
	{
		windows.GET("", h.ListDeploymentWindows)
		windows.POST("", h.CreateDeploymentWindow)
		windows.GET("/:id", h.GetDeploymentWindow)
		windows.PUT("/:id", h.UpdateDeploymentWindow)
		windows.DELETE("/:id", h.DeleteDeploymentWindow)
		windows.POST("/check", h.CheckDeploymentWindow)
	}

	// ── Batch Operations ────────────────────────────────────────
	batch := v1.Group("/batch-operations")
	{
		batch.GET("", h.ListBatchOperations)
		batch.POST("", h.CreateBatchOperation)
		batch.GET("/:id", h.GetBatchOperation)
		batch.POST("/:id/cancel", h.CancelBatchOperation)
	}

	// ── Compliance Policies ─────────────────────────────────────
	policies := v1.Group("/compliance-policies")
	{
		policies.GET("", h.ListCompliancePolicies)
		policies.POST("", h.CreateCompliancePolicy)
		policies.GET("/:id", h.GetCompliancePolicy)
		policies.PUT("/:id", h.UpdateCompliancePolicy)
		policies.DELETE("/:id", h.DeleteCompliancePolicy)
		policies.GET("/evaluations", h.ListComplianceEvaluations)
	}

	// ── Security Findings ───────────────────────────────────────
	findings := v1.Group("/security-findings")
	{
		findings.GET("", h.ListSecurityFindings)
		findings.GET("/summary", h.GetSecurityFindingSummary)
		findings.POST("", h.CreateSecurityFinding)
		findings.GET("/:id", h.GetSecurityFinding)
		findings.PUT("/:id", h.UpdateSecurityFinding)
	}

	// ── Secret Rotations ────────────────────────────────────────
	rotations := v1.Group("/secret-rotations")
	{
		rotations.GET("", h.ListSecretRotations)
		rotations.POST("", h.CreateSecretRotation)
		rotations.GET("/expiring", h.GetExpiringSecrets)
		rotations.GET("/:id", h.GetSecretRotation)
		rotations.PUT("/:id", h.UpdateSecretRotation)
		rotations.DELETE("/:id", h.DeleteSecretRotation)
		rotations.POST("/:id/rotate", h.TriggerRotation)
		rotations.GET("/:id/logs", h.GetSecretRotationLogs)
	}

	// ── Deployment Audit Trail ──────────────────────────────────
	audit := v1.Group("/deployment-audit")
	{
		audit.GET("", h.ListDeploymentAuditLogs)
		audit.GET("/deployment/:deploymentId", h.GetDeploymentAuditHistory)
	}

	// ── Pre-Deploy Gate (combined check) ────────────────────────
	v1.POST("/pre-deploy-gate", h.PreDeployGate)
}

// ============================================================
// Deployment Windows Handlers
// ============================================================

// ListDeploymentWindows returns all deployment windows.
func (h *DevOpsHandlers) ListDeploymentWindows(c *gin.Context) {
	tenantID := auth.GetTenantID(c)
	windows, err := h.repo.ListDeploymentWindows(c.Request.Context(), tenantID)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"windows": windows,
		"total":   len(windows),
	})
}

// GetDeploymentWindow returns a deployment window by ID.
func (h *DevOpsHandlers) GetDeploymentWindow(c *gin.Context) {
	tenantID := auth.GetTenantID(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid window ID"})
		return
	}
	window, err := h.repo.GetDeploymentWindow(c.Request.Context(), id, tenantID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, window)
}

// CreateDeploymentWindow creates a new deployment window.
func (h *DevOpsHandlers) CreateDeploymentWindow(c *gin.Context) {
	tenantID := auth.GetTenantID(c)
	userID := auth.GetUserID(c)

	var req models.CreateDeploymentWindowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	w := &models.DeploymentWindow{
		TenantID:      tenantID,
		Name:          req.Name,
		Description:   req.Description,
		WindowType:    req.WindowType,
		CronExpression: req.CronExpression,
		StartAt:       req.StartAt,
		EndAt:         req.EndAt,
		Timezone:      req.Timezone,
		Environments:  req.Environments,
		ServiceIDs:    req.ServiceIDs,
		Enabled:       true,
		Priority:      0,
		Reason:        req.Reason,
		OverrideRoles: req.OverrideRoles,
		CreatedBy:     userID,
	}

	if req.Enabled != nil {
		w.Enabled = *req.Enabled
	}
	if req.Priority != nil {
		w.Priority = *req.Priority
	}
	if w.Timezone == "" {
		w.Timezone = "UTC"
	}

	if err := h.repo.CreateDeploymentWindow(c.Request.Context(), w); err != nil {
		respondInternalError(c, err)
		return
	}

	logAudit(h.deps, c, "create", "deployment_window", w.ID.String(), nil, w)
	c.JSON(http.StatusCreated, w)
}

// UpdateDeploymentWindow updates a deployment window.
func (h *DevOpsHandlers) UpdateDeploymentWindow(c *gin.Context) {
	tenantID := auth.GetTenantID(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid window ID"})
		return
	}

	existing, err := h.repo.GetDeploymentWindow(c.Request.Context(), id, tenantID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	var req models.UpdateDeploymentWindowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.Description != nil {
		existing.Description = *req.Description
	}
	if req.WindowType != nil {
		existing.WindowType = *req.WindowType
	}
	if req.CronExpression != nil {
		existing.CronExpression = *req.CronExpression
	}
	if req.StartAt != nil {
		existing.StartAt = req.StartAt
	}
	if req.EndAt != nil {
		existing.EndAt = req.EndAt
	}
	if req.Timezone != nil {
		existing.Timezone = *req.Timezone
	}
	if req.Environments != nil {
		existing.Environments = req.Environments
	}
	if req.ServiceIDs != nil {
		existing.ServiceIDs = req.ServiceIDs
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}
	if req.Priority != nil {
		existing.Priority = *req.Priority
	}
	if req.Reason != nil {
		existing.Reason = *req.Reason
	}
	if req.OverrideRoles != nil {
		existing.OverrideRoles = req.OverrideRoles
	}

	if err := h.repo.UpdateDeploymentWindow(c.Request.Context(), existing); err != nil {
		respondInternalError(c, err)
		return
	}

	logAudit(h.deps, c, "update", "deployment_window", id.String(), nil, existing)
	c.JSON(http.StatusOK, existing)
}

// DeleteDeploymentWindow deletes a deployment window.
func (h *DevOpsHandlers) DeleteDeploymentWindow(c *gin.Context) {
	tenantID := auth.GetTenantID(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid window ID"})
		return
	}

	if err := h.repo.DeleteDeploymentWindow(c.Request.Context(), id, tenantID); err != nil {
		respondInternalError(c, err)
		return
	}

	logAudit(h.deps, c, "delete", "deployment_window", id.String(), nil, nil)
	c.JSON(http.StatusOK, gin.H{"message": "Deployment window deleted"})
}

// CheckDeploymentWindow checks if deployment is currently allowed.
func (h *DevOpsHandlers) CheckDeploymentWindow(c *gin.Context) {
	tenantID := auth.GetTenantID(c)

	var req struct {
		Environment string    `json:"environment" binding:"required"`
		ServiceID   *uuid.UUID `json:"service_id,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get user roles for override check
	userRoles := auth.GetRoles(c)

	result, err := h.repo.CheckDeploymentAllowed(c.Request.Context(), tenantID, req.Environment, req.ServiceID, userRoles)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// ============================================================
// Batch Operations Handlers
// ============================================================

// ListBatchOperations returns all batch operations.
func (h *DevOpsHandlers) ListBatchOperations(c *gin.Context) {
	tenantID := auth.GetTenantID(c)
	ops, err := h.repo.ListBatchOperations(c.Request.Context(), tenantID)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"operations": ops,
		"total":      len(ops),
	})
}

// GetBatchOperation returns a batch operation by ID.
func (h *DevOpsHandlers) GetBatchOperation(c *gin.Context) {
	tenantID := auth.GetTenantID(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid operation ID"})
		return
	}
	op, err := h.repo.GetBatchOperation(c.Request.Context(), id, tenantID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, op)
}

// CreateBatchOperation creates a new batch operation.
func (h *DevOpsHandlers) CreateBatchOperation(c *gin.Context) {
	tenantID := auth.GetTenantID(c)
	userID := auth.GetUserID(c)

	var req models.CreateBatchOperationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	op := &models.BatchOperation{
		TenantID:       tenantID,
		Name:           req.Name,
		OperationType:  req.OperationType,
		Status:         "pending",
		ServiceIDs:     req.ServiceIDs,
		Parameters:     req.Parameters,
		Results:        json.RawMessage("{}"),
		InitiatedBy:    userID,
		Reason:         req.Reason,
		IncidentID:     req.IncidentID,
		TimeoutSeconds: 3600,
	}

	if req.TimeoutSeconds != nil {
		op.TimeoutSeconds = *req.TimeoutSeconds
	}

	if err := h.repo.CreateBatchOperation(c.Request.Context(), op); err != nil {
		respondInternalError(c, err)
		return
	}

	logAudit(h.deps, c, "create", "batch_operation", op.ID.String(), nil, op)
	c.JSON(http.StatusCreated, op)
}

// CancelBatchOperation cancels a pending/running batch operation.
func (h *DevOpsHandlers) CancelBatchOperation(c *gin.Context) {
	tenantID := auth.GetTenantID(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid operation ID"})
		return
	}

	if err := h.repo.CancelBatchOperation(c.Request.Context(), id, tenantID); err != nil {
		respondInternalError(c, err)
		return
	}

	logAudit(h.deps, c, "cancel", "batch_operation", id.String(), nil, nil)
	c.JSON(http.StatusOK, gin.H{"message": "Batch operation cancelled"})
}

// ============================================================
// Compliance Policies Handlers
// ============================================================

// ListCompliancePolicies returns all compliance policies.
func (h *DevOpsHandlers) ListCompliancePolicies(c *gin.Context) {
	tenantID := auth.GetTenantID(c)
	policies, err := h.repo.ListCompliancePolicies(c.Request.Context(), tenantID)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"policies": policies,
		"total":    len(policies),
	})
}

// GetCompliancePolicy returns a compliance policy by ID.
func (h *DevOpsHandlers) GetCompliancePolicy(c *gin.Context) {
	tenantID := auth.GetTenantID(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid policy ID"})
		return
	}
	policy, err := h.repo.GetCompliancePolicy(c.Request.Context(), id, tenantID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, policy)
}

// CreateCompliancePolicy creates a new compliance policy.
func (h *DevOpsHandlers) CreateCompliancePolicy(c *gin.Context) {
	tenantID := auth.GetTenantID(c)
	userID := auth.GetUserID(c)

	var req models.CreateCompliancePolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	p := &models.CompliancePolicy{
		TenantID:     tenantID,
		Name:         req.Name,
		Description:  req.Description,
		PolicyType:   req.PolicyType,
		PolicySpec:   req.PolicySpec,
		Severity:     req.Severity,
		Blocking:     req.Severity == "block",
		Environments: req.Environments,
		ServiceIDs:   req.ServiceIDs,
		Enabled:      true,
		CreatedBy:    userID,
	}

	if req.Blocking != nil {
		p.Blocking = *req.Blocking
	}
	if req.Enabled != nil {
		p.Enabled = *req.Enabled
	}

	if err := h.repo.CreateCompliancePolicy(c.Request.Context(), p); err != nil {
		respondInternalError(c, err)
		return
	}

	logAudit(h.deps, c, "create", "compliance_policy", p.ID.String(), nil, p)
	c.JSON(http.StatusCreated, p)
}

// UpdateCompliancePolicy updates a compliance policy.
func (h *DevOpsHandlers) UpdateCompliancePolicy(c *gin.Context) {
	tenantID := auth.GetTenantID(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid policy ID"})
		return
	}

	existing, err := h.repo.GetCompliancePolicy(c.Request.Context(), id, tenantID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	var req models.UpdateCompliancePolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.Description != nil {
		existing.Description = *req.Description
	}
	if req.PolicyType != nil {
		existing.PolicyType = *req.PolicyType
	}
	if req.PolicySpec != nil {
		existing.PolicySpec = req.PolicySpec
	}
	if req.Severity != nil {
		existing.Severity = *req.Severity
	}
	if req.Blocking != nil {
		existing.Blocking = *req.Blocking
	}
	if req.Environments != nil {
		existing.Environments = req.Environments
	}
	if req.ServiceIDs != nil {
		existing.ServiceIDs = req.ServiceIDs
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}

	if err := h.repo.UpdateCompliancePolicy(c.Request.Context(), existing); err != nil {
		respondInternalError(c, err)
		return
	}

	logAudit(h.deps, c, "update", "compliance_policy", id.String(), nil, existing)
	c.JSON(http.StatusOK, existing)
}

// DeleteCompliancePolicy deletes a compliance policy.
func (h *DevOpsHandlers) DeleteCompliancePolicy(c *gin.Context) {
	tenantID := auth.GetTenantID(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid policy ID"})
		return
	}

	if err := h.repo.DeleteCompliancePolicy(c.Request.Context(), id, tenantID); err != nil {
		respondInternalError(c, err)
		return
	}

	logAudit(h.deps, c, "delete", "compliance_policy", id.String(), nil, nil)
	c.JSON(http.StatusOK, gin.H{"message": "Compliance policy deleted"})
}

// ListComplianceEvaluations returns compliance evaluations.
func (h *DevOpsHandlers) ListComplianceEvaluations(c *gin.Context) {
	tenantID := auth.GetTenantID(c)
	var serviceID *uuid.UUID
	if sid := c.Query("service_id"); sid != "" {
		parsed, err := uuid.Parse(sid)
		if err == nil {
			serviceID = &parsed
		}
	}

	evals, err := h.repo.GetComplianceEvaluations(c.Request.Context(), tenantID, serviceID, 100)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"evaluations": evals,
		"total":       len(evals),
	})
}

// ============================================================
// Security Findings Handlers
// ============================================================

// ListSecurityFindings returns security findings with filtering.
func (h *DevOpsHandlers) ListSecurityFindings(c *gin.Context) {
	tenantID := auth.GetTenantID(c)

	var filter models.SecurityFindingFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PerPage <= 0 {
		filter.PerPage = 50
	}

	findings, total, err := h.repo.ListSecurityFindings(c.Request.Context(), tenantID, filter)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	totalPages := int(total) / filter.PerPage
	if int(total)%filter.PerPage > 0 {
		totalPages++
	}

	c.JSON(http.StatusOK, gin.H{
		"items":       findings,
		"total":       total,
		"page":        filter.Page,
		"per_page":    filter.PerPage,
		"total_pages": totalPages,
	})
}

// GetSecurityFindingSummary returns aggregated finding counts.
func (h *DevOpsHandlers) GetSecurityFindingSummary(c *gin.Context) {
	tenantID := auth.GetTenantID(c)
	summary, err := h.repo.GetSecurityFindingSummary(c.Request.Context(), tenantID)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, summary)
}

// GetSecurityFinding returns a security finding by ID.
func (h *DevOpsHandlers) GetSecurityFinding(c *gin.Context) {
	tenantID := auth.GetTenantID(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid finding ID"})
		return
	}
	finding, err := h.repo.GetSecurityFinding(c.Request.Context(), id, tenantID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, finding)
}

// CreateSecurityFinding creates a new security finding.
func (h *DevOpsHandlers) CreateSecurityFinding(c *gin.Context) {
	tenantID := auth.GetTenantID(c)

	var req models.CreateSecurityFindingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	f := &models.SecurityFinding{
		TenantID:        tenantID,
		ScanRunID:       req.ScanRunID,
		FindingType:     req.FindingType,
		Severity:        req.Severity,
		Title:           req.Title,
		Description:     req.Description,
		Identifier:      req.Identifier,
		ResourceType:    req.ResourceType,
		ResourceName:    req.ResourceName,
		FixAvailable:    false,
		FixVersion:      req.FixVersion,
		FixInstructions: req.FixInstructions,
		Status:          "open",
	}

	if req.FixAvailable != nil {
		f.FixAvailable = *req.FixAvailable
	}

	if err := h.repo.CreateSecurityFinding(c.Request.Context(), f); err != nil {
		respondInternalError(c, err)
		return
	}

	c.JSON(http.StatusCreated, f)
}

// UpdateSecurityFinding updates a security finding status.
func (h *DevOpsHandlers) UpdateSecurityFinding(c *gin.Context) {
	tenantID := auth.GetTenantID(c)
	userID := auth.GetUserID(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid finding ID"})
		return
	}

	existing, err := h.repo.GetSecurityFinding(c.Request.Context(), id, tenantID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	var req models.UpdateSecurityFindingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Status != nil {
		existing.Status = *req.Status
	}
	if req.ResolutionNotes != nil {
		existing.ResolutionNotes = *req.ResolutionNotes
	}
	if existing.Status != "open" {
		now := time.Now().UTC()
		existing.ResolvedBy = userID
		existing.ResolvedAt = &now
	}

	if err := h.repo.UpdateSecurityFinding(c.Request.Context(), existing); err != nil {
		respondInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, existing)
}

// ============================================================
// Secret Rotations Handlers
// ============================================================

// ListSecretRotations returns all secret rotations.
func (h *DevOpsHandlers) ListSecretRotations(c *gin.Context) {
	tenantID := auth.GetTenantID(c)
	rotations, err := h.repo.ListSecretRotations(c.Request.Context(), tenantID)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"rotations": rotations,
		"total":     len(rotations),
	})
}

// GetSecretRotation returns a secret rotation by ID.
func (h *DevOpsHandlers) GetSecretRotation(c *gin.Context) {
	tenantID := auth.GetTenantID(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rotation ID"})
		return
	}
	rotation, err := h.repo.GetSecretRotation(c.Request.Context(), id, tenantID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, rotation)
}

// CreateSecretRotation creates a new secret rotation.
func (h *DevOpsHandlers) CreateSecretRotation(c *gin.Context) {
	tenantID := auth.GetTenantID(c)
	userID := auth.GetUserID(c)

	var req models.CreateSecretRotationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	rot := &models.SecretRotation{
		TenantID:             tenantID,
		Name:                 req.Name,
		Description:          req.Description,
		SecretPath:           req.SecretPath,
		RotationType:         req.RotationType,
		CronExpression:       req.CronExpression,
		ExpiresAt:            req.ExpiresAt,
		ServiceIDs:           req.ServiceIDs,
		Enabled:              true,
		Status:               "active",
		CreatedBy:            userID,
	}

	if req.RotationIntervalDays != nil {
		rot.RotationIntervalDays = *req.RotationIntervalDays
	}
	if req.Enabled != nil {
		rot.Enabled = *req.Enabled
	}

	if err := h.repo.CreateSecretRotation(c.Request.Context(), rot); err != nil {
		respondInternalError(c, err)
		return
	}

	logAudit(h.deps, c, "create", "secret_rotation", rot.ID.String(), nil, rot)
	c.JSON(http.StatusCreated, rot)
}

// UpdateSecretRotation updates a secret rotation.
func (h *DevOpsHandlers) UpdateSecretRotation(c *gin.Context) {
	tenantID := auth.GetTenantID(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rotation ID"})
		return
	}

	existing, err := h.repo.GetSecretRotation(c.Request.Context(), id, tenantID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	var req models.UpdateSecretRotationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.Description != nil {
		existing.Description = *req.Description
	}
	if req.SecretPath != nil {
		existing.SecretPath = *req.SecretPath
	}
	if req.RotationType != nil {
		existing.RotationType = *req.RotationType
	}
	if req.CronExpression != nil {
		existing.CronExpression = *req.CronExpression
	}
	if req.RotationIntervalDays != nil {
		existing.RotationIntervalDays = *req.RotationIntervalDays
	}
	if req.ExpiresAt != nil {
		existing.ExpiresAt = req.ExpiresAt
	}
	if req.ServiceIDs != nil {
		existing.ServiceIDs = req.ServiceIDs
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}
	if req.Status != nil {
		existing.Status = *req.Status
	}

	if err := h.repo.UpdateSecretRotation(c.Request.Context(), existing); err != nil {
		respondInternalError(c, err)
		return
	}

	logAudit(h.deps, c, "update", "secret_rotation", id.String(), nil, existing)
	c.JSON(http.StatusOK, existing)
}

// DeleteSecretRotation deletes a secret rotation.
func (h *DevOpsHandlers) DeleteSecretRotation(c *gin.Context) {
	tenantID := auth.GetTenantID(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rotation ID"})
		return
	}

	if err := h.repo.DeleteSecretRotation(c.Request.Context(), id, tenantID); err != nil {
		respondInternalError(c, err)
		return
	}

	logAudit(h.deps, c, "delete", "secret_rotation", id.String(), nil, nil)
	c.JSON(http.StatusOK, gin.H{"message": "Secret rotation deleted"})
}

// TriggerRotation manually triggers a secret rotation.
func (h *DevOpsHandlers) TriggerRotation(c *gin.Context) {
	tenantID := auth.GetTenantID(c)
	userID := auth.GetUserID(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rotation ID"})
		return
	}

	rotation, err := h.repo.GetSecretRotation(c.Request.Context(), id, tenantID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// Create a log entry for the manual trigger
	log := &models.SecretRotationLog{
		TenantID:    tenantID,
		RotationID:  rotation.ID,
		Status:      "success", // Placeholder — actual rotation would be async
		TriggeredBy: userID,
		TriggerType: "manual",
	}

	if err := h.repo.CreateSecretRotationLog(c.Request.Context(), log); err != nil {
		respondInternalError(c, err)
		return
	}

	// Mark rotation as executed
	now := time.Now().UTC()
	if err := h.repo.MarkRotationExecuted(c.Request.Context(), rotation.ID, tenantID, &now); err != nil {
		respondInternalError(c, err)
		return
	}

	logAudit(h.deps, c, "rotate", "secret_rotation", id.String(), nil, gin.H{"trigger": "manual"})
	c.JSON(http.StatusOK, gin.H{"message": "Secret rotation triggered", "log_id": log.ID})
}

// GetSecretRotationLogs returns rotation execution logs.
func (h *DevOpsHandlers) GetSecretRotationLogs(c *gin.Context) {
	tenantID := auth.GetTenantID(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rotation ID"})
		return
	}

	logs, err := h.repo.GetSecretRotationLogs(c.Request.Context(), id, tenantID, 50)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"logs":  logs,
		"total": len(logs),
	})
}

// GetExpiringSecrets returns secrets expiring soon.
func (h *DevOpsHandlers) GetExpiringSecrets(c *gin.Context) {
	tenantID := auth.GetTenantID(c)
	within := 7 * 24 * time.Hour // Default: 7 days

	if d := c.Query("within_days"); d != "" {
		var days int
		if _, err := fmt.Sscanf(d, "%d", &days); err == nil && days > 0 {
			within = time.Duration(days) * 24 * time.Hour
		}
	}

	secrets, err := h.repo.GetExpiringSecrets(c.Request.Context(), tenantID, within)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"secrets":       secrets,
		"total":         len(secrets),
		"within_days":   int(within.Hours() / 24),
	})
}

// ============================================================
// Deployment Audit Trail Handlers
// ============================================================

// ListDeploymentAuditLogs returns audit logs with filtering.
func (h *DevOpsHandlers) ListDeploymentAuditLogs(c *gin.Context) {
	tenantID := auth.GetTenantID(c)

	var filter models.DeploymentAuditFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PerPage <= 0 {
		filter.PerPage = 50
	}

	logs, total, err := h.repo.ListDeploymentAuditLogs(c.Request.Context(), tenantID, filter)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	totalPages := int(total) / filter.PerPage
	if int(total)%filter.PerPage > 0 {
		totalPages++
	}

	c.JSON(http.StatusOK, models.DeploymentAuditListResponse{
		Items:      logs,
		Total:      total,
		Page:       filter.Page,
		PerPage:    filter.PerPage,
		TotalPages: totalPages,
	})
}

// GetDeploymentAuditHistory returns audit logs for a specific deployment.
func (h *DevOpsHandlers) GetDeploymentAuditHistory(c *gin.Context) {
	tenantID := auth.GetTenantID(c)
	deploymentID, err := uuid.Parse(c.Param("deploymentId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deployment ID"})
		return
	}

	logs, err := h.repo.GetDeploymentAuditHistory(c.Request.Context(), tenantID, deploymentID)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"logs":  logs,
		"total": len(logs),
	})
}

// ============================================================
// Pre-Deploy Gate — Combined Check
// ============================================================

// PreDeployGate performs a combined check: windows + compliance + security.
func (h *DevOpsHandlers) PreDeployGate(c *gin.Context) {
	tenantID := auth.GetTenantID(c)
	userRoles := auth.GetRoles(c)

	var req models.PreDeployGateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result := models.PreDeployGateResult{Allowed: true}

	// 1. Check deployment windows
	windowResult, err := h.repo.CheckDeploymentAllowed(c.Request.Context(), tenantID, req.Environment, req.ServiceID, userRoles)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	result.WindowCheck = windowResult
	if !windowResult.Allowed {
		result.Allowed = false
		result.BlockedReasons = append(result.BlockedReasons, windowResult.Reason)
	}

	// 2. Check compliance policies
	policies, err := h.repo.GetApplicablePolicies(c.Request.Context(), tenantID, req.Environment, req.ServiceID)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	complianceResult := &models.ComplianceCheckResult{Passed: true}
	for _, policy := range policies {
		evaluation := h.evaluatePolicy(c.Request.Context(), policy, req.Config, req.ImageTag)
		evaluation.ServiceID = req.ServiceID
		evaluation.TenantID = tenantID

		// Persist evaluation to DB for audit history
		_ = h.repo.CreateComplianceEvaluation(c.Request.Context(), evaluation)

		if evaluation.Result == "fail" {
			complianceResult.Passed = false
			if policy.Blocking {
				complianceResult.Blocked = true
				result.Allowed = false
				result.BlockedReasons = append(result.BlockedReasons, "Compliance policy '"+policy.Name+"' violated")
			} else {
				result.Warnings = append(result.Warnings, "Compliance warning: "+policy.Name)
			}
		}
		complianceResult.Evaluations = append(complianceResult.Evaluations, *evaluation)
	}
	result.ComplianceCheck = complianceResult

	// 3. Check security findings (if service has known resource name)
	blockingFindings, err := h.repo.GetBlockingSecurityFindings(c.Request.Context(), tenantID, req.ImageTag)
	if err == nil && len(blockingFindings) > 0 {
		secResult := &models.SecurityCheckResult{
			Passed:       false,
			OpenFindings: len(blockingFindings),
		}
		for _, f := range blockingFindings {
			if f.Severity == "critical" {
				secResult.CriticalCount++
			} else if f.Severity == "high" {
				secResult.HighCount++
			}
			secResult.BlockingIssues = append(secResult.BlockingIssues, f.Title+" ("+f.Severity+")")
		}
		if secResult.CriticalCount > 0 || secResult.HighCount > 0 {
			result.Allowed = false
			result.BlockedReasons = append(result.BlockedReasons, fmt.Sprintf("Security gate: %d blocking findings", len(secResult.BlockingIssues)))
		}
		result.SecurityCheck = secResult
	} else {
		result.SecurityCheck = &models.SecurityCheckResult{Passed: true}
	}

	// Log the gate check
	userID := auth.GetUserID(c)
	auditLog := &models.DeploymentAuditLog{
		TenantID:            tenantID,
		ServiceID:           req.ServiceID,
		Action:              "pre_deploy_gate",
		ActorType:           "user",
		ActorID:             userID,
		Environment:         req.Environment,
		ImageTag:            req.ImageTag,
		ComplianceResults:   mustJSONRaw(complianceResult),
		SecurityGateResults: mustJSONRaw(result.SecurityCheck),
		Status:              "success",
	}
	if !result.Allowed {
		auditLog.Status = "blocked"
		auditLog.Metadata = mustJSONRaw(gin.H{"blocked_reasons": result.BlockedReasons})
	}
	_ = h.repo.CreateDeploymentAuditLog(c.Request.Context(), auditLog)

	c.JSON(http.StatusOK, result)
}

// evaluatePolicy evaluates a single compliance policy against config.
func (h *DevOpsHandlers) evaluatePolicy(ctx context.Context, policy models.CompliancePolicy, config json.RawMessage, imageTag string) *models.ComplianceEvaluation {
	evaluation := &models.ComplianceEvaluation{
		PolicyID: policy.ID,
		TenantID: policy.TenantID,
		Result:   "pass",
	}

	// Parse policy spec
	var spec map[string]interface{}
	if err := json.Unmarshal(policy.PolicySpec, &spec); err != nil {
		evaluation.Result = "skip"
		evaluation.Violations = mustJSONRaw([]models.ComplianceViolation{{
			Field:    "policy_spec",
			Message:  "Invalid policy specification",
			Severity: "info",
		}})
		return evaluation
	}

	// Simple resource_limits policy check
	if policy.PolicyType == "resource_limits" && config != nil {
		var cfg map[string]interface{}
		if err := json.Unmarshal(config, &cfg); err != nil {
			return evaluation
		}

		var violations []models.ComplianceViolation

		// Check for required resource limits
		if resources, ok := cfg["resources"].(map[string]interface{}); ok {
			limits, hasLimits := resources["limits"]
			if !hasLimits || limits == nil {
				violations = append(violations, models.ComplianceViolation{
					Field:    "resources.limits",
					Message:  "Resource limits are required",
					Expected: "CPU and memory limits must be set",
					Actual:   "No limits defined",
					Severity: policy.Severity,
				})
			}
		} else {
			// No resources key at all
			if required, ok := spec["require_resource_limits"].(bool); ok && required {
				violations = append(violations, models.ComplianceViolation{
					Field:    "resources",
					Message:  "Resource configuration is required",
					Expected: "resources.limits.cpu and resources.limits.memory",
					Actual:   "No resources defined",
					Severity: policy.Severity,
				})
			}
		}

		if len(violations) > 0 {
			evaluation.Result = "fail"
			evaluation.Violations = mustJSONRaw(violations)
		}
	}

	// Security scan policy — check for blocking findings.
	// Fetch findings once and reuse for both require_recent_scan and max_critical checks.
	if policy.PolicyType == "security_scan" && imageTag != "" {
		requireRecentScan, _ := spec["require_recent_scan"].(bool)
		maxCriticalRaw, hasMaxCritical := spec["max_critical"].(float64)

		if requireRecentScan || hasMaxCritical {
			findings, err := h.repo.GetBlockingSecurityFindings(ctx, policy.TenantID, imageTag)
			if err != nil {
				slog.Warn("failed to fetch blocking security findings, skipping policy check",
					"policy_id", policy.ID, "image_tag", imageTag, "error", err)
				// Mark as skip so the gate doesn't silently pass
				evaluation.Result = "skip"
				evaluation.Violations = mustJSONRaw([]models.ComplianceViolation{{
					Field:    "security_scan",
					Message:  "Unable to verify security scan results",
					Severity: "warning",
				}})
				return evaluation
			}

			var violations []models.ComplianceViolation

			// Count critical findings once
			criticalCount := 0
			for _, f := range findings {
				if f.Severity == "critical" {
					criticalCount++
				}
			}

			// Check require_recent_scan: block if any critical findings exist
			if requireRecentScan && criticalCount > 0 {
				violations = append(violations, models.ComplianceViolation{
					Field:    "security_scan",
					Message:  fmt.Sprintf("%d open critical vulnerabilities found for %s", criticalCount, imageTag),
					Expected: "No critical vulnerabilities",
					Actual:   fmt.Sprintf("%d critical findings", criticalCount),
					Severity: policy.Severity,
				})
			}

			// Check max_critical threshold
			if hasMaxCritical && float64(criticalCount) > maxCriticalRaw {
				violations = append(violations, models.ComplianceViolation{
					Field:    "security_scan.max_critical",
					Message:  fmt.Sprintf("Too many critical vulnerabilities (%d > %.0f)", criticalCount, maxCriticalRaw),
					Expected: fmt.Sprintf("At most %.0f critical", maxCriticalRaw),
					Actual:   fmt.Sprintf("%d critical findings", criticalCount),
					Severity: policy.Severity,
				})
			}

			if len(violations) > 0 {
				evaluation.Result = "fail"
				evaluation.Violations = mustJSONRaw(violations)
			}
		}
	}

	return evaluation
}

// mustJSONRaw marshals to JSON as json.RawMessage, returning empty object on error.
func mustJSONRaw(v interface{}) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("{}")
	}
	return json.RawMessage(data)
}
