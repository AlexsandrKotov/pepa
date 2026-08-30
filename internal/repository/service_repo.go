package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pepa/pepa/internal/database"
	"github.com/pepa/pepa/pkg/models"
)

// ServiceRepository handles service persistence.
type ServiceRepository struct {
	pool *pgxpool.Pool
}

// NewServiceRepository creates a new service repository.
func NewServiceRepository(db *database.DB) *ServiceRepository {
	return &ServiceRepository{pool: db.Pool}
}

// ── Service Templates ────────────────────────────────────────

// ListTemplates returns all enabled service templates.
func (r *ServiceRepository) ListTemplates(ctx context.Context) ([]models.ServiceTemplate, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, name, slug, description, category, icon,
		       language, framework, dockerfile_tmpl, helm_chart, cicd_tmpl,
		       default_values, resource_defaults, tags, is_enabled, is_system,
		       created_by, created_at, updated_at
		FROM service_templates
		WHERE is_enabled = true
		ORDER BY category, name
	`)
	if err != nil {
		return nil, fmt.Errorf("query templates: %w", err)
	}
	defer rows.Close()

	var templates []models.ServiceTemplate
	for rows.Next() {
		t, err := scanTemplate(rows)
		if err != nil {
			return nil, err
		}
		templates = append(templates, *t)
	}
	return templates, nil
}

// GetTemplate returns a template by slug.
func (r *ServiceRepository) GetTemplate(ctx context.Context, slug string) (*models.ServiceTemplate, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, name, slug, description, category, icon,
		       language, framework, dockerfile_tmpl, helm_chart, cicd_tmpl,
		       default_values, resource_defaults, tags, is_enabled, is_system,
		       created_by, created_at, updated_at
		FROM service_templates WHERE slug = $1 AND is_enabled = true
	`, slug)
	return scanTemplateRow(row)
}

// GetTemplateByID returns a template by ID.
func (r *ServiceRepository) GetTemplateByID(ctx context.Context, id uuid.UUID) (*models.ServiceTemplate, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, name, slug, description, category, icon,
		       language, framework, dockerfile_tmpl, helm_chart, cicd_tmpl,
		       default_values, resource_defaults, tags, is_enabled, is_system,
		       created_by, created_at, updated_at
		FROM service_templates WHERE id = $1
	`, id)
	return scanTemplateRow(row)
}

// ── Services ─────────────────────────────────────────────────

// List returns services with filtering and pagination.
func (r *ServiceRepository) List(ctx context.Context, filter models.ServiceFilter) (*models.ServiceListResponse, error) {
	query := `
		SELECT s.id, s.tenant_id, s.template_id, s.name, s.slug, s.description,
		       s.owner_team_id, s.language, s.framework, s.gitlab_project_url,
		       s.helm_chart_url, s.image_repository, s.namespace, s.status,
		       s.resource_config, s.environment_variables, s.vault_secrets,
		       s.deployment_strategy, s.target_clusters, s.metadata,
		       s.created_by, s.created_at, s.updated_at
		FROM services s
		WHERE 1=1`

	args := []interface{}{}
	argIdx := 1

	if filter.Status != "" {
		query += fmt.Sprintf(" AND s.status = $%d", argIdx)
		args = append(args, filter.Status)
		argIdx++
	}
	if filter.Search != "" {
		query += fmt.Sprintf(" AND (s.name ILIKE $%d OR s.description ILIKE $%d)", argIdx, argIdx)
		args = append(args, "%"+filter.Search+"%")
		argIdx++
	}

	countQuery := "SELECT COUNT(*) FROM (" + query + ") sub"
	var total int64
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count services: %w", err)
	}

	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PerPage < 1 {
		filter.PerPage = 20
	}
	offset := (filter.Page - 1) * filter.PerPage

	query += fmt.Sprintf(" ORDER BY s.updated_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, filter.PerPage, offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query services: %w", err)
	}
	defer rows.Close()

	services := make([]models.Service, 0)
	for rows.Next() {
		s, err := scanService(rows)
		if err != nil {
			return nil, err
		}
		services = append(services, *s)
	}

	totalPages := int(total) / filter.PerPage
	if int(total)%filter.PerPage > 0 {
		totalPages++
	}

	return &models.ServiceListResponse{
		Items:      services,
		Total:      total,
		Page:       filter.Page,
		PerPage:    filter.PerPage,
		TotalPages: totalPages,
	}, nil
}

// Get returns a single service by ID.
func (r *ServiceRepository) Get(ctx context.Context, id uuid.UUID) (*models.Service, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, template_id, name, slug, description,
		       owner_team_id, language, framework, gitlab_project_url,
		       helm_chart_url, image_repository, namespace, status,
		       resource_config, environment_variables, vault_secrets,
		       deployment_strategy, target_clusters, metadata,
		       created_by, created_at, updated_at
		FROM services WHERE id = $1
	`, id)
	return scanServiceRow(row)
}

