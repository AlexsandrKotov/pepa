package pipeline

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// WorkflowGraph represents the parsed structure of a CI/CD workflow file.
type WorkflowGraph struct {
	Name     string      `json:"name"`     // workflow name
	Source   string      `json:"source"`   // "github_actions" | "gitlab_ci"
	Stages   []StageInfo `json:"stages"`   // ordered stages/layers
	Jobs     []JobNode   `json:"jobs"`     // all jobs with dependencies
	Triggers []string    `json:"triggers"` // e.g. ["push", "workflow_dispatch"]
}

// StageInfo describes a pipeline stage or layer.
type StageInfo struct {
	Name  string `json:"name"`  // e.g. "build", "test", "deploy"
	Order int    `json:"order"` // stage order index (0-based)
}

// JobNode describes a single job within the workflow graph.
type JobNode struct {
	Name   string   `json:"name"`
	Stage  string   `json:"stage"`
	Needs  []string `json:"needs"`              // job names this depends on
	RunsOn string   `json:"runs_on,omitempty"`  // e.g. "ubuntu-latest"
	If     string   `json:"if,omitempty"`       // conditional expression
}

// ── GitHub Actions parser ──────────────────────────────────────

// ParseGitHubWorkflowGraph parses a GitHub Actions workflow YAML into a WorkflowGraph.
func ParseGitHubWorkflowGraph(yamlContent string) (*WorkflowGraph, error) {
	if yamlContent == "" {
		return nil, fmt.Errorf("empty workflow content")
	}

	var workflow map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlContent), &workflow); err != nil {
		return nil, fmt.Errorf("parse workflow YAML: %w", err)
	}

	graph := &WorkflowGraph{Source: "github_actions"}

	// Workflow name
	if name, ok := workflow["name"].(string); ok {
		graph.Name = name
	}

	// Triggers from "on" key
	graph.Triggers = ghParseTriggers(workflow["on"])
	if graph.Triggers == nil {
		graph.Triggers = []string{}
	}

	// Jobs
	jobsRaw, ok := workflow["jobs"].(map[string]interface{})
	if !ok {
		graph.Jobs = []JobNode{}
		return graph, nil // no jobs defined
	}

	// First pass: collect all job nodes
	jobMap := make(map[string]*JobNode)
	for jobID, jobRaw := range jobsRaw {
		jobDef, ok := jobRaw.(map[string]interface{})
		if !ok {
			continue
		}
		node := &JobNode{Name: jobID}

		// Display name
		if name, ok := jobDef["name"].(string); ok && name != "" {
			node.Name = name
		}

		// runs-on
		if runsOn, ok := jobDef["runs-on"]; ok {
			node.RunsOn = ghStringifyValue(runsOn)
		}

		// if condition
		if ifCond, ok := jobDef["if"]; ok {
			node.If = ghStringifyValue(ifCond)
		}

		// needs (can be string or []string)
		if needsRaw, ok := jobDef["needs"]; ok {
			node.Needs = ghStringifyList(needsRaw)
		}
		if node.Needs == nil {
			node.Needs = []string{}
		}

		jobMap[jobID] = node
	}

	// Compute topological layers for ordering
	layers := ghTopologicalLayers(jobMap)

	// Assign stages based on layers (use numeric names to avoid confusion)
	for jobID, node := range jobMap {
		layer := layers[jobID]
		node.Stage = fmt.Sprintf("%d", layer)
		graph.Jobs = append(graph.Jobs, *node)
	}

	// Sort jobs by stage (layer) order, then by name
	sort.Slice(graph.Jobs, func(i, j int) bool {
		li := ghLayerIndex(graph.Jobs[i].Stage)
		lj := ghLayerIndex(graph.Jobs[j].Stage)
		if li != lj {
			return li < lj
		}
		return graph.Jobs[i].Name < graph.Jobs[j].Name
	})

	// Build stages list
	stageSet := make(map[string]bool)
	for _, j := range graph.Jobs {
		stageSet[j.Stage] = true
	}
	for name := range stageSet {
		graph.Stages = append(graph.Stages, StageInfo{
			Name:  name,
			Order: ghLayerIndex(name),
		})
	}
	sort.Slice(graph.Stages, func(i, j int) bool {
		return graph.Stages[i].Order < graph.Stages[j].Order
	})

	return graph, nil
}

