package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ============================================================
// Entity — Universal entity stored in the platform
// ============================================================

type Entity struct {
	ID             uuid.UUID       `json:"id" db:"id"`
	TypeID         uuid.UUID       `json:"type_id" db:"type_id"`
	TypeKey        string          `json:"type_key" db:"type_key"`
	Name           string          `json:"name" db:"name"`
	Description    string          `json:"description,omitempty" db:"description"`
	ExternalID     string          `json:"external_id,omitempty" db:"external_id"`
	TenantID       uuid.UUID       `json:"tenant_id" db:"tenant_id"`
	OrganizationID uuid.UUID       `json:"organization_id" db:"organization_id"`
	Metadata       json.RawMessage `json:"metadata" db:"metadata"`
	Status         string          `json:"status" db:"status"`
	StatusDetail   string          `json:"status_detail,omitempty" db:"status_detail"`
	PluginName     string          `json:"plugin_name,omitempty" db:"plugin_name"`
	SyncStatus     string          `json:"sync_status" db:"sync_status"`
	LastSyncedAt   *time.Time      `json:"last_synced_at,omitempty" db:"last_synced_at"`
	CreatedBy      *uuid.UUID      `json:"created_by,omitempty" db:"created_by"`
	UpdatedBy      *uuid.UUID      `json:"updated_by,omitempty" db:"updated_by"`
	CreatedAt      time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at" db:"updated_at"`
	DeletedAt      *time.Time      `json:"deleted_at,omitempty" db:"deleted_at"`
}

// EntityType — registered type definition for entities
type EntityType struct {
	ID             uuid.UUID       `json:"id" db:"id"`
	TypeKey        string          `json:"type_key" db:"type_key"`
	DisplayName    string          `json:"display_name" db:"display_name"`
	Description    string          `json:"description,omitempty" db:"description"`
	PluginName     string          `json:"plugin_name,omitempty" db:"plugin_name"`
	Icon           string          `json:"icon,omitempty" db:"icon"`
	Category       string          `json:"category,omitempty" db:"category"`
	MetadataSchema json.RawMessage `json:"metadata_schema" db:"metadata_schema"`
	UIConfig       json.RawMessage `json:"ui_config,omitempty" db:"ui_config"`
	IsSystem       bool            `json:"is_system" db:"is_system"`
	IsEnabled      bool            `json:"is_enabled" db:"is_enabled"`
	CreatedAt      time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at" db:"updated_at"`
}

// EntityRelationship — edge between two entities
type EntityRelationship struct {
	ID                 uuid.UUID       `json:"id" db:"id"`
	RelationshipTypeID uuid.UUID       `json:"relationship_type_id" db:"relationship_type_id"`
	TypeKey            string          `json:"type_key" db:"type_key"`
	SourceID           uuid.UUID       `json:"source_id" db:"source_id"`
	TargetID           uuid.UUID       `json:"target_id" db:"target_id"`
	Metadata           json.RawMessage `json:"metadata,omitempty" db:"metadata"`
	TenantID           uuid.UUID       `json:"tenant_id" db:"tenant_id"`
	PluginName         string          `json:"plugin_name,omitempty" db:"plugin_name"`
	CreatedAt          time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at" db:"updated_at"`
}

// RelationshipType — defines valid relationship between entity types
type RelationshipType struct {
	ID            uuid.UUID `json:"id" db:"id"`
	TypeKey       string    `json:"type_key" db:"type_key"`
	DisplayName   string    `json:"display_name" db:"display_name"`
	Description   string    `json:"description,omitempty" db:"description"`
	SourceTypes   []string  `json:"source_types" db:"source_types"`
	TargetTypes   []string  `json:"target_types" db:"target_types"`
	Cardinality   string    `json:"cardinality" db:"cardinality"`
	DisplayColor  string    `json:"display_color,omitempty" db:"display_color"`
	DisplayLabel  string    `json:"display_label,omitempty" db:"display_label"`
	IsDirectional bool      `json:"is_directional" db:"is_directional"`
	PluginName    string    `json:"plugin_name,omitempty" db:"plugin_name"`
	IsSystem      bool      `json:"is_system" db:"is_system"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
}

// GraphNode — entity with graph context
type GraphNode struct {
	Entity Entity   `json:"entity"`
	Depth  int      `json:"depth"`
	Path   []string `json:"path"`
}

// GraphEdge — relationship with resolved endpoints
type GraphEdge struct {
	Relationship EntityRelationship `json:"relationship"`
	Source       Entity             `json:"source"`
	Target       Entity             `json:"target"`
	Direction    string             `json:"direction"` // outgoing, incoming
}

// GraphResult — full graph traversal result
type GraphResult struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// ── Request/Response types ──────────────────────────────────

type CreateEntityRequest struct {
	TypeKey     string          `json:"type_key" binding:"required"`
	Name        string          `json:"name" binding:"required"`
	Description string          `json:"description,omitempty"`
	ExternalID  string          `json:"external_id,omitempty"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
}

type UpdateEntityRequest struct {
	Name        *string         `json:"name,omitempty"`
	Description *string         `json:"description,omitempty"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
	Status      *string         `json:"status,omitempty"`
}

type EntityFilter struct {
	TypeKey    string   `form:"type"`
	Status     string   `form:"status"`
	PluginName string   `form:"plugin"`
	Search     string   `form:"search"`
	Tags       []string `form:"tags"`
	Page       int      `form:"page,default=1"`
	PerPage    int      `form:"per_page,default=20"`
	// TenantID scopes the query to a single tenant. It is set server-side from
	// the authenticated request and must never be bound from query parameters.
	// A zero UUID means no tenant filtering (platform admins / internal use).
	TenantID uuid.UUID `form:"-"`
}

type EntityListResponse struct {
	Items      []Entity `json:"items"`
	Total      int64    `json:"total"`
	Page       int      `json:"page"`
	PerPage    int      `json:"per_page"`
	TotalPages int      `json:"total_pages"`
}
