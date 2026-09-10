package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EnvironmentOverview represents the overview data for a single service in an environment.
type EnvironmentOverview struct {
	ServiceID       uuid.UUID  `json:"service_id"`
	ServiceName     string     `json:"service_name"`
	ServiceSlug     string     `json:"service_slug"`
	EnvironmentID   uuid.UUID  `json:"environment_id"`
	EnvironmentName string     `json:"environment_name"`
	EnvColor        string     `json:"env_color"`
	EnvSlug         string     `json:"env_slug"`
	Status          string     `json:"status"`           // deployed, deploying, failed, not_deployed, unknown
	ImageTag        string     `json:"image_tag"`        // deployed image tag or version
	DeploymentID    *uuid.UUID `json:"deployment_id"`    // latest deployment ID
	DeployedAt      *time.Time `json:"deployed_at"`      // when it was deployed
	HealthStatus    string     `json:"health_status"`    // healthy, degraded, unknown
	DriftCount      int        `json:"drift_count"`      // number of drift items
	BindingID       *uuid.UUID `json:"binding_id"`       // GitOps binding ID if exists
	EngineType      string     `json:"engine_type"`      // argocd, fluxcd, or empty
	SyncStatus      string     `json:"sync_status"`      // synced, out_of_sync, unknown
	ReplicasReady   int        `json:"replicas_ready"`   // ready replicas
	ReplicasDesired int        `json:"replicas_desired"` // desired replicas
}

// EnvironmentOverviewRow represents a row in the overview matrix (one service across all environments).
type EnvironmentOverviewRow struct {
	ServiceID    uuid.UUID            `json:"service_id"`
	ServiceName  string               `json:"service_name"`
	ServiceSlug  string               `json:"service_slug"`
	OwnerTeam    string               `json:"owner_team"`
	Status       string               `json:"status"`
	Environments map[string]*EnvironmentOverview `json:"environments"` // keyed by environment slug
}

// EnvironmentOverviewResponse is the full response for the overview endpoint.
type EnvironmentOverviewResponse struct {
	Environments []Environment               `json:"environments"`
	Services     []EnvironmentOverviewRow    `json:"services"`
	Problems     []EnvironmentProblem        `json:"problems"`
	Summary      EnvironmentOverviewSummary  `json:"summary"`
}

// EnvironmentProblem represents a problem detected in an environment.
type EnvironmentProblem struct {
	ID            string    `json:"id"`
	Type          string    `json:"type"`           // drift, failed_deploy, unhealthy, security
	Severity      string    `json:"severity"`       // critical, warning, info
	ServiceID     uuid.UUID `json:"service_id"`
	ServiceName   string    `json:"service_name"`
	EnvironmentID uuid.UUID `json:"environment_id"`
	EnvName       string    `json:"env_name"`
	Message       string    `json:"message"`
	Details       string    `json:"details"`
	DetectedAt    time.Time `json:"detected_at"`
}

// EnvironmentOverviewSummary holds aggregate counts.
type EnvironmentOverviewSummary struct {
	TotalServices      int `json:"total_services"`
	HealthyServices    int `json:"healthy_services"`
	DegradedServices   int `json:"degraded_services"`
	TotalDrifts        int `json:"total_drifts"`
	FailedDeployments  int `json:"failed_deployments"`
	TotalEnvironments  int `json:"total_environments"`
}

// EnvironmentOverviewRepository handles environment overview queries.
type EnvironmentOverviewRepository struct {
	pool *pgxpool.Pool
}

// NewEnvironmentOverviewRepository creates a new overview repository.
func NewEnvironmentOverviewRepository(pool *pgxpool.Pool) *EnvironmentOverviewRepository {
	return &EnvironmentOverviewRepository{pool: pool}
}

