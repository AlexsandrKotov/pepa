package pipeline

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// JenkinsPipelineConfig is the expected shape of PipelineSource.Config for jenkins sources.
type JenkinsPipelineConfig struct {
	URL      string `json:"url"`        // Jenkins URL
	Username string `json:"username"`   // Jenkins username
	Token    string `json:"api_token"`  // API token
	JobName  string `json:"job_name"`   // full job path (e.g. "folder/subfolder/my-pipeline")
	Insecure bool   `json:"insecure"`   // allow self-signed TLS
}

func parseJenkinsConfig(raw json.RawMessage) (*JenkinsPipelineConfig, error) {
	var cfg JenkinsPipelineConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("invalid jenkins config: %w", err)
	}
	if cfg.URL == "" {
		return nil, fmt.Errorf("jenkins config: url is required")
	}
	if cfg.JobName == "" {
		return nil, fmt.Errorf("jenkins config: job_name is required")
	}
	cfg.URL = strings.TrimRight(cfg.URL, "/")

	// Validate URL scheme to prevent SSRF via non-HTTP schemes.
	parsed, err := url.Parse(cfg.URL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("jenkins config: url must be http or https")
	}

	return &cfg, nil
}

// jenkinsHTTPClient creates an HTTP client for Jenkins API calls.
func jenkinsHTTPClient(insecure bool) *http.Client {
	transport := &http.Transport{}
	if insecure {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // admin opt-in
	}
	return &http.Client{Transport: transport, Timeout: 30 * time.Second}
}

// jenkinsJobPath converts "folder/subfolder/myjob" to "job/folder/job/subfolder/job/myjob".
func jenkinsJobPath(jobName string) string {
	if jobName == "" {
		return ""
	}
	parts := strings.Split(jobName, "/")
	segments := make([]string, 0, len(parts)*2)
	for _, p := range parts {
		if p == "" {
			continue
		}
		segments = append(segments, "job", url.PathEscape(p))
	}
	return strings.Join(segments, "/")
}

// jenkinsDo performs an authenticated HTTP request to Jenkins.
func jenkinsDo(ctx context.Context, client *http.Client, method, reqURL, username, token string, body io.Reader, contentType string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, reqURL, body)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(username, token)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return client.Do(req)
}

// jenkinsGetJSON performs a GET request and decodes JSON.
func jenkinsGetJSON(ctx context.Context, client *http.Client, reqURL, username, token string, result interface{}) error {
	resp, err := jenkinsDo(ctx, client, "GET", reqURL, username, token, nil, "")
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("jenkins API returned %d: %s", resp.StatusCode, string(b))
	}
	return json.NewDecoder(resp.Body).Decode(result)
}

// jenkinsMapStatus maps a Jenkins result string to PEPA status.
func jenkinsMapStatus(result *string, building bool) string {
	if building || result == nil {
		return "running"
	}
	switch *result {
	case "SUCCESS":
		return "success"
	case "FAILURE":
		return "failed"
	case "ABORTED":
		return "cancelled"
	case "UNSTABLE":
		return "warning"
	case "NOT_BUILT":
		return "skipped"
	default:
		return "unknown"
	}
}

// JenkinsAdapter implements pipeline.Provider for Jenkins CI/CD.
type JenkinsAdapter struct{}

// NewJenkinsAdapter creates a new Jenkins pipeline adapter.
func NewJenkinsAdapter() *JenkinsAdapter {
	return &JenkinsAdapter{}
}

func (a *JenkinsAdapter) Name() string { return "jenkins" }

