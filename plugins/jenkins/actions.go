package main

import (
	"context"
	"encoding/json"
	"fmt"
)

// ── Input types ──────────────────────────────────────────────

type jobParams struct {
	JobName string `json:"job_name"`
	Folder  string `json:"folder,omitempty"`
	Start   int    `json:"start,omitempty"`
	Limit   int    `json:"limit,omitempty"`
}

type buildParams struct {
	JobName string `json:"job_name"`
	Number  int    `json:"number"`
}

type triggerParams struct {
	JobName string            `json:"job_name"`
	Params  map[string]string `json:"params,omitempty"`
}

type createJobParams struct {
	JobName string    `json:"job_name"`
	Folder  string    `json:"folder,omitempty"`
	Script  string    `json:"script"`
	Params  []ParamDef `json:"params,omitempty"`
	Sandbox bool      `json:"sandbox"`
}

type updateJobParams struct {
	JobName     string    `json:"job_name"`
	Description string    `json:"description,omitempty"`
	Script      string    `json:"script,omitempty"`
	Params      []ParamDef `json:"params,omitempty"`
	Sandbox     *bool     `json:"sandbox,omitempty"`
}

type updateScriptParams struct {
	JobName string `json:"job_name"`
	Script  string `json:"script"`
}

type updateConfigXMLParams struct {
	JobName   string `json:"job_name"`
	ConfigXML string `json:"config_xml"`
}

type progressiveLogParams struct {
	JobName string `json:"job_name"`
	Number  int    `json:"number"`
	Start   int64  `json:"start,omitempty"`
}

// ── Jobs & Folders ───────────────────────────────────────────

func (p *JenkinsPlugin) listJobs(ctx context.Context, c *JenkinsClient, data []byte) ([]byte, error) {
	var params jobParams
	if err := actionInput(data, &params); err != nil {
		return nil, err
	}
	if params.Limit <= 0 {
		params.Limit = 50
	}

	result, err := c.ListJobs(ctx, params.Folder, params.Start, params.Limit)
	if err != nil {
		return nil, err
	}
	return actionOutput(result)
}

func (p *JenkinsPlugin) listFolders(ctx context.Context, c *JenkinsClient, data []byte) ([]byte, error) {
	var params struct {
		Folder string `json:"folder,omitempty"`
	}
	if err := actionInput(data, &params); err != nil {
		return nil, err
	}

	result, err := c.ListJobs(ctx, params.Folder, 0, 200)
	if err != nil {
		return nil, err
	}

	// Filter to only folders (cloudbees-folder)
	var folders []JenkinsJob
	for _, j := range result.Jobs {
		if j.Class == "com.cloudbees.hudson.plugins.folder.Folder" {
			folders = append(folders, j)
		}
	}
	return actionOutput(map[string]interface{}{"folders": folders})
}

func (p *JenkinsPlugin) getJob(ctx context.Context, c *JenkinsClient, data []byte) ([]byte, error) {
	var params struct {
		JobName string `json:"job_name"`
	}
	if err := actionInput(data, &params); err != nil {
		return nil, err
	}
	if params.JobName == "" {
		return nil, fmt.Errorf("job_name is required")
	}

	result, err := c.GetJobDetails(ctx, params.JobName)
	if err != nil {
		return nil, err
	}
	return actionOutput(result)
}

func (p *JenkinsPlugin) createJob(ctx context.Context, c *JenkinsClient, data []byte) ([]byte, error) {
	var params createJobParams
	if err := actionInput(data, &params); err != nil {
		return nil, err
	}
	if params.JobName == "" {
		return nil, fmt.Errorf("job_name is required")
	}
	if params.Script == "" {
		return nil, fmt.Errorf("script is required")
	}

	xmlBody := buildPipelineJobXML(params.Script, params.Params, params.Sandbox)

	if params.Folder != "" {
		err := c.CreateJobInFolder(ctx, params.JobName, params.Folder, xmlBody)
		if err != nil {
			return nil, err
		}
	} else {
		err := c.CreateJob(ctx, params.JobName, xmlBody)
		if err != nil {
			return nil, err
		}
	}

	return actionOutput(map[string]interface{}{
		"success": true,
		"job":     params.JobName,
		"message": fmt.Sprintf("Pipeline job %q created successfully", params.JobName),
	})
}

