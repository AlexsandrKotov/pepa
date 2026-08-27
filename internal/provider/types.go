package provider

import (
	"encoding/json"
	"time"
)

// ── Git Provider Types ────────────────────────────────────────

// GitGroup represents a grouping entity that varies by provider:
// GitLab=Group/Subgroup, GitHub=Organization, Gitea=Organization, Bitbucket=Workspace.
type GitGroup struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	URL      string `json:"url,omitempty"`
	Kind     string `json:"kind"` // "group", "organization", "workspace"
}

// GitBrowseResult is the unified response for hierarchical git browsing.
type GitBrowseResult struct {
	Groups    []GitGroup    `json:"groups,omitempty"`
	Repos     []Repository  `json:"repos,omitempty"`
	Pipelines []PipelineRun `json:"pipelines,omitempty"`
	HasMore   bool          `json:"has_more"`
}

type Repository struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	FullName      string    `json:"full_name"`
	Description   string    `json:"description,omitempty"`
	URL           string    `json:"url"`
	DefaultBranch string    `json:"default_branch"`
	Private       bool      `json:"private"`
	CreatedAt     time.Time `json:"created_at,omitempty"`
	UpdatedAt     time.Time `json:"updated_at,omitempty"`
}

type Branch struct {
	Name      string `json:"name"`
	SHA       string `json:"sha"`
	Protected bool   `json:"protected"`
	URL       string `json:"url,omitempty"`
}

type MergeRequest struct {
	ID           int        `json:"id"`
	IID          int        `json:"iid"`
	Title        string     `json:"title"`
	Description  string     `json:"description,omitempty"`
	SourceBranch string     `json:"source_branch"`
	TargetBranch string     `json:"target_branch"`
	State        string     `json:"state"` // opened, merged, closed
	Author       string     `json:"author,omitempty"`
	URL          string     `json:"url,omitempty"`
	CreatedAt    time.Time  `json:"created_at,omitempty"`
	MergedAt     *time.Time `json:"merged_at,omitempty"`
}

type CreateMRRequest struct {
	RepoID       string `json:"repo_id"`
	Title        string `json:"title"`
	Description  string `json:"description,omitempty"`
	SourceBranch string `json:"source_branch"`
	TargetBranch string `json:"target_branch"`
}

type CreateBranchRequest struct {
	RepoID string `json:"repo_id"`
	Name   string `json:"name"`
	Ref    string `json:"ref"` // SHA or branch name to branch from
}

// ── Task Tracker Types ────────────────────────────────────────

type Project struct {
	ID       string `json:"id"`
	Key      string `json:"key"`
	Name     string `json:"name"`
	URL      string `json:"url,omitempty"`
	Category string `json:"category,omitempty"`
}

type Issue struct {
	ID          string            `json:"id"`
	Key         string            `json:"key"`
	Summary     string            `json:"summary"`
	Description string            `json:"description,omitempty"`
	Status      string            `json:"status"`
	Priority    string            `json:"priority,omitempty"`
	Type        string            `json:"type,omitempty"` // bug, story, task, etc.
	Assignee    string            `json:"assignee,omitempty"`
	Reporter    string            `json:"reporter,omitempty"`
	Labels      []string          `json:"labels,omitempty"`
	URL         string            `json:"url,omitempty"`
	Fields      map[string]string `json:"fields,omitempty"`
	CreatedAt   time.Time         `json:"created_at,omitempty"`
	UpdatedAt   time.Time         `json:"updated_at,omitempty"`
}

type CreateIssueRequest struct {
	ProjectKey  string            `json:"project_key"`
	Summary     string            `json:"summary"`
	Description string            `json:"description,omitempty"`
	Type        string            `json:"type"` // bug, story, task
	Priority    string            `json:"priority,omitempty"`
	Assignee    string            `json:"assignee,omitempty"`
	Labels      []string          `json:"labels,omitempty"`
	Fields      map[string]string `json:"fields,omitempty"`
}

type TransitionRequest struct {
	IssueKey       string `json:"issue_key"`
	TransitionID   string `json:"transition_id"`
	TransitionName string `json:"transition_name,omitempty"`
	Comment        string `json:"comment,omitempty"`
}

