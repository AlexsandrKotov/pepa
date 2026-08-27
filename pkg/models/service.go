package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ============================================================
// ServiceTemplate — reusable template for creating services
// ============================================================

type ServiceTemplate struct {
	ID               uuid.UUID       `json:"id" db:"id"`
	TenantID         uuid.UUID       `json:"tenant_id" db:"tenant_id"`
	Name             string          `json:"name" db:"name"`
	Slug             string          `json:"slug" db:"slug"`
	Description      string          `json:"description,omitempty" db:"description"`
	Category         string          `json:"category" db:"category"`
	Icon             string          `json:"icon,omitempty" db:"icon"`
	Language         string          `json:"language,omitempty" db:"language"`
	Framework        string          `json:"framework,omitempty" db:"framework"`
	DockerfileTmpl   string          `json:"dockerfile_tmpl,omitempty" db:"dockerfile_tmpl"`
	HelmChart        json.RawMessage `json:"helm_chart,omitempty" db:"helm_chart"`
	CICDTmpl         string          `json:"cicd_tmpl,omitempty" db:"cicd_tmpl"`
	DefaultValues    json.RawMessage `json:"default_values,omitempty" db:"default_values"`
	ResourceDefaults json.RawMessage `json:"resource_defaults,omitempty" db:"resource_defaults"`
	Tags             []string        `json:"tags" db:"tags"`
	IsEnabled        bool            `json:"is_enabled" db:"is_enabled"`
	IsSystem         bool            `json:"is_system" db:"is_system"`
	CreatedBy        *uuid.UUID      `json:"created_by,omitempty" db:"created_by"`
	CreatedAt        time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at" db:"updated_at"`
}

// ============================================================
// Service — a service created from a template
// ============================================================

type Service struct {
	ID                 uuid.UUID       `json:"id" db:"id"`
	TenantID           uuid.UUID       `json:"tenant_id" db:"tenant_id"`
	TemplateID         *uuid.UUID      `json:"template_id,omitempty" db:"template_id"`
	Name               string          `json:"name" db:"name"`
	Slug               string          `json:"slug" db:"slug"`
	Description        string          `json:"description,omitempty" db:"description"`
	OwnerTeamID        *uuid.UUID      `json:"owner_team_id,omitempty" db:"owner_team_id"`
	Language           string          `json:"language,omitempty" db:"language"`
	Framework          string          `json:"framework,omitempty" db:"framework"`
	GitLabProjectURL   string          `json:"gitlab_project_url,omitempty" db:"gitlab_project_url"`
	HelmChartURL       string          `json:"helm_chart_url,omitempty" db:"helm_chart_url"`
	ImageRepository    string          `json:"image_repository,omitempty" db:"image_repository"`
	Namespace          string          `json:"namespace" db:"namespace"`
	Status             string          `json:"status" db:"status"`
	ResourceConfig     json.RawMessage `json:"resource_config,omitempty" db:"resource_config"`
	EnvironmentVars    json.RawMessage `json:"environment_variables,omitempty" db:"environment_variables"`
	VaultSecrets       json.RawMessage `json:"vault_secrets,omitempty" db:"vault_secrets"`
	DeploymentStrategy string          `json:"deployment_strategy" db:"deployment_strategy"`
	TargetClusters     []uuid.UUID     `json:"target_clusters" db:"target_clusters"`
	Metadata           json.RawMessage `json:"metadata,omitempty" db:"metadata"`
	CreatedBy          *uuid.UUID      `json:"created_by,omitempty" db:"created_by"`
	CreatedAt          time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at" db:"updated_at"`
}

// ============================================================
// ServiceDeployment — tracks deployment per environment
// ============================================================

