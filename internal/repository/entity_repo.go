package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pepa/pepa/internal/database"
	"github.com/pepa/pepa/pkg/models"
)

// EntityRepository handles entity persistence.
type EntityRepository struct {
	pool *pgxpool.Pool
}

// defaultTimeout is the default context timeout for repository operations.
const defaultTimeout = 10 * time.Second

// withDefaultTimeout returns a context with a timeout if the parent context
// does not already have a deadline.
func withDefaultTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, defaultTimeout)
}

// NewEntityRepository creates a new entity repository.
func NewEntityRepository(db *database.DB) *EntityRepository {
	return &EntityRepository{pool: db.Pool}
}

// List returns entities with filtering and pagination.
func (r *EntityRepository) List(ctx context.Context, filter models.EntityFilter) (*models.EntityListResponse, error) {
	ctx, cancel := withDefaultTimeout(ctx)
	defer cancel()

	query := `
		SELECT e.id, e.type_id, e.type_key, e.name, e.description, e.external_id,
		       e.tenant_id, e.organization_id, e.metadata, e.status, e.status_detail,
		       e.plugin_name, e.sync_status, e.last_synced_at,
		       e.created_by, e.updated_by, e.created_at, e.updated_at, e.deleted_at
		FROM entities e
		WHERE e.deleted_at IS NULL`

	args := []interface{}{}
	argIdx := 1

	if filter.TypeKey != "" {
		query += fmt.Sprintf(" AND e.type_key = $%d", argIdx)
		args = append(args, filter.TypeKey)
		argIdx++
	}
	if filter.Status != "" {
		query += fmt.Sprintf(" AND e.status = $%d", argIdx)
		args = append(args, filter.Status)
		argIdx++
	}
	if filter.PluginName != "" {
		query += fmt.Sprintf(" AND e.plugin_name = $%d", argIdx)
		args = append(args, filter.PluginName)
		argIdx++
	}
	if filter.Search != "" {
		query += fmt.Sprintf(" AND (e.name ILIKE $%d OR e.description ILIKE $%d)", argIdx, argIdx)
		args = append(args, "%"+filter.Search+"%", "%"+filter.Search+"%")
		argIdx++
	}
	if filter.TenantID != uuid.Nil {
		query += fmt.Sprintf(" AND e.tenant_id = $%d", argIdx)
		args = append(args, filter.TenantID)
		argIdx++
	}

	// Count
	countQuery := "SELECT COUNT(*) FROM (" + query + ") sub"
	var total int64
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count entities: %w", err)
	}

	// Pagination
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PerPage < 1 {
		filter.PerPage = 20
	}
	offset := (filter.Page - 1) * filter.PerPage

	query += fmt.Sprintf(" ORDER BY e.updated_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, filter.PerPage, offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query entities: %w", err)
	}
	defer rows.Close()

	entities := make([]models.Entity, 0)
	for rows.Next() {
		e, err := scanEntity(rows)
		if err != nil {
			return nil, err
		}
		entities = append(entities, *e)
	}

	totalPages := int(total) / filter.PerPage
	if int(total)%filter.PerPage > 0 {
		totalPages++
	}

	return &models.EntityListResponse{
		Items:      entities,
		Total:      total,
		Page:       filter.Page,
		PerPage:    filter.PerPage,
		TotalPages: totalPages,
	}, nil
}

// Get returns a single entity by ID, scoped to tenantID. A zero tenantID means
// no tenant filtering (platform admins / trusted internal callers).
func (r *EntityRepository) Get(ctx context.Context, id, tenantID uuid.UUID) (*models.Entity, error) {
	ctx, cancel := withDefaultTimeout(ctx)
	defer cancel()

	query := `
		SELECT id, type_id, type_key, name, description, external_id,
		       tenant_id, organization_id, metadata, status, status_detail,
		       plugin_name, sync_status, last_synced_at,
		       created_by, updated_by, created_at, updated_at, deleted_at
		FROM entities WHERE id = $1 AND deleted_at IS NULL`
	args := []interface{}{id}
	if tenantID != uuid.Nil {
		query += " AND tenant_id = $2"
		args = append(args, tenantID)
	}

	row := r.pool.QueryRow(ctx, query, args...)

	e, err := scanEntity(row)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("entity not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get entity: %w", err)
	}
	return e, nil
}

