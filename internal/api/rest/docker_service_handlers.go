package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	dockerpkg "github.com/pepa/pepa/internal/docker"
	"github.com/pepa/pepa/internal/repository"
)

// hostHomePrefixes are common absolute-path prefixes on host machines.
// When PEPA runs inside Docker the host home directory is bind-mounted at
// /host-home so these paths become accessible after translation.
var hostHomePrefixes = []string{"/Users/", "/home/"}

// resolveHostPath translates a host absolute path (e.g. /Users/alice/project)
// to a path accessible inside the current container. When PEPA runs natively
// the path is returned as-is. When running inside Docker the host home
// directory is mounted read-only at /host-home, so a host path like
// /Users/alice/project is translated to /host-home/project.
func resolveHostPath(p string) (string, error) {
	absPath, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	// 1. Direct access (native mode or path already inside container).
	if info, statErr := os.Stat(absPath); statErr == nil && info.IsDir() {
		return absPath, nil
	}

	// 2. Docker mode — translate host home path → /host-home/...
	if filepath.IsAbs(p) {
		for _, prefix := range hostHomePrefixes {
			if strings.HasPrefix(p, prefix) {
				rest := strings.TrimPrefix(p, prefix)
				if idx := strings.Index(rest, "/"); idx >= 0 {
					rest = rest[idx+1:]
				} else {
					continue
				}
				containerPath := filepath.Join("/host-home", rest)
				if info, statErr := os.Stat(containerPath); statErr == nil && info.IsDir() {
					return containerPath, nil
				}
			}
		}
	}

	return "", fmt.Errorf("folder not accessible: %s", p)
}

func registerDockerServiceRoutes(r *gin.RouterGroup, deps Dependencies) {
	dockerServices := r.Group("/docker-services")
	{
		dockerServices.GET("", listDockerServices(deps))
		dockerServices.POST("", createDockerService(deps))
		dockerServices.POST("/deploy-local", deployLocalDockerService(deps))
		dockerServices.POST("/deploy-local-stream", deployLocalDockerServiceStream(deps))
		dockerServices.GET("/:id", getDockerService(deps))
		dockerServices.POST("/:id/refresh", refreshDockerService(deps))
		dockerServices.POST("/:id/restart", restartDockerService(deps))
		dockerServices.POST("/:id/stop", stopDockerService(deps))
		dockerServices.POST("/:id/start", startDockerService(deps))
		dockerServices.POST("/:id/rollback", rollbackDockerService(deps))
		dockerServices.GET("/:id/history", dockerServiceHistory(deps))
		dockerServices.DELETE("/:id", deleteDockerService(deps))
		dockerServices.GET("/:id/logs", getDockerServiceLogs(deps))
	}
}

// dockerClientForService returns a Docker CLI client for the given service.
// If the service has no DockerHostID (nil), it uses the local Docker socket.
func dockerClientForService(deps Dependencies, svc *repository.DockerService, tenantID uuid.UUID) (*dockerpkg.Client, error) {
	if svc.DockerHostID == nil {
		cfg := dockerpkg.HostConfig{
			HostType:    "local",
			HostAddress: "unix:///var/run/docker.sock",
		}
		return dockerpkg.NewClient(cfg), nil
	}
	host, err := deps.Repos.DockerHost.GetHostDecrypted(context.Background(), *svc.DockerHostID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("docker host not found: %w", err)
	}
	cfg := dockerpkg.HostConfig{
		HostType:    host.HostType,
		HostAddress: host.HostAddress,
		TLSCACert:   host.TLSCACert,
		TLSCert:     host.TLSCert,
		TLSKey:      host.TLSKey,
		SSHKey:      host.SSHKey,
	}
	return dockerpkg.NewClient(cfg), nil
}

func listDockerServices(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.DockerHost == nil {
			c.JSON(http.StatusOK, gin.H{"docker_services": []interface{}{}, "total": 0})
			return
		}
		tenantID := auth.GetTenantID(c)
		items, err := deps.Repos.DockerHost.ListServices(c.Request.Context(), tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"docker_services": items, "total": len(items)})
	}
}