// ResolveSchema fetches the Jenkins job's parameter definitions and builds a JSON Schema.
// This mirrors how GitLab CI auto-extracts variables from .gitlab-ci.yml.
func (a *JenkinsAdapter) ResolveSchema(ctx context.Context, raw json.RawMessage) (*ParameterSchema, error) {
	cfg, err := parseJenkinsConfig(raw)
	if err != nil {
		return nil, err
	}

	client := jenkinsHTTPClient(cfg.Insecure)
	props := make(map[string]PropertyDef)

	// Fetch job parameter definitions
	tree := "property[parameterDefinitions[name,type,description,defaultParameterValue[value],choices]]"
	apiURL := fmt.Sprintf("%s/%s/api/json?tree=%s", cfg.URL, jenkinsJobPath(cfg.JobName), url.QueryEscape(tree))

	var jobInfo struct {
		Property []struct {
			ParameterDefinitions []struct {
				Name                  string `json:"name"`
				Type                  string `json:"type"`
				Description           string `json:"description"`
				DefaultParameterValue struct {
					Value interface{} `json:"value"`
				} `json:"defaultParameterValue"`
				Choices []string `json:"choices"`
			} `json:"parameterDefinitions"`
		} `json:"property"`
	}

	if err := jenkinsGetJSON(ctx, client, apiURL, cfg.Username, cfg.Token, &jobInfo); err != nil {
		return nil, fmt.Errorf("fetch Jenkins job parameters: %w", err)
	}

	for _, prop := range jobInfo.Property {
		for _, pd := range prop.ParameterDefinitions {
			var schemaType string
			var enumVals []string

			switch pd.Type {
			case "BooleanParameterDefinition":
				schemaType = "boolean"
			case "ChoiceParameterDefinition":
				schemaType = "enum"
				enumVals = pd.Choices
			case "TextParameterDefinition":
				schemaType = "string"
			case "PasswordParameterDefinition":
				schemaType = "string"
			default:
				schemaType = "string"
			}

			defVal := ""
			if pd.DefaultParameterValue.Value != nil {
				defVal = fmt.Sprintf("%v", pd.DefaultParameterValue.Value)
			}

			props[pd.Name] = PropertyDef{
				Type:        schemaType,
				Description: pd.Description,
				Default:     defVal,
				Enum:        enumVals,
			}
		}
	}

	return &ParameterSchema{
		Type:       "object",
		Properties: props,
	}, nil
}