// Create inserts a new entity.
func (r *EntityRepository) Create(ctx context.Context, req models.CreateEntityRequest, tenantID, orgID uuid.UUID, userID *uuid.UUID) (*models.Entity, error) {
	id := uuid.New()
	now := time.Now().UTC()

	// Resolve type_id from type_key
	var typeID uuid.UUID
	err := r.pool.QueryRow(ctx,
		"SELECT id FROM entity_types WHERE type_key = $1 AND is_enabled = true",
		req.TypeKey,
	).Scan(&typeID)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("entity type not found: %s", req.TypeKey)
	}
	if err != nil {
		return nil, fmt.Errorf("resolve type: %w", err)
	}

	metadata := req.Metadata
	if metadata == nil {
		metadata = json.RawMessage("{}")
	}

	// Auto-generate external_id from name when not provided,
	// to avoid unique constraint violations on (type_key, external_id, tenant_id).
	externalID := req.ExternalID
	if externalID == "" {
		externalID = req.Name
	}

	_, err = r.pool.Exec(ctx, `
		INSERT INTO entities (id, type_id, type_key, name, description, external_id,
		                      tenant_id, organization_id, metadata, status, sync_status,
		                      created_by, updated_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'active', 'pending', $10, $11, $12, $13)
	`, id, typeID, req.TypeKey, req.Name, req.Description, externalID,
		tenantID, orgID, metadata, userID, userID, now, now)
	if err != nil {
		return nil, fmt.Errorf("create entity: %w", err)
	}

	return r.Get(ctx, id, tenantID)
}

// Update modifies an existing entity, scoped to tenantID (zero = no filter).
func (r *EntityRepository) Update(ctx context.Context, id uuid.UUID, req models.UpdateEntityRequest, userID *uuid.UUID, tenantID uuid.UUID) (*models.Entity, error) {
	now := time.Now().UTC()

	setClauses := []string{"updated_at = $1", "updated_by = $2"}
	args := []interface{}{now, userID}
	argIdx := 3

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
	if req.Metadata != nil {
		setClauses = append(setClauses, fmt.Sprintf("metadata = $%d", argIdx))
		args = append(args, req.Metadata)
		argIdx++
	}
	if req.Status != nil {
		setClauses = append(setClauses, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, *req.Status)
		argIdx++
	}

	where := fmt.Sprintf("WHERE id = $%d AND deleted_at IS NULL", argIdx)
	args = append(args, id)
	argIdx++
	if tenantID != uuid.Nil {
		where += fmt.Sprintf(" AND tenant_id = $%d", argIdx)
		args = append(args, tenantID)
	}

	query := fmt.Sprintf("UPDATE entities SET %s %s", strings.Join(setClauses, ", "), where)

	tag, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("update entity: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, fmt.Errorf("entity not found: %s", id)
	}

	return r.Get(ctx, id, tenantID)
}

// Delete soft-deletes an entity, scoped to tenantID (zero = no filter).
func (r *EntityRepository) Delete(ctx context.Context, id, tenantID uuid.UUID) error {
	now := time.Now().UTC()

	query := "UPDATE entities SET deleted_at = $1 WHERE id = $2 AND deleted_at IS NULL"
	args := []interface{}{now, id}
	if tenantID != uuid.Nil {
		query += " AND tenant_id = $3"
		args = append(args, tenantID)
	}

	tag, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("delete entity: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("entity not found: %s", id)
	}
	return nil
}

// UpdateEmbedding stores a vector embedding for an entity.
func (r *EntityRepository) UpdateEmbedding(ctx context.Context, id uuid.UUID, embedding []float32) error {
	ctx, cancel := withDefaultTimeout(ctx)
	defer cancel()

	// Convert []float32 to pgvector string format: [0.1,0.2,...]
	parts := make([]string, len(embedding))
	for i, v := range embedding {
		parts[i] = strconv.FormatFloat(float64(v), 'f', 6, 32)
	}
	vecStr := "[" + strings.Join(parts, ",") + "]"

	_, err := r.pool.Exec(ctx, "UPDATE entities SET embedding = $1::vector WHERE id = $2", vecStr, id)
	if err != nil {
		return fmt.Errorf("update embedding: %w", err)
	}
	return nil
}

