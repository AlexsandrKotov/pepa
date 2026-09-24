package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// xmlVersionRe normalises XML 1.1 declarations to 1.0 so that Go's xml
// package (which only supports 1.0) can parse Jenkins config.xml files.
var xmlVersionRe = regexp.MustCompile(`<\?xml[^>]*version=['"]1\.1['"]`)

// JenkinsClient is an HTTP client for the Jenkins REST API.
type JenkinsClient struct {
	baseURL    string
	username   string
	token      string
	httpClient *http.Client

	// CSRF crumb cache
	crumbMu    sync.Mutex
	crumbField string
	crumbValue string
	crumbExp   time.Time
}

// NewJenkinsClient creates a new Jenkins API client.
// The baseURL must be an admin-configured connection URL (trusted source).
func NewJenkinsClient(baseURL, username, token string, insecure bool) (*JenkinsClient, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("jenkins: baseURL is required")
	}
	baseURL = strings.TrimRight(baseURL, "/")

	// Validate URL scheme to prevent SSRF via non-HTTP schemes.
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("jenkins: baseURL must be http or https")
	}

	transport := &http.Transport{}
	if insecure {
		// Dev/self-signed certs only — never enable in production.
		// The insecure flag is set by the admin in the connection config.
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // admin opt-in for self-signed
	}

	return &JenkinsClient{
		baseURL:  baseURL,
		username: username,
		token:    token,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
	}, nil
}