func (p *JenkinsPlugin) updateJob(ctx context.Context, c *JenkinsClient, data []byte) ([]byte, error) {
	var params updateJobParams
	if err := actionInput(data, &params); err != nil {
		return nil, err
	}
	if params.JobName == "" {
		return nil, fmt.Errorf("job_name is required")
	}

	// Fetch current config.xml
	currentXML, err := c.GetJobConfigXML(ctx, params.JobName)
	if err != nil {
		return nil, fmt.Errorf("fetch current config: %w", err)
	}

	// If a new script is provided, update it in the XML
	if params.Script != "" {
		updatedXML, err := updateScriptInXML(currentXML, params.Script)
		if err != nil {
			return nil, fmt.Errorf("update script in config: %w", err)
		}
		currentXML = updatedXML
	}

	// Push updated config back
	if err := c.UpdateJobConfigXML(ctx, params.JobName, currentXML); err != nil {
		return nil, err
	}

	return actionOutput(map[string]interface{}{
		"success": true,
		"job":     params.JobName,
		"message": fmt.Sprintf("Pipeline job %q updated successfully", params.JobName),
	})
}

func (p *JenkinsPlugin) deleteJob(ctx context.Context, c *JenkinsClient, data []byte) ([]byte, error) {
	var params struct {
		JobName string `json:"job_name"`
	}
	if err := actionInput(data, &params); err != nil {
		return nil, err
	}
	if params.JobName == "" {
		return nil, fmt.Errorf("job_name is required")
	}

	if err := c.DeleteJob(ctx, params.JobName); err != nil {
		return nil, err
	}
	return actionOutput(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Job %q deleted", params.JobName),
	})
}

func (p *JenkinsPlugin) enableJob(ctx context.Context, c *JenkinsClient, data []byte) ([]byte, error) {
	var params struct {
		JobName string `json:"job_name"`
	}
	if err := actionInput(data, &params); err != nil {
		return nil, err
	}
	if params.JobName == "" {
		return nil, fmt.Errorf("job_name is required")
	}

	if err := c.EnableJob(ctx, params.JobName); err != nil {
		return nil, err
	}
	return actionOutput(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Job %q enabled", params.JobName),
	})
}

func (p *JenkinsPlugin) disableJob(ctx context.Context, c *JenkinsClient, data []byte) ([]byte, error) {
	var params struct {
		JobName string `json:"job_name"`
	}
	if err := actionInput(data, &params); err != nil {
		return nil, err
	}
	if params.JobName == "" {
		return nil, fmt.Errorf("job_name is required")
	}

	if err := c.DisableJob(ctx, params.JobName); err != nil {
		return nil, err
	}
	return actionOutput(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Job %q disabled", params.JobName),
	})
}

// ── Builds & Parameters ──────────────────────────────────────

func (p *JenkinsPlugin) listBuilds(ctx context.Context, c *JenkinsClient, data []byte) ([]byte, error) {
	var params struct {
		JobName string `json:"job_name"`
		Start   int    `json:"start,omitempty"`
		Limit   int    `json:"limit,omitempty"`
	}
	if err := actionInput(data, &params); err != nil {
		return nil, err
	}
	if params.JobName == "" {
		return nil, fmt.Errorf("job_name is required")
	}
	if params.Limit <= 0 {
		params.Limit = 20
	}

	builds, err := c.ListBuilds(ctx, params.JobName, params.Start, params.Limit)
	if err != nil {
		return nil, err
	}

	// Enrich builds with status mapping
	type enrichedBuild struct {
		JenkinsBuild
		Status string `json:"status"`
	}
	enriched := make([]enrichedBuild, 0, len(builds))
	for _, b := range builds {
		enriched = append(enriched, enrichedBuild{
			JenkinsBuild: b,
			Status:       mapJenkinsStatus(b.Result, b.Building),
		})
	}

	return actionOutput(map[string]interface{}{"builds": enriched})
}

