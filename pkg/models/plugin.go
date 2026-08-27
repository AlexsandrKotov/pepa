package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ============================================================
// Plugin — registered plugin instance
// ============================================================

type PluginType string

const (
	PluginTypeGitProvider    PluginType = "git_provider"
	PluginTypeTaskTracker    PluginType = "task_tracker"
	PluginTypeCDEngine       PluginType = "cd_engine"
	PluginTypeCloudProvider  PluginType = "cloud_provider"
	PluginTypeMonitoring     PluginType = "monitoring"
	PluginTypeSecretManager  PluginType = "secret_manager"
	PluginTypeNotification   PluginType = "notification"
	PluginTypeCIEngine       PluginType = "ci_engine"
	PluginTypeIdentity       PluginType = "identity"
	PluginTypeCustom         PluginType = "custom"
	PluginTypeVirtualization PluginType = "virtualization"
)

type PluginStatus string

const (
	PluginStatusAvailable    PluginStatus = "available"
	PluginStatusInstalled    PluginStatus = "installed"
	PluginStatusInitializing PluginStatus = "initializing"
	PluginStatusRunning      PluginStatus = "running"
	PluginStatusDegraded     PluginStatus = "degraded"
	PluginStatusStopping     PluginStatus = "stopping"
	PluginStatusStopped      PluginStatus = "stopped"
	PluginStatusDisabled     PluginStatus = "disabled"
	PluginStatusFailed       PluginStatus = "failed"
)

// Plugin — a registered plugin in the system
type Plugin struct {
	ID          uuid.UUID       `json:"id" db:"id"`
	Name        string          `json:"name" db:"name"`
	Version     string          `json:"version" db:"version"`
	Type        PluginType      `json:"type" db:"type"`
	Status      PluginStatus    `json:"status" db:"status"`
	Config      json.RawMessage `json:"config,omitempty" db:"config"`
	Enabled     bool            `json:"enabled" db:"enabled"`
	TenantID    *uuid.UUID      `json:"tenant_id,omitempty" db:"tenant_id"`
	InstalledAt time.Time       `json:"installed_at" db:"installed_at"`
	UpdatedAt   time.Time       `json:"updated_at" db:"updated_at"`
}

// PluginManifest — parsed from plugin.yaml
type PluginManifest struct {
	APIVersion string         `json:"api_version" yaml:"apiVersion"`
	Kind       string         `json:"kind" yaml:"kind"`
	Metadata   PluginMetadata `json:"metadata" yaml:"metadata"`
	Spec       PluginSpec     `json:"spec" yaml:"spec"`
}

type PluginMetadata struct {
	Name        string   `json:"name" yaml:"name"`
	Version     string   `json:"version" yaml:"version"`
	Description string   `json:"description" yaml:"description"`
	Author      string   `json:"author" yaml:"author"`
	License     string   `json:"license" yaml:"license"`
	Icon        string   `json:"icon" yaml:"icon"`
	Categories  []string `json:"categories" yaml:"categories"`
}

type PluginSpec struct {
	Type         PluginType      `json:"type" yaml:"type"`
	Runtime      PluginRuntime   `json:"runtime" yaml:"runtime"`
	ConfigSchema json.RawMessage `json:"config_schema" yaml:"configSchema"`
	Capabilities []string        `json:"capabilities" yaml:"capabilities"`
	Events       []string        `json:"events" yaml:"events"`
	HealthCheck  HealthCheckSpec `json:"health_check" yaml:"healthCheck"`
	Resources    ResourceSpec    `json:"resources" yaml:"resources"`
}

type PluginRuntime struct {
	Type       string `json:"type" yaml:"type"` // binary, container, wasm
	Binary     string `json:"binary,omitempty" yaml:"binary"`
	Image      string `json:"container,omitempty" yaml:"container"`
	WasmModule string `json:"wasm,omitempty" yaml:"wasm"`
}

type HealthCheckSpec struct {
	Interval string `json:"interval" yaml:"interval"`
	Timeout  string `json:"timeout" yaml:"timeout"`
}

type ResourceSpec struct {
	Memory string `json:"memory" yaml:"memory"`
	CPU    string `json:"cpu" yaml:"cpu"`
}

// PluginHealth — current health status
type PluginHealth struct {
	PluginName string        `json:"plugin_name"`
	Status     string        `json:"status"` // healthy, degraded, unhealthy
	Message    string        `json:"message,omitempty"`
	CheckedAt  time.Time     `json:"checked_at"`
	Latency    time.Duration `json:"latency"`
}

// ── Request types ────────────────────────────────────────────

type InstallPluginRequest struct {
	Name    string          `json:"name" binding:"required"`
	Version string          `json:"version" binding:"required"`
	Config  json.RawMessage `json:"config,omitempty"`
}

type ConfigurePluginRequest struct {
	Config json.RawMessage `json:"config" binding:"required"`
}