// ghParseTriggers extracts trigger names from the "on" key.
func ghParseTriggers(onRaw interface{}) []string {
	var triggers []string
	switch v := onRaw.(type) {
	case string:
		triggers = append(triggers, v)
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok {
				triggers = append(triggers, s)
			}
		}
	case map[string]interface{}:
		for key := range v {
			triggers = append(triggers, key)
		}
		sort.Strings(triggers)
	}
	return triggers
}

// ghStringifyValue converts a YAML value to its string representation.
func ghStringifyValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case int:
		return fmt.Sprintf("%d", val)
	case float64:
		return fmt.Sprintf("%.0f", val)
	case bool:
		return fmt.Sprintf("%t", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// ghStringifyList converts a YAML value (string or []interface{}) to []string.
func ghStringifyList(v interface{}) []string {
	switch val := v.(type) {
	case string:
		return []string{val}
	case []interface{}:
		var result []string
		for _, item := range val {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	default:
		return nil
	}
}

// ghTopologicalLayers assigns each job to a layer based on dependency depth.
// Layer 0 = no dependencies, Layer 1 = depends on layer 0 jobs, etc.
func ghTopologicalLayers(jobMap map[string]*JobNode) map[string]int {
	layers := make(map[string]int)

	// Build a lookup from job name/ID to node
	// GitHub Actions "needs" references the job key (ID), not the display name.
	// We need to map both ways.
	idByName := make(map[string]string) // display name -> job key
	for key, node := range jobMap {
		idByName[node.Name] = key
	}

	// Resolve needs to job keys
	resolvedNeeds := make(map[string][]string) // job key -> list of dependency job keys
	for key, node := range jobMap {
		for _, need := range node.Needs {
			// "needs" references job keys directly, but also check by name
			if _, exists := jobMap[need]; exists {
				resolvedNeeds[key] = append(resolvedNeeds[key], need)
			} else if id, ok := idByName[need]; ok {
				resolvedNeeds[key] = append(resolvedNeeds[key], id)
			}
		}
	}

	// Compute layers using BFS
	var computeLayer func(key string, visited map[string]bool) int
	computeLayer = func(key string, visited map[string]bool) int {
		if l, ok := layers[key]; ok {
			return l
		}
		if visited[key] {
			return 0 // cycle detected, break it
		}
		visited[key] = true

		needs := resolvedNeeds[key]
		if len(needs) == 0 {
			layers[key] = 0
			return 0
		}

		maxDep := 0
		for _, dep := range needs {
			d := computeLayer(dep, visited)
			if d+1 > maxDep {
				maxDep = d + 1
			}
		}
		layers[key] = maxDep
		return maxDep
	}

	for key := range jobMap {
		computeLayer(key, make(map[string]bool))
	}

	return layers
}

// ghLayerIndex extracts the layer index from a stage name.
// Handles both numeric stages ("0", "1", "2") and named stages ("setup", "build", etc.)
func ghLayerIndex(stage string) int {
	// Try parsing as numeric first
	if n, err := strconv.Atoi(stage); err == nil {
		return n
	}
	// Fallback to named stages for backward compatibility
	if strings.HasPrefix(stage, "stage-") {
		var n int
		if _, err := fmt.Sscanf(stage, "stage-%d", &n); err == nil {
			return n
		}
	}
	switch stage {
	case "setup":
		return 0
	case "build":
		return 1
	case "test":
		return 2
	case "deploy":
		return 3
	default:
		return 99
	}
}

// ── GitLab CI parser ───────────────────────────────────────────

// glReservedKeys are top-level YAML keys in .gitlab-ci.yml that are not job definitions.
var glReservedKeys = map[string]bool{
	"image": true, "services": true, "stages": true, "variables": true,
	"before_script": true, "after_script": true, "cache": true,
	"include": true, "default": true, "workflow": true,
}

// ParseGitLabWorkflowGraph parses a GitLab CI YAML into a WorkflowGraph.
func ParseGitLabWorkflowGraph(yamlContent string) (*WorkflowGraph, error) {
	if yamlContent == "" {
		return nil, fmt.Errorf("empty workflow content")
	}

	var ci map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlContent), &ci); err != nil {
		return nil, fmt.Errorf("parse GitLab CI YAML: %w", err)
	}

	graph := &WorkflowGraph{Source: "gitlab_ci"}

	// Workflow name (GitLab doesn't have a top-level name, use filename context)
	graph.Name = ".gitlab-ci.yml"
	graph.Triggers = []string{}

	// Stages
	var stagesList []string
	if stagesRaw, ok := ci["stages"].([]interface{}); ok {
		for i, s := range stagesRaw {
			if name, ok := s.(string); ok {
				stagesList = append(stagesList, name)
				graph.Stages = append(graph.Stages, StageInfo{Name: name, Order: i})
			}
		}
	}
	// Ensure default stages if not defined
	if len(stagesList) == 0 {
		stagesList = []string{"build", "test", "deploy"}
		graph.Stages = []StageInfo{
			{Name: "build", Order: 0},
			{Name: "test", Order: 1},
			{Name: "deploy", Order: 2},
		}
	}

	// Build stage index lookup
	stageOrder := make(map[string]int)
	for i, s := range stagesList {
		stageOrder[s] = i
	}

	// Jobs (keys not in reserved list, not starting with ".")
	for key, val := range ci {
		if glReservedKeys[key] {
			continue
		}
		if strings.HasPrefix(key, ".") {
			continue // hidden/template job
		}
		jobDef, ok := val.(map[string]interface{})
		if !ok {
			continue
		}

		node := JobNode{Name: key, Needs: []string{}}

		// Stage
		if stage, ok := jobDef["stage"].(string); ok && stage != "" {
			node.Stage = stage
		} else {
			node.Stage = "build" // default stage
		}

		// needs / dependencies
		if needsRaw, ok := jobDef["needs"]; ok {
			node.Needs = glParseNeeds(needsRaw)
		} else if depsRaw, ok := jobDef["dependencies"]; ok {
			node.Needs = glParseNeeds(depsRaw)
		}

		// image as runs_on equivalent
		if img, ok := jobDef["image"]; ok {
			node.RunsOn = glStringifyImage(img)
		}

		// rules/only/when as if equivalent
		if rules, ok := jobDef["rules"].([]interface{}); ok && len(rules) > 0 {
			node.If = "rules defined"
		} else if only, ok := jobDef["only"]; ok {
			node.If = fmt.Sprintf("only: %v", only)
		}

		graph.Jobs = append(graph.Jobs, node)
	}

	// Sort jobs by stage order, then by name
	sort.Slice(graph.Jobs, func(i, j int) bool {
		oi := stageOrder[graph.Jobs[i].Stage]
		oj := stageOrder[graph.Jobs[j].Stage]
		if oi != oj {
			return oi < oj
		}
		return graph.Jobs[i].Name < graph.Jobs[j].Name
	})

	return graph, nil
}

// glParseNeeds converts GitLab needs/dependencies YAML to []string.
func glParseNeeds(raw interface{}) []string {
	switch v := raw.(type) {
	case []interface{}:
		var result []string
		for _, item := range v {
			switch val := item.(type) {
			case string:
				result = append(result, val)
			case map[string]interface{}:
				// needs: [{job: "build", artifacts: true}]
				if job, ok := val["job"].(string); ok {
					result = append(result, job)
				}
			}
		}
		return result
	case string:
		return []string{v}
	default:
		return nil
	}
}

// glStringifyImage converts a GitLab image value to a string.
func glStringifyImage(img interface{}) string {
	switch v := img.(type) {
	case string:
		return v
	case map[string]interface{}:
		if name, ok := v["name"].(string); ok {
			return name
		}
	}
	return ""
}