// GetSubgraph returns the entity graph around a given entity, scoped to
// tenantID (zero = no filter). The tenant filter is applied to the root entity,
// the relationship traversal, and the batch-fetched neighbours so a caller can
// never observe another tenant's graph.
func (r *EntityRepository) GetSubgraph(ctx context.Context, id uuid.UUID, depth int, tenantID uuid.UUID) (*models.GraphResult, error) {
	entity, err := r.Get(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}

	result := &models.GraphResult{
		Nodes: []models.GraphNode{{Entity: *entity, Depth: 0, Path: []string{entity.Name}}},
		Edges: []models.GraphEdge{},
	}

	// Get relationships recursively, optionally confined to a single tenant.
	tenantClause := ""
	args := []interface{}{id, depth}
	if tenantID != uuid.Nil {
		tenantClause = " AND er.tenant_id = $3"
		args = append(args, tenantID)
	}

	rows, err := r.pool.Query(ctx, `
		WITH RECURSIVE graph AS (
			SELECT er.id, er.relationship_type_id, er.type_key,
			       er.source_id, er.target_id, er.metadata,
			       er.tenant_id, er.plugin_name, er.created_at, er.updated_at,
			       1 as depth
			FROM entity_relationships er
			WHERE (er.source_id = $1 OR er.target_id = $1)`+tenantClause+`

			UNION

			SELECT er.id, er.relationship_type_id, er.type_key,
			       er.source_id, er.target_id, er.metadata,
			       er.tenant_id, er.plugin_name, er.created_at, er.updated_at,
			       g.depth + 1
			FROM entity_relationships er
			JOIN graph g ON (er.source_id = g.target_id OR er.target_id = g.source_id)
			WHERE g.depth < $2`+tenantClause+`
		)
		SELECT DISTINCT * FROM graph
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("query graph: %w", err)
	}
	defer rows.Close()

	// First pass: collect all relationships and unique entity IDs
	type relWithDepth struct {
		Rel   models.EntityRelationship
		Depth int
	}
	var rels []relWithDepth
	entityIDs := map[uuid.UUID]bool{id: true}

	for rows.Next() {
		var rel models.EntityRelationship
		var relDepth int
		var metadataJSON []byte
		var pluginName sql.NullString

		if err := rows.Scan(&rel.ID, &rel.RelationshipTypeID, &rel.TypeKey,
			&rel.SourceID, &rel.TargetID, &metadataJSON,
			&rel.TenantID, &pluginName, &rel.CreatedAt, &rel.UpdatedAt,
			&relDepth); err != nil {
			return nil, fmt.Errorf("scan relationship: %w", err)
		}
		if metadataJSON != nil {
			rel.Metadata = metadataJSON
		}
		if pluginName.Valid {
			rel.PluginName = pluginName.String
		}

		rels = append(rels, relWithDepth{Rel: rel, Depth: relDepth})
		entityIDs[rel.SourceID] = true
		entityIDs[rel.TargetID] = true
	}

	// Batch-fetch all referenced entities (fixes N+1), tenant-scoped.
	entities, err := r.batchGetEntities(ctx, entityIDs, tenantID)
	if err != nil {
		return nil, fmt.Errorf("batch get entities: %w", err)
	}

	// Second pass: build graph edges and nodes
	seenNodes := map[uuid.UUID]bool{id: true}

	for _, rd := range rels {
		rel := rd.Rel
		relDepth := rd.Depth

		// Determine direction
		direction := "outgoing"
		relatedID := rel.TargetID
		if rel.SourceID != id {
			direction = "incoming"
			relatedID = rel.SourceID
		}

		edge := models.GraphEdge{
			Relationship: rel,
			Direction:    direction,
		}

		// Populate source and target from batch-fetched entities
		if src, ok := entities[rel.SourceID]; ok {
			edge.Source = src
		}
		if tgt, ok := entities[rel.TargetID]; ok {
			edge.Target = tgt
		}

		result.Edges = append(result.Edges, edge)

		// Add related node
		if !seenNodes[relatedID] {
			if relEntity, ok := entities[relatedID]; ok {
				result.Nodes = append(result.Nodes, models.GraphNode{
					Entity: relEntity,
					Depth:  relDepth,
				})
			}
			seenNodes[relatedID] = true
		}
	}

	return result, nil
}

// batchGetEntities fetches multiple entities by ID in a single query, scoped to
// tenantID (zero = no filter).
func (r *EntityRepository) batchGetEntities(ctx context.Context, ids map[uuid.UUID]bool, tenantID uuid.UUID) (map[uuid.UUID]models.Entity, error) {
	if len(ids) == 0 {
		return map[uuid.UUID]models.Entity{}, nil
	}

	idSlice := make([]uuid.UUID, 0, len(ids))
	for id := range ids {
		idSlice = append(idSlice, id)
	}

	query := `
		SELECT id, type_id, type_key, name, description, external_id,
		       tenant_id, organization_id, metadata, status, status_detail,
		       plugin_name, sync_status, last_synced_at,
		       created_by, updated_by, created_at, updated_at, deleted_at
		FROM entities WHERE id = ANY($1) AND deleted_at IS NULL
	`
	args := []interface{}{idSlice}
	if tenantID != uuid.Nil {
		query += " AND tenant_id = $2"
		args = append(args, tenantID)
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("batch query entities: %w", err)
	}
	defer rows.Close()

	result := make(map[uuid.UUID]models.Entity, len(ids))
	for rows.Next() {
		e, err := scanEntity(rows)
		if err != nil {
			return nil, err
		}
		result[e.ID] = *e
	}

	return result, nil
}

// GetRelationships returns all relationships for an entity, scoped to tenantID
// (zero = no filter).
func (r *EntityRepository) GetRelationships(ctx context.Context, id, tenantID uuid.UUID) ([]models.EntityRelationship, error) {
	query := `
		SELECT id, relationship_type_id, type_key, source_id, target_id,
		       metadata, tenant_id, plugin_name, created_at, updated_at
		FROM entity_relationships
		WHERE (source_id = $1 OR target_id = $1)`
	args := []interface{}{id}
	if tenantID != uuid.Nil {
		query += " AND tenant_id = $2"
		args = append(args, tenantID)
	}
	query += " ORDER BY created_at DESC"

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query relationships: %w", err)
	}
	defer rows.Close()

	var relationships []models.EntityRelationship
	for rows.Next() {
		var rel models.EntityRelationship
		var metadataJSON []byte
		var pluginName sql.NullString
		if err := rows.Scan(&rel.ID, &rel.RelationshipTypeID, &rel.TypeKey,
			&rel.SourceID, &rel.TargetID, &metadataJSON,
			&rel.TenantID, &pluginName, &rel.CreatedAt, &rel.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan relationship: %w", err)
		}
		if metadataJSON != nil {
			rel.Metadata = metadataJSON
		}
		if pluginName.Valid {
			rel.PluginName = pluginName.String
		}
		relationships = append(relationships, rel)
	}

	return relationships, nil
}

// CreateRelationship creates a relationship between two entities.
func (r *EntityRepository) CreateRelationship(ctx context.Context, sourceID, targetID uuid.UUID, relTypeKey string, tenantID uuid.UUID, metadata json.RawMessage) (*models.EntityRelationship, error) {
	// Resolve relationship type
	var relTypeID uuid.UUID
	var pluginName sql.NullString
	err := r.pool.QueryRow(ctx,
		"SELECT id, plugin_name FROM relationship_types WHERE type_key = $1",
		relTypeKey,
	).Scan(&relTypeID, &pluginName)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("relationship type not found: %s", relTypeKey)
	}
	if err != nil {
		return nil, fmt.Errorf("resolve relationship type: %w", err)
	}

	if metadata == nil {
		metadata = json.RawMessage("{}")
	}

	id := uuid.New()
	now := time.Now().UTC()
	var pn sql.NullString
	if pluginName.Valid {
		pn = pluginName
	}

	err = r.pool.QueryRow(ctx, `
		INSERT INTO entity_relationships (id, relationship_type_id, type_key, source_id, target_id,
		                                  metadata, tenant_id, plugin_name, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, relationship_type_id, type_key, source_id, target_id,
		          metadata, tenant_id, plugin_name, created_at, updated_at
	`, id, relTypeID, relTypeKey, sourceID, targetID, metadata, tenantID, pn, now, now,
	).Scan(&id, &relTypeID, &relTypeKey, &sourceID, &targetID, &metadata,
		&tenantID, &pn, &now, &now)
	if err != nil {
		return nil, fmt.Errorf("create relationship: %w", err)
	}

	rel := &models.EntityRelationship{
		ID:                 id,
		RelationshipTypeID: relTypeID,
		TypeKey:            relTypeKey,
		SourceID:           sourceID,
		TargetID:           targetID,
		Metadata:           metadata,
		TenantID:           tenantID,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if pn.Valid {
		rel.PluginName = pn.String
	}

	return rel, nil
}

// DeleteRelationship removes a relationship, scoped to tenantID (zero = no filter).
func (r *EntityRepository) DeleteRelationship(ctx context.Context, id, tenantID uuid.UUID) error {
	query := "DELETE FROM entity_relationships WHERE id = $1"
	args := []interface{}{id}
	if tenantID != uuid.Nil {
		query += " AND tenant_id = $2"
		args = append(args, tenantID)
	}

	tag, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("delete relationship: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("relationship not found: %s", id)
	}
	return nil
}

// ListEntityTypes returns all entity types.
func (r *EntityRepository) ListEntityTypes(ctx context.Context) ([]models.EntityType, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, type_key, display_name, description, plugin_name, icon,
		       category, metadata_schema, ui_config, is_system, is_enabled,
		       created_at, updated_at
		FROM entity_types
		WHERE is_enabled = true
		ORDER BY display_name
	`)
	if err != nil {
		return nil, fmt.Errorf("query entity types: %w", err)
	}
	defer rows.Close()

	var types []models.EntityType
	for rows.Next() {
		var et models.EntityType
		var schemaJSON, uiJSON []byte
		var desc, pluginName, icon, category sql.NullString
		if err := rows.Scan(&et.ID, &et.TypeKey, &et.DisplayName, &desc,
			&pluginName, &icon, &category, &schemaJSON, &uiJSON,
			&et.IsSystem, &et.IsEnabled, &et.CreatedAt, &et.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan entity type: %w", err)
		}
		if desc.Valid {
			et.Description = desc.String
		}
		if pluginName.Valid {
			et.PluginName = pluginName.String
		}
		if icon.Valid {
			et.Icon = icon.String
		}
		if category.Valid {
			et.Category = category.String
		}
		if schemaJSON != nil {
			et.MetadataSchema = schemaJSON
		}
		if uiJSON != nil {
			et.UIConfig = uiJSON
		}
		types = append(types, et)
	}

	return types, nil
}

// CreateEntityType creates a new entity type.
func (r *EntityRepository) CreateEntityType(ctx context.Context, et *models.EntityType) error {
	id := uuid.New()
	now := time.Now().UTC()
	et.ID = id
	et.CreatedAt = now
	et.UpdatedAt = now

	schema := et.MetadataSchema
	if schema == nil {
		schema = json.RawMessage("{}")
	}
	ui := et.UIConfig
	if ui == nil {
		ui = json.RawMessage("{}")
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO entity_types (id, type_key, display_name, description, plugin_name,
		                          icon, category, metadata_schema, ui_config,
		                          is_system, is_enabled, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, id, et.TypeKey, et.DisplayName, et.Description, et.PluginName,
		et.Icon, et.Category, schema, ui,
		et.IsSystem, et.IsEnabled, now, now)
	if err != nil {
		return fmt.Errorf("create entity type: %w", err)
	}

	return nil
}

// --- scan helper ---

func scanEntity(row pgx.Row) (*models.Entity, error) {
	var e models.Entity
	var metadataJSON []byte
	var desc, extID, statusDetail, pluginName sql.NullString
	err := row.Scan(
		&e.ID, &e.TypeID, &e.TypeKey, &e.Name, &desc, &extID,
		&e.TenantID, &e.OrganizationID, &metadataJSON, &e.Status, &statusDetail,
		&pluginName, &e.SyncStatus, &e.LastSyncedAt,
		&e.CreatedBy, &e.UpdatedBy, &e.CreatedAt, &e.UpdatedAt, &e.DeletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan entity: %w", err)
	}
	if metadataJSON != nil {
		e.Metadata = metadataJSON
	}
	if desc.Valid {
		e.Description = desc.String
	}
	if extID.Valid {
		e.ExternalID = extID.String
	}
	if statusDetail.Valid {
		e.StatusDetail = statusDetail.String
	}
	if pluginName.Valid {
		e.PluginName = pluginName.String
	}
	return &e, nil
}
