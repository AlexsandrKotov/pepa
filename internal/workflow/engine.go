package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/events"
	"github.com/pepa/pepa/internal/provider"
	"github.com/pepa/pepa/internal/repository"
	"github.com/pepa/pepa/pkg/models"
	"github.com/pepa/pepa/pkg/utils"
)

// Engine executes workflow DAGs step by step.
type Engine struct {
	workflowRepo     *repository.WorkflowRepository
	entityRepo       *repository.EntityRepository
	deploymentRepo   *repository.DeploymentRepository
	eventBus         *events.Bus
	providerRegistry *provider.Registry
}

// NewEngine creates a new workflow execution engine.
func NewEngine(repo *repository.WorkflowRepository, entityRepo *repository.EntityRepository, deploymentRepo *repository.DeploymentRepository, bus *events.Bus, registry *provider.Registry) *Engine {
	return &Engine{
		workflowRepo:     repo,
		entityRepo:       entityRepo,
		deploymentRepo:   deploymentRepo,
		eventBus:         bus,
		providerRegistry: registry,
	}
}

// Execute runs a workflow execution to completion.
func (e *Engine) Execute(ctx context.Context, workflowID, executionID uuid.UUID) error {
	// Load workflow
	wf, err := e.workflowRepo.Get(ctx, workflowID)
	if err != nil {
		return fmt.Errorf("load workflow: %w", err)
	}

	// Parse spec
	var spec models.WorkflowSpec
	if err := json.Unmarshal(wf.Spec, &spec); err != nil {
		return fmt.Errorf("parse workflow spec: %w", err)
	}

	if len(spec.Steps) == 0 {
		return fmt.Errorf("workflow has no steps")
	}

	// Load execution to obtain trigger parameters (inputs) for substitution.
	exec, err := e.workflowRepo.GetExecution(ctx, executionID)
	if err != nil {
		return fmt.Errorf("load execution: %w", err)
	}
	inputs := parseTriggerInputs(exec.TriggerPayload)

	// Build DAG and validate
	dag, err := buildDAG(spec.Steps)
	if err != nil {
		return fmt.Errorf("build DAG: %w", err)
	}

	// Mark execution as running
	if err := e.workflowRepo.UpdateExecutionStatus(ctx, executionID, models.ExecutionRunning, nil); err != nil {
		return fmt.Errorf("update execution status: %w", err)
	}

	e.publishEvent("workflow.running", wf.TenantID, map[string]interface{}{
		"workflow_id":  workflowID.String(),
		"execution_id": executionID.String(),
		"steps":        len(spec.Steps),
	})

	// Execute steps in topological order (parallel within each level)
	stepResults := make(map[string]*stepResult)
	var resultsMu sync.Mutex
	startTime := time.Now()
	var firstErr error

	for _, level := range dag {
		if len(level) == 1 {
			// Single step — execute directly
			stepName := level[0]
			step := spec.Steps[stepIndex(spec.Steps, stepName)]
			output, status, err := e.executeStepWithConditions(ctx, &step, executionID, stepResults, wf.TenantID, startTime, inputs)
			resultsMu.Lock()
			stepResults[stepName] = &stepResult{status: status, output: output, err: err}
			resultsMu.Unlock()
			if err != nil && firstErr == nil {
				firstErr = fmt.Errorf("step %q: %w", stepName, err)
			}
		} else {
			// Multiple steps in this level — execute in parallel
			var wg sync.WaitGroup
			errs := make([]error, len(level))

			for i, stepName := range level {
				wg.Add(1)
				go func(idx int, name string) {
					defer wg.Done()
					step := spec.Steps[stepIndex(spec.Steps, name)]
					output, status, err := e.executeStepWithConditions(ctx, &step, executionID, stepResults, wf.TenantID, startTime, inputs)
					resultsMu.Lock()
					stepResults[name] = &stepResult{status: status, output: output, err: err}
					resultsMu.Unlock()
					if err != nil {
						errs[idx] = fmt.Errorf("step %q: %w", name, err)
					}
				}(i, stepName)
			}
			wg.Wait()

			// Record first error but continue processing remaining levels
			// so that run_when="always" steps can still execute.
			for _, err := range errs {
				if err != nil && firstErr == nil {
					firstErr = err
				}
			}
		}
	}

	duration := int(time.Since(startTime).Milliseconds())
	resultJSON, _ := json.Marshal(stepResultsToMap(stepResults))

	if firstErr != nil {
		// Mark execution as failed but all run_when="always" steps were still processed
		_ = e.workflowRepo.UpdateExecutionResult(ctx, executionID, models.ExecutionFailed, resultJSON, &duration)
		e.publishEvent("workflow.failed", wf.TenantID, map[string]interface{}{
			"workflow_id":  workflowID.String(),
			"execution_id": executionID.String(),
			"error":        firstErr.Error(),
			"duration_ms":  duration,
		})
		log.Printf("Workflow %q execution %s failed in %dms: %v", wf.Name, executionID, duration, firstErr)
		return firstErr
	}

	// Mark execution as success
	_ = e.workflowRepo.UpdateExecutionResult(ctx, executionID, models.ExecutionSuccess, resultJSON, &duration)

	e.publishEvent("workflow.completed", wf.TenantID, map[string]interface{}{
		"workflow_id":  workflowID.String(),
		"execution_id": executionID.String(),
		"duration_ms":  duration,
		"steps_total":  len(spec.Steps),
	})

	log.Printf("Workflow %q execution %s completed in %dms (%d steps)", wf.Name, executionID, duration, len(spec.Steps))
	return nil
}