// Create inserts a new service.
func (r *ServiceRepository) Create(ctx context.Context, req models.CreateServiceRequest, tenantID uuid.UUID, userID *uuid.UUID) (*models.Service, error) {
	id := uuid.New()
	now := time.Now().UTC()

	// Resolve template (optional)
	var templateID sql.NullString
	if req.TemplateSlug != "" {
		err := r.pool.QueryRow(ctx,
			"SELECT id FROM service_templates WHERE slug = $1 AND is_enabled = true",
			req.TemplateSlug,
		).Scan(&templateID)
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("template not found: %s", req.TemplateSlug)
		}
		if err != nil {
			return nil, fmt.Errorf("resolve template: %w", err)
		}
	}

	namespace := req.Namespace
	if namespace == "" {
		namespace = "default"
	}
	strategy := req.DeploymentStrategy
	if strategy == "" {
		strategy = "rolling"
	}
	slug := req.Slug
	if slug == "" {
		slug = generateSlug(req.Name)
	}

	// Ensure slug uniqueness within tenant — append suffix on collision
	baseSlug := slug
	for i := 2; i <= 100; i++ {
		var exists bool
		if err := r.pool.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM services WHERE tenant_id = $1 AND slug = $2)",
			tenantID, slug,
		).Scan(&exists); err != nil {
			return nil, fmt.Errorf("check slug uniqueness: %w", err)
		}
		if !exists {
			break
		}
		slug = fmt.Sprintf("%s-%d", baseSlug, i)
	}

	resourceConfig, _ := json.Marshal(req.ResourceConfig)
	if req.ResourceConfig == nil {
		resourceConfig = json.RawMessage("{}")
	}
	envVars, _ := json.Marshal(req.EnvironmentVars)
	if req.EnvironmentVars == nil {
		envVars = json.RawMessage("{}")
	}
	metadata, _ := json.Marshal(req.Metadata)
	if req.Metadata == nil {
		metadata = json.RawMessage("{}")
	}

	// Parse target cluster IDs
	var clusterIDs []uuid.UUID
	for _, cid := range req.TargetClusterIDs {
		parsed, err := uuid.Parse(cid)
		if err == nil {
			clusterIDs = append(clusterIDs, parsed)
		}
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO services (id, tenant_id, template_id, name, slug, description,
		                      language, framework, gitlab_project_url, helm_chart_url,
		                      image_repository, namespace, status, resource_config, environment_variables,
		                      deployment_strategy, target_clusters, metadata,
		                      created_by, created_at, updated_at)
		VALUES ($1, $2, $3::UUID, $4, $5, $6, $7, $8, $9, $10, $11, $12, 'configured',
		        $13, $14, $15, $16, $17, $18, $19, $20)
	`, id, tenantID, templateID, req.Name, slug, req.Description,
		req.Language, req.Framework, req.GitLabProjectURL, req.HelmChartURL,
		req.ImageRepository, namespace, resourceConfig, envVars,
		strategy, clusterIDs, metadata, userID, now, now)
	if err != nil {
		return nil, fmt.Errorf("create service: %w", err)
	}

	return r.Get(ctx, id)
}

// Update modifies an existing service.
func (r *ServiceRepository) Update(ctx context.Context, id uuid.UUID, req models.UpdateServiceRequest, userID *uuid.UUID) (*models.Service, error) {
	now := time.Now().UTC()

	setClauses := []string{"updated_at = $1"}
	args := []interface{}{now}
	argIdx := 2

	if req.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *req.Name)
		argIdx++
	}
	if req.Description != nil {
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, *req.Description)
		argIdx++
	}
	if req.Namespace != nil {
		setClauses = append(setClauses, fmt.Sprintf("namespace = $%d", argIdx))
		args = append(args, *req.Namespace)
		argIdx++
	}
	if req.DeploymentStrategy != nil {
		setClauses = append(setClauses, fmt.Sprintf("deployment_strategy = $%d", argIdx))
		args = append(args, *req.DeploymentStrategy)
		argIdx++
	}
	if req.ResourceConfig != nil {
		setClauses = append(setClauses, fmt.Sprintf("resource_config = $%d", argIdx))
		args = append(args, req.ResourceConfig)
		argIdx++
	}
	if req.EnvironmentVars != nil {
		setClauses = append(setClauses, fmt.Sprintf("environment_variables = $%d", argIdx))
		args = append(args, req.EnvironmentVars)
		argIdx++
	}
	if req.Status != nil {
		setClauses = append(setClauses, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, *req.Status)
		argIdx++
	}
	if req.Metadata != nil {
		setClauses = append(setClauses, fmt.Sprintf("metadata = $%d", argIdx))
		args = append(args, req.Metadata)
		argIdx++
	}

	query := fmt.Sprintf("UPDATE services SET %s WHERE id = $%d",
		strings.Join(setClauses, ", "), argIdx)
	args = append(args, id)

	tag, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("update service: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, fmt.Errorf("service not found: %s", id)
	}

	return r.Get(ctx, id)
}

// Delete removes a service.
func (r *ServiceRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, "DELETE FROM services WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete service: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("service not found: %s", id)
	}
	return nil
}

// SetStatus updates the service status.
func (r *ServiceRepository) SetStatus(ctx context.Context, id uuid.UUID, status string) error {
	now := time.Now().UTC()
	_, err := r.pool.Exec(ctx, "UPDATE services SET status = $2, updated_at = $3 WHERE id = $1", id, status, now)
	if err != nil {
		return fmt.Errorf("set service status: %w", err)
	}
	return nil
}

// ── Service Deployments ──────────────────────────────────────

// ListDeployments returns deployments for a service.
func (r *ServiceRepository) ListDeployments(ctx context.Context, serviceID uuid.UUID) ([]models.ServiceDeployment, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, service_id, environment, cluster_id, namespace,
		       branch, image_tag, helm_release, deploy_type, status,
		       verification_status, verification_details, flux_synced,
		       pods_ready, pods_total, mr_url, pipeline_url,
		       deployed_at, verified_at, promoted_at, created_at, updated_at
		FROM service_deployments
		WHERE service_id = $1
		ORDER BY
			CASE environment
				WHEN 'dev' THEN 1
				WHEN 'testing' THEN 2
				WHEN 'staging' THEN 3
				WHEN 'production' THEN 4
				ELSE 5
			END
	`, serviceID)
	if err != nil {
		return nil, fmt.Errorf("query deployments: %w", err)
	}
	defer rows.Close()

	var deployments []models.ServiceDeployment
	for rows.Next() {
		d, err := scanDeployment(rows)
		if err != nil {
			return nil, err
		}
		deployments = append(deployments, *d)
	}
	return deployments, nil
}