type Transition struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	To   string `json:"to"` // target status
}

// Comment represents a Jira issue comment.
type Comment struct {
	ID      string    `json:"id"`
	Body    string    `json:"body"`
	Author  string    `json:"author"`
	Created time.Time `json:"created,omitempty"`
	Updated time.Time `json:"updated,omitempty"`
}

// AutomationAction represents a Jira automation action payload.
type AutomationAction struct {
	Type   string            `json:"type"` // add_comment, transition, update_field, notify
	Config map[string]string `json:"config"`
}

// DeploymentNotification holds data for auto-commenting on Jira issues.
type DeploymentNotification struct {
	IssueKey    string `json:"issue_key"`
	Event       string `json:"event"` // deployment_started, deployment_succeeded, deployment_failed
	ServiceName string `json:"service_name"`
	Environment string `json:"environment"`
	Cluster     string `json:"cluster"`
	ImageTag    string `json:"image_tag"`
	User        string `json:"user"`
	Duration    string `json:"duration"`
	Error       string `json:"error"`
	LogURL      string `json:"log_url"`
}

// ── CD Engine Types ───────────────────────────────────────────

type CDApplication struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace,omitempty"`
	Status     string `json:"status"` // Healthy, Degraded, Progressing, Missing
	Health     string `json:"health"`
	SyncStatus string `json:"sync_status"` // Synced, OutOfSync
	Revision   string `json:"revision,omitempty"`
	RepoURL    string `json:"repo_url,omitempty"`
	TargetPath string `json:"target_path,omitempty"`
	URL        string `json:"url,omitempty"`
}

type DeployStatus struct {
	Application string    `json:"application"`
	Status      string    `json:"status"`
	Revision    string    `json:"revision"`
	Synced      bool      `json:"synced"`
	Health      string    `json:"health"`
	UpdatedAt   time.Time `json:"updated_at"`
	Operations  []string  `json:"operations,omitempty"`
}

type SyncRequest struct {
	Application string `json:"application"`
	Revision    string `json:"revision,omitempty"`
	Prune       bool   `json:"prune,omitempty"`
	DryRun      bool   `json:"dry_run,omitempty"`
}

type RollbackRequest struct {
	Application string `json:"application"`
	Revision    string `json:"revision"`
}

// ── CI Provider Types ─────────────────────────────────────────