// executeStep dispatches a single step to the appropriate handler.
func (e *Engine) executeStep(ctx context.Context, step *models.StepSpec, executionID uuid.UUID, prev map[string]*stepResult, tenantID uuid.UUID, inputs map[string]string) (json.RawMessage, error) {
	log.Printf("Executing step %q (type=%s, plugin=%s, action=%s)", step.Name, step.Type, step.Plugin, step.Action)

	switch {
	case step.Type == "condition":
		return e.executeCondition(step, prev, inputs)
	case step.Type == "approval":
		return e.executeApproval(ctx, step, executionID)
	case step.Type == "entity_update":
		return e.executeEntityUpdate(ctx, step, tenantID)
	case step.Type == "deploy":
		return e.executeDeploy(ctx, step, tenantID)
	case step.Type == "deploy_sim":
		return e.executeDeploySim(step)
	case step.Plugin != "":
		return e.executePluginAction(ctx, step, tenantID)
	default:
		// Generic step — log and succeed
		return json.RawMessage(`{"status":"completed"}`), nil
	}
}

func (e *Engine) executeCondition(step *models.StepSpec, prev map[string]*stepResult, inputs map[string]string) (json.RawMessage, error) {
	result := evaluateCondition(step.Condition, prev, inputs)
	out, _ := json.Marshal(map[string]interface{}{
		"condition": step.Condition,
		"result":    result,
	})
	if !result {
		return out, fmt.Errorf("condition not met: %s", step.Condition)
	}
	return out, nil
}

func (e *Engine) executeApproval(ctx context.Context, step *models.StepSpec, executionID uuid.UUID) (json.RawMessage, error) {
	// Create an approval request in the workflow execution result
	log.Printf("Step %q: creating approval request for execution %s", step.Name, executionID)

	// Parse approval metadata
	var approvalReq struct {
		Approvers []string `json:"approvers"`
		Message   string   `json:"message"`
	}
	if step.Params != nil {
		json.Unmarshal(step.Params, &approvalReq)
	}

	// Mark execution as waiting for approval
	waitingResult, _ := json.Marshal(map[string]interface{}{
		"waiting_for_approval": true,
		"step":                 step.Name,
		"approvers":            approvalReq.Approvers,
		"message":              approvalReq.Message,
		"requested_at":         time.Now().UTC().Format(time.RFC3339),
	})
	_ = e.workflowRepo.UpdateExecutionResult(ctx, executionID, models.ExecutionWaiting, waitingResult, nil)

	return waitingResult, fmt.Errorf("waiting for approval: %s", step.Name)
}

