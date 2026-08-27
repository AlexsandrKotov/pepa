package rest

import (
	"context"
	"encoding/json"
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
		dockerServices.GET("/:id", getDockerService(deps))
		dockerServices.POST("/:id/refresh", refreshDockerService(deps))
		dockerServices.POST("/:id/restart", restartDockerService(deps))
		dockerServices.POST("/:id/stop", stopDockerService(deps))
		dockerServices.POST("/:id/start", startDockerService(deps))
		dockerServices.DELETE("/:id", deleteDockerService(deps))
		dockerServices.GET("/:id/logs", getDockerServiceLogs(deps))
	}
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
			ComposeYaml  string            `json:"compose_yaml" binding:"required"`
			EnvVars      map[string]string `json:"env_vars"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
			DockerHostID: req.DockerHostID,
			Name:         req.Name,
			ComposeYaml:  req.ComposeYaml,
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

		ctx, cancel := context.WithTimeout(c.Request.Context(), 120*time.Second)
		defer cancel()

		if err := client.ComposeUp(ctx, svc.Name, svc.ComposeYaml, envVars); err != nil {
			svc.Status = "error"
			_ = deps.Repos.DockerHost.UpdateService(c.Request.Context(), svc)
			respondInternalError(c, err)
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

		logAudit(deps, c, "create", "docker_service", svc.ID.String(), nil, gin.H{"name": svc.Name})
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

		host, err := deps.Repos.DockerHost.GetHostDecrypted(c.Request.Context(), svc.DockerHostID, tenantID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "docker host not found"})
			return
		}

		cfg := dockerpkg.HostConfig{
			HostType:    host.HostType,
			HostAddress: host.HostAddress,
			TLSCACert:   host.TLSCACert,
			TLSCert:     host.TLSCert,
			TLSKey:      host.TLSKey,
			SSHKey:      host.SSHKey,
		}
		client := dockerpkg.NewClient(cfg)

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

		host, err := deps.Repos.DockerHost.GetHostDecrypted(c.Request.Context(), svc.DockerHostID, tenantID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "docker host not found"})
			return
		}

		var req struct {
			ServiceName string `json:"service_name"`
		}
		_ = c.ShouldBindJSON(&req)

		cfg := dockerpkg.HostConfig{
			HostType:    host.HostType,
			HostAddress: host.HostAddress,
			TLSCACert:   host.TLSCACert,
			TLSCert:     host.TLSCert,
			TLSKey:      host.TLSKey,
			SSHKey:      host.SSHKey,
		}
		client := dockerpkg.NewClient(cfg)

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

		host, err := deps.Repos.DockerHost.GetHostDecrypted(c.Request.Context(), svc.DockerHostID, tenantID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "docker host not found"})
			return
		}

		cfg := dockerpkg.HostConfig{
			HostType:    host.HostType,
			HostAddress: host.HostAddress,
			TLSCACert:   host.TLSCACert,
			TLSCert:     host.TLSCert,
			TLSKey:      host.TLSKey,
			SSHKey:      host.SSHKey,
		}
		client := dockerpkg.NewClient(cfg)

		ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
		defer cancel()

		if err := client.ComposeStop(ctx, svc.Name); err != nil {
			respondInternalError(c, err)
			return
		}

		svc.Status = "stopped"
		deps.Repos.DockerHost.UpdateService(c.Request.Context(), svc)
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

		host, err := deps.Repos.DockerHost.GetHostDecrypted(c.Request.Context(), svc.DockerHostID, tenantID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "docker host not found"})
			return
		}

		cfg := dockerpkg.HostConfig{
			HostType:    host.HostType,
			HostAddress: host.HostAddress,
			TLSCACert:   host.TLSCACert,
			TLSCert:     host.TLSCert,
			TLSKey:      host.TLSKey,
			SSHKey:      host.SSHKey,
		}
		client := dockerpkg.NewClient(cfg)

		ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
		defer cancel()

		if err := client.ComposeStart(ctx, svc.Name); err != nil {
			respondInternalError(c, err)
			return
		}

		svc.Status = "running"
		deps.Repos.DockerHost.UpdateService(c.Request.Context(), svc)
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

		// Try to tear down compose stack (use decrypted host to get real TLS/SSH credentials)
		host, err := deps.Repos.DockerHost.GetHostDecrypted(c.Request.Context(), svc.DockerHostID, tenantID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "docker host not found"})
			return
		}

		cfg := dockerpkg.HostConfig{
			HostType:    host.HostType,
			HostAddress: host.HostAddress,
			TLSCACert:   host.TLSCACert,
			TLSCert:     host.TLSCert,
			TLSKey:      host.TLSKey,
			SSHKey:      host.SSHKey,
		}
		client := dockerpkg.NewClient(cfg)

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

		host, err := deps.Repos.DockerHost.GetHostDecrypted(c.Request.Context(), svc.DockerHostID, tenantID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "docker host not found"})
			return
		}

		serviceName := c.Query("service")
		tail := 200

		cfg := dockerpkg.HostConfig{
			HostType:    host.HostType,
			HostAddress: host.HostAddress,
			TLSCACert:   host.TLSCACert,
			TLSCert:     host.TLSCert,
			TLSKey:      host.TLSKey,
			SSHKey:      host.SSHKey,
		}
		client := dockerpkg.NewClient(cfg)

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