type ServiceDeployment struct {
	ID                  uuid.UUID       `json:"id" db:"id"`
	TenantID            uuid.UUID       `json:"tenant_id" db:"tenant_id"`
	ServiceID           uuid.UUID       `json:"service_id" db:"service_id"`
	Environment         string          `json:"environment" db:"environment"`
	ClusterID           *uuid.UUID      `json:"cluster_id,omitempty" db:"cluster_id"`
	Namespace           string          `json:"namespace,omitempty" db:"namespace"`
	Branch              string          `json:"branch,omitempty" db:"branch"`
	ImageTag            string          `json:"image_tag,omitempty" db:"image_tag"`
	HelmRelease         string          `json:"helm_release,omitempty" db:"helm_release"`
	DeployType          string          `json:"deploy_type" db:"deploy_type"`
	Status              string          `json:"status" db:"status"`
	VerificationStatus  string          `json:"verification_status" db:"verification_status"`
	VerificationDetails json.RawMessage `json:"verification_details,omitempty" db:"verification_details"`
	FluxSynced          bool            `json:"flux_synced" db:"flux_synced"`
	PodsReady           int             `json:"pods_ready" db:"pods_ready"`
	PodsTotal           int             `json:"pods_total" db:"pods_total"`
	MRUrl               string          `json:"mr_url,omitempty" db:"mr_url"`
	PipelineURL         string          `json:"pipeline_url,omitempty" db:"pipeline_url"`
	DeployedAt          *time.Time      `json:"deployed_at,omitempty" db:"deployed_at"`
	VerifiedAt          *time.Time      `json:"verified_at,omitempty" db:"verified_at"`
	PromotedAt          *time.Time      `json:"promoted_at,omitempty" db:"promoted_at"`
	CreatedAt           time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at" db:"updated_at"`
}

// ── Request/Response types ──────────────────────────────────

type CreateServiceRequest struct {
	TemplateSlug       string                 `json:"template_slug,omitempty"`
	Name               string                 `json:"name" binding:"required"`
	Slug               string                 `json:"slug,omitempty"`
	Description        string                 `json:"description,omitempty"`
	Language           string                 `json:"language,omitempty"`
	Framework          string                 `json:"framework,omitempty"`
	Namespace          string                 `json:"namespace,omitempty"`
	DeploymentStrategy string                 `json:"deployment_strategy,omitempty"`
	ResourceConfig     map[string]interface{} `json:"resource_config,omitempty"`
	EnvironmentVars    map[string]string      `json:"environment_variables,omitempty"`
	GitLabProjectURL   string                 `json:"gitlab_project_url,omitempty"`
	HelmChartURL       string                 `json:"helm_chart_url,omitempty"`
	ImageRepository    string                 `json:"image_repository,omitempty"`
	TargetClusterIDs   []string               `json:"target_clusters,omitempty"`
	Metadata           map[string]interface{} `json:"metadata,omitempty"`
}

type UpdateServiceRequest struct {
	Name               *string         `json:"name,omitempty"`
	Description        *string         `json:"description,omitempty"`
	Namespace          *string         `json:"namespace,omitempty"`
	DeploymentStrategy *string         `json:"deployment_strategy,omitempty"`
	ResourceConfig     json.RawMessage `json:"resource_config,omitempty"`
	EnvironmentVars    json.RawMessage `json:"environment_variables,omitempty"`
	Status             *string         `json:"status,omitempty"`
	Metadata           json.RawMessage `json:"metadata,omitempty"`
}

type ServiceFilter struct {
	Status   string `form:"status"`
	Template string `form:"template"`
	Search   string `form:"search"`
	Page     int    `form:"page,default=1"`
	PerPage  int    `form:"per_page,default=20"`
}

type ServiceListResponse struct {
	Items      []Service `json:"items"`
	Total      int64     `json:"total"`
	Page       int       `json:"page"`
	PerPage    int       `json:"per_page"`
	TotalPages int       `json:"total_pages"`
}

type DeployServiceRequest struct {
	Environment string `json:"environment" binding:"required"`
	ClusterID   string `json:"cluster_id,omitempty"`
	Branch      string `json:"branch,omitempty"`
	ImageTag    string `json:"image_tag,omitempty"`
	DeployType  string `json:"deploy_type,omitempty"`
}

type VerifyDeploymentRequest struct {
	Checks []string `json:"checks,omitempty"`
}
