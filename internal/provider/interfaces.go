package provider

import "context"

// GitProvider abstracts Git hosting platforms (GitLab, GitHub, Bitbucket).
type GitProvider interface {
	ListRepos(ctx context.Context) ([]Repository, error)
	GetBranches(ctx context.Context, repoID string) ([]Branch, error)
	CreateBranch(ctx context.Context, req CreateBranchRequest) (*Branch, error)
	CreateMergeRequest(ctx context.Context, req CreateMRRequest) (*MergeRequest, error)
	GetMergeRequest(ctx context.Context, repoID string, id int) (*MergeRequest, error)
}

// TaskTrackerProvider abstracts issue tracking systems (Jira, Linear, YouTrack, GitHub Issues).
type TaskTrackerProvider interface {
	ListProjects(ctx context.Context) ([]Project, error)
	ListIssues(ctx context.Context, projectKey string) ([]Issue, error)
	GetIssue(ctx context.Context, issueKey string) (*Issue, error)
	CreateIssue(ctx context.Context, req CreateIssueRequest) (*Issue, error)
	TransitionIssue(ctx context.Context, req TransitionRequest) error
	ListTransitions(ctx context.Context, issueKey string) ([]Transition, error)
}

// CDEngineProvider abstracts GitOps/CD engines (ArgoCD, FluxCD).
type CDEngineProvider interface {
	ListApplications(ctx context.Context) ([]CDApplication, error)
	GetApplication(ctx context.Context, name string) (*CDApplication, error)
	Sync(ctx context.Context, req SyncRequest) error
	Rollback(ctx context.Context, req RollbackRequest) error
	GetDeploymentStatus(ctx context.Context, name string) (*DeployStatus, error)
}

// CIProvider abstracts CI systems (GitLab CI, GitHub Actions, Jenkins).
type CIProvider interface {
	TriggerPipeline(ctx context.Context, req TriggerPipelineRequest) (*PipelineRun, error)
	GetPipelineStatus(ctx context.Context, repoID string, pipelineID string) (*PipelineRun, error)
	ListPipelines(ctx context.Context, repoID string) ([]PipelineRun, error)
}

// NotificationProvider abstracts messaging platforms (Slack, Teams, Discord).
type NotificationProvider interface {
	Send(ctx context.Context, req SendNotificationRequest) error
	ListChannels(ctx context.Context) ([]Channel, error)
}

// MonitoringProvider abstracts observability backends (Prometheus, Datadog, Grafana).
type MonitoringProvider interface {
	QueryMetrics(ctx context.Context, q MetricQuery) ([]MetricResult, error)
	ListAlerts(ctx context.Context) ([]Alert, error)
}

// Provider is the union interface that all PEPA plugins implement.
// A plugin may implement one or more of the sub-interfaces above.
// The Execute method is the generic escape hatch for actions not covered by typed interfaces.
type Provider interface {
	// Name returns the unique plugin name.
	Name() string
	// Version returns the semver version.
	Version() string
	// Description returns a human-readable description.
	Description() string
	// PluginType returns the provider type (git_provider, task_tracker, etc.)
	PluginType() string
	// Actions returns the list of action names this plugin supports.
	Actions() []string
	// Execute runs a named action with JSON params and returns JSON output.
	Execute(ctx context.Context, action string, params []byte, config map[string]string) ([]byte, error)
	// HealthCheck returns the current health status.
	HealthCheck(ctx context.Context) (*HealthStatus, error)
}
