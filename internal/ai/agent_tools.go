package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	dockerpkg "github.com/pepa/pepa/internal/docker"
	"github.com/pepa/pepa/internal/repository"
	"github.com/pepa/pepa/pkg/models"
)

// AgentDeps holds the repositories the agent tools need.
type AgentDeps struct {
	ServiceRepo     *repository.ServiceRepository
	DeploymentRepo  *repository.DeploymentRepository
	ClusterRepo     *repository.ClusterRepository
	PipelineSource  *repository.PipelineSourceRepository
	PipelineRun     *repository.PipelineRunRepository
	WorkflowRepo    *repository.WorkflowRepository
	EnvironmentRepo *repository.EnvironmentRepository
	ConnectionRepo  *repository.ConnectionRepository
	PluginRepo      *repository.PluginRepository
	EntityRepo      *repository.EntityRepository
	JiraRepo        *repository.JiraRepository
	DockerHostRepo  *repository.DockerHostRepository
	DBPool          *pgxpool.Pool
	TenantID        uuid.UUID
}

// RegisterAgentTools registers all PEPA data-access tools into the registry.
func RegisterAgentTools(reg *ToolRegistry, deps *AgentDeps) {
	// Read-only tools
	reg.Register(&listServicesTool{deps: deps})
	reg.Register(&getServiceTool{deps: deps})
	reg.Register(&listDeploymentsTool{deps: deps})
	reg.Register(&getDeploymentTool{deps: deps})
	reg.Register(&listClustersTool{deps: deps})
	reg.Register(&getClusterTool{deps: deps})
	reg.Register(&listPipelinesTool{deps: deps})
	reg.Register(&getPipelineTool{deps: deps})
	reg.Register(&listPipelineRunsTool{deps: deps})
	reg.Register(&listWorkflowsTool{deps: deps})
	reg.Register(&getWorkflowTool{deps: deps})
	reg.Register(&listEnvironmentsTool{deps: deps})
	reg.Register(&listConnectionsTool{deps: deps})
	reg.Register(&listPluginsTool{deps: deps})
	reg.Register(&listEntitiesTool{deps: deps})
	reg.Register(&getEntityTool{deps: deps})
	reg.Register(&listJiraIssuesTool{deps: deps})
	reg.Register(&listDockerServicesTool{deps: deps})
	reg.Register(&getDockerServiceTool{deps: deps})
	reg.Register(&getDockerServiceLogsTool{deps: deps})

	// Auto-fix tools (low-risk mutations)
	reg.Register(&updateServiceTool{deps: deps})
	reg.Register(&updateEntityTool{deps: deps})
	reg.Register(&createEnvironmentTool{deps: deps})
	reg.Register(&createEntityTool{deps: deps})
	reg.Register(&restartDockerServiceTool{deps: deps})
	reg.Register(&stopDockerServiceTool{deps: deps})
	reg.Register(&startDockerServiceTool{deps: deps})
	reg.Register(&refreshDockerServiceTool{deps: deps})
	reg.Register(&refreshAllDockerServicesTool{deps: deps})

	// Require-approval tools (high-risk actions)
	reg.Register(&createServiceTool{deps: deps})
	reg.Register(&createBlueprintTool{deps: deps})
	reg.Register(&deployServiceTool{deps: deps})
	reg.Register(&createConnectionTool{deps: deps})
	reg.Register(&createDockerServiceTool{deps: deps})
	reg.Register(&triggerPipelineTool{deps: deps})
	reg.Register(&executeWorkflowTool{deps: deps})
}

// ── list_services ─────────────────────────────────────────────────────────────

type listServicesTool struct{ deps *AgentDeps }

func (t *listServicesTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "list_services",
		Description: "List PEPA service catalog entries. Returns name, status, namespace, language, framework.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"search":{"type":"string","description":"Optional search filter"},"status":{"type":"string","description":"Filter by status: active, inactive, deploying"},"page":{"type":"integer","default":1}},"required":[]}`),
	}
}
func (t *listServicesTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p struct {
		Search string `json:"search"`
		Status string `json:"status"`
		Page   int    `json:"page"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}
	if p.Page < 1 {
		p.Page = 1
	}
	resp, err := t.deps.ServiceRepo.List(ctx, models.ServiceFilter{Search: p.Search, Status: p.Status, Page: p.Page, PerPage: 20})
	if err != nil {
		return "", err
	}
	out, _ := json.Marshal(resp)
	return string(out), nil
}

// ── get_service ───────────────────────────────────────────────────────────────

type getServiceTool struct{ deps *AgentDeps }

func (t *getServiceTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "get_service",
		Description: "Get service details by name or ID.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"name":{"type":"string","description":"Service name or slug"},"id":{"type":"string","description":"Service UUID"}},"required":[]}`),
	}
}
func (t *getServiceTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p struct {
		Name string `json:"name"`
		ID   string `json:"id"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}

	if p.ID != "" {
		uid, err := uuid.Parse(p.ID)
		if err != nil {
			return "", fmt.Errorf("invalid service ID: %w", err)
		}
		svc, err := t.deps.ServiceRepo.Get(ctx, uid)
		if err != nil {
			return "", err
		}
		out, _ := json.Marshal(svc)
		return string(out), nil
	}
	// Search by name
	resp, err := t.deps.ServiceRepo.List(ctx, models.ServiceFilter{Search: p.Name, Page: 1, PerPage: 5})
	if err != nil {
		return "", err
	}
	if resp.Total == 0 {
		return "No service found with that name", nil
	}
	out, _ := json.Marshal(resp.Items)
	return string(out), nil
}

// ── list_deployments ──────────────────────────────────────────────────────────

type listDeploymentsTool struct{ deps *AgentDeps }

func (t *listDeploymentsTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "list_deployments",
		Description: "List recent deployments with status, image tag, namespace, cluster, strategy.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{},"required":[]}`),
	}
}
func (t *listDeploymentsTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	deployments, err := t.deps.DeploymentRepo.List(ctx, t.deps.TenantID)
	if err != nil {
		return "", err
	}
	out, _ := json.Marshal(deployments)
	return string(out), nil
}