func (p *JenkinsPlugin) getBuild(ctx context.Context, c *JenkinsClient, data []byte) ([]byte, error) {
	var params buildParams
	if err := actionInput(data, &params); err != nil {
		return nil, err
	}
	if params.JobName == "" {
		return nil, fmt.Errorf("job_name is required")
	}
	if params.Number == 0 {
		return nil, fmt.Errorf("number is required")
	}

	build, err := c.GetBuild(ctx, params.JobName, params.Number)
	if err != nil {
		return nil, err
	}

	return actionOutput(map[string]interface{}{
		"build":  build,
		"status": mapJenkinsStatus(build.Result, build.Building),
	})
}

func (p *JenkinsPlugin) triggerBuild(ctx context.Context, c *JenkinsClient, data []byte) ([]byte, error) {
	var params triggerParams
	if err := actionInput(data, &params); err != nil {
		return nil, err
	}
	if params.JobName == "" {
		return nil, fmt.Errorf("job_name is required")
	}

	if len(params.Params) > 0 {
		if err := c.TriggerBuildWithParams(ctx, params.JobName, params.Params); err != nil {
			return nil, err
		}
	} else {
		if err := c.TriggerBuild(ctx, params.JobName); err != nil {
			return nil, err
		}
	}

	return actionOutput(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Build triggered for %q", params.JobName),
	})
}

func (p *JenkinsPlugin) rebuild(ctx context.Context, c *JenkinsClient, data []byte) ([]byte, error) {
	var params buildParams
	if err := actionInput(data, &params); err != nil {
		return nil, err
	}
	if params.JobName == "" {
		return nil, fmt.Errorf("job_name is required")
	}
	if params.Number == 0 {
		return nil, fmt.Errorf("number is required")
	}

	if err := c.Rebuild(ctx, params.JobName, params.Number); err != nil {
		return nil, err
	}
	return actionOutput(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Rebuild triggered for %s #%d", params.JobName, params.Number),
	})
}

func (p *JenkinsPlugin) stopBuild(ctx context.Context, c *JenkinsClient, data []byte) ([]byte, error) {
	var params buildParams
	if err := actionInput(data, &params); err != nil {
		return nil, err
	}
	if params.JobName == "" {
		return nil, fmt.Errorf("job_name is required")
	}
	if params.Number == 0 {
		return nil, fmt.Errorf("number is required")
	}

	if err := c.StopBuild(ctx, params.JobName, params.Number); err != nil {
		return nil, err
	}
	return actionOutput(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Build %s #%d stopped", params.JobName, params.Number),
	})
}

func (p *JenkinsPlugin) getBuildParameters(ctx context.Context, c *JenkinsClient, data []byte) ([]byte, error) {
	var params buildParams
	if err := actionInput(data, &params); err != nil {
		return nil, err
	}
	if params.JobName == "" {
		return nil, fmt.Errorf("job_name is required")
	}
	if params.Number == 0 {
		return nil, fmt.Errorf("number is required")
	}

	// Fetch build details with parameter actions
	build, err := c.GetBuild(ctx, params.JobName, params.Number)
	if err != nil {
		return nil, err
	}

	// Extract parameters from actions
	type paramValue struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	var buildParams []paramValue
	for _, raw := range build.Actions {
		var action struct {
			Class      string `json:"_class"`
			Parameters []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			} `json:"parameters"`
		}
		if err := json.Unmarshal(raw, &action); err != nil {
			continue
		}
		if action.Class == "hudson.model.ParametersAction" {
			for _, p := range action.Parameters {
				buildParams = append(buildParams, paramValue{Name: p.Name, Value: p.Value})
			}
		}
	}

	return actionOutput(map[string]interface{}{"parameters": buildParams})
}

// ── Groovy Pipeline Script ───────────────────────────────────

func (p *JenkinsPlugin) getPipelineScript(ctx context.Context, c *JenkinsClient, data []byte) ([]byte, error) {
	var params struct {
		JobName string `json:"job_name"`
	}
	if err := actionInput(data, &params); err != nil {
		return nil, err
	}
	if params.JobName == "" {
		return nil, fmt.Errorf("job_name is required")
	}

	configXML, err := c.GetJobConfigXML(ctx, params.JobName)
	if err != nil {
		return nil, err
	}

	script, pipelineType, _, err := parseJobConfigXML(configXML)
	if err != nil {
		return nil, err
	}

	return actionOutput(map[string]interface{}{
		"script":        script,
		"type":          pipelineType,
		"config_xml":    configXML,
	})
}