// ListDeploymentsByEnvironment returns all service deployments for a given environment slug.
func (r *ServiceRepository) ListDeploymentsByEnvironment(ctx context.Context, tenantID uuid.UUID, envSlug string) ([]models.ServiceDeployment, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, service_id, environment, cluster_id, namespace,
		       branch, image_tag, helm_release, deploy_type, status,
		       verification_status, verification_details, flux_synced,
		       pods_ready, pods_total, mr_url, pipeline_url,
		       deployed_at, verified_at, promoted_at, created_at, updated_at
		FROM service_deployments
		WHERE tenant_id = $1 AND environment = $2
		ORDER BY created_at DESC
	`, tenantID, envSlug)
	if err != nil {
		return nil, fmt.Errorf("query deployments by environment: %w", err)
	}
	defer rows.Close()

	var deployments []models.ServiceDeployment
	for rows.Next() {
		d, err := scanDeployment(rows)
		if err != nil {
			return nil, err
		}
		deployments = append(deployments, *d)
	}
	return deployments, nil
}

// CreateDeployment creates a new service deployment record.
func (r *ServiceRepository) CreateDeployment(ctx context.Context, serviceID uuid.UUID, req models.DeployServiceRequest, tenantID uuid.UUID) (*models.ServiceDeployment, error) {
	id := uuid.New()
	now := time.Now().UTC()

	var clusterID *uuid.UUID
	if req.ClusterID != "" {
		parsed, err := uuid.Parse(req.ClusterID)
		if err == nil {
			clusterID = &parsed
		}
	}

	deployType := req.DeployType
	if deployType == "" {
		deployType = "automatic"
	}

	// Deployment starts as 'pending' when no cluster is assigned, 'deploying' when a cluster is targeted
	initialDeployStatus := "pending"
	if req.ClusterID != "" {
		initialDeployStatus = "deploying"
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO service_deployments (id, tenant_id, service_id, environment,
		                                  cluster_id, namespace, branch, image_tag,
		                                  deploy_type, status, verification_status,
		                                  created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'pending', $11, $12)
	`, id, tenantID, serviceID, req.Environment, clusterID,
		"", req.Branch, req.ImageTag, deployType, initialDeployStatus, now, now)
	if err != nil {
		return nil, fmt.Errorf("create deployment: %w", err)
	}

	// Only update service status when deploying to a real cluster
	if req.ClusterID != "" {
		if _, err := r.pool.Exec(ctx, "UPDATE services SET status = 'deploying', updated_at = $1 WHERE id = $2", now, serviceID); err != nil {
			slog.Info("failed to update status to 'deploying' for service ", "id", serviceID, "error", err)
		}
	}

	return r.GetDeployment(ctx, id)
}