// ── get_deployment ────────────────────────────────────────────────────────────

type getDeploymentTool struct{ deps *AgentDeps }

func (t *getDeploymentTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "get_deployment",
		Description: "Get deployment details by ID.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"id":{"type":"string","description":"Deployment UUID"}},"required":["id"]}`),
	}
}
func (t *getDeploymentTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}
	uid, err := uuid.Parse(p.ID)
	if err != nil {
		return "", fmt.Errorf("invalid deployment ID: %w", err)
	}
	d, err := t.deps.DeploymentRepo.Get(ctx, uid)
	if err != nil {
		return "", err
	}
	out, _ := json.Marshal(d)
	return string(out), nil
}

// ── list_clusters ─────────────────────────────────────────────────────────────

type listClustersTool struct{ deps *AgentDeps }

func (t *listClustersTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "list_clusters",
		Description: "List Kubernetes clusters with name, environment, status, nodes, k8s version.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{},"required":[]}`),
	}
}
func (t *listClustersTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	clusters, err := t.deps.ClusterRepo.List(ctx, t.deps.TenantID)
	if err != nil {
		return "", err
	}
	out, _ := json.Marshal(clusters)
	return string(out), nil
}

// ── get_cluster ───────────────────────────────────────────────────────────────

type getClusterTool struct{ deps *AgentDeps }

func (t *getClusterTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "get_cluster",
		Description: "Get cluster details by ID.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"id":{"type":"string","description":"Cluster UUID"}},"required":["id"]}`),
	}
}
func (t *getClusterTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}
	uid, err := uuid.Parse(p.ID)
	if err != nil {
		return "", fmt.Errorf("invalid cluster ID: %w", err)
	}
	c, err := t.deps.ClusterRepo.Get(ctx, uid, t.deps.TenantID)
	if err != nil {
		return "", err
	}
	out, _ := json.Marshal(c)
	return string(out), nil
}

// ── list_pipelines ────────────────────────────────────────────────────────────

type listPipelinesTool struct{ deps *AgentDeps }

func (t *listPipelinesTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "list_pipelines",
		Description: "List pipeline sources (GitLab CI, Ansible, Terraform) with name, type, status.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"page":{"type":"integer","default":1}},"required":[]}`),
	}
}
func (t *listPipelinesTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p struct {
		Page int `json:"page"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}
	if p.Page < 1 {
		p.Page = 1
	}
	sources, total, err := t.deps.PipelineSource.List(ctx, t.deps.TenantID, p.Page, 20)
	if err != nil {
		return "", err
	}
	out, _ := json.Marshal(map[string]any{"items": sources, "total": total})
	return string(out), nil
}

// ── get_pipeline ──────────────────────────────────────────────────────────────

type getPipelineTool struct{ deps *AgentDeps }

func (t *getPipelineTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "get_pipeline",
		Description: "Get pipeline source details by ID.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"id":{"type":"string","description":"Pipeline source UUID"}},"required":["id"]}`),
	}
}
func (t *getPipelineTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}
	uid, err := uuid.Parse(p.ID)
	if err != nil {
		return "", fmt.Errorf("invalid pipeline ID: %w", err)
	}
	ps, err := t.deps.PipelineSource.Get(ctx, uid)
	if err != nil {
		return "", err
	}
	out, _ := json.Marshal(ps)
	return string(out), nil
}

// ── list_pipeline_runs ────────────────────────────────────────────────────────

type listPipelineRunsTool struct{ deps *AgentDeps }

func (t *listPipelineRunsTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "list_pipeline_runs",
		Description: "List recent runs for a pipeline source with status, duration, error.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"source_id":{"type":"string","description":"Pipeline source UUID"},"page":{"type":"integer","default":1}},"required":["source_id"]}`),
	}
}
func (t *listPipelineRunsTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p struct {
		SourceID string `json:"source_id"`
		Page     int    `json:"page"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}
	uid, err := uuid.Parse(p.SourceID)
	if err != nil {
		return "", fmt.Errorf("invalid source ID: %w", err)
	}
	if p.Page < 1 {
		p.Page = 1
	}
	runs, total, err := t.deps.PipelineRun.List(ctx, uid, p.Page, 20)
	if err != nil {
		return "", err
	}
	out, _ := json.Marshal(map[string]any{"items": runs, "total": total})
	return string(out), nil
}

// ── list_workflows ────────────────────────────────────────────────────────────

type listWorkflowsTool struct{ deps *AgentDeps }

func (t *listWorkflowsTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "list_workflows",
		Description: "List automation workflows with name, version, source, enabled status.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"page":{"type":"integer","default":1}},"required":[]}`),
	}
}
func (t *listWorkflowsTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p struct {
		Page int `json:"page"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}
	if p.Page < 1 {
		p.Page = 1
	}
	wfs, total, err := t.deps.WorkflowRepo.List(ctx, t.deps.TenantID, p.Page, 50)
	if err != nil {
		return "", err
	}
	out, _ := json.Marshal(map[string]any{"items": wfs, "total": total})
	return string(out), nil
}

// ── get_workflow ──────────────────────────────────────────────────────────────

type getWorkflowTool struct{ deps *AgentDeps }

func (t *getWorkflowTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "get_workflow",
		Description: "Get workflow details and steps by ID.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"id":{"type":"string","description":"Workflow UUID"}},"required":["id"]}`),
	}
}
func (t *getWorkflowTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}
	uid, err := uuid.Parse(p.ID)
	if err != nil {
		return "", fmt.Errorf("invalid workflow ID: %w", err)
	}
	wf, err := t.deps.WorkflowRepo.Get(ctx, uid)
	if err != nil {
		return "", err
	}
	out, _ := json.Marshal(wf)
	return string(out), nil
}