// GetOverview returns the full environment overview matrix for a tenant.
func (r *EnvironmentOverviewRepository) GetOverview(ctx context.Context, tenantID uuid.UUID) (*EnvironmentOverviewResponse, error) {
	// 1. Get all environments
	envs, err := r.getEnvironments(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("get environments: %w", err)
	}

	// 2. Get all services with their deployment status
	services, err := r.getServicesWithDeployments(ctx, tenantID, envs)
	if err != nil {
		return nil, fmt.Errorf("get services: %w", err)
	}

	// 3. Get drift counts
	driftCounts, err := r.getDriftCounts(ctx, tenantID)
	if err != nil {
		// Non-fatal: drift detection may not be configured
		driftCounts = make(map[string]int)
	}

	// 4. Get GitOps bindings
	bindings, err := r.getBindingsOverview(ctx, tenantID)
	if err != nil {
		bindings = make(map[string]*BindingOverview)
	}

	// 5. Build the matrix
	for i := range services {
		if services[i].Environments == nil {
			services[i].Environments = make(map[string]*EnvironmentOverview)
		}
		for _, env := range envs {
			overview := &EnvironmentOverview{
				ServiceID:       services[i].ServiceID,
				ServiceName:     services[i].ServiceName,
				ServiceSlug:     services[i].ServiceSlug,
				EnvironmentID:   env.ID,
				EnvironmentName: env.Name,
				EnvColor:        env.Color,
				EnvSlug:         env.Slug,
				Status:          "not_deployed",
				HealthStatus:    "unknown",
				SyncStatus:      "unknown",
			}

			// Check if there's a binding for this service+environment
			bindingKey := fmt.Sprintf("%s:%s", services[i].ServiceID.String(), env.ID.String())
			if b, ok := bindings[bindingKey]; ok {
				overview.BindingID = &b.BindingID
				overview.EngineType = b.EngineType
				overview.SyncStatus = b.SyncStatus
				if overview.Status == "not_deployed" && b.SyncStatus == "synced" {
					overview.Status = "deployed"
					overview.HealthStatus = "healthy"
				}
			}

			// Check for drift
			driftKey := fmt.Sprintf("%s:%s", services[i].ServiceID.String(), env.ID.String())
			if count, ok := driftCounts[driftKey]; ok {
				overview.DriftCount = count
			}

			services[i].Environments[env.Slug] = overview
		}
	}

	// 6. Build problems list
	problems := r.buildProblems(services, envs)

	// 7. Build summary
	summary := r.buildSummary(services, envs, problems)

	return &EnvironmentOverviewResponse{
		Environments: envs,
		Services:     services,
		Problems:     problems,
		Summary:      summary,
	}, nil
}