// GetDeployment returns a single deployment.
func (r *ServiceRepository) GetDeployment(ctx context.Context, id uuid.UUID) (*models.ServiceDeployment, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, service_id, environment, cluster_id, namespace,
		       branch, image_tag, helm_release, deploy_type, status,
		       verification_status, verification_details, flux_synced,
		       pods_ready, pods_total, mr_url, pipeline_url,
		       deployed_at, verified_at, promoted_at, created_at, updated_at
		FROM service_deployments WHERE id = $1
	`, id)
	return scanDeploymentRow(row)
}

// UpdateDeployment updates deployment status.
func (r *ServiceRepository) UpdateDeployment(ctx context.Context, id uuid.UUID, status string, verificationStatus string) error {
	now := time.Now().UTC()
	_, err := r.pool.Exec(ctx, `
		UPDATE service_deployments
		SET status = $2, verification_status = $3, updated_at = $4,
		    deployed_at = CASE WHEN $2 = 'deployed' THEN $4 ELSE deployed_at END,
		    verified_at = CASE WHEN $3 = 'verified' THEN $4 ELSE verified_at END
		WHERE id = $1
	`, id, status, verificationStatus, now)
	return err
}

// CompleteDeployment marks a deployment as deployed and updates the service status to active.
func (r *ServiceRepository) CompleteDeployment(ctx context.Context, deploymentID uuid.UUID, serviceID uuid.UUID) error {
	now := time.Now().UTC()
	// Update deployment status to 'deployed'
	_, err := r.pool.Exec(ctx, `
		UPDATE service_deployments
		SET status = 'deployed', deployed_at = $2, updated_at = $2,
		    pods_ready = 1, pods_total = 1
		WHERE id = $1
	`, deploymentID, now)
	if err != nil {
		return fmt.Errorf("update deployment status: %w", err)
	}

	// Update service status to 'active'
	_, err = r.pool.Exec(ctx, `
		UPDATE services SET status = 'active', updated_at = $1 WHERE id = $2
	`, now, serviceID)
	if err != nil {
		return fmt.Errorf("update service status: %w", err)
	}
	return nil
}

// PromoteDeployment marks a deployment as promoted to next environment.
func (r *ServiceRepository) PromoteDeployment(ctx context.Context, id uuid.UUID) error {
	now := time.Now().UTC()
	_, err := r.pool.Exec(ctx, `
		UPDATE service_deployments
		SET promoted_at = $2, status = 'promoted', updated_at = $2
		WHERE id = $1
	`, id, now)
	return err
}

// ── Scan helpers ─────────────────────────────────────────────

func scanTemplate(rows pgx.Rows) (*models.ServiceTemplate, error) {
	var t models.ServiceTemplate
	var desc, icon, lang, fw, dockerTmpl, cicdTmpl sql.NullString
	var helmJSON, defaultsJSON, resourceJSON []byte
	var tags []string
	var createdBy sql.NullString

	err := rows.Scan(&t.ID, &t.TenantID, &t.Name, &t.Slug, &desc, &t.Category,
		&icon, &lang, &fw, &dockerTmpl, &helmJSON, &cicdTmpl,
		&defaultsJSON, &resourceJSON, &tags, &t.IsEnabled, &t.IsSystem,
		&createdBy, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan template: %w", err)
	}
	if desc.Valid {
		t.Description = desc.String
	}
	if icon.Valid {
		t.Icon = icon.String
	}
	if lang.Valid {
		t.Language = lang.String
	}
	if fw.Valid {
		t.Framework = fw.String
	}
	if dockerTmpl.Valid {
		t.DockerfileTmpl = dockerTmpl.String
	}
	if cicdTmpl.Valid {
		t.CICDTmpl = cicdTmpl.String
	}
	if helmJSON != nil {
		t.HelmChart = helmJSON
	}
	if defaultsJSON != nil {
		t.DefaultValues = defaultsJSON
	}
	if resourceJSON != nil {
		t.ResourceDefaults = resourceJSON
	}
	if tags != nil {
		t.Tags = tags
	}
	if createdBy.Valid {
		parsed, _ := uuid.Parse(createdBy.String)
		t.CreatedBy = &parsed
	}
	return &t, nil
}

func scanTemplateRow(row pgx.Row) (*models.ServiceTemplate, error) {
	var t models.ServiceTemplate
	var desc, icon, lang, fw, dockerTmpl, cicdTmpl sql.NullString
	var helmJSON, defaultsJSON, resourceJSON []byte
	var tags []string
	var createdBy sql.NullString

	err := row.Scan(&t.ID, &t.TenantID, &t.Name, &t.Slug, &desc, &t.Category,
		&icon, &lang, &fw, &dockerTmpl, &helmJSON, &cicdTmpl,
		&defaultsJSON, &resourceJSON, &tags, &t.IsEnabled, &t.IsSystem,
		&createdBy, &t.CreatedAt, &t.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("template not found")
	}
	if err != nil {
		return nil, fmt.Errorf("scan template: %w", err)
	}
	if desc.Valid {
		t.Description = desc.String
	}
	if icon.Valid {
		t.Icon = icon.String
	}
	if lang.Valid {
		t.Language = lang.String
	}
	if fw.Valid {
		t.Framework = fw.String
	}
	if dockerTmpl.Valid {
		t.DockerfileTmpl = dockerTmpl.String
	}
	if cicdTmpl.Valid {
		t.CICDTmpl = cicdTmpl.String
	}
	if helmJSON != nil {
		t.HelmChart = helmJSON
	}
	if defaultsJSON != nil {
		t.DefaultValues = defaultsJSON
	}
	if resourceJSON != nil {
		t.ResourceDefaults = resourceJSON
	}
	if tags != nil {
		t.Tags = tags
	}
	if createdBy.Valid {
		parsed, _ := uuid.Parse(createdBy.String)
		t.CreatedBy = &parsed
	}
	return &t, nil
}

func scanService(rows pgx.Rows) (*models.Service, error) {
	var s models.Service
	var desc, lang, fw, gitlabURL, helmURL, imgRepo sql.NullString
	var templateID sql.NullString
	var ownerTeamID sql.NullString
	var resourceJSON, envJSON, vaultJSON, metadataJSON []byte
	var clusterIDs []uuid.UUID
	var createdBy sql.NullString

	err := rows.Scan(&s.ID, &s.TenantID, &templateID, &s.Name, &s.Slug, &desc,
		&ownerTeamID, &lang, &fw, &gitlabURL, &helmURL, &imgRepo,
		&s.Namespace, &s.Status, &resourceJSON, &envJSON, &vaultJSON,
		&s.DeploymentStrategy, &clusterIDs, &metadataJSON,
		&createdBy, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan service: %w", err)
	}
	if templateID.Valid {
		parsed, _ := uuid.Parse(templateID.String)
		s.TemplateID = &parsed
	}
	if desc.Valid {
		s.Description = desc.String
	}
	if ownerTeamID.Valid {
		parsed, _ := uuid.Parse(ownerTeamID.String)
		s.OwnerTeamID = &parsed
	}
	if lang.Valid {
		s.Language = lang.String
	}
	if fw.Valid {
		s.Framework = fw.String
	}
	if gitlabURL.Valid {
		s.GitLabProjectURL = gitlabURL.String
	}
	if helmURL.Valid {
		s.HelmChartURL = helmURL.String
	}
	if imgRepo.Valid {
		s.ImageRepository = imgRepo.String
	}
	if resourceJSON != nil {
		s.ResourceConfig = resourceJSON
	}
	if envJSON != nil {
		s.EnvironmentVars = envJSON
	}
	if vaultJSON != nil {
		s.VaultSecrets = vaultJSON
	}
	if metadataJSON != nil {
		s.Metadata = metadataJSON
	}
	if clusterIDs != nil {
		s.TargetClusters = clusterIDs
	}
	if createdBy.Valid {
		parsed, _ := uuid.Parse(createdBy.String)
		s.CreatedBy = &parsed
	}
	return &s, nil
}

func scanServiceRow(row pgx.Row) (*models.Service, error) {
	var s models.Service
	var desc, lang, fw, gitlabURL, helmURL, imgRepo sql.NullString
	var templateID sql.NullString
	var ownerTeamID sql.NullString
	var resourceJSON, envJSON, vaultJSON, metadataJSON []byte
	var clusterIDs []uuid.UUID
	var createdBy sql.NullString

	err := row.Scan(&s.ID, &s.TenantID, &templateID, &s.Name, &s.Slug, &desc,
		&ownerTeamID, &lang, &fw, &gitlabURL, &helmURL, &imgRepo,
		&s.Namespace, &s.Status, &resourceJSON, &envJSON, &vaultJSON,
		&s.DeploymentStrategy, &clusterIDs, &metadataJSON,
		&createdBy, &s.CreatedAt, &s.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("service not found")
	}
	if err != nil {
		return nil, fmt.Errorf("scan service: %w", err)
	}
	if templateID.Valid {
		parsed, _ := uuid.Parse(templateID.String)
		s.TemplateID = &parsed
	}
	if desc.Valid {
		s.Description = desc.String
	}
	if ownerTeamID.Valid {
		parsed, _ := uuid.Parse(ownerTeamID.String)
		s.OwnerTeamID = &parsed
	}
	if lang.Valid {
		s.Language = lang.String
	}
	if fw.Valid {
		s.Framework = fw.String
	}
	if gitlabURL.Valid {
		s.GitLabProjectURL = gitlabURL.String
	}
	if helmURL.Valid {
		s.HelmChartURL = helmURL.String
	}
	if imgRepo.Valid {
		s.ImageRepository = imgRepo.String
	}
	if resourceJSON != nil {
		s.ResourceConfig = resourceJSON
	}
	if envJSON != nil {
		s.EnvironmentVars = envJSON
	}
	if vaultJSON != nil {
		s.VaultSecrets = vaultJSON
	}
	if metadataJSON != nil {
		s.Metadata = metadataJSON
	}
	if clusterIDs != nil {
		s.TargetClusters = clusterIDs
	}
	if createdBy.Valid {
		parsed, _ := uuid.Parse(createdBy.String)
		s.CreatedBy = &parsed
	}
	return &s, nil
}

// generateSlug creates a URL-safe slug from a name.
func generateSlug(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		if r == ' ' || r == '-' || r == '_' {
			return '-'
		}
		return -1
	}, s)
	// Collapse multiple dashes
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}

func scanDeployment(rows pgx.Rows) (*models.ServiceDeployment, error) {
	var d models.ServiceDeployment
	var clusterID sql.NullString
	var ns, branch, imgTag, helmRelease, mrURL, pipelineURL sql.NullString
	var verifyDetailsJSON []byte

	err := rows.Scan(&d.ID, &d.TenantID, &d.ServiceID, &d.Environment, &clusterID,
		&ns, &branch, &imgTag, &helmRelease, &d.DeployType, &d.Status,
		&d.VerificationStatus, &verifyDetailsJSON, &d.FluxSynced,
		&d.PodsReady, &d.PodsTotal, &mrURL, &pipelineURL,
		&d.DeployedAt, &d.VerifiedAt, &d.PromotedAt, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan deployment: %w", err)
	}
	if clusterID.Valid {
		parsed, _ := uuid.Parse(clusterID.String)
		d.ClusterID = &parsed
	}
	if ns.Valid {
		d.Namespace = ns.String
	}
	if branch.Valid {
		d.Branch = branch.String
	}
	if imgTag.Valid {
		d.ImageTag = imgTag.String
	}
	if helmRelease.Valid {
		d.HelmRelease = helmRelease.String
	}
	if mrURL.Valid {
		d.MRUrl = mrURL.String
	}
	if pipelineURL.Valid {
		d.PipelineURL = pipelineURL.String
	}
	if verifyDetailsJSON != nil {
		d.VerificationDetails = verifyDetailsJSON
	}
	return &d, nil
}

func scanDeploymentRow(row pgx.Row) (*models.ServiceDeployment, error) {
	var d models.ServiceDeployment
	var clusterID sql.NullString
	var ns, branch, imgTag, helmRelease, mrURL, pipelineURL sql.NullString
	var verifyDetailsJSON []byte

	err := row.Scan(&d.ID, &d.TenantID, &d.ServiceID, &d.Environment, &clusterID,
		&ns, &branch, &imgTag, &helmRelease, &d.DeployType, &d.Status,
		&d.VerificationStatus, &verifyDetailsJSON, &d.FluxSynced,
		&d.PodsReady, &d.PodsTotal, &mrURL, &pipelineURL,
		&d.DeployedAt, &d.VerifiedAt, &d.PromotedAt, &d.CreatedAt, &d.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("deployment not found")
	}
	if err != nil {
		return nil, fmt.Errorf("scan deployment: %w", err)
	}
	if clusterID.Valid {
		parsed, _ := uuid.Parse(clusterID.String)
		d.ClusterID = &parsed
	}
	if ns.Valid {
		d.Namespace = ns.String
	}
	if branch.Valid {
		d.Branch = branch.String
	}
	if imgTag.Valid {
		d.ImageTag = imgTag.String
	}
	if helmRelease.Valid {
		d.HelmRelease = helmRelease.String
	}
	if mrURL.Valid {
		d.MRUrl = mrURL.String
	}
	if pipelineURL.Valid {
		d.PipelineURL = pipelineURL.String
	}
	if verifyDetailsJSON != nil {
		d.VerificationDetails = verifyDetailsJSON
	}
	return &d, nil
}