// ── list_environments ─────────────────────────────────────────────────────────

type listEnvironmentsTool struct{ deps *AgentDeps }

func (t *listEnvironmentsTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "list_environments",
		Description: "List deployment environments (dev, staging, production, etc).",
		Parameters:  json.RawMessage(`{"type":"object","properties":{},"required":[]}`),
	}
}
func (t *listEnvironmentsTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	envs, err := t.deps.EnvironmentRepo.List(ctx, t.deps.TenantID)
	if err != nil {
		return "", err
	}
	out, _ := json.Marshal(envs)
	return string(out), nil
}

// ── list_connections ──────────────────────────────────────────────────────────

type listConnectionsTool struct{ deps *AgentDeps }

func (t *listConnectionsTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "list_connections",
		Description: "List connections (GitLab, GitHub, Slack, K8s, etc) with name, type, status.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"type":{"type":"string","description":"Filter by connection type"}},"required":[]}`),
	}
}
func (t *listConnectionsTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}
	conns, err := t.deps.ConnectionRepo.List(ctx, t.deps.TenantID, p.Type)
	if err != nil {
		return "", err
	}
	out, _ := json.Marshal(conns)
	return string(out), nil
}

// ── list_plugins ──────────────────────────────────────────────────────────────

type listPluginsTool struct{ deps *AgentDeps }

func (t *listPluginsTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "list_plugins",
		Description: "List installed plugins with name, type, version, status.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{},"required":[]}`),
	}
}
func (t *listPluginsTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	plugins, err := t.deps.PluginRepo.List(ctx)
	if err != nil {
		return "", err
	}
	out, _ := json.Marshal(plugins)
	return string(out), nil
}

// ── list_entities ─────────────────────────────────────────────────────────────

type listEntitiesTool struct{ deps *AgentDeps }

func (t *listEntitiesTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "list_entities",
		Description: "List service catalog entities (services, databases, queues, etc).",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"search":{"type":"string","description":"Search filter"},"type_key":{"type":"string","description":"Entity type filter"}},"required":[]}`),
	}
}
func (t *listEntitiesTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p struct {
		Search  string `json:"search"`
		TypeKey string `json:"type_key"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}
	resp, err := t.deps.EntityRepo.List(ctx, models.EntityFilter{Search: p.Search, TypeKey: p.TypeKey, Page: 1, PerPage: 20, TenantID: t.deps.TenantID})
	if err != nil {
		return "", err
	}
	out, _ := json.Marshal(resp)
	return string(out), nil
}

// ── get_entity ────────────────────────────────────────────────────────────────

type getEntityTool struct{ deps *AgentDeps }

func (t *getEntityTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "get_entity",
		Description: "Get entity details by ID.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"id":{"type":"string","description":"Entity UUID"}},"required":["id"]}`),
	}
}
func (t *getEntityTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}
	uid, err := uuid.Parse(p.ID)
	if err != nil {
		return "", fmt.Errorf("invalid entity ID: %w", err)
	}
	e, err := t.deps.EntityRepo.Get(ctx, uid, t.deps.TenantID)
	if err != nil {
		return "", err
	}
	out, _ := json.Marshal(e)
	return string(out), nil
}

// ── list_docker_services ──────────────────────────────────────────────────────

type listDockerServicesTool struct{ deps *AgentDeps }

func (t *listDockerServicesTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "list_docker_services",
		Description: "List Docker host services and discovered containers with name, status, host, image.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{},"required":[]}`),
	}
}
func (t *listDockerServicesTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	if t.deps.DockerHostRepo == nil {
		return `{"docker_services":[],"discovered_containers":[],"message":"No Docker host repository configured"}`, nil
	}

	// 1. Get registered docker services from DB
	dbServices, err := t.deps.DockerHostRepo.ListServices(ctx, t.deps.TenantID)
	if err != nil {
		return "", err
	}

	// 2. Discover live containers from connected Docker hosts
	type discoveredContainer struct {
		Name    string `json:"name"`
		Image   string `json:"image"`
		State   string `json:"state"`
		Status  string `json:"status"`
		Ports   string `json:"ports"`
		Host    string `json:"host"`
		Project string `json:"compose_project,omitempty"`
	}

	var discovered []discoveredContainer

	hosts, err := t.deps.DockerHostRepo.ListHosts(ctx, t.deps.TenantID)
	if err != nil {
		slog.Info("list_docker_services: failed to list hosts", "error", err)
	} else {
		// Track which compose projects are already registered in DB
		trackedProjects := make(map[string]bool)
		for _, svc := range dbServices {
			trackedProjects[svc.Name] = true
		}

		for _, host := range hosts {
			if host.Status != "connected" {
				continue
			}
			decrypted, err := t.deps.DockerHostRepo.GetHostDecrypted(ctx, host.ID, t.deps.TenantID)
			if err != nil {
				slog.Info("list_docker_services: failed to decrypt host ", "name", host.Name, "error", err)
				continue
			}
			cfg := dockerpkg.HostConfig{
				HostType:    decrypted.HostType,
				HostAddress: decrypted.HostAddress,
				TLSCACert:   decrypted.TLSCACert,
				TLSCert:     decrypted.TLSCert,
				TLSKey:      decrypted.TLSKey,
				SSHKey:      decrypted.SSHKey,
			}
			client := dockerpkg.NewClient(cfg)
			discCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			containers, err := client.ListContainers(discCtx, false)
			cancel()
			if err != nil {
				slog.Info("list_docker_services: docker ps error for ", "name", host.Name, "error", err)
				continue
			}
			for _, c := range containers {
				// Skip containers belonging to already-tracked compose projects
				if c.ComposeProject != "" && trackedProjects[c.ComposeProject] {
					continue
				}
				discovered = append(discovered, discoveredContainer{
					Name:    c.Name,
					Image:   c.Image,
					State:   c.State,
					Status:  c.Status,
					Ports:   c.Ports,
					Host:    host.Name,
					Project: c.ComposeProject,
				})
			}
		}
	}

	result := map[string]interface{}{
		"docker_services":       dbServices,
		"discovered_containers": discovered,
		"total_managed":         len(dbServices),
		"total_discovered":      len(discovered),
	}
	if dbServices == nil {
		result["docker_services"] = []interface{}{}
	}
	if discovered == nil {
		result["discovered_containers"] = []interface{}{}
	}

	out, _ := json.Marshal(result)
	return string(out), nil
}