func (e *Engine) executeEntityUpdate(ctx context.Context, step *models.StepSpec, tenantID uuid.UUID) (json.RawMessage, error) {
	log.Printf("Step %q: entity_update (params=%s)", step.Name, redactParams(step.Params))
	var params struct {
		EntityID string          `json:"entity_id"`
		Status   string          `json:"status"`
		Metadata json.RawMessage `json:"metadata"`
	}
	if err := json.Unmarshal(step.Params, &params); err != nil {
		return nil, fmt.Errorf("parse entity_update params: %w", err)
	}

	if e.entityRepo == nil {
		return nil, fmt.Errorf("entity repository not available")
	}

	entityID, err := uuid.Parse(params.EntityID)
	if err != nil {
		return nil, fmt.Errorf("invalid entity_id: %w", err)
	}

	update := models.UpdateEntityRequest{}
	if params.Status != "" {
		update.Status = &params.Status
	}
	if params.Metadata != nil {
		update.Metadata = params.Metadata
	}

	updated, err := e.entityRepo.Update(ctx, entityID, update, nil, uuid.Nil)
	if err != nil {
		return nil, fmt.Errorf("update entity %s: %w", params.EntityID, err)
	}

	log.Printf("Step %q: entity %s updated successfully", step.Name, params.EntityID)
	out, _ := json.Marshal(map[string]interface{}{
		"entity_id": params.EntityID,
		"updated":   true,
		"name":      updated.Name,
		"status":    updated.Status,
	})
	return out, nil
}

// executeDeploy creates a real deployment record so the workflow materializes
// on the GitOps Workflow board. Without a connected cluster the lifecycle is
// simulated (status goes straight to "deployed").
func (e *Engine) executeDeploy(ctx context.Context, step *models.StepSpec, tenantID uuid.UUID) (json.RawMessage, error) {
	var params struct {
		ProjectName     string `json:"project_name"`
		ImageTag        string `json:"image_tag"`
		ImageRepository string `json:"image_repository"`
		Stage           string `json:"stage"`
		TeamName        string `json:"team_name"`
		Namespace       string `json:"namespace"`
		JiraIssueKey    string `json:"jira_issue_key"`
		JiraSummary     string `json:"jira_summary"`
	}
	if step.Params != nil {
		if err := json.Unmarshal(step.Params, &params); err != nil {
			return nil, fmt.Errorf("parse deploy params: %w", err)
		}
	}
	if params.ImageTag == "" {
		params.ImageTag = "latest"
	}
	if params.Stage == "" {
		params.Stage = "dev"
	}
	if params.Namespace == "" {
		params.Namespace = "app-" + params.Stage
	}

	log.Printf("Step %q: deploy project=%s image=%s team=%s stage=%s", step.Name, params.ProjectName, params.ImageTag, params.TeamName, params.Stage)

	if e.deploymentRepo == nil {
		return nil, fmt.Errorf("deployment repository not available")
	}

	deployment := &repository.Deployment{
		TenantID:          tenantID,
		GitlabProjectName: params.ProjectName,
		ImageTag:          params.ImageTag,
		ImageRepository:   params.ImageRepository,
		TargetNamespace:   params.Namespace,
		TeamName:          params.TeamName,
		Stage:             params.Stage,
		JiraIssueKey:      params.JiraIssueKey,
		JiraSummary:       params.JiraSummary,
		Status:            "deployed",
		CreatedBy:         "workflow-engine",
	}
	if err := e.deploymentRepo.Create(ctx, deployment); err != nil {
		return nil, fmt.Errorf("create deployment: %w", err)
	}

	out, _ := json.Marshal(map[string]interface{}{
		"deployment_id": deployment.ID.String(),
		"project_name":  params.ProjectName,
		"image_tag":     params.ImageTag,
		"team":          params.TeamName,
		"stage":         params.Stage,
		"namespace":     params.Namespace,
		"status":        deployment.Status,
	})
	return out, nil
}

func (e *Engine) executeDeploySim(step *models.StepSpec) (json.RawMessage, error) {
	log.Printf("Step %q: deploy_sim (simulated deployment)", step.Name)
	// Parse params for deployment simulation
	var params struct {
		ServiceName string `json:"service_name"`
		Namespace   string `json:"namespace"`
		Image       string `json:"image"`
	}
	if err := json.Unmarshal(step.Params, &params); err != nil {
		// Use defaults if params are missing
		params.ServiceName = "unknown-service"
		params.Namespace = "default"
		params.Image = "latest"
	}

	// Simulate a successful deployment
	out, _ := json.Marshal(map[string]interface{}{
		"deployed":  true,
		"service":   params.ServiceName,
		"namespace": params.Namespace,
		"image":     params.Image,
		"replicas":  1,
		"simulated": true,
		"status":    "running",
		"message":   "Simulated deployment successful. In a real environment, this would deploy to Kubernetes.",
	})
	return out, nil
}

