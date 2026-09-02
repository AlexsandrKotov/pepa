package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/database"
)

// SSHCommandLog represents a single SSH command executed via the remote console.
type SSHCommandLog struct {
	ID        uuid.UUID  `json:"id" db:"id"`
	TenantID  uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	UserID    *uuid.UUID `json:"user_id,omitempty" db:"user_id"`
	HostID    uuid.UUID  `json:"host_id" db:"host_id"`
	HostName  string     `json:"host_name" db:"host_name"`
	Username  string     `json:"username" db:"username"`
	Command   string     `json:"command" db:"command"`
	ExitCode  *int      `json:"exit_code,omitempty" db:"exit_code"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// PluginActionLog represents a plugin action (VM start/stop/create/delete etc.).
type PluginActionLog struct {
	ID           uuid.UUID       `json:"id" db:"id"`
	TenantID     uuid.UUID       `json:"tenant_id" db:"tenant_id"`
	UserID       *uuid.UUID      `json:"user_id,omitempty" db:"user_id"`
	PluginName   string          `json:"plugin_name" db:"plugin_name"`
	Action       string          `json:"action" db:"action"`
	EntityType   string          `json:"entity_type" db:"entity_type"`
	EntityID     string          `json:"entity_id" db:"entity_id"`
	EntityName   string          `json:"entity_name" db:"entity_name"`
	Params       json.RawMessage `json:"params,omitempty" db:"params"`
	Status       string          `json:"status" db:"status"`
	ErrorMessage string          `json:"error_message,omitempty" db:"error_message"`
	IPAddress    string          `json:"ip_address,omitempty" db:"ip_address"`
	CreatedAt    time.Time       `json:"created_at" db:"created_at"`
}

// PluginActivityRepository handles SSH command and plugin action log operations.
type PluginActivityRepository struct {
	db *database.DB
}

func NewPluginActivityRepository(db *database.DB) *PluginActivityRepository {
	return &PluginActivityRepository{db: db}
}

// LogSSHCommand inserts an SSH command log entry.
func (r *PluginActivityRepository) LogSSHCommand(ctx context.Context, entry *SSHCommandLog) error {
	entry.ID = uuid.New()
	query := `INSERT INTO ssh_command_log (id, tenant_id, user_id, host_id, host_name, username, command)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`
	_, err := r.db.Pool.Exec(ctx, query,
		entry.ID, entry.TenantID, entry.UserID, entry.HostID,
		entry.HostName, entry.Username, entry.Command,
	)
	return err
}

// ListSSHCommands returns SSH command logs matching the filter.
func (r *PluginActivityRepository) ListSSHCommands(ctx context.Context, filter map[string]string) ([]SSHCommandLog, int64, error) {
	where := []string{}
	args := []interface{}{}
	argIdx := 0

	if v := filter["tenant_id"]; v != "" {
		argIdx++
		where = append(where, fmt.Sprintf("tenant_id = $%d::uuid", argIdx))
		args = append(args, v)
	}
	if v := filter["user_id"]; v != "" {
		argIdx++
		where = append(where, fmt.Sprintf("user_id = $%d::uuid", argIdx))
		args = append(args, v)
	}
	if v := filter["host_id"]; v != "" {
		argIdx++
		where = append(where, fmt.Sprintf("host_id = $%d::uuid", argIdx))
		args = append(args, v)
	}
	if v := filter["host_name"]; v != "" {
		argIdx++
		where = append(where, fmt.Sprintf("host_name ILIKE $%d", argIdx))
		args = append(args, "%"+v+"%")
	}
	if v := filter["command"]; v != "" {
		argIdx++
		where = append(where, fmt.Sprintf("command ILIKE $%d", argIdx))
		args = append(args, "%"+v+"%")
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM ssh_command_log %s", whereClause)
	if err := r.db.Pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	perPage := 50
	page := 1
	if v := filter["page"]; v != "" {
		fmt.Sscanf(v, "%d", &page)
	}
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * perPage

	query := fmt.Sprintf(`SELECT id, tenant_id, user_id, host_id, host_name, username, command, exit_code, created_at
		FROM ssh_command_log %s ORDER BY created_at DESC LIMIT %d OFFSET %d`, whereClause, perPage, offset)
	rows, err := r.db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []SSHCommandLog
	for rows.Next() {
		var entry SSHCommandLog
		if err := rows.Scan(&entry.ID, &entry.TenantID, &entry.UserID, &entry.HostID,
			&entry.HostName, &entry.Username, &entry.Command, &entry.ExitCode, &entry.CreatedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, entry)
	}
	if items == nil {
		items = []SSHCommandLog{}
	}
	return items, total, rows.Err()
}

// LogPluginAction inserts a plugin action log entry.
func (r *PluginActivityRepository) LogPluginAction(ctx context.Context, entry *PluginActionLog) error {
	entry.ID = uuid.New()
	query := `INSERT INTO plugin_action_log (id, tenant_id, user_id, plugin_name, action, entity_type, entity_id, entity_name, params, status, error_message, ip_address)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`
	_, err := r.db.Pool.Exec(ctx, query,
		entry.ID, entry.TenantID, entry.UserID, entry.PluginName,
		entry.Action, entry.EntityType, entry.EntityID, entry.EntityName,
		entry.Params, entry.Status, entry.ErrorMessage, entry.IPAddress,
	)
	return err
}

// ListPluginActions returns plugin action logs matching the filter.
func (r *PluginActivityRepository) ListPluginActions(ctx context.Context, filter map[string]string) ([]PluginActionLog, int64, error) {
	where := []string{}
	args := []interface{}{}
	argIdx := 0

	if v := filter["tenant_id"]; v != "" {
		argIdx++
		where = append(where, fmt.Sprintf("tenant_id = $%d::uuid", argIdx))
		args = append(args, v)
	}
	if v := filter["user_id"]; v != "" {
		argIdx++
		where = append(where, fmt.Sprintf("user_id = $%d::uuid", argIdx))
		args = append(args, v)
	}
	if v := filter["plugin_name"]; v != "" {
		argIdx++
		where = append(where, fmt.Sprintf("plugin_name = $%d", argIdx))
		args = append(args, v)
	}
	if v := filter["action"]; v != "" {
		argIdx++
		where = append(where, fmt.Sprintf("action = $%d", argIdx))
		args = append(args, v)
	}
	if v := filter["entity_type"]; v != "" {
		argIdx++
		where = append(where, fmt.Sprintf("entity_type = $%d", argIdx))
		args = append(args, v)
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM plugin_action_log %s", whereClause)
	if err := r.db.Pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	perPage := 50
	page := 1
	if v := filter["page"]; v != "" {
		fmt.Sscanf(v, "%d", &page)
	}
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * perPage

	query := fmt.Sprintf(`SELECT id, tenant_id, user_id, plugin_name, action, entity_type, entity_id, entity_name, params, status, error_message, ip_address, created_at
		FROM plugin_action_log %s ORDER BY created_at DESC LIMIT %d OFFSET %d`, whereClause, perPage, offset)
	rows, err := r.db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []PluginActionLog
	for rows.Next() {
		var entry PluginActionLog
		if err := rows.Scan(&entry.ID, &entry.TenantID, &entry.UserID, &entry.PluginName,
			&entry.Action, &entry.EntityType, &entry.EntityID, &entry.EntityName,
			&entry.Params, &entry.Status, &entry.ErrorMessage, &entry.IPAddress, &entry.CreatedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, entry)
	}
	if items == nil {
		items = []PluginActionLog{}
	}
	return items, total, rows.Err()
}