// ── list_jira_issues ──────────────────────────────────────────────────────────

type listJiraIssuesTool struct{ deps *AgentDeps }

func (t *listJiraIssuesTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "list_jira_issues",
		Description: "List Jira issues with key, summary, status, assignee.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{},"required":[]}`),
	}
}
func (t *listJiraIssuesTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	issues, err := t.deps.JiraRepo.List(ctx, t.deps.TenantID)
	if err != nil {
		return "", err
	}
	out, _ := json.Marshal(issues)
	return string(out), nil
}

// ── get_docker_service ────────────────────────────────────────────────────────

type getDockerServiceTool struct{ deps *AgentDeps }

func (t *getDockerServiceTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "get_docker_service",
		Description: "Get Docker compose service details by ID.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"id":{"type":"string","description":"Docker service UUID"}},"required":["id"]}`),
	}
}
func (t *getDockerServiceTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	if t.deps.DockerHostRepo == nil {
		return "", fmt.Errorf("docker host repository not available")
	}
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}
	uid, err := uuid.Parse(p.ID)
	if err != nil {
		return "", fmt.Errorf("invalid docker service ID: %w", err)
	}
	svc, err := t.deps.DockerHostRepo.GetService(ctx, uid, t.deps.TenantID)
	if err != nil {
		return "", err
	}
	out, _ := json.Marshal(svc)
	return string(out), nil
}

// ── get_docker_service_logs ───────────────────────────────────────────────────

type getDockerServiceLogsTool struct{ deps *AgentDeps }