// Trigger starts a new Jenkins build.
func (a *JenkinsAdapter) Trigger(ctx context.Context, raw json.RawMessage, params map[string]any) (*TriggerResult, error) {
	cfg, err := parseJenkinsConfig(raw)
	if err != nil {
		return nil, err
	}

	client := jenkinsHTTPClient(cfg.Insecure)
	jobPath := jenkinsJobPath(cfg.JobName)

	// Separate build params from meta params
	buildParams := make(url.Values)
	for k, v := range params {
		if k == "" {
			continue
		}
		buildParams.Set(k, fmt.Sprintf("%v", v))
	}

	var apiURL string
	var body io.Reader
	var contentType string

	if len(buildParams) > 0 {
		apiURL = fmt.Sprintf("%s/%s/buildWithParameters", cfg.URL, jobPath)
		body = strings.NewReader(buildParams.Encode())
		contentType = "application/x-www-form-urlencoded"
	} else {
		apiURL = fmt.Sprintf("%s/%s/build", cfg.URL, jobPath)
		contentType = ""
	}

	resp, err := jenkinsDo(ctx, client, "POST", apiURL, cfg.Username, cfg.Token, body, contentType)
	if err != nil {
		return nil, fmt.Errorf("trigger jenkins build: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("trigger jenkins build (%d): %s", resp.StatusCode, string(b))
	}

	queueURL, err := url.Parse(resp.Header.Get("Location"))
	if err != nil || queueURL.Path == "" {
		return nil, fmt.Errorf("Jenkins accepted the build but did not return a queue location; check Jenkins before retrying")
	}
	parts := strings.Split(strings.Trim(queueURL.Path, "/"), "/")
	if len(parts) < 3 || parts[len(parts)-3] != "queue" || parts[len(parts)-2] != "item" {
		return nil, fmt.Errorf("Jenkins returned an invalid queue location")
	}
	queueID := parts[len(parts)-1]
	if _, err := strconv.ParseUint(queueID, 10, 64); err != nil {
		return nil, fmt.Errorf("Jenkins returned an invalid queue ID")
	}
	return &TriggerResult{
		ExternalRunID: "queue:" + queueID,
		ExternalURL:   fmt.Sprintf("%s/%s/", cfg.URL, jobPath),
		Status:        "pending",
	}, nil
}

// resolveJenkinsBuild follows queued builds without sending credentials to the
// server-provided Location URL. All requests stay on the configured Jenkins URL.
func resolveJenkinsBuild(ctx context.Context, cfg *JenkinsPipelineConfig, externalID string) (string, bool, error) {
	if !strings.HasPrefix(externalID, "queue:") {
		n, err := strconv.ParseUint(externalID, 10, 64)
		if err != nil || n == 0 {
			return "", false, fmt.Errorf("invalid Jenkins build number")
		}
		return externalID, false, nil
	}
	queueID := strings.TrimPrefix(externalID, "queue:")
	if _, err := strconv.ParseUint(queueID, 10, 64); err != nil {
		return "", false, fmt.Errorf("invalid Jenkins queue ID")
	}
	var item struct {
		Cancelled bool `json:"cancelled"`
		Executable *struct { Number int `json:"number"` } `json:"executable"`
	}
	err := jenkinsGetJSON(ctx, jenkinsHTTPClient(cfg.Insecure), cfg.URL+"/queue/item/"+queueID+"/api/json", cfg.Username, cfg.Token, &item)
	if err != nil {
		return "", false, err
	}
	if item.Executable != nil && item.Executable.Number > 0 {
		return strconv.Itoa(item.Executable.Number), false, nil
	}
	return "", item.Cancelled, nil
}

// Status returns the current status of a Jenkins build.
func (a *JenkinsAdapter) Status(ctx context.Context, raw json.RawMessage, externalRunID string) (*RunStatus, error) {
	cfg, err := parseJenkinsConfig(raw)
	if err != nil {
		return nil, err
	}

	buildID, cancelled, err := resolveJenkinsBuild(ctx, cfg, externalRunID)
	if err != nil {
		return nil, err
	}
	if buildID == "" {
		status := "pending"
		if cancelled { status = "cancelled" }
		return &RunStatus{ExternalRunID: externalRunID, Status: status}, nil
	}
	externalRunID = buildID
	client := jenkinsHTTPClient(cfg.Insecure)
	apiURL := fmt.Sprintf("%s/%s/%s/api/json", cfg.URL, jenkinsJobPath(cfg.JobName), externalRunID)

	var build struct {
		Number    int     `json:"number"`
		Result    *string `json:"result"`
		Building  bool    `json:"building"`
		Duration  int64   `json:"duration"`
		Timestamp int64   `json:"timestamp"`
		URL       string  `json:"url"`
	}

	if err := jenkinsGetJSON(ctx, client, apiURL, cfg.Username, cfg.Token, &build); err != nil {
		return nil, err
	}

	durMs := int(build.Duration)
	return &RunStatus{
		ExternalRunID: externalRunID,
		Status:        jenkinsMapStatus(build.Result, build.Building),
		ExternalURL:   build.URL,
		DurationMs:    &durMs,
	}, nil
}

// Jobs returns the stages/steps of a Jenkins build.
func (a *JenkinsAdapter) Jobs(ctx context.Context, raw json.RawMessage, externalRunID string) ([]JobInfo, error) {
	cfg, err := parseJenkinsConfig(raw)
	if err != nil {
		return nil, err
	}

	buildID, _, err := resolveJenkinsBuild(ctx, cfg, externalRunID)
	if err != nil { return nil, err }
	if buildID == "" { return []JobInfo{}, nil }
	externalRunID = buildID
	client := jenkinsHTTPClient(cfg.Insecure)

	// Try Pipeline Stage View API first
	stageURL := fmt.Sprintf("%s/%s/%s/wfapi/describe", cfg.URL, jenkinsJobPath(cfg.JobName), externalRunID)
	var stageView struct {
		Stages []struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			Status     string `json:"status"`
			DurationMs int64  `json:"durationMillis"`
		} `json:"stages"`
	}

	if err := jenkinsGetJSON(ctx, client, stageURL, cfg.Username, cfg.Token, &stageView); err == nil && len(stageView.Stages) > 0 {
		jobs := make([]JobInfo, 0, len(stageView.Stages))
		for _, s := range stageView.Stages {
			status := strings.ToLower(s.Status)
			jobs = append(jobs, JobInfo{
				ExternalJobID: s.ID,
				Name:          s.Name,
				Status:        status,
			})
		}
		return jobs, nil
	}

	// Fallback: fetch build details and derive status
	apiURL := fmt.Sprintf("%s/%s/%s/api/json", cfg.URL, jenkinsJobPath(cfg.JobName), externalRunID)
	var build struct {
		Result   *string `json:"result"`
		Building bool    `json:"building"`
		URL      string  `json:"url"`
	}

	if err := jenkinsGetJSON(ctx, client, apiURL, cfg.Username, cfg.Token, &build); err != nil {
		return nil, err
	}

	status := jenkinsMapStatus(build.Result, build.Building)

	// Return a single job representing the build
	return []JobInfo{
		{
			ExternalJobID: externalRunID,
			Name:          cfg.JobName,
			Status:        status,
			LogURL:        build.URL,
		},
	}, nil
}