// jobPath converts a job name like "folder/subfolder/myjob" to the Jenkins URL
// path segment "job/folder/job/subfolder/job/myjob".
func jobPath(jobName string) string {
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

// crumb fetches and caches the CSRF crumb from Jenkins.
func (c *JenkinsClient) crumb(ctx context.Context) (field, value string, err error) {
	c.crumbMu.Lock()
	defer c.crumbMu.Unlock()

	if c.crumbValue != "" && time.Now().Before(c.crumbExp) {
		return c.crumbField, c.crumbValue, nil
	}

	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/crumbIssuer/api/json", nil)
	if err != nil {
		return "", "", err
	}
	req.SetBasicAuth(c.username, c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("fetch crumb: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		// CSRF protection is disabled on this Jenkins instance
		return "", "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("crumb issuer returned %d", resp.StatusCode)
	}

	var result struct {
		CrumbRequestField string `json:"crumbRequestField"`
		Crumb             string `json:"crumb"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", fmt.Errorf("parse crumb: %w", err)
	}

	c.crumbField = result.CrumbRequestField
	c.crumbValue = result.Crumb
	c.crumbExp = time.Now().Add(5 * time.Minute)

	return c.crumbField, c.crumbValue, nil
}

// do executes an HTTP request with authentication.
func (c *JenkinsClient) do(ctx context.Context, method, path string, body io.Reader, contentType string) (*http.Response, error) {
	reqURL := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, reqURL, body)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.username, c.token)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	// Add CSRF crumb for mutating requests
	if method == "POST" || method == "PUT" || method == "DELETE" {
		field, value, crumbErr := c.crumb(ctx)
		if crumbErr == nil && field != "" && value != "" {
			req.Header.Set(field, value)
		}
	}

	return c.httpClient.Do(req)
}

// getJSON performs a GET request and decodes the JSON response.
func (c *JenkinsClient) getJSON(ctx context.Context, path string, result interface{}) error {
	resp, err := c.do(ctx, "GET", path, nil, "")
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("jenkins API %s returned %d: %s", path, resp.StatusCode, string(body))
	}

	return json.NewDecoder(resp.Body).Decode(result)
}

// getText performs a GET request and returns the response body as a string.
func (c *JenkinsClient) getText(ctx context.Context, path string) (string, error) {
	resp, err := c.do(ctx, "GET", path, nil, "")
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("jenkins API %s returned %d: %s", path, resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// getTextProgressive performs a GET request for progressive text output.
func (c *JenkinsClient) getTextProgressive(ctx context.Context, path string) (string, bool, int64, error) {
	reqURL := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return "", false, 0, err
	}
	req.SetBasicAuth(c.username, c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", false, 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", false, 0, fmt.Errorf("jenkins progressive text %s returned %d: %s", path, resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", false, 0, err
	}

	hasMore := resp.Header.Get("X-More-Data") == "true"

	var newSize int64
	if ts := resp.Header.Get("X-Text-Size"); ts != "" {
		if v, err := strconv.ParseInt(ts, 10, 64); err == nil {
			newSize = v
		}
	}

	return string(body), hasMore, newSize, nil
}

// postForm performs a POST request with form-encoded data.
func (c *JenkinsClient) postForm(ctx context.Context, path string, data url.Values) error {
	resp, err := c.do(ctx, "POST", path, strings.NewReader(data.Encode()), "application/x-www-form-urlencoded")
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("jenkins POST %s returned %d: %s", path, resp.StatusCode, string(body))
	}
	return nil
}

// postXML performs a POST request with XML body.
func (c *JenkinsClient) postXML(ctx context.Context, path string, xmlBody string) error {
	resp, err := c.do(ctx, "POST", path, strings.NewReader(xmlBody), "application/xml; charset=utf-8")
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("jenkins POST %s returned %d: %s", path, resp.StatusCode, string(body))
	}
	return nil
}

// postRaw performs a POST request with raw body and content type.
func (c *JenkinsClient) postRaw(ctx context.Context, path string, contentType string, body io.Reader) error {
	resp, err := c.do(ctx, "POST", path, body, contentType)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("jenkins POST %s returned %d: %s", path, resp.StatusCode, string(respBody))
	}
	return nil
}

// ── API Methods ──────────────────────────────────────────────

// ListJobsResponse is the JSON response for listing jobs.
type ListJobsResponse struct {
	Jobs []JenkinsJob `json:"jobs"`
}

// JenkinsJob represents a Jenkins job.
type JenkinsJob struct {
	Class  string `json:"_class"`
	Name   string `json:"name"`
	URL    string `json:"url"`
	Color  string `json:"color"`
	Health []struct {
		Score int `json:"score"`
	} `json:"healthReport,omitempty"`
	LastBuild    *JenkinsBuildRef `json:"lastBuild,omitempty"`
	LastBuildRef *JenkinsBuildRef `json:"lastSuccessfulBuild,omitempty"`
}

// JenkinsBuildRef is a lightweight build reference.
type JenkinsBuildRef struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
}

// ListJobs lists jobs at the root or within a folder.
func (c *JenkinsClient) ListJobs(ctx context.Context, folder string, start, limit int) (*ListJobsResponse, error) {
	tree := fmt.Sprintf("jobs[_class,name,url,color,healthReport[score],lastBuild[number,url],lastSuccessfulBuild[number,url]]{%d,%d}", start, start+limit-1)
	path := "/api/json?tree=" + url.QueryEscape(tree)
	if folder != "" {
		path = "/" + jobPath(folder) + "/api/json?tree=" + url.QueryEscape(tree)
	}

	var result struct {
		Jobs []JenkinsJob `json:"jobs"`
	}
	if err := c.getJSON(ctx, path, &result); err != nil {
		return nil, err
	}
	return &ListJobsResponse{Jobs: result.Jobs}, nil
}

// JenkinsBuild represents a Jenkins build.
type JenkinsBuild struct {
	Class       string            `json:"_class"`
	Number      int               `json:"number"`
	URL         string            `json:"url"`
	Result      *string           `json:"result"` // null if still building
	Building    bool              `json:"building"`
	Duration    int64             `json:"duration"`
	Timestamp   int64             `json:"timestamp"`
	DisplayName string            `json:"displayName"`
	Description string            `json:"description,omitempty"`
	Actions     []json.RawMessage `json:"actions"`
	ChangeSets  []json.RawMessage `json:"changeSets,omitempty"`
}

// GetBuild fetches build details.
func (c *JenkinsClient) GetBuild(ctx context.Context, jobName string, number int) (*JenkinsBuild, error) {
	path := "/" + jobPath(jobName) + fmt.Sprintf("/%d/api/json", number)
	var build JenkinsBuild
	if err := c.getJSON(ctx, path, &build); err != nil {
		return nil, err
	}
	return &build, nil
}

// ListBuilds lists builds for a job.
func (c *JenkinsClient) ListBuilds(ctx context.Context, jobName string, start, limit int) ([]JenkinsBuild, error) {
	tree := fmt.Sprintf("builds[number,url,result,building,duration,timestamp,displayName]{%d,%d}", start, start+limit-1)
	path := "/" + jobPath(jobName) + "/api/json?tree=" + url.QueryEscape(tree)

	var result struct {
		Builds []JenkinsBuild `json:"builds"`
	}
	if err := c.getJSON(ctx, path, &result); err != nil {
		return nil, err
	}
	return result.Builds, nil
}

// TriggerBuild triggers a build without parameters.
func (c *JenkinsClient) TriggerBuild(ctx context.Context, jobName string) error {
	path := "/" + jobPath(jobName) + "/build"
	return c.postForm(ctx, path, nil)
}

// TriggerBuildWithParams triggers a build with parameters.
func (c *JenkinsClient) TriggerBuildWithParams(ctx context.Context, jobName string, params map[string]string) error {
	path := "/" + jobPath(jobName) + "/buildWithParameters"
	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}
	return c.postForm(ctx, path, form)
}

// StopBuild aborts a running build.
func (c *JenkinsClient) StopBuild(ctx context.Context, jobName string, number int) error {
	path := "/" + jobPath(jobName) + fmt.Sprintf("/%d/stop", number)
	return c.postForm(ctx, path, nil)
}

// Rebuild triggers a rebuild of a previous build.
func (c *JenkinsClient) Rebuild(ctx context.Context, jobName string, number int) error {
	path := "/" + jobPath(jobName) + fmt.Sprintf("/%d/rebuild", number)
	return c.postForm(ctx, path, nil)
}

// GetBuildLog fetches the console output of a build.
func (c *JenkinsClient) GetBuildLog(ctx context.Context, jobName string, number int) (string, error) {
	path := "/" + jobPath(jobName) + fmt.Sprintf("/%d/consoleText", number)
	return c.getText(ctx, path)
}

// GetBuildLogProgressive fetches progressive console output.
func (c *JenkinsClient) GetBuildLogProgressive(ctx context.Context, jobName string, number int, start int64) (string, bool, int64, error) {
	path := "/" + jobPath(jobName) + fmt.Sprintf("/%d/progressiveText?start=%d", number, start)
	return c.getTextProgressive(ctx, path)
}

// GetBuildArtifacts lists artifacts of a build.
func (c *JenkinsClient) GetBuildArtifacts(ctx context.Context, jobName string, number int) ([]JenkinsArtifact, error) {
	path := "/" + jobPath(jobName) + fmt.Sprintf("/%d/api/json?tree=artifacts[fileName,relativePath,displayPath]", number)
	var result struct {
		Artifacts []JenkinsArtifact `json:"artifacts"`
	}
	if err := c.getJSON(ctx, path, &result); err != nil {
		return nil, err
	}
	return result.Artifacts, nil
}

// JenkinsArtifact represents a build artifact.
type JenkinsArtifact struct {
	FileName     string `json:"fileName"`
	RelativePath string `json:"relativePath"`
	DisplayPath  string `json:"displayPath,omitempty"`
}

// GetBuildChanges fetches SCM changes for a build.
func (c *JenkinsClient) GetBuildChanges(ctx context.Context, jobName string, number int) (json.RawMessage, error) {
	path := "/" + jobPath(jobName) + fmt.Sprintf("/%d/api/json?tree=changeSets[items[commitId,author[fullName],timestamp,msg,affectedPaths]]", number)
	var result struct {
		ChangeSets json.RawMessage `json:"changeSets"`
	}
	if err := c.getJSON(ctx, path, &result); err != nil {
		return nil, err
	}
	return result.ChangeSets, nil
}

// GetBuildTestResults fetches test results for a build.
func (c *JenkinsClient) GetBuildTestResults(ctx context.Context, jobName string, number int) (json.RawMessage, error) {
	path := "/" + jobPath(jobName) + fmt.Sprintf("/%d/testReport/api/json", number)
	var result json.RawMessage
	if err := c.getJSON(ctx, path, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetPipelineStages fetches Pipeline stage view.
func (c *JenkinsClient) GetPipelineStages(ctx context.Context, jobName string, number int) (json.RawMessage, error) {
	path := "/" + jobPath(jobName) + fmt.Sprintf("/%d/wfapi/describe", number)
	var result json.RawMessage
	if err := c.getJSON(ctx, path, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetJobConfigXML fetches the raw config.xml of a job.
func (c *JenkinsClient) GetJobConfigXML(ctx context.Context, jobName string) (string, error) {
	path := "/" + jobPath(jobName) + "/config.xml"
	return c.getText(ctx, path)
}

// UpdateJobConfigXML updates the config.xml of a job.
func (c *JenkinsClient) UpdateJobConfigXML(ctx context.Context, jobName string, xmlBody string) error {
	path := "/" + jobPath(jobName) + "/config.xml"
	return c.postXML(ctx, path, xmlBody)
}

// CreateJob creates a new Jenkins job.
func (c *JenkinsClient) CreateJob(ctx context.Context, name string, xmlBody string) error {
	path := "/createItem?name=" + url.QueryEscape(name)
	return c.postRaw(ctx, path, "application/xml; charset=utf-8", strings.NewReader(xmlBody))
}

// CreateJobInFolder creates a new Jenkins job inside a folder.
func (c *JenkinsClient) CreateJobInFolder(ctx context.Context, name string, folder string, xmlBody string) error {
	path := "/" + jobPath(folder) + "/createItem?name=" + url.QueryEscape(name)
	return c.postRaw(ctx, path, "application/xml; charset=utf-8", strings.NewReader(xmlBody))
}

// DeleteJob deletes a Jenkins job.
func (c *JenkinsClient) DeleteJob(ctx context.Context, jobName string) error {
	path := "/" + jobPath(jobName) + "/doDelete"
	return c.postForm(ctx, path, nil)
}

// EnableJob enables a disabled job.
func (c *JenkinsClient) EnableJob(ctx context.Context, jobName string) error {
	path := "/" + jobPath(jobName) + "/enable"
	return c.postForm(ctx, path, nil)
}

// DisableJob disables a job.
func (c *JenkinsClient) DisableJob(ctx context.Context, jobName string) error {
	path := "/" + jobPath(jobName) + "/disable"
	return c.postForm(ctx, path, nil)
}

// GetJobDetails fetches full job details.
func (c *JenkinsClient) GetJobDetails(ctx context.Context, jobName string) (json.RawMessage, error) {
	path := "/" + jobPath(jobName) + "/api/json?tree=_class,name,description,url,color,buildable,disabled,healthReport[score,description],lastBuild[number,url,result,building,timestamp,duration],property[parameterDefinitions[name,type,description,defaultParameterValue[value],choices]],"
	var result json.RawMessage
	if err := c.getJSON(ctx, path, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ListViews lists Jenkins views.
func (c *JenkinsClient) ListViews(ctx context.Context) (json.RawMessage, error) {
	path := "/api/json?tree=views[_class,name,url,description]"
	var result json.RawMessage
	if err := c.getJSON(ctx, path, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ListNodes lists Jenkins agent nodes.
func (c *JenkinsClient) ListNodes(ctx context.Context) (json.RawMessage, error) {
	path := "/computer/api/json?tree=computer[displayName,description,offline,idle,executors[busy],numExecutors]"
	var result json.RawMessage
	if err := c.getJSON(ctx, path, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetNode fetches details of a specific agent node.
func (c *JenkinsClient) GetNode(ctx context.Context, name string) (json.RawMessage, error) {
	path := "/computer/" + url.PathEscape(name) + "/api/json"
	var result json.RawMessage
	if err := c.getJSON(ctx, path, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetQueue lists queued builds.
func (c *JenkinsClient) GetQueue(ctx context.Context) (json.RawMessage, error) {
	path := "/queue/api/json?tree=items[task[name,url],why,inQueueSince,buildable]"
	var result json.RawMessage
	if err := c.getJSON(ctx, path, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ListCredentials lists Jenkins credential IDs.
func (c *JenkinsClient) ListCredentials(ctx context.Context) (json.RawMessage, error) {
	path := "/credentials/store/system/domain/_/api/json?tree=credentials[id,displayName,typeName,description]"
	var result json.RawMessage
	if err := c.getJSON(ctx, path, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetSystemInfo fetches Jenkins system info.
func (c *JenkinsClient) GetSystemInfo(ctx context.Context) (*JenkinsSystemInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/json", nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.username, c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("jenkins system info returned %d: %s", resp.StatusCode, string(body))
	}

	info := &JenkinsSystemInfo{
		Version: resp.Header.Get("X-Jenkins"),
	}

	var result json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&result); err == nil {
		info.RawInfo = result
	}

	return info, nil
}

// JenkinsSystemInfo holds Jenkins system information.
type JenkinsSystemInfo struct {
	Version string          `json:"version"`
	RawInfo json.RawMessage `json:"info,omitempty"`
}

// parseJobConfigXML extracts the Groovy script from a Jenkins Pipeline job config.xml.
func parseJobConfigXML(xmlContent string) (script string, pipelineType string, sandbox bool, err error) {
	// Jenkins may emit XML 1.1 declarations; Go's xml package only supports 1.0.
	sanitized := xmlVersionRe.ReplaceAllString(xmlContent, `<?xml version='1.0'`)
	decoder := xml.NewDecoder(strings.NewReader(sanitized))
	var inScript, inDefinition bool
	var scriptBuilder strings.Builder

	for {
		tok, tokErr := decoder.Token()
		if tokErr == io.EOF {
			break
		}
		if tokErr != nil {
			return "", "", false, fmt.Errorf("parse config.xml: %w", tokErr)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "definition":
				inDefinition = true
				for _, attr := range t.Attr {
					if attr.Name.Local == "plugin" {
						if strings.Contains(attr.Value, "workflow-cps") {
							pipelineType = "cps"
						}
					}
				}
			case "script":
				if inDefinition {
					inScript = true
				}
			case "sandbox":
				// next char data will tell us the value
			case "scm":
				if inDefinition {
					pipelineType = "cps-scm"
				}
			}
		case xml.EndElement:
			if t.Name.Local == "definition" {
				inDefinition = false
			}
			if t.Name.Local == "script" && inScript {
				inScript = false
			}
		case xml.CharData:
			if inScript {
				scriptBuilder.Write(t)
			}
		}
	}

	return scriptBuilder.String(), pipelineType, false, nil
}