func (e *Engine) executePluginAction(ctx context.Context, step *models.StepSpec, tenantID uuid.UUID) (json.RawMessage, error) {
	parts := strings.SplitN(step.Plugin, ":", 2)
	pluginName := parts[0]
	actionName := step.Action
	if len(parts) > 1 {
		actionName = parts[1]
	}

	log.Printf("Step %q: plugin=%s action=%s params=%s", step.Name, pluginName, actionName, redactParams(step.Params))

	// Try real plugin dispatch via provider registry
	if e.providerRegistry != nil {
		resp, err := e.providerRegistry.ExecuteAction(ctx, pluginName, actionName, step.Params, nil)
		if err == nil && resp.Success {
			log.Printf("Step %q: plugin %s action %s succeeded (%d bytes output)", step.Name, pluginName, actionName, len(resp.Output))
			return json.RawMessage(resp.Output), nil
		}
		if err != nil {
			log.Printf("Step %q: plugin dispatch to %s failed: %v (falling back to simulated)", step.Name, pluginName, err)
		} else {
			// The engine does not pass connection config, so an unconfigured
			// plugin (e.g. Slack without webhook) must not break the pipeline.
			log.Printf("Step %q: plugin %s action %s reported failure: %s (falling back to simulated)", step.Name, pluginName, actionName, resp.Error)
		}
	}

	// Fallback: simulated execution
	out, _ := json.Marshal(map[string]interface{}{
		"plugin": pluginName,
		"action": actionName,
		"params": json.RawMessage(step.Params),
		"status": "simulated",
	})
	return out, nil
}

// executeStepWithConditions handles skip/run_when conditions and executes the step.
func (e *Engine) executeStepWithConditions(ctx context.Context, step *models.StepSpec, execID uuid.UUID, results map[string]*stepResult, tenantID uuid.UUID, startTime time.Time, inputs map[string]string) (json.RawMessage, string, error) {
	// Resolve {{ input.X }} placeholders from trigger parameters.
	resolved := *step
	if len(inputs) > 0 {
		resolved.Params = substituteInputs(step.Params, inputs)
		resolved.Condition = substituteString(step.Condition, inputs)
		resolved.SkipWhen = substituteString(step.SkipWhen, inputs)
	}
	step = &resolved

	// Check skip condition
	if step.SkipWhen != "" {
		if evaluateCondition(step.SkipWhen, results, inputs) {
			log.Printf("Step %q skipped (skip_when: %s)", step.Name, step.SkipWhen)
			e.recordStepExecution(ctx, execID, step, models.ExecutionSuccess, nil, nil, "skipped by condition", 0)
			return nil, "skipped", nil
		}
	}

	// Check run_when
	if step.RunWhen == "on_failure" && !hasFailed(results) {
		return nil, "skipped", nil
	}

	// Check if dependencies failed (unless run_when=always)
	if step.RunWhen != "always" && hasFailedDeps(step.DependsOn, results) {
		e.recordStepExecution(ctx, execID, step, models.ExecutionFailed, nil, nil, "dependency failed", 0)
		return nil, "failed", fmt.Errorf("dependency failed")
	}

	// Execute the step
	stepStart := time.Now()
	output, err := e.executeStep(ctx, step, execID, results, tenantID, inputs)
	stepDuration := int(time.Since(stepStart).Milliseconds())

	if err != nil {
		log.Printf("Step %q failed: %v", step.Name, err)
		e.recordStepExecution(ctx, execID, step, models.ExecutionFailed, nil, nil, err.Error(), stepDuration)
		return nil, "failed", err
	}

	e.recordStepExecution(ctx, execID, step, models.ExecutionSuccess, output, nil, "", stepDuration)
	return output, "success", nil
}