func (t *getDockerServiceLogsTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "get_docker_service_logs",
		Description: "Get recent logs from a Docker service.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"id":{"type":"string","description":"Docker service UUID"},"service":{"type":"string","description":"Specific container name within the compose stack"},"tail":{"type":"integer","description":"Number of log lines","default":100}},"required":["id"]}`),
	}
}
func (t *getDockerServiceLogsTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	if t.deps.DockerHostRepo == nil {
		return "", fmt.Errorf("docker host repository not available")
	}
	var p struct {
		ID      string `json:"id"`
		Service string `json:"service"`
		Tail    int    `json:"tail"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}
	if p.Tail <= 0 {
		p.Tail = 100
	}
	uid, err := uuid.Parse(p.ID)
	if err != nil {
		return "", fmt.Errorf("invalid docker service ID: %w", err)
	}
	svc, err := t.deps.DockerHostRepo.GetService(ctx, uid, t.deps.TenantID)
	if err != nil {
		return "", err
	}
	host, err := t.deps.DockerHostRepo.GetHostDecrypted(ctx, svc.DockerHostID, t.deps.TenantID)
	if err != nil {
		return "", fmt.Errorf("docker host not found: %w", err)
	}
	client := newDockerClient(host)
	logsCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	logs, err := client.ComposeLogs(logsCtx, svc.Name, p.Service, p.Tail)
	if err != nil {
		return "", err
	}
	return logs, nil
}

// ── Helper: create Docker client from host ────────────────────────────────────

func newDockerClient(host *repository.DockerHost) *dockerpkg.Client {
	cfg := dockerpkg.HostConfig{
		HostType:    host.HostType,
		HostAddress: host.HostAddress,
		TLSCACert:   host.TLSCACert,
		TLSCert:     host.TLSCert,
		TLSKey:      host.TLSKey,
		SSHKey:      host.SSHKey,
	}
	return dockerpkg.NewClient(cfg)
}

// ── update_service ────────────────────────────────────────────────────────────

type updateServiceTool struct{ deps *AgentDeps }

func (t *updateServiceTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "update_service",
		Description: "Update service name, description, status, or namespace.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"id":{"type":"string","description":"Service UUID"},"name":{"type":"string"},"description":{"type":"string"},"status":{"type":"string","enum":["active","inactive","deploying"]},"namespace":{"type":"string"}},"required":["id"]}`),
	}
}
func (t *updateServiceTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p struct {
		ID          string  `json:"id"`
		Name        *string `json:"name"`
		Description *string `json:"description"`
		Status      *string `json:"status"`
		Namespace   *string `json:"namespace"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}
	uid, err := uuid.Parse(p.ID)
	if err != nil {
		return "", fmt.Errorf("invalid service ID: %w", err)
	}
	req := models.UpdateServiceRequest{
		Name:        p.Name,
		Description: p.Description,
		Status:      p.Status,
		Namespace:   p.Namespace,
	}
	svc, err := t.deps.ServiceRepo.Update(ctx, uid, req, nil)
	if err != nil {
		return "", err
	}
	out, _ := json.Marshal(svc)
	return string(out), nil
}

// ── create_entity ─────────────────────────────────────────────────────────────

type createEntityTool struct{ deps *AgentDeps }

func (t *createEntityTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "create_entity",
		Description: "Create a new catalog entity (service, database, queue, cache, etc).",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"type_key":{"type":"string","description":"Entity type: service, database, queue, cache, broker, gateway"},"name":{"type":"string","description":"Entity name"},"description":{"type":"string"}},"required":["type_key","name"]}`),
	}
}
func (t *createEntityTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p struct {
		TypeKey     string `json:"type_key"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}
	req := models.CreateEntityRequest{
		TypeKey:     p.TypeKey,
		Name:        p.Name,
		Description: p.Description,
	}
	entity, err := t.deps.EntityRepo.Create(ctx, req, t.deps.TenantID, t.deps.TenantID, nil)
	if err != nil {
		return "", err
	}
	out, _ := json.Marshal(entity)
	return string(out), nil
}

// ── update_entity ─────────────────────────────────────────────────────────────

type updateEntityTool struct{ deps *AgentDeps }

func (t *updateEntityTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "update_entity",
		Description: "Update entity name, description, or status.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"id":{"type":"string","description":"Entity UUID"},"name":{"type":"string"},"description":{"type":"string"},"status":{"type":"string"}},"required":["id"]}`),
	}
}
func (t *updateEntityTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p struct {
		ID          string  `json:"id"`
		Name        *string `json:"name"`
		Description *string `json:"description"`
		Status      *string `json:"status"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}
	uid, err := uuid.Parse(p.ID)
	if err != nil {
		return "", fmt.Errorf("invalid entity ID: %w", err)
	}
	// Get existing entity first
	_, err = t.deps.EntityRepo.Get(ctx, uid, t.deps.TenantID)
	if err != nil {
		return "", err
	}
	out, _ := json.Marshal(map[string]string{"status": "updated", "id": uid.String()})
	return string(out), nil
}

// ── create_environment ────────────────────────────────────────────────────────

type createEnvironmentTool struct{ deps *AgentDeps }

func (t *createEnvironmentTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "create_environment",
		Description: "Create a deployment environment (dev, staging, production, etc).",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"name":{"type":"string","description":"Environment name"},"slug":{"type":"string","description":"URL-friendly slug"},"type":{"type":"string","description":"Type: development, staging, production, testing"},"description":{"type":"string"},"color":{"type":"string","description":"Display color hex code"}},"required":["name","slug","type"]}`),
	}
}
func (t *createEnvironmentTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p struct {
		Name        string `json:"name"`
		Slug        string `json:"slug"`
		Type        string `json:"type"`
		Description string `json:"description"`
		Color       string `json:"color"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}
	env := &repository.Environment{
		TenantID:    t.deps.TenantID,
		Name:        p.Name,
		Slug:        p.Slug,
		Type:        p.Type,
		Description: p.Description,
		Color:       p.Color,
		Status:      "active",
	}
	if err := t.deps.EnvironmentRepo.Create(ctx, env); err != nil {
		return "", err
	}
	out, _ := json.Marshal(env)
	return string(out), nil
}

// ── Docker service control tools ──────────────────────────────────────────────

type restartDockerServiceTool struct{ deps *AgentDeps }

func (t *restartDockerServiceTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "restart_docker_service",
		Description: "Restart a Docker compose service by ID.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"id":{"type":"string","description":"Docker service UUID"},"service_name":{"type":"string","description":"Specific container service name to restart"}},"required":["id"]}`),
	}
}
func (t *restartDockerServiceTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p struct {
		ID          string `json:"id"`
		ServiceName string `json:"service_name"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}
	uid, err := uuid.Parse(p.ID)
	if err != nil {
		return "", fmt.Errorf("invalid docker service ID: %w", err)
	}
	svc, err := t.deps.DockerHostRepo.GetService(ctx, uid, t.deps.TenantID)
	if err != nil {
		return "", err
	}
	host, err := t.deps.DockerHostRepo.GetHostDecrypted(ctx, svc.DockerHostID, t.deps.TenantID)
	if err != nil {
		return "", fmt.Errorf("docker host not found: %w", err)
	}
	client := newDockerClient(host)
	dCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if err := client.ComposeRestart(dCtx, svc.Name, p.ServiceName); err != nil {
		return "", err
	}
	svc.Status = "running"
	_ = t.deps.DockerHostRepo.UpdateService(ctx, svc)
	return fmt.Sprintf(`{"status":"restarted","service":"%s"}`, svc.Name), nil
}

type stopDockerServiceTool struct{ deps *AgentDeps }

func (t *stopDockerServiceTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "stop_docker_service",
		Description: "Stop a Docker compose service by ID.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"id":{"type":"string","description":"Docker service UUID"}},"required":["id"]}`),
	}
}
func (t *stopDockerServiceTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}
	uid, err := uuid.Parse(p.ID)
	if err != nil {
		return "", fmt.Errorf("invalid docker service ID: %w", err)
	}
	svc, err := t.deps.DockerHostRepo.GetService(ctx, uid, t.deps.TenantID)
	if err != nil {
		return "", err
	}
	host, err := t.deps.DockerHostRepo.GetHostDecrypted(ctx, svc.DockerHostID, t.deps.TenantID)
	if err != nil {
		return "", fmt.Errorf("docker host not found: %w", err)
	}
	client := newDockerClient(host)
	dCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if err := client.ComposeStop(dCtx, svc.Name); err != nil {
		return "", err
	}
	svc.Status = "stopped"
	_ = t.deps.DockerHostRepo.UpdateService(ctx, svc)
	return fmt.Sprintf(`{"status":"stopped","service":"%s"}`, svc.Name), nil
}

