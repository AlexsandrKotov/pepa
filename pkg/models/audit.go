package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ============================================================
// AuditLog — Immutable audit trail
// ============================================================

type AuditLog struct {
	ID         uuid.UUID       `json:"id" db:"id"`
	TenantID   uuid.UUID       `json:"tenant_id" db:"tenant_id"`
	UserID     *uuid.UUID      `json:"user_id,omitempty" db:"user_id"`
	APIKeyID   *uuid.UUID      `json:"api_key_id,omitempty" db:"api_key_id"`
	PluginName string          `json:"plugin_name,omitempty" db:"plugin_name"`
	Action     string          `json:"action" db:"action"`                     // create, update, delete, login, execute, etc.
	EntityType string          `json:"entity_type,omitempty" db:"entity_type"` // entity, workflow, plugin, etc.
	EntityID   *uuid.UUID      `json:"entity_id,omitempty" db:"entity_id"`
	OldValues  json.RawMessage `json:"old_values,omitempty" db:"old_values"`
	NewValues  json.RawMessage `json:"new_values,omitempty" db:"new_values"`
	Diff       json.RawMessage `json:"diff,omitempty" db:"diff"`
	IPAddress  string          `json:"ip_address,omitempty" db:"ip_address"`
	UserAgent  string          `json:"user_agent,omitempty" db:"user_agent"`
	RequestID  *uuid.UUID      `json:"request_id,omitempty" db:"request_id"`
	CreatedAt  time.Time       `json:"created_at" db:"created_at"`
}

// AuditFilter — query parameters for audit log listing
type AuditFilter struct {
	Action     string `form:"action"`
	EntityType string `form:"entity_type"`
	EntityID   string `form:"entity_id"`
	UserID     string `form:"user_id"`
	StartDate  string `form:"start_date"`
	EndDate    string `form:"end_date"`
	Page       int    `form:"page,default=1"`
	PerPage    int    `form:"per_page,default=50"`
}

type AuditListResponse struct {
	Items      []AuditLog `json:"items"`
	Total      int64      `json:"total"`
	Page       int        `json:"page"`
	PerPage    int        `json:"per_page"`
	TotalPages int        `json:"total_pages"`
}