type PipelineRun struct {
	ID         string     `json:"id"`
	Ref        string     `json:"ref"` // branch or tag
	SHA        string     `json:"sha"`
	Status     string     `json:"status"` // pending, running, success, failed, canceled
	Source     string     `json:"source,omitempty"`
	URL        string     `json:"url,omitempty"`
	Duration   int        `json:"duration_seconds,omitempty"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

type TriggerPipelineRequest struct {
	RepoID    string            `json:"repo_id"`
	Ref       string            `json:"ref"`
	Variables map[string]string `json:"variables,omitempty"`
}

// ── Notification Types ────────────────────────────────────────

type Channel struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Type  string `json:"type,omitempty"` // channel, group, dm
	Topic string `json:"topic,omitempty"`
}

type SendNotificationRequest struct {
	ChannelID string          `json:"channel_id,omitempty"`
	Channel   string          `json:"channel,omitempty"` // channel name
	User      string          `json:"user,omitempty"`    // user ID for DM
	Text      string          `json:"text"`
	Blocks    json.RawMessage `json:"blocks,omitempty"` // rich formatting
}

// ── Monitoring Types ──────────────────────────────────────────

type MetricQuery struct {
	Query string    `json:"query"`
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
	Step  string    `json:"step,omitempty"` // resolution
}

type MetricResult struct {
	Metric map[string]string `json:"metric"`
	Values []MetricValue     `json:"values"`
}

type MetricValue struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

type Alert struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Status      string            `json:"status"` // firing, pending, resolved
	Severity    string            `json:"severity"`
	Summary     string            `json:"summary"`
	Description string            `json:"description,omitempty"`
	URL         string            `json:"url,omitempty"`
	StartsAt    time.Time         `json:"starts_at"`
	EndsAt      *time.Time        `json:"ends_at,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// ── Virtualization Types ──────────────────────────────────────

// VirtualNode represents a hypervisor node (Proxmox node, VMware host, etc.)
type VirtualNode struct {
	Name     string  `json:"name"`
	Status   string  `json:"status"` // online, offline
	CPU      float64 `json:"cpu_usage"`
	MemUsed  uint64  `json:"mem_used"`
	MemTotal uint64  `json:"mem_total"`
	Uptime   int64   `json:"uptime_seconds"`
	Version  string  `json:"version,omitempty"`
}

// VirtualMachine represents a VM or LXC container in a virtualization platform.
type VirtualMachine struct {
	VMID      int      `json:"vmid"`
	Name      string   `json:"name"`
	Node      string   `json:"node"`
	Status    string   `json:"status"` // running, stopped, paused
	Type      string   `json:"type"`   // qemu, lxc
	CPU       float64  `json:"cpu_usage"`
	MemUsed   uint64   `json:"mem_used"`
	MemTotal  uint64   `json:"mem_max"`
	DiskUsed  uint64   `json:"disk_used"`
	DiskTotal uint64   `json:"disk_total"`
	Uptime    int64    `json:"uptime_seconds"`
	IP        string   `json:"ip,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	Template  bool     `json:"template,omitempty"`
}

// CreateVMRequest is the payload for creating a new virtual machine.
type CreateVMRequest struct {
	Node     string `json:"node"`
	VMID     int    `json:"vmid,omitempty"` // auto if 0
	Name     string `json:"name"`
	Template int    `json:"template,omitempty"` // clone from template VMID
	Cores    int    `json:"cores"`
	Memory   int    `json:"memory_mb"`
	DiskSize string `json:"disk_size,omitempty"` // e.g. "32G"
	Storage  string `json:"storage,omitempty"`   // disk storage, defaults to local-lvm
	ISO      string `json:"iso,omitempty"`       // e.g. "local:iso/debian-12.iso" (boot from ISO)
	Network  string `json:"network,omitempty"`   // bridge name
	Start    bool   `json:"start,omitempty"`
}

// CreateContainerRequest is the payload for creating a new LXC container.
type CreateContainerRequest struct {
	Node     string `json:"node"`
	VMID     int    `json:"vmid,omitempty"`
	Hostname string `json:"hostname"`
	Template string `json:"template"` // e.g. "local:vztmpl/ubuntu-22.04-standard.tar.zst"
	Password string `json:"password,omitempty"`
	Cores    int    `json:"cores"`
	Memory   int    `json:"memory_mb"`
	DiskSize string `json:"disk_size,omitempty"`
	Storage  string `json:"storage,omitempty"`  // rootfs storage, defaults to local-lvm
	SSHKeys  string `json:"ssh_keys,omitempty"` // newline-separated public keys (URL-encoded by client)
	Network  string `json:"network,omitempty"`
	Start    bool   `json:"start,omitempty"`
}

// ClusterResource is a generic resource from the cluster overview.
type ClusterResource struct {
	ID       string  `json:"id"`
	Type     string  `json:"type"` // node, qemu, lxc, storage, net, pool
	Name     string  `json:"name"`
	Node     string  `json:"node,omitempty"`
	Status   string  `json:"status"`
	CPU      float64 `json:"cpu,omitempty"`
	MaxCPU   int     `json:"maxcpu,omitempty"`
	Mem      uint64  `json:"mem,omitempty"`
	MaxMem   uint64  `json:"maxmem,omitempty"`
	Disk     uint64  `json:"disk,omitempty"`
	MaxDisk  uint64  `json:"maxdisk,omitempty"`
	Uptime   int64   `json:"uptime,omitempty"`
	Pool     string  `json:"pool,omitempty"`
	Template bool    `json:"template,omitempty"`
}

// ── Common Action Result ──────────────────────────────────────

type ActionResult struct {
	Success bool            `json:"success"`
	Output  json.RawMessage `json:"output,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// HealthStatus represents the health of a plugin.
type HealthStatus struct {
	Status    string `json:"status"` // "healthy", "degraded", "unhealthy"
	Message   string `json:"message,omitempty"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
}