func (p *JenkinsPlugin) updatePipelineScript(ctx context.Context, c *JenkinsClient, data []byte) ([]byte, error) {
	var params updateScriptParams
	if err := actionInput(data, &params); err != nil {
		return nil, err
	}
	if params.JobName == "" {
		return nil, fmt.Errorf("job_name is required")
	}
	if params.Script == "" {
		return nil, fmt.Errorf("script is required")
	}

	// Fetch current config.xml
	currentXML, err := c.GetJobConfigXML(ctx, params.JobName)
	if err != nil {
		return nil, fmt.Errorf("fetch current config: %w", err)
	}

	// Update the script block
	updatedXML, err := updateScriptInXML(currentXML, params.Script)
	if err != nil {
		return nil, fmt.Errorf("update script: %w", err)
	}

	// Push back
	if err := c.UpdateJobConfigXML(ctx, params.JobName, updatedXML); err != nil {
		return nil, err
	}

	return actionOutput(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Pipeline script for %q updated", params.JobName),
	})
}

func (p *JenkinsPlugin) getJobConfigXML(ctx context.Context, c *JenkinsClient, data []byte) ([]byte, error) {
	var params struct {
		JobName string `json:"job_name"`
	}
	if err := actionInput(data, &params); err != nil {
		return nil, err
	}
	if params.JobName == "" {
		return nil, fmt.Errorf("job_name is required")
	}

	configXML, err := c.GetJobConfigXML(ctx, params.JobName)
	if err != nil {
		return nil, err
	}

	return actionOutput(map[string]interface{}{"config_xml": configXML})
}

func (p *JenkinsPlugin) updateJobConfigXML(ctx context.Context, c *JenkinsClient, data []byte) ([]byte, error) {
	var params updateConfigXMLParams
	if err := actionInput(data, &params); err != nil {
		return nil, err
	}
	if params.JobName == "" {
		return nil, fmt.Errorf("job_name is required")
	}
	if params.ConfigXML == "" {
		return nil, fmt.Errorf("config_xml is required")
	}

	if err := c.UpdateJobConfigXML(ctx, params.JobName, params.ConfigXML); err != nil {
		return nil, err
	}

	return actionOutput(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Config XML for %q updated", params.JobName),
	})
}

// ── Logs & Artifacts ─────────────────────────────────────────

func (p *JenkinsPlugin) getBuildLog(ctx context.Context, c *JenkinsClient, data []byte) ([]byte, error) {
	var params buildParams
	if err := actionInput(data, &params); err != nil {
		return nil, err
	}
	if params.JobName == "" {
		return nil, fmt.Errorf("job_name is required")
	}
	if params.Number == 0 {
		return nil, fmt.Errorf("number is required")
	}

	log, err := c.GetBuildLog(ctx, params.JobName, params.Number)
	if err != nil {
		return nil, err
	}
	return actionOutput(map[string]interface{}{"log": log})
}

func (p *JenkinsPlugin) getBuildLogProgressive(ctx context.Context, c *JenkinsClient, data []byte) ([]byte, error) {
	var params progressiveLogParams
	if err := actionInput(data, &params); err != nil {
		return nil, err
	}
	if params.JobName == "" {
		return nil, fmt.Errorf("job_name is required")
	}
	if params.Number == 0 {
		return nil, fmt.Errorf("number is required")
	}

	text, hasMore, newSize, err := c.GetBuildLogProgressive(ctx, params.JobName, params.Number, params.Start)
	if err != nil {
		return nil, err
	}
	return actionOutput(map[string]interface{}{
		"text":     text,
		"has_more": hasMore,
		"offset":   newSize,
	})
}

func (p *JenkinsPlugin) getBuildArtifacts(ctx context.Context, c *JenkinsClient, data []byte) ([]byte, error) {
	var params buildParams
	if err := actionInput(data, &params); err != nil {
		return nil, err
	}
	if params.JobName == "" {
		return nil, fmt.Errorf("job_name is required")
	}
	if params.Number == 0 {
		return nil, fmt.Errorf("number is required")
	}

	artifacts, err := c.GetBuildArtifacts(ctx, params.JobName, params.Number)
	if err != nil {
		return nil, err
	}
	return actionOutput(map[string]interface{}{"artifacts": artifacts})
}

