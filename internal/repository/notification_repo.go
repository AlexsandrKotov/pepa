package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pepa/pepa/internal/database"
)

// NotificationRule represents a routing rule that maps event types to a notification connection.
type NotificationRule struct {
	ID              uuid.UUID         `json:"id"`
	TenantID        uuid.UUID         `json:"tenant_id"`
	Name            string            `json:"name"`
	Description     string            `json:"description"`
	Enabled         bool              `json:"enabled"`
	EventTypes      []string          `json:"event_types"`
	ConnectionID    uuid.UUID         `json:"connection_id"`
	SubjectTemplate string            `json:"subject_template,omitempty"`
	BodyTemplate    string            `json:"body_template"`
	FormatConfig    map[string]any    `json:"format_config"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

// NotificationLog represents a single delivery attempt in the notification history.
type NotificationLog struct {
	ID              uuid.UUID  `json:"id"`
	TenantID        uuid.UUID  `json:"tenant_id"`
	RuleID          *uuid.UUID `json:"rule_id,omitempty"`
	ConnectionID    uuid.UUID  `json:"connection_id"`
	EventType       string     `json:"event_type"`
	EventPayload    map[string]any `json:"event_payload,omitempty"`
	RenderedSubject string     `json:"rendered_subject,omitempty"`
	RenderedBody    string     `json:"rendered_body"`
	Provider        string     `json:"provider"`
	Status          string     `json:"status"` // pending, delivered, failed
	ResponseText    string     `json:"response_text,omitempty"`
	ErrorText       string     `json:"error_text,omitempty"`
	SentAt          time.Time  `json:"sent_at"`
	DeliveredAt     *time.Time `json:"delivered_at,omitempty"`
}

// NotificationStat holds aggregated delivery statistics for a connection.
type NotificationStat struct {
	ConnectionID   uuid.UUID  `json:"connection_id"`
	ConnectionName string     `json:"connection_name"`
	Provider       string     `json:"provider"`
	TotalSent      int        `json:"total_sent"`
	Delivered      int        `json:"delivered"`
	Failed         int        `json:"failed"`
	LastSentAt     *time.Time `json:"last_sent_at,omitempty"`
}

// NotificationRuleRepository handles persistence of notification routing rules.
type NotificationRuleRepository struct {
	pool *pgxpool.Pool
}

// NewNotificationRuleRepository creates a new notification rule repository.
func NewNotificationRuleRepository(db *database.DB) *NotificationRuleRepository {
	return &NotificationRuleRepository{pool: db.Pool}
}

// FindByTenant returns all notification rules for a tenant.
func (r *NotificationRuleRepository) FindByTenant(ctx context.Context, tenantID uuid.UUID) ([]NotificationRule, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, name, COALESCE(description,''), enabled,
		       event_types, connection_id, COALESCE(subject_template,''),
		       body_template, COALESCE(format_config,'{}'::jsonb),
		       created_at, updated_at
		FROM notification_rules
		WHERE tenant_id = $1
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query notification rules: %w", err)
	}
	defer rows.Close()

	items := make([]NotificationRule, 0)
	for rows.Next() {
		var rule NotificationRule
		var formatJSON []byte
		if err := rows.Scan(&rule.ID, &rule.TenantID, &rule.Name, &rule.Description,
			&rule.Enabled, &rule.EventTypes, &rule.ConnectionID,
			&rule.SubjectTemplate, &rule.BodyTemplate, &formatJSON,
			&rule.CreatedAt, &rule.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan notification rule: %w", err)
		}
		_ = json.Unmarshal(formatJSON, &rule.FormatConfig)
		if rule.FormatConfig == nil {
			rule.FormatConfig = map[string]any{}
		}
		items = append(items, rule)
	}
	return items, nil
}

// FindEnabledByEventType returns enabled rules that match a given event type for a tenant.
func (r *NotificationRuleRepository) FindEnabledByEventType(ctx context.Context, tenantID uuid.UUID, eventType string) ([]NotificationRule, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, name, COALESCE(description,''), enabled,
		       event_types, connection_id, COALESCE(subject_template,''),
		       body_template, COALESCE(format_config,'{}'::jsonb),
		       created_at, updated_at
		FROM notification_rules
		WHERE tenant_id = $1 AND enabled = true AND $2 = ANY(event_types)
		ORDER BY created_at DESC
	`, tenantID, eventType)
	if err != nil {
		return nil, fmt.Errorf("query notification rules by event type: %w", err)
	}
	defer rows.Close()

	items := make([]NotificationRule, 0)
	for rows.Next() {
		var rule NotificationRule
		var formatJSON []byte
		if err := rows.Scan(&rule.ID, &rule.TenantID, &rule.Name, &rule.Description,
			&rule.Enabled, &rule.EventTypes, &rule.ConnectionID,
			&rule.SubjectTemplate, &rule.BodyTemplate, &formatJSON,
			&rule.CreatedAt, &rule.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan notification rule: %w", err)
		}
		_ = json.Unmarshal(formatJSON, &rule.FormatConfig)
		if rule.FormatConfig == nil {
			rule.FormatConfig = map[string]any{}
		}
		items = append(items, rule)
	}
	return items, nil
}