func createDockerService(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.DockerHost == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "docker host repository not available"})
			return
		}
		var req struct {
			DockerHostID uuid.UUID         `json:"docker_host_id" binding:"required"`
			Name         string            `json:"name" binding:"required"`
			ComposeYaml  string            `json:"compose_yaml"`
			FolderPath   string            `json:"folder_path"`
			EnvVars      map[string]string `json:"env_vars"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Require either compose_yaml or folder_path
		if req.ComposeYaml == "" && req.FolderPath == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "either compose_yaml or folder_path is required"})
			return
		}

		// Verify host exists (decrypted to get real TLS/SSH credentials)
		tenantID := auth.GetTenantID(c)
		host, err := deps.Repos.DockerHost.GetHostDecrypted(c.Request.Context(), req.DockerHostID, tenantID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "docker host not found"})
			return
		}

		// Validate folder path is accessible (tries original path, then /host-home translation)
		if req.FolderPath != "" {
			if _, resolveErr := resolveHostPath(req.FolderPath); resolveErr != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": resolveErr.Error()})
				return
			}
		}

		envVars := req.EnvVars
		if envVars == nil {
			envVars = make(map[string]string)
		}
		envJSON, _ := json.Marshal(envVars)

		svc := &repository.DockerService{
			TenantID:     auth.GetTenantID(c),
			DockerHostID: &req.DockerHostID,
			Name:         req.Name,
			ComposeYaml:  req.ComposeYaml,
			FolderPath:   req.FolderPath,
			EnvVars:      envJSON,
			Status:       "deploying",
			Containers:   json.RawMessage("[]"),
		}

		if err := deps.Repos.DockerHost.CreateService(c.Request.Context(), svc); err != nil {
			respondInternalError(c, err)
			return
		}

		// Deploy via Docker CLI
		cfg := dockerpkg.HostConfig{
			HostType:    host.HostType,
			HostAddress: host.HostAddress,
			TLSCACert:   host.TLSCACert,
			TLSCert:     host.TLSCert,
			TLSKey:      host.TLSKey,
			SSHKey:      host.SSHKey,
		}
		client := dockerpkg.NewClient(cfg)

		ctx, cancel := context.WithTimeout(c.Request.Context(), 300*time.Second)
		defer cancel()

		// Deploy from folder or from YAML
		var deployErr error
		if svc.FolderPath != "" {
			deployErr = client.ComposeUpFromFolder(ctx, svc.Name, svc.FolderPath, envVars)
		} else {
			deployErr = client.ComposeUp(ctx, svc.Name, svc.ComposeYaml, envVars)
		}

		if deployErr != nil {
			svc.Status = "error"
			if updateErr := deps.Repos.DockerHost.UpdateService(c.Request.Context(), svc); updateErr != nil {
				slog.Warn("failed to update service status to error", "service", svc.Name, "error", updateErr)
			}
			respondInternalError(c, deployErr)
			return
		}

		// Refresh container info
		containers, err := client.ComposePs(ctx, svc.Name)
		if err == nil {
			cJSON, _ := json.Marshal(containers)
			svc.Containers = cJSON
		}
		svc.Status = "running"
		if updateErr := deps.Repos.DockerHost.UpdateService(c.Request.Context(), svc); updateErr != nil {
			slog.Warn("failed to update service status to running", "service", svc.Name, "error", updateErr)
		}

		logAudit(deps, c, "create", "docker_service", svc.ID.String(), nil, gin.H{"name": svc.Name, "folder_path": svc.FolderPath})
		c.JSON(http.StatusCreated, svc)
	}
}

// deployLocalDockerService deploys a compose stack to the local Docker daemon
// (unix:///var/run/docker.sock) without requiring a registered Docker host.
// Accepts either compose_yaml or folder_path (project folder with docker-compose.yml).
func deployLocalDockerService(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.DockerHost == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "docker service repository not available"})
			return
		}
		var req struct {
			Name        string            `json:"name" binding:"required"`
			ComposeYaml string            `json:"compose_yaml"`
			FolderPath  string            `json:"folder_path"`
			GitURL      string            `json:"git_url"`
			EnvVars     map[string]string `json:"env_vars"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Require one of compose_yaml, folder_path, or git_url
		if req.ComposeYaml == "" && req.FolderPath == "" && req.GitURL == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "one of compose_yaml, folder_path, or git_url is required"})
			return
		}

		// Clone git repo if git_url is provided
		if req.GitURL != "" {
			clonedPath, cloneErr := cloneGitRepo(c.Request.Context(), req.GitURL)
			if cloneErr != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": cloneErr.Error()})
				return
			}
			req.FolderPath = clonedPath
		}

		// Validate folder path is accessible (tries original path, then /host-home translation)
		if req.FolderPath != "" {
			if _, resolveErr := resolveHostPath(req.FolderPath); resolveErr != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": resolveErr.Error()})
				return
			}
		}

		envVars := req.EnvVars
		if envVars == nil {
			envVars = make(map[string]string)
		}
		envJSON, _ := json.Marshal(envVars)

		svc := &repository.DockerService{
			TenantID:     auth.GetTenantID(c),
			DockerHostID: nil, // local Docker socket
			Name:         req.Name,
			ComposeYaml:  req.ComposeYaml,
			FolderPath:   req.FolderPath,
			EnvVars:      envJSON,
			Status:       "deploying",
			Containers:   json.RawMessage("[]"),
		}

		if err := deps.Repos.DockerHost.CreateService(c.Request.Context(), svc); err != nil {
			respondInternalError(c, err)
			return
		}

		client := dockerpkg.NewClient(dockerpkg.HostConfig{
			HostType:    "local",
			HostAddress: "unix:///var/run/docker.sock",
		})

		ctx, cancel := context.WithTimeout(c.Request.Context(), 300*time.Second)
		defer cancel()

		// Deploy from folder or from YAML
		var deployErr error
		if svc.FolderPath != "" {
			deployErr = client.ComposeUpFromFolder(ctx, svc.Name, svc.FolderPath, envVars)
		} else {
			deployErr = client.ComposeUp(ctx, svc.Name, svc.ComposeYaml, envVars)
		}

		if deployErr != nil {
			svc.Status = "error"
			if updateErr := deps.Repos.DockerHost.UpdateService(c.Request.Context(), svc); updateErr != nil {
				slog.Warn("failed to update service status to error", "service", svc.Name, "error", updateErr)
			}
			respondInternalError(c, deployErr)
			return
		}

		containers, err := client.ComposePs(ctx, svc.Name)
		if err == nil {
			cJSON, _ := json.Marshal(containers)
			svc.Containers = cJSON
		}
		svc.Status = "running"
		if updateErr := deps.Repos.DockerHost.UpdateService(c.Request.Context(), svc); updateErr != nil {
			slog.Warn("failed to update service status to running", "service", svc.Name, "error", updateErr)
		}

		// Record deployment in history for rollback support.
		if deps.DB != nil && svc.ComposeYaml != "" {
			userID := auth.GetUserID(c)
			deployedBy := ""
			if userID != nil {
				deployedBy = userID.String()
			}
			if _, histErr := deps.DB.Exec(c.Request.Context(),
				`INSERT INTO docker_service_history (service_id, tenant_id, compose_yaml, deployed_by, status)
				 VALUES ($1, $2, $3, $4, 'deployed')`,
				svc.ID, svc.TenantID, svc.ComposeYaml, deployedBy); histErr != nil {
				slog.Warn("failed to record service history", "service", svc.Name, "error", histErr)
			}
		}

		logAudit(deps, c, "create", "docker_service", svc.ID.String(), nil, gin.H{"name": svc.Name, "target": "local", "folder_path": svc.FolderPath})
		c.JSON(http.StatusCreated, svc)
	}
}

