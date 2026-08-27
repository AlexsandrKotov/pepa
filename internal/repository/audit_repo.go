package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/database"
	"github.com/pepa/pepa/pkg/models"
)

// AuditRepository handles audit log operations.
type AuditRepository struct {
	db *database.DB
}

func NewAuditRepository(db *database.DB) *AuditRepository {
	return &AuditRepository{db: db}
}

// Create inserts a new audit log entry.
func (r *AuditRepository) Create(ctx context.Context, entry *models.AuditLog) error {
	entry.ID = uuid.New()
	query := `INSERT INTO audit_log (id, tenant_id, user_id, api_key_id, plugin_name, action, entity_type, entity_id, old_values, new_values, diff, ip_address, user_agent, request_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`
	_, err := r.db.Pool.Exec(ctx, query,
		entry.ID, entry.TenantID, entry.UserID, entry.APIKeyID, entry.PluginName,
		entry.Action, entry.EntityType, entry.EntityID,
		entry.OldValues, entry.NewValues, entry.Diff,
		entry.IPAddress, entry.UserAgent, entry.RequestID,
	)
	return err
}

// List returns audit log entries matching the filter.
func (r *AuditRepository) List(ctx context.Context, filter models.AuditFilter) (*models.AuditListResponse, error) {
	where := []string{}
	args := []interface{}{}
	argIdx := 0

	if filter.Action != "" {
		argIdx++
		where = append(where, fmt.Sprintf("action = $%d", argIdx))
		args = append(args, filter.Action)
	}
	if filter.EntityType != "" {
		argIdx++
		where = append(where, fmt.Sprintf("entity_type = $%d", argIdx))
		args = append(args, filter.EntityType)
	}
	if filter.EntityID != "" {
		argIdx++
		where = append(where, fmt.Sprintf("entity_id = $%d::uuid", argIdx))
		args = append(args, filter.EntityID)
	}
	if filter.UserID != "" {
		argIdx++
		where = append(where, fmt.Sprintf("user_id = $%d", argIdx))
		args = append(args, filter.UserID)
	}
	if filter.StartDate != "" {
		argIdx++
		where = append(where, fmt.Sprintf("created_at >= $%d::timestamptz", argIdx))
		args = append(args, filter.StartDate)
	}
	if filter.EndDate != "" {
		argIdx++
		where = append(where, fmt.Sprintf("created_at <= $%d::timestamptz", argIdx))
		args = append(args, filter.EndDate)
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	// Count total
	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM audit_log %s", whereClause)
	if err := r.db.Pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, err
	}

	// Apply pagination
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PerPage < 1 {
		filter.PerPage = 50
	}
	offset := (filter.Page - 1) * filter.PerPage

	argIdx++
	query := fmt.Sprintf(`SELECT id, tenant_id, user_id, api_key_id, plugin_name, action, entity_type, entity_id, old_values, new_values, diff, ip_address, user_agent, request_id, created_at
		FROM audit_log %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, whereClause, argIdx, argIdx+1)
	args = append(args, filter.PerPage, offset)

	rows, err := r.db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []models.AuditLog
	for rows.Next() {
		var entry models.AuditLog
		if err := rows.Scan(&entry.ID, &entry.TenantID, &entry.UserID, &entry.APIKeyID,
			&entry.PluginName, &entry.Action, &entry.EntityType, &entry.EntityID,
			&entry.OldValues, &entry.NewValues, &entry.Diff,
			&entry.IPAddress, &entry.UserAgent, &entry.RequestID, &entry.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, entry)
	}

	totalPages := int(total) / filter.PerPage
	if int(total)%filter.PerPage > 0 {
		totalPages++
	}

	if items == nil {
		items = []models.AuditLog{}
	}

	return &models.AuditListResponse{
		Items:      items,
		Total:      total,
		Page:       filter.Page,
		PerPage:    filter.PerPage,
		TotalPages: totalPages,
	}, rows.Err()
}

// CountByAction returns counts grouped by action type.
func (r *AuditRepository) CountByAction(ctx context.Context) (map[string]int64, error) {
	query := `SELECT action, COUNT(*) as cnt FROM audit_log GROUP BY action ORDER BY cnt DESC LIMIT 20`
	rows, err := r.db.Pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]int64)
	for rows.Next() {
		var action string
		var count int64
		if err := rows.Scan(&action, &count); err != nil {
			return nil, err
		}
		result[action] = count
	}
	return result, rows.Err()
}

// CountByResource returns counts grouped by resource type.
func (r *AuditRepository) CountByResource(ctx context.Context) (map[string]int64, error) {
	query := `SELECT entity_type, COUNT(*) as cnt FROM audit_log WHERE entity_type != '' GROUP BY entity_type ORDER BY cnt DESC LIMIT 20`
	rows, err := r.db.Pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]int64)
	for rows.Next() {
		var resource string
		var count int64
		if err := rows.Scan(&resource, &count); err != nil {
			return nil, err
		}
		result[resource] = count
	}
	return result, rows.Err()
}