// Get returns a notification rule by ID, scoped to a tenant.
func (r *NotificationRuleRepository) Get(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*NotificationRule, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, name, COALESCE(description,''), enabled,
		       event_types, connection_id, COALESCE(subject_template,''),
		       body_template, COALESCE(format_config,'{}'::jsonb),
		       created_at, updated_at
		FROM notification_rules
		WHERE id = $1 AND tenant_id = $2
	`, id, tenantID)

	var rule NotificationRule
	var formatJSON []byte
	if err := row.Scan(&rule.ID, &rule.TenantID, &rule.Name, &rule.Description,
		&rule.Enabled, &rule.EventTypes, &rule.ConnectionID,
		&rule.SubjectTemplate, &rule.BodyTemplate, &formatJSON,
		&rule.CreatedAt, &rule.UpdatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("notification rule not found: %s", id)
		}
		return nil, fmt.Errorf("get notification rule: %w", err)
	}
	_ = json.Unmarshal(formatJSON, &rule.FormatConfig)
	if rule.FormatConfig == nil {
		rule.FormatConfig = map[string]any{}
	}
	return &rule, nil
}

// Create inserts a new notification rule.
func (r *NotificationRuleRepository) Create(ctx context.Context, rule *NotificationRule) error {
	rule.ID = uuid.New()
	now := time.Now().UTC()
	rule.CreatedAt = now
	rule.UpdatedAt = now

	formatJSON, _ := json.Marshal(rule.FormatConfig)

	_, err := r.pool.Exec(ctx, `
		INSERT INTO notification_rules (id, tenant_id, name, description, enabled, event_types,
			connection_id, subject_template, body_template, format_config, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`, rule.ID, rule.TenantID, rule.Name, rule.Description, rule.Enabled,
		rule.EventTypes, rule.ConnectionID, rule.SubjectTemplate,
		rule.BodyTemplate, formatJSON, rule.CreatedAt, rule.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create notification rule: %w", err)
	}
	return nil
}

// Update modifies an existing notification rule.
func (r *NotificationRuleRepository) Update(ctx context.Context, rule *NotificationRule) error {
	rule.UpdatedAt = time.Now().UTC()
	formatJSON, _ := json.Marshal(rule.FormatConfig)

	_, err := r.pool.Exec(ctx, `
		UPDATE notification_rules
		SET name=$3, description=$4, enabled=$5, event_types=$6,
		    connection_id=$7, subject_template=$8, body_template=$9,
		    format_config=$10, updated_at=$11
		WHERE id=$1 AND tenant_id=$2
	`, rule.ID, rule.TenantID, rule.Name, rule.Description, rule.Enabled,
		rule.EventTypes, rule.ConnectionID, rule.SubjectTemplate,
		rule.BodyTemplate, formatJSON, rule.UpdatedAt)
	if err != nil {
		return fmt.Errorf("update notification rule: %w", err)
	}
	return nil
}

// Delete removes a notification rule.
func (r *NotificationRuleRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM notification_rules WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete notification rule: %w", err)
	}
	return nil
}

// NotificationLogRepository handles persistence of notification delivery logs.
type NotificationLogRepository struct {
	pool *pgxpool.Pool
}

// NewNotificationLogRepository creates a new notification log repository.
func NewNotificationLogRepository(db *database.DB) *NotificationLogRepository {
	return &NotificationLogRepository{pool: db.Pool}
}

// Create inserts a new notification log entry.
func (r *NotificationLogRepository) Create(ctx context.Context, log *NotificationLog) error {
	log.ID = uuid.New()
	if log.SentAt.IsZero() {
		log.SentAt = time.Now().UTC()
	}

	payloadJSON, _ := json.Marshal(log.EventPayload)

	_, err := r.pool.Exec(ctx, `
		INSERT INTO notification_logs (id, tenant_id, rule_id, connection_id, event_type,
			event_payload, rendered_subject, rendered_body, provider, status,
			response_text, error_text, sent_at, delivered_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
	`, log.ID, log.TenantID, log.RuleID, log.ConnectionID, log.EventType,
		payloadJSON, log.RenderedSubject, log.RenderedBody, log.Provider,
		log.Status, log.ResponseText, log.ErrorText, log.SentAt, log.DeliveredAt)
	if err != nil {
		return fmt.Errorf("create notification log: %w", err)
	}
	return nil
}

// LogFilter holds optional filters for listing notification logs.
type LogFilter struct {
	ConnectionID *uuid.UUID
	Status       string
	EventType    string
	Page         int
	PerPage      int
}

// FindByTenant returns paginated notification logs for a tenant with optional filters.
func (r *NotificationLogRepository) FindByTenant(ctx context.Context, tenantID uuid.UUID, f LogFilter) ([]NotificationLog, int, error) {
	// Build WHERE clause dynamically
	where := "WHERE tenant_id = $1"
	args := []interface{}{tenantID}
	argIdx := 2

	if f.ConnectionID != nil {
		where += fmt.Sprintf(" AND connection_id = $%d", argIdx)
		args = append(args, *f.ConnectionID)
		argIdx++
	}
	if f.Status != "" {
		where += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, f.Status)
		argIdx++
	}
	if f.EventType != "" {
		where += fmt.Sprintf(" AND event_type = $%d", argIdx)
		args = append(args, f.EventType)
		argIdx++
	}

	// Count total
	var total int
	countQuery := "SELECT COUNT(*) FROM notification_logs " + where
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count notification logs: %w", err)
	}

	// Apply pagination defaults
	page := f.Page
	if page < 1 {
		page = 1
	}
	perPage := f.PerPage
	if perPage < 1 {
		perPage = 50
	}
	if perPage > 200 {
		perPage = 200
	}
	offset := (page - 1) * perPage

	// Fetch page
	dataQuery := fmt.Sprintf(`
		SELECT id, tenant_id, rule_id, connection_id, event_type,
		       COALESCE(event_payload,'{}'::jsonb), COALESCE(rendered_subject,''),
		       COALESCE(rendered_body,''), provider, status,
		       COALESCE(response_text,''), COALESCE(error_text,''),
		       sent_at, delivered_at
		FROM notification_logs %s
		ORDER BY sent_at DESC
		LIMIT $%d OFFSET $%d
	`, where, argIdx, argIdx+1)
	args = append(args, perPage, offset)

	rows, err := r.pool.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query notification logs: %w", err)
	}
	defer rows.Close()

	items := make([]NotificationLog, 0)
	for rows.Next() {
		var log NotificationLog
		var payloadJSON []byte
		if err := rows.Scan(&log.ID, &log.TenantID, &log.RuleID, &log.ConnectionID,
			&log.EventType, &payloadJSON, &log.RenderedSubject, &log.RenderedBody,
			&log.Provider, &log.Status, &log.ResponseText, &log.ErrorText,
			&log.SentAt, &log.DeliveredAt); err != nil {
			return nil, 0, fmt.Errorf("scan notification log: %w", err)
		}
		_ = json.Unmarshal(payloadJSON, &log.EventPayload)
		items = append(items, log)
	}
	return items, total, nil
}

// StatsByConnection returns aggregated delivery statistics per connection for a tenant.
func (r *NotificationLogRepository) StatsByConnection(ctx context.Context, tenantID uuid.UUID) ([]NotificationStat, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT nl.connection_id,
		       COALESCE(c.name, '') AS connection_name,
		       nl.provider,
		       COUNT(*) AS total_sent,
		       COUNT(*) FILTER (WHERE nl.status = 'delivered') AS delivered,
		       COUNT(*) FILTER (WHERE nl.status = 'failed') AS failed,
		       MAX(nl.sent_at) AS last_sent_at
		FROM notification_logs nl
		LEFT JOIN connections c ON c.id = nl.connection_id
		WHERE nl.tenant_id = $1
		GROUP BY nl.connection_id, c.name, nl.provider
		ORDER BY total_sent DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query notification stats: %w", err)
	}
	defer rows.Close()

	stats := make([]NotificationStat, 0)
	for rows.Next() {
		var s NotificationStat
		if err := rows.Scan(&s.ConnectionID, &s.ConnectionName, &s.Provider,
			&s.TotalSent, &s.Delivered, &s.Failed, &s.LastSentAt); err != nil {
			return nil, fmt.Errorf("scan notification stat: %w", err)
		}
		stats = append(stats, s)
	}
	return stats, nil
}