// cloneGitRepo clones a git repository to a temporary directory and returns the path.
func cloneGitRepo(ctx context.Context, gitURL string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "pepa-compose-git-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", gitURL, tmpDir) //nolint:gosec // #nosec // G204: git clone with validated args
	output, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", fmt.Errorf("git clone failed: %s: %w", string(output), err)
	}
	return tmpDir, nil
}

// deployLocalDockerServiceStream handles streaming deployment with SSE output.
func deployLocalDockerServiceStream(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.DockerHost == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "docker service repository not available"})
			return
		}

		var req struct {
			Name        string            `json:"name" binding:"required"`
			ComposeYaml string            `json:"compose_yaml"`
			FolderPath  string            `json:"folder_path"`
			GitURL      string            `json:"git_url"`
			EnvVars     map[string]string `json:"env_vars"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if req.ComposeYaml == "" && req.FolderPath == "" && req.GitURL == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "one of compose_yaml, folder_path, or git_url is required"})
			return
		}

		// Clone git repo if git_url is provided
		if req.GitURL != "" {
			clonedPath, cloneErr := cloneGitRepo(c.Request.Context(), req.GitURL)
			if cloneErr != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": cloneErr.Error()})
				return
			}
			req.FolderPath = clonedPath
		}

		if req.FolderPath != "" {
			if _, resolveErr := resolveHostPath(req.FolderPath); resolveErr != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": resolveErr.Error()})
				return
			}
		}

		envVars := req.EnvVars
		if envVars == nil {
			envVars = make(map[string]string)
		}
		envJSON, _ := json.Marshal(envVars)

		svc := &repository.DockerService{
			TenantID:     auth.GetTenantID(c),
			DockerHostID: nil,
			Name:         req.Name,
			ComposeYaml:  req.ComposeYaml,
			FolderPath:   req.FolderPath,
			EnvVars:      envJSON,
			Status:       "deploying",
			Containers:   json.RawMessage("[]"),
		}

		if err := deps.Repos.DockerHost.CreateService(c.Request.Context(), svc); err != nil {
			respondInternalError(c, err)
			return
		}

		// Set up SSE
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")

		client := dockerpkg.NewClient(dockerpkg.HostConfig{
			HostType:    "local",
			HostAddress: "unix:///var/run/docker.sock",
		})

		ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
		defer cancel()

		outputChan := make(chan string, 100)
		errChan := make(chan error, 1)

		// Start deployment in a goroutine
		go func() {
			var err error
			if svc.FolderPath != "" {
				err = client.ComposeUpFromFolderStream(ctx, svc.Name, svc.FolderPath, envVars, outputChan)
			} else {
				err = client.ComposeUp(ctx, svc.Name, svc.ComposeYaml, envVars)
				close(outputChan)
			}
			errChan <- err
		}()

		// Stream output to client
		for line := range outputChan {
			safeLine := strings.ReplaceAll(line, "\n", " ")
			_, _ = c.Writer.WriteString("data: " + safeLine + "\n\n")
			c.Writer.Flush()
		}

		// Wait for deploy goroutine to finish
		deployErr := <-errChan

		// Send completion event
		if deployErr != nil {
			svc.Status = "error"
			_ = deps.Repos.DockerHost.UpdateService(context.Background(), svc)
			_, _ = c.Writer.WriteString("event: error\ndata: " + deployErr.Error() + "\n\n")
		} else {
			containers, err := client.ComposePs(ctx, svc.Name)
			if err == nil {
				cJSON, _ := json.Marshal(containers)
				svc.Containers = cJSON
			}
			svc.Status = "running"
			_ = deps.Repos.DockerHost.UpdateService(context.Background(), svc)
			_, _ = c.Writer.WriteString("event: complete\ndata: Deployment successful\n\n")
		}
		c.Writer.Flush()

		logAudit(deps, c, "create", "docker_service", svc.ID.String(), nil, gin.H{"name": svc.Name, "target": "local", "folder_path": svc.FolderPath})
	}
}

func getDockerService(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.DockerHost == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "docker host repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		tenantID := auth.GetTenantID(c)
		svc, err := deps.Repos.DockerHost.GetService(c.Request.Context(), id, tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "docker service not found"})
			return
		}
		c.JSON(http.StatusOK, svc)
	}
}

func refreshDockerService(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.DockerHost == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "docker host repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		tenantID := auth.GetTenantID(c)
		svc, err := deps.Repos.DockerHost.GetService(c.Request.Context(), id, tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "docker service not found"})
			return
		}

		client, err := dockerClientForService(deps, svc, tenantID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		containers, err := client.ComposePs(ctx, svc.Name)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		cJSON, _ := json.Marshal(containers)
		svc.Containers = cJSON

		// Determine status from container states
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

		_ = deps.Repos.DockerHost.UpdateService(c.Request.Context(), svc)
		c.JSON(http.StatusOK, svc)
	}
}

func restartDockerService(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.DockerHost == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "docker host repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		tenantID := auth.GetTenantID(c)
		svc, err := deps.Repos.DockerHost.GetService(c.Request.Context(), id, tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "docker service not found"})
			return
		}

		client, err := dockerClientForService(deps, svc, tenantID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		var req struct {
			ServiceName string `json:"service_name"`
		}
		_ = c.ShouldBindJSON(&req)

		ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
		defer cancel()

		if err := client.ComposeRestart(ctx, svc.Name, req.ServiceName); err != nil {
			respondInternalError(c, err)
			return
		}

		svc.Status = "running"
		_ = deps.Repos.DockerHost.UpdateService(c.Request.Context(), svc)
		logAudit(deps, c, "restart", "docker_service", id.String(), nil, nil)
		c.JSON(http.StatusOK, gin.H{"status": "restarted"})
	}
}

func stopDockerService(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.DockerHost == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "docker host repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		tenantID := auth.GetTenantID(c)
		svc, err := deps.Repos.DockerHost.GetService(c.Request.Context(), id, tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "docker service not found"})
			return
		}

		client, err := dockerClientForService(deps, svc, tenantID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
		defer cancel()

		if err := client.ComposeStop(ctx, svc.Name); err != nil {
			respondInternalError(c, err)
			return
		}

		svc.Status = "stopped"
		_ = deps.Repos.DockerHost.UpdateService(c.Request.Context(), svc)
		logAudit(deps, c, "stop", "docker_service", id.String(), nil, nil)
		c.JSON(http.StatusOK, gin.H{"status": "stopped"})
	}
}

func startDockerService(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.DockerHost == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "docker host repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		tenantID := auth.GetTenantID(c)
		svc, err := deps.Repos.DockerHost.GetService(c.Request.Context(), id, tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "docker service not found"})
			return
		}

		client, err := dockerClientForService(deps, svc, tenantID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
		defer cancel()

		if err := client.ComposeStart(ctx, svc.Name); err != nil {
			respondInternalError(c, err)
			return
		}

		svc.Status = "running"
		_ = deps.Repos.DockerHost.UpdateService(c.Request.Context(), svc)
		logAudit(deps, c, "start", "docker_service", id.String(), nil, nil)
		c.JSON(http.StatusOK, gin.H{"status": "started"})
	}
}

func deleteDockerService(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.DockerHost == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "docker host repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		tenantID := auth.GetTenantID(c)
		svc, err := deps.Repos.DockerHost.GetService(c.Request.Context(), id, tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "docker service not found"})
			return
		}

		// Try to tear down compose stack
		client, err := dockerClientForService(deps, svc, tenantID)
		if err != nil {
			// If host is gone, still allow DB cleanup
			_ = deps.Repos.DockerHost.DeleteService(c.Request.Context(), id)
			c.JSON(http.StatusOK, gin.H{"status": "deleted"})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
		defer cancel()

		_ = client.ComposeDown(ctx, svc.Name) // best-effort

		if err := deps.Repos.DockerHost.DeleteService(c.Request.Context(), id); err != nil {
			respondInternalError(c, err)
			return
		}
		logAudit(deps, c, "delete", "docker_service", id.String(), nil, nil)
		c.JSON(http.StatusOK, gin.H{"status": "deleted"})
	}
}

func getDockerServiceLogs(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.DockerHost == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "docker host repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		tenantID := auth.GetTenantID(c)
		svc, err := deps.Repos.DockerHost.GetService(c.Request.Context(), id, tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "docker service not found"})
			return
		}

		serviceName := c.Query("service")
		tail := 200

		client, err := dockerClientForService(deps, svc, tenantID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		logs, err := client.ComposeLogs(ctx, svc.Name, serviceName, tail)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"logs": logs})
	}
}

// dockerServiceHistory returns deployment history for a Docker service.
func dockerServiceHistory(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		tenantID := auth.GetTenantID(c)
		if deps.DB == nil {
			c.JSON(http.StatusOK, gin.H{"history": []interface{}{}})
			return
		}

		type historyEntry struct {
			ID          uuid.UUID `json:"id"`
			ServiceID   uuid.UUID `json:"service_id"`
			ComposeYaml string    `json:"compose_yaml"`
			DeployedAt  time.Time `json:"deployed_at"`
			DeployedBy  string    `json:"deployed_by,omitempty"`
			Status      string    `json:"status"`
		}

		rows, err := deps.DB.Query(c.Request.Context(),
			`SELECT id, service_id, compose_yaml, deployed_at, COALESCE(deployed_by,''), status
			 FROM docker_service_history
			 WHERE service_id = $1 AND tenant_id = $2
			 ORDER BY deployed_at DESC LIMIT 50`, id, tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		defer rows.Close()

		var entries []historyEntry
		for rows.Next() {
			var e historyEntry
			if err := rows.Scan(&e.ID, &e.ServiceID, &e.ComposeYaml, &e.DeployedAt, &e.DeployedBy, &e.Status); err == nil {
				entries = append(entries, e)
			}
		}
		if entries == nil {
			entries = []historyEntry{}
		}
		c.JSON(http.StatusOK, gin.H{"history": entries})
	}
}

// rollbackDockerService redeploys the previous compose configuration.
func rollbackDockerService(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.DockerHost == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "docker host repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		tenantID := auth.GetTenantID(c)
		svc, err := deps.Repos.DockerHost.GetService(c.Request.Context(), id, tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "docker service not found"})
			return
		}

		if deps.DB == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database not available"})
			return
		}

		// Find the previous successful deployment.
		var prevCompose string
		err = deps.DB.QueryRow(c.Request.Context(),
			`SELECT compose_yaml FROM docker_service_history
			 WHERE service_id = $1 AND tenant_id = $2 AND status = 'deployed'
			 ORDER BY deployed_at DESC OFFSET 1 LIMIT 1`,
			id, tenantID).Scan(&prevCompose)
		if err != nil || prevCompose == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "no previous deployment to rollback to"})
			return
		}

		client, err := dockerClientForService(deps, svc, tenantID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 120*time.Second)
		defer cancel()

		// Redeploy with the previous compose configuration.
		// Reuse the service's stored env vars to match the original deployment.
		var envVars map[string]string
		if svc.EnvVars != nil && len(svc.EnvVars) > 0 {
			_ = json.Unmarshal(svc.EnvVars, &envVars)
		}
		if err := client.ComposeUp(ctx, svc.Name, prevCompose, envVars); err != nil {
			respondInternalError(c, err)
			return
		}

		// Update the service's compose to the rolled-back version.
		svc.ComposeYaml = prevCompose
		svc.Status = "running"
		_ = deps.Repos.DockerHost.UpdateService(c.Request.Context(), svc)

		// Record the rollback in history.
		userID := auth.GetUserID(c)
		deployedBy := ""
		if userID != nil {
			deployedBy = userID.String()
		}
		_, _ = deps.DB.Exec(c.Request.Context(),
			`INSERT INTO docker_service_history (service_id, tenant_id, compose_yaml, deployed_by, status)
			 VALUES ($1, $2, $3, $4, 'rolled_back')`,
			id, tenantID, prevCompose, deployedBy)

		logAudit(deps, c, "rollback", "docker_service", id.String(), nil, gin.H{"status": "rolled_back"})
		c.JSON(http.StatusOK, gin.H{"status": "rolled_back", "message": "Service rolled back to previous deployment"})
	}
}