func (p *JenkinsPlugin) getBuildChanges(ctx context.Context, c *JenkinsClient, data []byte) ([]byte, error) {
	var params buildParams
	if err := actionInput(data, &params); err != nil {
		return nil, err
	}
	if params.JobName == "" {
		return nil, fmt.Errorf("job_name is required")
	}
	if params.Number == 0 {
		return nil, fmt.Errorf("number is required")
	}

	changes, err := c.GetBuildChanges(ctx, params.JobName, params.Number)
	if err != nil {
		return nil, err
	}
	return actionOutput(map[string]interface{}{"changes": changes})
}

func (p *JenkinsPlugin) getBuildTestResults(ctx context.Context, c *JenkinsClient, data []byte) ([]byte, error) {
	var params buildParams
	if err := actionInput(data, &params); err != nil {
		return nil, err
	}
	if params.JobName == "" {
		return nil, fmt.Errorf("job_name is required")
	}
	if params.Number == 0 {
		return nil, fmt.Errorf("number is required")
	}

	results, err := c.GetBuildTestResults(ctx, params.JobName, params.Number)
	if err != nil {
		return nil, err
	}
	return actionOutput(map[string]interface{}{"test_results": results})
}

// ── Pipeline Stages ──────────────────────────────────────────

func (p *JenkinsPlugin) getPipelineStages(ctx context.Context, c *JenkinsClient, data []byte) ([]byte, error) {
	var params buildParams
	if err := actionInput(data, &params); err != nil {
		return nil, err
	}
	if params.JobName == "" {
		return nil, fmt.Errorf("job_name is required")
	}
	if params.Number == 0 {
		return nil, fmt.Errorf("number is required")
	}

	stages, err := c.GetPipelineStages(ctx, params.JobName, params.Number)
	if err != nil {
		return nil, err
	}
	return actionOutput(map[string]interface{}{"stages": stages})
}

// ── Infrastructure ───────────────────────────────────────────

func (p *JenkinsPlugin) listViews(ctx context.Context, c *JenkinsClient, _ []byte) ([]byte, error) {
	views, err := c.ListViews(ctx)
	if err != nil {
		return nil, err
	}
	return actionOutput(map[string]interface{}{"views": views})
}

func (p *JenkinsPlugin) listNodes(ctx context.Context, c *JenkinsClient, _ []byte) ([]byte, error) {
	nodes, err := c.ListNodes(ctx)
	if err != nil {
		return nil, err
	}
	return actionOutput(map[string]interface{}{"nodes": nodes})
}

func (p *JenkinsPlugin) getNode(ctx context.Context, c *JenkinsClient, data []byte) ([]byte, error) {
	var params struct {
		Name string `json:"name"`
	}
	if err := actionInput(data, &params); err != nil {
		return nil, err
	}
	if params.Name == "" {
		return nil, fmt.Errorf("name is required")
	}

	node, err := c.GetNode(ctx, params.Name)
	if err != nil {
		return nil, err
	}
	return actionOutput(map[string]interface{}{"node": node})
}

func (p *JenkinsPlugin) getQueue(ctx context.Context, c *JenkinsClient, _ []byte) ([]byte, error) {
	queue, err := c.GetQueue(ctx)
	if err != nil {
		return nil, err
	}
	return actionOutput(map[string]interface{}{"queue": queue})
}

func (p *JenkinsPlugin) listCredentials(ctx context.Context, c *JenkinsClient, _ []byte) ([]byte, error) {
	creds, err := c.ListCredentials(ctx)
	if err != nil {
		return nil, err
	}
	return actionOutput(map[string]interface{}{"credentials": creds})
}

// ── System ───────────────────────────────────────────────────

func (p *JenkinsPlugin) testConnection(ctx context.Context, c *JenkinsClient, _ []byte) ([]byte, error) {
	info, err := c.GetSystemInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("connection test failed: %w", err)
	}
	return actionOutput(map[string]interface{}{
		"connected": true,
		"version":   info.Version,
		"message":   fmt.Sprintf("Connected to Jenkins %s", info.Version),
	})
}

func (p *JenkinsPlugin) getSystemInfo(ctx context.Context, c *JenkinsClient, _ []byte) ([]byte, error) {
	info, err := c.GetSystemInfo(ctx)
	if err != nil {
		return nil, err
	}
	return actionOutput(info)
}
