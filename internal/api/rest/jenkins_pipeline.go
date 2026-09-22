package rest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/pipeline"
	"github.com/pepa/pepa/internal/repository"
	"github.com/pepa/pepa/pkg/models"
)

// Jenkins credentials and endpoint come from its linked connection, never from
// public source config. Apply the caller's credential fallback and Vault policy.
func resolvePipelineRequestConfig(c *gin.Context, deps Dependencies, source *models.PipelineSource) (json.RawMessage, bool) {
	if source.SourceType != "jenkins" {
		return resolvePipelineConfig(c.Request.Context(), deps, source, auth.GetTenantID(c)), true
	}
	if deps.ProviderRegistry == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Jenkins plugin is unavailable"})
		return nil, false
	}
	if _, ok := deps.ProviderRegistry.GetEnabled("jenkins"); !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Install and enable the Jenkins plugin in Marketplace"})
		return nil, false
	}
	if source.ConnectionID == nil || deps.Repos == nil || deps.Repos.Connection == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "A Jenkins connection is required"})
		return nil, false
	}
	conn, err := deps.Repos.Connection.GetDecrypted(c.Request.Context(), *source.ConnectionID, auth.GetTenantID(c))
	if err != nil || conn.Type != repository.ConnectionJenkins {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Linked Jenkins connection is unavailable"})
		return nil, false
	}
	resolved, err := ResolveConnectionCredential(c.Request.Context(), deps, conn, auth.GetUserID(c), "jenkins", "url")
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return nil, false
	}
	conn.Config = make(map[string]any, len(resolved.Config))
	for key, value := range resolved.Config {
		conn.Config[key] = value
	}
	config, err := resolveConnectionTestConfig(deps, c, c.Request.Context(), conn)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return nil, false
	}
	var sourceConfig struct {
		JobName string `json:"job_name"`
	}
	if json.Unmarshal(source.Config, &sourceConfig) != nil || sourceConfig.JobName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Jenkins job_name is required"})
		return nil, false
	}
	token, _ := config["api_token"].(string)
	if token == "" {
		token, _ = config["token"].(string)
	}
	endpoint, _ := config["url"].(string)
	username, _ := config["username"].(string)
	insecure, _ := strconv.ParseBool(fmt.Sprint(config["insecure"]))
	// The token is forwarded in-memory to the Jenkins pipeline adapter so it
	// can authenticate to the configured Jenkins server. It originates from
	// the encrypted connection config (or Vault) and is never persisted or
	// returned to the caller. gosec G117 is suppressed intentionally.
	raw, err := json.Marshal(pipeline.JenkinsPipelineConfig{ // #nosec G117 //nolint:gosec // in-memory transfer to pipeline adapter; token is not persisted or returned
		URL: endpoint, Username: username, Token: token, Insecure: insecure, JobName: sourceConfig.JobName,
	})
	if err != nil {
		respondInternalError(c, err)
		return nil, false
	}
	return raw, true
}