func (r *EnvironmentOverviewRepository) getEnvironments(ctx context.Context, tenantID uuid.UUID) ([]Environment, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, name, slug, COALESCE(type,''), COALESCE(cluster,''), COALESCE(namespace,''), 
		       COALESCE(status,'active'), description, color, is_default, created_at, updated_at
		FROM environments
		WHERE tenant_id = $1
		ORDER BY is_default DESC, name ASC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var envs []Environment
	for rows.Next() {
		var e Environment
		if err := rows.Scan(&e.ID, &e.TenantID, &e.Name, &e.Slug, &e.Type, &e.Cluster, &e.Namespace,
			&e.Status, &e.Description, &e.Color, &e.IsDefault, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		envs = append(envs, e)
	}
	return envs, nil
}

func (r *EnvironmentOverviewRepository) getServicesWithDeployments(ctx context.Context, tenantID uuid.UUID, envs []Environment) ([]EnvironmentOverviewRow, error) {
	// Get services with their latest deployment per environment
	rows, err := r.pool.Query(ctx, `
		WITH latest_deployments AS (
			SELECT DISTINCT ON (d.service_id, d.environment_id)
				d.service_id, d.environment_id, d.id as deployment_id, 
				d.status, d.image_tag, d.created_at, d.replicas_ready, d.replicas_desired
			FROM deployments d
			WHERE d.tenant_id = $1
			ORDER BY d.service_id, d.environment_id, d.created_at DESC
		)
		SELECT s.id, s.name, s.slug, s.status,
		       ld.environment_id, ld.deployment_id, ld.status as deploy_status,
		       ld.image_tag, ld.created_at as deployed_at,
		       ld.replicas_ready, ld.replicas_desired
		FROM services s
		LEFT JOIN latest_deployments ld ON ld.service_id = s.id
		WHERE s.tenant_id = $1
		ORDER BY s.name
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	serviceMap := make(map[uuid.UUID]*EnvironmentOverviewRow)
	for rows.Next() {
		var serviceID uuid.UUID
		var serviceName, serviceSlug, serviceStatus string
		var envID *uuid.UUID
		var deploymentID *uuid.UUID
		var deployStatus *string
		var imageTag *string
		var deployedAt *time.Time
		var replicasReady, replicasDesired *int

		if err := rows.Scan(&serviceID, &serviceName, &serviceSlug, &serviceStatus,
			&envID, &deploymentID, &deployStatus, &imageTag, &deployedAt,
			&replicasReady, &replicasDesired); err != nil {
			return nil, err
		}

		row, exists := serviceMap[serviceID]
		if !exists {
			row = &EnvironmentOverviewRow{
				ServiceID:    serviceID,
				ServiceName:  serviceName,
				ServiceSlug:  serviceSlug,
				Status:       serviceStatus,
				Environments: make(map[string]*EnvironmentOverview),
			}
			serviceMap[serviceID] = row
		}

		if envID != nil && deployStatus != nil {
			// Find the environment slug
			var envSlug, envName, envColor string
			for _, e := range envs {
				if e.ID == *envID {
					envSlug = e.Slug
					envName = e.Name
					envColor = e.Color
					break
				}
			}
			if envSlug == "" {
				continue
			}

			status := "not_deployed"
			healthStatus := "unknown"
			switch *deployStatus {
			case "deployed", "completed":
				status = "deployed"
				healthStatus = "healthy"
			case "deploying", "pending":
				status = "deploying"
				healthStatus = "unknown"
			case "failed", "error":
				status = "failed"
				healthStatus = "degraded"
			case "rolled_back":
				status = "deployed"
				healthStatus = "degraded"
			}

			overview := &EnvironmentOverview{
				ServiceID:       serviceID,
				ServiceName:     serviceName,
				ServiceSlug:     serviceSlug,
				EnvironmentID:   *envID,
				EnvironmentName: envName,
				EnvColor:        envColor,
				EnvSlug:         envSlug,
				Status:          status,
				DeploymentID:    deploymentID,
				HealthStatus:    healthStatus,
				SyncStatus:      "unknown",
			}
			if imageTag != nil {
				overview.ImageTag = *imageTag
			}
			if deployedAt != nil {
				overview.DeployedAt = deployedAt
			}
			if replicasReady != nil {
				overview.ReplicasReady = *replicasReady
			}
			if replicasDesired != nil {
				overview.ReplicasDesired = *replicasDesired
			}
			row.Environments[envSlug] = overview
		}
	}

	result := make([]EnvironmentOverviewRow, 0, len(serviceMap))
	for _, row := range serviceMap {
		result = append(result, *row)
	}
	return result, nil
}

type BindingOverview struct {
	BindingID  uuid.UUID
	EngineType string
	SyncStatus string
}

func (r *EnvironmentOverviewRepository) getBindingsOverview(ctx context.Context, tenantID uuid.UUID) (map[string]*BindingOverview, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, service_id, environment_id, engine_type, 
		       COALESCE(sync_status, 'unknown') as sync_status
		FROM gitops_application_bindings
		WHERE tenant_id = $1
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]*BindingOverview)
	for rows.Next() {
		var bindingID uuid.UUID
		var serviceID, envID *uuid.UUID
		var engineType, syncStatus string

		if err := rows.Scan(&bindingID, &serviceID, &envID, &engineType, &syncStatus); err != nil {
			return nil, err
		}

		if serviceID != nil && envID != nil {
			key := fmt.Sprintf("%s:%s", serviceID.String(), envID.String())
			result[key] = &BindingOverview{
				BindingID:  bindingID,
				EngineType: engineType,
				SyncStatus: syncStatus,
			}
		}
	}
	return result, nil
}

func (r *EnvironmentOverviewRepository) getDriftCounts(ctx context.Context, tenantID uuid.UUID) (map[string]int, error) {
	// Get drift counts per service+environment from drift detection results
	rows, err := r.pool.Query(ctx, `
		SELECT service_id, environment_id, COUNT(*) as drift_count
		FROM drift_detection_results
		WHERE tenant_id = $1 AND detected_at > NOW() - INTERVAL '24 hours'
		GROUP BY service_id, environment_id
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]int)
	for rows.Next() {
		var serviceID, envID uuid.UUID
		var count int
		if err := rows.Scan(&serviceID, &envID, &count); err != nil {
			return nil, err
		}
		key := fmt.Sprintf("%s:%s", serviceID.String(), envID.String())
		result[key] = count
	}
	return result, nil
}

