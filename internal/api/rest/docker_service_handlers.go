package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	dockerpkg "github.com/pepa/pepa/internal/docker"
	"github.com/pepa/pepa/internal/repository"
)

func registerDockerServiceRoutes(r *gin.RouterGroup, deps Dependencies) {
	dockerServices := r.Group("/docker-services")
	{
		dockerServices.GET("", listDockerServices(deps))
		dockerServices.POST("", createDockerService(deps))
		dockerServices.POST("/deploy-local", deployLocalDockerService(deps))
		dockerServices.GET("/:id", getDockerService(deps))
		dockerServices.POST("/:id/refresh", refreshDockerService(deps))
		dockerServices.POST("/:id/restart", restartDockerService(deps))
		dockerServices.POST("/:id/stop", stopDockerService(deps))
		dockerServices.POST("/:id/start", startDockerService(deps))
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
			_ = deps.Repos.DockerHost.UpdateService(c.Request.Context(), svc)
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
		_ = deps.Repos.DockerHost.UpdateService(c.Request.Context(), svc)

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
			EnvVars     map[string]string `json:"env_vars"`
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
			_ = deps.Repos.DockerHost.UpdateService(c.Request.Context(), svc)
			respondInternalError(c, deployErr)
			return
		}

		containers, err := client.ComposePs(ctx, svc.Name)
		if err == nil {
			cJSON, _ := json.Marshal(containers)
			svc.Containers = cJSON
		}
		svc.Status = "running"
		_ = deps.Repos.DockerHost.UpdateService(c.Request.Context(), svc)

		logAudit(deps, c, "create", "docker_service", svc.ID.String(), nil, gin.H{"name": svc.Name, "target": "local", "folder_path": svc.FolderPath})
		c.JSON(http.StatusCreated, svc)
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