// recordStepExecution persists a step execution record.
func (e *Engine) recordStepExecution(ctx context.Context, execID uuid.UUID, step *models.StepSpec, status models.ExecutionStatus, output json.RawMessage, params json.RawMessage, errMsg string, durationMs int) {
	if params == nil {
		params = step.Params
	}
	stepExec := &models.StepExecution{
		ID:          uuid.New(),
		ExecutionID: execID,
		StepName:    step.Name,
		StepType:    step.Type,
		PluginName:  step.Plugin,
		ActionName:  step.Action,
		Params:      params,
		Status:      status,
		Output:      output,
		Error:       errMsg,
		DurationMs:  &durationMs,
		CreatedAt:   time.Now().UTC(),
	}
	now := time.Now().UTC()
	stepExec.StartedAt = &now
	if status == models.ExecutionSuccess || status == models.ExecutionFailed {
		completed := time.Now().UTC()
		stepExec.CompletedAt = &completed
	}

	if err := e.workflowRepo.CreateStepExecution(ctx, stepExec); err != nil {
		log.Printf("Failed to record step execution for %q: %v", step.Name, err)
	}
}

func (e *Engine) publishEvent(eventType string, tenantID uuid.UUID, payload map[string]interface{}) {
	if e.eventBus == nil {
		return
	}
	_ = e.eventBus.Publish(events.Event{
		Type:     eventType,
		TenantID: tenantID.String(),
		Payload:  payload,
	})
}

// ── DAG helpers ────────────────────────────────────────────

type stepResult struct {
	status string
	output json.RawMessage
	err    error
}

// buildDAG returns steps in topological levels (each level can run in parallel).
func buildDAG(steps []models.StepSpec) ([][]string, error) {
	// Build adjacency
	stepNames := make(map[string]bool)
	for _, s := range steps {
		if s.Name == "" {
			return nil, fmt.Errorf("step has empty name")
		}
		if stepNames[s.Name] {
			return nil, fmt.Errorf("duplicate step name: %s", s.Name)
		}
		stepNames[s.Name] = true
	}

	// Validate dependencies exist
	for _, s := range steps {
		for _, dep := range s.DependsOn {
			if !stepNames[dep] {
				return nil, fmt.Errorf("step %q depends on unknown step %q", s.Name, dep)
			}
		}
	}

	// Kahn's algorithm for topological sort with levels
	inDegree := make(map[string]int)
	dependents := make(map[string][]string)
	for _, s := range steps {
		if _, ok := inDegree[s.Name]; !ok {
			inDegree[s.Name] = 0
		}
		for _, dep := range s.DependsOn {
			inDegree[s.Name]++
			dependents[dep] = append(dependents[dep], s.Name)
		}
	}

	var levels [][]string
	processed := 0

	for {
		var level []string
		for name, deg := range inDegree {
			if deg == 0 {
				level = append(level, name)
			}
		}
		if len(level) == 0 {
			break
		}

		levels = append(levels, level)
		processed += len(level)

		for _, name := range level {
			delete(inDegree, name)
			for _, dep := range dependents[name] {
				inDegree[dep]--
			}
		}
	}

	if processed < len(steps) {
		return nil, fmt.Errorf("circular dependency detected in workflow steps")
	}

	return levels, nil
}

func stepIndex(steps []models.StepSpec, name string) int {
	for i, s := range steps {
		if s.Name == name {
			return i
		}
	}
	return -1
}

func hasFailed(results map[string]*stepResult) bool {
	for _, r := range results {
		if r.status == "failed" {
			return true
		}
	}
	return false
}

func hasFailedDeps(deps []string, results map[string]*stepResult) bool {
	for _, dep := range deps {
		if r, ok := results[dep]; ok && r.status == "failed" {
			return true
		}
	}
	return false
}