func (r *EnvironmentOverviewRepository) buildProblems(services []EnvironmentOverviewRow, envs []Environment) []EnvironmentProblem {
	var problems []EnvironmentProblem
	envMap := make(map[uuid.UUID]Environment)
	for _, e := range envs {
		envMap[e.ID] = e
	}

	for _, svc := range services {
		for _, overview := range svc.Environments {
			// Failed deployments
			if overview.Status == "failed" {
				problems = append(problems, EnvironmentProblem{
					ID:            fmt.Sprintf("failed-%s-%s", svc.ServiceID.String(), overview.EnvSlug),
					Type:          "failed_deploy",
					Severity:      "critical",
					ServiceID:     svc.ServiceID,
					ServiceName:   svc.ServiceName,
					EnvironmentID: overview.EnvironmentID,
					EnvName:       overview.EnvironmentName,
					Message:       "Deployment failed",
					Details:       fmt.Sprintf("Service %s deployment to %s has failed", svc.ServiceName, overview.EnvironmentName),
					DetectedAt:    time.Now(),
				})
			}

			// Drift detected
			if overview.DriftCount > 0 {
				severity := "warning"
				if overview.DriftCount > 5 {
					severity = "critical"
				}
				problems = append(problems, EnvironmentProblem{
					ID:            fmt.Sprintf("drift-%s-%s", svc.ServiceID.String(), overview.EnvSlug),
					Type:          "drift",
					Severity:      severity,
					ServiceID:     svc.ServiceID,
					ServiceName:   svc.ServiceName,
					EnvironmentID: overview.EnvironmentID,
					EnvName:       overview.EnvironmentName,
					Message:       fmt.Sprintf("%d drift(s) detected", overview.DriftCount),
					Details:       fmt.Sprintf("Service %s has %d configuration drift(s) in %s", svc.ServiceName, overview.DriftCount, overview.EnvironmentName),
					DetectedAt:    time.Now(),
				})
			}

			// Unhealthy
			if overview.HealthStatus == "degraded" {
				problems = append(problems, EnvironmentProblem{
					ID:            fmt.Sprintf("unhealthy-%s-%s", svc.ServiceID.String(), overview.EnvSlug),
					Type:          "unhealthy",
					Severity:      "warning",
					ServiceID:     svc.ServiceID,
					ServiceName:   svc.ServiceName,
					EnvironmentID: overview.EnvironmentID,
					EnvName:       overview.EnvironmentName,
					Message:       "Service unhealthy",
					Details:       fmt.Sprintf("Service %s is degraded in %s", svc.ServiceName, overview.EnvironmentName),
					DetectedAt:    time.Now(),
				})
			}
		}
	}
	return problems
}

func (r *EnvironmentOverviewRepository) buildSummary(services []EnvironmentOverviewRow, envs []Environment, problems []EnvironmentProblem) EnvironmentOverviewSummary {
	summary := EnvironmentOverviewSummary{
		TotalServices:     len(services),
		TotalEnvironments: len(envs),
	}

	// Track per-service flags to avoid double-counting
	failedByService := make(map[uuid.UUID]bool)
	degradedByService := make(map[uuid.UUID]bool)

	for _, p := range problems {
		switch p.Type {
		case "drift":
			summary.TotalDrifts++
		case "failed_deploy":
			failedByService[p.ServiceID] = true
		case "unhealthy":
			degradedByService[p.ServiceID] = true
		}
	}

	// Count failed and degraded at the service level (not problem level)
	for _, svc := range services {
		if failedByService[svc.ServiceID] {
			summary.FailedDeployments++
		} else if degradedByService[svc.ServiceID] {
			summary.DegradedServices++
		} else {
			summary.HealthyServices++
		}
	}

	return summary
}