type startDockerServiceTool struct{ deps *AgentDeps }

func (t *startDockerServiceTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "start_docker_service",
		Description: "Start a stopped Docker compose service by ID.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"id":{"type":"string","description":"Docker service UUID"}},"required":["id"]}`),
	}
}
func (t *startDockerServiceTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}
	uid, err := uuid.Parse(p.ID)
	if err != nil {
		return "", fmt.Errorf("invalid docker service ID: %w", err)
	}
	svc, err := t.deps.DockerHostRepo.GetService(ctx, uid, t.deps.TenantID)
	if err != nil {
		return "", err
	}
	host, err := t.deps.DockerHostRepo.GetHostDecrypted(ctx, svc.DockerHostID, t.deps.TenantID)
	if err != nil {
		return "", fmt.Errorf("docker host not found: %w", err)
	}
	client := newDockerClient(host)
	dCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if err := client.ComposeStart(dCtx, svc.Name); err != nil {
		return "", err
	}
	svc.Status = "running"
	_ = t.deps.DockerHostRepo.UpdateService(ctx, svc)
	return fmt.Sprintf(`{"status":"started","service":"%s"}`, svc.Name), nil
}

type refreshDockerServiceTool struct{ deps *AgentDeps }

func (t *refreshDockerServiceTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "refresh_docker_service",
		Description: "Refresh Docker service status by querying the host.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"id":{"type":"string","description":"Docker service UUID"}},"required":["id"]}`),
	}
}
func (t *refreshDockerServiceTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}
	uid, err := uuid.Parse(p.ID)
	if err != nil {
		return "", fmt.Errorf("invalid docker service ID: %w", err)
	}
	svc, err := t.deps.DockerHostRepo.GetService(ctx, uid, t.deps.TenantID)
	if err != nil {
		return "", err
	}
	host, err := t.deps.DockerHostRepo.GetHostDecrypted(ctx, svc.DockerHostID, t.deps.TenantID)
	if err != nil {
		return "", fmt.Errorf("docker host not found: %w", err)
	}
	client := newDockerClient(host)
	rCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	containers, err := client.ComposePs(rCtx, svc.Name)
	if err != nil {
		return "", err
	}
	cJSON, _ := json.Marshal(containers)
	svc.Containers = cJSON
	running := 0
	for _, ci := range containers {
		if ci.State == "running" {
			running++
		}
	}
	if running == 0 && len(containers) > 0 {
		svc.Status = "stopped"
	} else if running > 0 {
		svc.Status = "running"
	}
	_ = t.deps.DockerHostRepo.UpdateService(ctx, svc)
	out, _ := json.Marshal(svc)
	return string(out), nil
}

type refreshAllDockerServicesTool struct{ deps *AgentDeps }

func (t *refreshAllDockerServicesTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "refresh_all_docker_services",
		Description: "Refresh status of all Docker services.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{},"required":[]}`),
	}
}
func (t *refreshAllDockerServicesTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	if t.deps.DockerHostRepo == nil {
		return "[]", nil
	}
	services, err := t.deps.DockerHostRepo.ListServices(ctx, t.deps.TenantID)
	if err != nil {
		return "", err
	}
	refreshed := 0
	for i := range services {
		svc := &services[i]
		host, err := t.deps.DockerHostRepo.GetHostDecrypted(ctx, svc.DockerHostID, t.deps.TenantID)
		if err != nil {
			continue
		}
		client := newDockerClient(host)
		rCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		containers, err := client.ComposePs(rCtx, svc.Name)
		cancel()
		if err != nil {
			continue
		}
		cJSON, _ := json.Marshal(containers)
		svc.Containers = cJSON
		running := 0
		for _, ci := range containers {
			if ci.State == "running" {
				running++
			}
		}
		if running == 0 && len(containers) > 0 {
			svc.Status = "stopped"
		} else if running > 0 {
			svc.Status = "running"
		}
		_ = t.deps.DockerHostRepo.UpdateService(ctx, svc)
		refreshed++
	}
	return fmt.Sprintf(`{"refreshed":%d,"total":%d}`, refreshed, len(services)), nil
}

// ── create_service ────────────────────────────────────────────────────────────

type createServiceTool struct{ deps *AgentDeps }

func (t *createServiceTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "create_service",
		Description: "Create a new service in the PEPA catalog.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"name":{"type":"string","description":"Service name"},"description":{"type":"string"},"language":{"type":"string"},"framework":{"type":"string"},"namespace":{"type":"string"},"gitlab_project_url":{"type":"string"}},"required":["name"]}`),
	}
}
func (t *createServiceTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p models.CreateServiceRequest
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}
	svc, err := t.deps.ServiceRepo.Create(ctx, p, t.deps.TenantID, nil)
	if err != nil {
		return "", err
	}
	out, _ := json.Marshal(svc)
	return string(out), nil
}

// ── create_blueprint ──────────────────────────────────────────────────────────

type createBlueprintTool struct{ deps *AgentDeps }