// evaluateCondition is a simple condition evaluator.
// Supports basic expressions like "entity.status == active",
// "steps.<name> == <status>" and "input.<name> == <value>".
func evaluateCondition(cond string, results map[string]*stepResult, inputs map[string]string) bool {
	if cond == "" {
		return true
	}

	// Simple parser for "key == value" patterns
	parts := strings.SplitN(cond, "==", 2)
	if len(parts) == 2 {
		left := strings.TrimSpace(parts[0])
		right := strings.TrimSpace(parts[1])

		// Strip negation prefix so "!input.x == v" is recognized.
		negated := false
		if strings.HasPrefix(left, "!") {
			negated = true
			left = strings.TrimSpace(strings.TrimPrefix(left, "!"))
		}

		result := func() bool {
			// Check if left side references a step result
			if strings.HasPrefix(left, "steps.") {
				stepName := strings.TrimPrefix(left, "steps.")
				if r, ok := results[stepName]; ok {
					return r.status == right
				}
			}

			// Check if left side references a trigger input
			if strings.HasPrefix(left, "input.") {
				name := strings.TrimPrefix(left, "input.")
				if v, ok := inputs[name]; ok {
					return v == right
				}
				return false
			}

			// For now, simple string comparison
			return left == right
		}()
		if negated {
			return !result
		}
		return result
	}

	// Negation
	if strings.HasPrefix(cond, "!") {
		return !evaluateCondition(cond[1:], results, inputs)
	}

	// Default: true
	return true
}

func stepResultsToMap(results map[string]*stepResult) map[string]interface{} {
	out := make(map[string]interface{})
	for name, r := range results {
		entry := map[string]interface{}{"status": r.status}
		if r.output != nil {
			entry["output"] = json.RawMessage(r.output)
		}
		if r.err != nil {
			entry["error"] = r.err.Error()
		}
		out[name] = entry
	}
	return out
}

// redactParams returns a truncated view of step params for safe logging.
// Sensitive-looking values (tokens, keys, passwords) are replaced with "[REDACTED]".
func redactParams(raw json.RawMessage) string {
	const maxLen = 200

	// Try to parse as a JSON object and redact sensitive keys.
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err == nil {
		redacted := make(map[string]interface{}, len(m))
		for k, v := range m {
			if isSensitiveKey(k) {
				redacted[k] = "[REDACTED]"
			} else {
				redacted[k] = json.RawMessage(v)
			}
		}
		out, _ := json.Marshal(redacted)
		if len(out) > maxLen {
			return string(out[:maxLen]) + "...(truncated)"
		}
		return string(out)
	}

	// Fallback: truncate raw string.
	s := string(raw)
	if len(s) > maxLen {
		return s[:maxLen] + "...(truncated)"
	}
	return s
}

func isSensitiveKey(key string) bool {
	return utils.IsSensitiveKey(key)
}

// ── Input substitution ────────────────────────────────────

// inputPlaceholder matches {{ input.NAME }} placeholders in step params.
var inputPlaceholder = regexp.MustCompile(`\{\{\s*input\.([a-zA-Z0-9_.-]+)\s*\}\}`)

// parseTriggerInputs converts the execution trigger payload into a flat
// string map usable for {{ input.X }} substitution.
func parseTriggerInputs(payload json.RawMessage) map[string]string {
	out := map[string]string{}
	if len(payload) == 0 {
		return out
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(payload, &raw); err != nil {
		return out
	}
	for k, v := range raw {
		switch t := v.(type) {
		case string:
			out[k] = t
		case nil:
			// skip
		default:
			b, _ := json.Marshal(t)
			out[k] = strings.Trim(string(b), "\"")
		}
	}
	return out
}

// substituteInputs replaces {{ input.X }} placeholders inside all string
// values of a JSON document with values from the trigger parameters.
func substituteInputs(raw json.RawMessage, inputs map[string]string) json.RawMessage {
	if raw == nil || len(inputs) == 0 {
		return raw
	}
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return raw
	}
	out, _ := json.Marshal(walkSubstitute(v, inputs))
	return out
}

// substituteString replaces {{ input.X }} placeholders in a plain string.
func substituteString(s string, inputs map[string]string) string {
	if s == "" || len(inputs) == 0 {
		return s
	}
	return inputPlaceholder.ReplaceAllStringFunc(s, func(m string) string {
		name := inputPlaceholder.FindStringSubmatch(m)[1]
		if val, ok := inputs[name]; ok {
			return val
		}
		return m
	})
}

func walkSubstitute(v interface{}, inputs map[string]string) interface{} {
	switch t := v.(type) {
	case string:
		return substituteString(t, inputs)
	case map[string]interface{}:
		for k, val := range t {
			t[k] = walkSubstitute(val, inputs)
		}
		return t
	case []interface{}:
		for i, val := range t {
			t[i] = walkSubstitute(val, inputs)
		}
		return t
	default:
		return v
	}
}