// Logs fetches the console output of a Jenkins build.
func (a *JenkinsAdapter) Logs(ctx context.Context, raw json.RawMessage, externalRunID string, jobID string) (string, error) {
	cfg, err := parseJenkinsConfig(raw)
	if err != nil {
		return "", err
	}

	buildID, _, err := resolveJenkinsBuild(ctx, cfg, externalRunID)
	if err != nil { return "", err }
	if buildID == "" { return "", nil }
	externalRunID = buildID
	client := jenkinsHTTPClient(cfg.Insecure)
	logURL := fmt.Sprintf("%s/%s/%s/consoleText", cfg.URL, jenkinsJobPath(cfg.JobName), externalRunID)

	resp, err := jenkinsDo(ctx, client, "GET", logURL, cfg.Username, cfg.Token, nil, "")
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("jenkins logs returned %d: %s", resp.StatusCode, string(b))
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Cancel aborts a running Jenkins build or removes a queued item.
func (a *JenkinsAdapter) Cancel(ctx context.Context, raw json.RawMessage, externalRunID string) error {
	cfg, err := parseJenkinsConfig(raw)
	if err != nil {
		return err
	}

	client := jenkinsHTTPClient(cfg.Insecure)
	var stopURL string
	if strings.HasPrefix(externalRunID, "queue:") {
		queueID := strings.TrimPrefix(externalRunID, "queue:")
		// If the queue item already has a build number, stop that build instead.
		buildID, _, resolveErr := resolveJenkinsBuild(ctx, cfg, externalRunID)
		if resolveErr != nil {
			return resolveErr
		}
		if buildID != "" {
			stopURL = fmt.Sprintf("%s/%s/%s/stop", cfg.URL, jenkinsJobPath(cfg.JobName), buildID)
		} else {
			stopURL = cfg.URL + "/queue/cancelItem?id=" + queueID
		}
	} else {
		stopURL = fmt.Sprintf("%s/%s/%s/stop", cfg.URL, jenkinsJobPath(cfg.JobName), externalRunID)
	}

	resp, err := jenkinsDo(ctx, client, "POST", stopURL, cfg.Username, cfg.Token, nil, "")
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("jenkins cancel returned %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// ListRemoteRuns fetches recent builds from Jenkins.
func (a *JenkinsAdapter) ListRemoteRuns(ctx context.Context, raw json.RawMessage, perPage int) ([]RunStatus, error) {
	cfg, err := parseJenkinsConfig(raw)
	if err != nil {
		return nil, err
	}
	if perPage < 1 || perPage > 100 {
		perPage = 30
	}

	client := jenkinsHTTPClient(cfg.Insecure)
	tree := fmt.Sprintf("builds[number,result,timestamp,duration,url,building]{0,%d}", perPage-1)
	apiURL := fmt.Sprintf("%s/%s/api/json?tree=%s", cfg.URL, jenkinsJobPath(cfg.JobName), url.QueryEscape(tree))

	var result struct {
		Builds []struct {
			Number    int     `json:"number"`
			Result    *string `json:"result"`
			Building  bool    `json:"building"`
			Timestamp int64   `json:"timestamp"`
			Duration  int64   `json:"duration"`
			URL       string  `json:"url"`
		} `json:"builds"`
	}

	if err := jenkinsGetJSON(ctx, client, apiURL, cfg.Username, cfg.Token, &result); err != nil {
		return nil, err
	}

	runs := make([]RunStatus, 0, len(result.Builds))
	for _, b := range result.Builds {
		durMs := int(b.Duration)
		runs = append(runs, RunStatus{
			ExternalRunID: fmt.Sprintf("%d", b.Number),
			Status:        jenkinsMapStatus(b.Result, b.Building),
			ExternalURL:   b.URL,
			DurationMs:    &durMs,
		})
	}
	return runs, nil
}

// GetWorkflowGraph fetches the Pipeline stage view and returns a WorkflowGraph.
func (a *JenkinsAdapter) GetWorkflowGraph(ctx context.Context, raw json.RawMessage) (*WorkflowGraph, error) {
	cfg, err := parseJenkinsConfig(raw)
	if err != nil {
		return nil, err
	}

	client := jenkinsHTTPClient(cfg.Insecure)
	stageURL := fmt.Sprintf("%s/%s/lastBuild/wfapi/describe", cfg.URL, jenkinsJobPath(cfg.JobName))

	var stageView struct {
		Name   string `json:"name"`
		Stages []struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			Status     string `json:"status"`
			DurationMs int64  `json:"durationMillis"`
		} `json:"stages"`
	}

	if err := jenkinsGetJSON(ctx, client, stageURL, cfg.Username, cfg.Token, &stageView); err != nil {
		return nil, fmt.Errorf("fetch pipeline stages: %w", err)
	}

	graph := &WorkflowGraph{
		Name:   stageView.Name,
		Source: "jenkins",
	}

	// Build stages and jobs from the stage view
	stageSet := make(map[string]bool)
	for i, s := range stageView.Stages {
		stageName := fmt.Sprintf("Stage %d", i+1)
		if s.Name != "" {
			stageName = s.Name
		}
		if !stageSet[stageName] {
			graph.Stages = append(graph.Stages, StageInfo{
				Name:  stageName,
				Order: i,
			})
			stageSet[stageName] = true
		}
		graph.Jobs = append(graph.Jobs, JobNode{
			Name:  s.Name,
			Stage: stageName,
		})
	}

	return graph, nil
}

// Inspect returns structured metadata about the Jenkins job.
func (a *JenkinsAdapter) Inspect(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	cfg, err := parseJenkinsConfig(raw)
	if err != nil {
		return nil, err
	}

	client := jenkinsHTTPClient(cfg.Insecure)
	apiURL := fmt.Sprintf("%s/%s/api/json?tree=_class,name,description,url,buildable,disabled,property[parameterDefinitions[name,type,description,defaultParameterValue[value],choices]]", cfg.URL, jenkinsJobPath(cfg.JobName))

	var jobInfo json.RawMessage
	if err := jenkinsGetJSON(ctx, client, apiURL, cfg.Username, cfg.Token, &jobInfo); err != nil {
		return nil, err
	}

	// Wrap with metadata
	result := map[string]interface{}{
		"job_type":  "jenkins_pipeline",
		"job_name":  cfg.JobName,
		"job_url":   fmt.Sprintf("%s/%s/", cfg.URL, jenkinsJobPath(cfg.JobName)),
		"details":   jobInfo,
	}

	return json.Marshal(result)
}