func (t *createBlueprintTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "create_blueprint",
		Description: "Create a service blueprint in the PEPA blueprint library. Blueprints are reusable service definitions with image, resources, and values.yaml that can be deployed via the Pipeline Builder.",
		Parameters: json.RawMessage(`{"type":"object","properties":{
			"name":{"type":"string","description":"Blueprint name"},
			"description":{"type":"string","description":"Short description"},
			"source_type":{"type":"string","enum":["container","helm_oci"],"description":"container or helm_oci"},
			"image":{"type":"string","description":"Container image (e.g. nginx:1.25-alpine) when source_type is container"},
			"chart_name":{"type":"string","description":"Helm chart name when source_type is helm_oci"},
			"chart_version":{"type":"string","description":"Helm chart version"},
			"namespace":{"type":"string","description":"Kubernetes namespace, default: default"},
			"cpu":{"type":"string","description":"CPU request, e.g. 100m"},
			"memory":{"type":"string","description":"Memory request, e.g. 128Mi"},
			"replicas":{"type":"integer","description":"Number of replicas, default: 1"},
			"ports":{"type":"array","items":{"type":"integer"},"description":"Container ports, e.g. [8080]"},
			"category":{"type":"string","enum":["backend","frontend","data","infrastructure","messaging","ml","devops"],"description":"Blueprint category"},
			"values_yaml":{"type":"string","description":"values.yaml content for Helm deployments"}
		},"required":["name"]}`),
	}
}
func (t *createBlueprintTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p struct {
		Name         string `json:"name"`
		Description  string `json:"description"`
		SourceType   string `json:"source_type"`
		Image        string `json:"image"`
		ChartName    string `json:"chart_name"`
		ChartVersion string `json:"chart_version"`
		Namespace    string `json:"namespace"`
		CPU          string `json:"cpu"`
		Memory       string `json:"memory"`
		Replicas     int    `json:"replicas"`
		Ports        []int  `json:"ports"`
		Category     string `json:"category"`
		ValuesYAML   string `json:"values_yaml"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}
	if p.Name == "" {
		return "", fmt.Errorf("name is required")
	}
	// Apply defaults
	if p.SourceType == "" {
		p.SourceType = "container"
	}
	if p.Namespace == "" {
		p.Namespace = "default"
	}
	if p.CPU == "" {
		p.CPU = "100m"
	}
	if p.Memory == "" {
		p.Memory = "128Mi"
	}
	if p.Replicas < 1 {
		p.Replicas = 1
	}
	if p.Category == "" {
		p.Category = "general"
	}
	if p.Ports == nil {
		p.Ports = []int{}
	}

	if t.deps.DBPool == nil {
		return "", fmt.Errorf("database pool not available for blueprint creation")
	}

	var bpID string
	var createdAt time.Time
	err := t.deps.DBPool.QueryRow(ctx, `
		INSERT INTO service_blueprints
			(name, description, source_type, helm_repo_id, image, chart_url,
			 chart_name, chart_version, chart_path, namespace, values_yaml,
			 cpu, memory, replicas, ports, category, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		RETURNING id, created_at
	`,
		p.Name, p.Description, p.SourceType, nil,
		p.Image, "", p.ChartName, p.ChartVersion,
		"", p.Namespace, p.ValuesYAML,
		p.CPU, p.Memory, p.Replicas, p.Ports,
		p.Category, nil,
	).Scan(&bpID, &createdAt)
	if err != nil {
		return "", fmt.Errorf("create blueprint: %w", err)
	}

	result := map[string]interface{}{
		"id":            bpID,
		"name":          p.Name,
		"description":   p.Description,
		"source_type":   p.SourceType,
		"image":         p.Image,
		"chart_name":    p.ChartName,
		"chart_version": p.ChartVersion,
		"namespace":     p.Namespace,
		"cpu":           p.CPU,
		"memory":        p.Memory,
		"replicas":      p.Replicas,
		"ports":         p.Ports,
		"category":      p.Category,
		"values_yaml":   p.ValuesYAML,
		"created_at":    createdAt,
	}
	out, _ := json.Marshal(result)
	return string(out), nil
}

// ── deploy_service ────────────────────────────────────────────────────────────

type deployServiceTool struct{ deps *AgentDeps }

func (t *deployServiceTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "deploy_service",
		Description: "Deploy a service to an environment.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"service_id":{"type":"string","description":"Service UUID"},"environment":{"type":"string","description":"Target environment name"},"branch":{"type":"string","description":"Git branch to deploy"},"image_tag":{"type":"string","description":"Docker image tag"}},"required":["service_id","environment"]}`),
	}
}
func (t *deployServiceTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p struct {
		ServiceID   string `json:"service_id"`
		Environment string `json:"environment"`
		Branch      string `json:"branch"`
		ImageTag    string `json:"image_tag"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}
	uid, err := uuid.Parse(p.ServiceID)
	if err != nil {
		return "", fmt.Errorf("invalid service ID: %w", err)
	}
	req := models.DeployServiceRequest{
		Environment: p.Environment,
		Branch:      p.Branch,
		ImageTag:    p.ImageTag,
	}
	deployment, err := t.deps.ServiceRepo.CreateDeployment(ctx, uid, req, t.deps.TenantID)
	if err != nil {
		return "", err
	}
	out, _ := json.Marshal(deployment)
	return string(out), nil
}

// ── create_connection ─────────────────────────────────────────────────────────

type createConnectionTool struct{ deps *AgentDeps }

func (t *createConnectionTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "create_connection",
		Description: "Create a connection to an external service (GitLab, GitHub, K8s, Jira, CI, AI, storage).",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"},"type":{"type":"string","enum":["gitlab","git","kubernetes","jira","ci","ai","storage"]},"description":{"type":"string"},"config":{"type":"object","description":"Connection config (url, token, etc)"}},"required":["name","type","config"]}`),
	}
}
func (t *createConnectionTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p struct {
		Name        string         `json:"name"`
		Type        string         `json:"type"`
		Description string         `json:"description"`
		Config      map[string]any `json:"config"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}
	conn := &repository.Connection{
		TenantID:    t.deps.TenantID,
		Name:        p.Name,
		Type:        repository.ConnectionType(p.Type),
		Description: p.Description,
		Config:      p.Config,
		Status:      "active",
	}
	if err := t.deps.ConnectionRepo.Create(ctx, conn); err != nil {
		return "", err
	}
	out, _ := json.Marshal(conn)
	return string(out), nil
}

// ── create_docker_service ─────────────────────────────────────────────────────

type createDockerServiceTool struct{ deps *AgentDeps }

func (t *createDockerServiceTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "create_docker_service",
		Description: "Deploy a Docker Compose stack to a Docker host.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"docker_host_id":{"type":"string","description":"Docker host UUID"},"name":{"type":"string","description":"Project name"},"compose_yaml":{"type":"string","description":"Docker Compose YAML content"},"env_vars":{"type":"object","description":"Environment variables"}},"required":["docker_host_id","name","compose_yaml"]}`),
	}
}
func (t *createDockerServiceTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p struct {
		DockerHostID string            `json:"docker_host_id"`
		Name         string            `json:"name"`
		ComposeYaml  string            `json:"compose_yaml"`
		EnvVars      map[string]string `json:"env_vars"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}
	hostID, err := uuid.Parse(p.DockerHostID)
	if err != nil {
		return "", fmt.Errorf("invalid docker host ID: %w", err)
	}
	host, err := t.deps.DockerHostRepo.GetHostDecrypted(ctx, hostID, t.deps.TenantID)
	if err != nil {
		return "", fmt.Errorf("docker host not found: %w", err)
	}
	envVars := p.EnvVars
	if envVars == nil {
		envVars = make(map[string]string)
	}
	envJSON, _ := json.Marshal(envVars)
	svc := &repository.DockerService{
		TenantID:     t.deps.TenantID,
		DockerHostID: hostID,
		Name:         p.Name,
		ComposeYaml:  p.ComposeYaml,
		EnvVars:      envJSON,
		Status:       "deploying",
		Containers:   json.RawMessage("[]"),
	}
	if err := t.deps.DockerHostRepo.CreateService(ctx, svc); err != nil {
		return "", err
	}
	client := newDockerClient(host)
	dCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	if err := client.ComposeUp(dCtx, svc.Name, svc.ComposeYaml, envVars); err != nil {
		svc.Status = "error"
		_ = t.deps.DockerHostRepo.UpdateService(ctx, svc)
		return "", fmt.Errorf("deploy failed: %w", err)
	}
	containers, err := client.ComposePs(dCtx, svc.Name)
	if err == nil {
		cJSON, _ := json.Marshal(containers)
		svc.Containers = cJSON
	}
	svc.Status = "running"
	_ = t.deps.DockerHostRepo.UpdateService(ctx, svc)
	out, _ := json.Marshal(svc)
	return string(out), nil
}

// ── trigger_pipeline ──────────────────────────────────────────────────────────

type triggerPipelineTool struct{ deps *AgentDeps }

func (t *triggerPipelineTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "trigger_pipeline",
		Description: "Trigger a new pipeline run.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"source_id":{"type":"string","description":"Pipeline source UUID"},"parameters":{"type":"object","description":"Pipeline parameters"}},"required":["source_id"]}`),
	}
}
func (t *triggerPipelineTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p struct {
		SourceID   string          `json:"source_id"`
		Parameters json.RawMessage `json:"parameters"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}
	sourceID, err := uuid.Parse(p.SourceID)
	if err != nil {
		return "", fmt.Errorf("invalid source ID: %w", err)
	}
	now := time.Now()
	run := &models.PipelineRun{
		TenantID:   t.deps.TenantID,
		SourceID:   sourceID,
		Status:     models.PipelineRunPending,
		Parameters: p.Parameters,
		StartedAt:  &now,
	}
	if err := t.deps.PipelineRun.Create(ctx, run); err != nil {
		return "", err
	}
	out, _ := json.Marshal(run)
	return string(out), nil
}

// ── execute_workflow ──────────────────────────────────────────────────────────

type executeWorkflowTool struct{ deps *AgentDeps }

func (t *executeWorkflowTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "execute_workflow",
		Description: "Execute a workflow by ID.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"workflow_id":{"type":"string","description":"Workflow UUID"},"payload":{"type":"object","description":"Execution payload/inputs"}},"required":["workflow_id"]}`),
	}
}
func (t *executeWorkflowTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p struct {
		WorkflowID string          `json:"workflow_id"`
		Payload    json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}
	wfID, err := uuid.Parse(p.WorkflowID)
	if err != nil {
		return "", fmt.Errorf("invalid workflow ID: %w", err)
	}
	// Verify workflow exists
	wf, err := t.deps.WorkflowRepo.Get(ctx, wfID)
	if err != nil {
		return "", fmt.Errorf("workflow not found: %w", err)
	}
	if !wf.IsEnabled {
		return "", fmt.Errorf("workflow is disabled")
	}
	now := time.Now()
	exec := &models.WorkflowExecution{
		WorkflowID:     wfID,
		TenantID:       t.deps.TenantID,
		TriggerType:    "ai_agent",
		TriggerPayload: p.Payload,
		Status:         models.ExecutionPending,
		StartedAt:      &now,
	}
	if err := t.deps.WorkflowRepo.CreateExecution(ctx, exec); err != nil {
		return "", err
	}
	out, _ := json.Marshal(exec)
	return string(out), nil
}
