package rest

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	dockerpkg "github.com/pepa/pepa/internal/docker"
	"github.com/pepa/pepa/internal/repository"
)

func registerDockerHostRoutes(r *gin.RouterGroup, deps Dependencies) {
	dockerHosts := r.Group("/docker-hosts")
	{
		dockerHosts.GET("", listDockerHosts(deps))
		dockerHosts.POST("", createDockerHost(deps))
		dockerHosts.GET("/:id", getDockerHost(deps))
		dockerHosts.PUT("/:id", updateDockerHost(deps))
		dockerHosts.DELETE("/:id", deleteDockerHost(deps))
		dockerHosts.POST("/:id/test", testDockerHost(deps))
		dockerHosts.GET("/:id/info", getDockerHostInfo(deps))
		dockerHosts.GET("/:id/containers", listDockerHostContainers(deps))
	}
}

func listDockerHosts(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.DockerHost == nil {
			c.JSON(http.StatusOK, gin.H{"docker_hosts": []interface{}{}, "total": 0})
			return
		}
		tenantID := auth.GetTenantID(c)
		items, err := deps.Repos.DockerHost.ListHosts(c.Request.Context(), tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		// Strip sensitive fields from list response
		for i := range items {
			items[i].TLSCACert = ""
			items[i].TLSCert = ""
			items[i].TLSKey = ""
			items[i].SSHKey = ""
		}
		c.JSON(http.StatusOK, gin.H{"docker_hosts": items, "total": len(items)})
	}
}

func createDockerHost(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.DockerHost == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "docker host repository not available"})
			return
		}
		var req struct {
			Name        string `json:"name" binding:"required"`
			Description string `json:"description"`
			HostType    string `json:"host_type" binding:"required"`
			HostAddress string `json:"host_address"`
			TLSCACert   string `json:"tls_ca_cert"`
			TLSCert     string `json:"tls_cert"`
			TLSKey      string `json:"tls_key"`
			SSHKey      string `json:"ssh_key"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Default address for local type
		addr := req.HostAddress
		if req.HostType == "local" && addr == "" {
			addr = "unix:///var/run/docker.sock"
		}
		if addr == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "host_address is required"})
			return
		}

		tenantID := auth.GetTenantID(c)
		host := &repository.DockerHost{
			TenantID:    tenantID,
			Name:        req.Name,
			Description: req.Description,
			HostType:    req.HostType,
			HostAddress: addr,
			TLSCACert:   req.TLSCACert,
			TLSCert:     req.TLSCert,
			TLSKey:      req.TLSKey,
			SSHKey:      req.SSHKey,
			Status:      "disconnected",
		}

		if err := deps.Repos.DockerHost.CreateHost(c.Request.Context(), host); err != nil {
			respondInternalError(c, err)
			return
		}

		// Strip sensitive fields
		host.TLSCACert = ""
		host.TLSCert = ""
		host.TLSKey = ""
		host.SSHKey = ""
		logAudit(deps, c, "create", "docker_host", host.ID.String(), nil, gin.H{"name": host.Name, "host_type": host.HostType})
		c.JSON(http.StatusCreated, host)
	}
}

func getDockerHost(deps Dependencies) gin.HandlerFunc {
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
		host, err := deps.Repos.DockerHost.GetHost(c.Request.Context(), id, tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "docker host not found"})
			return
		}
		// Strip sensitive fields
		host.TLSCACert = ""
		host.TLSCert = ""
		host.TLSKey = ""
		host.SSHKey = ""
		c.JSON(http.StatusOK, host)
	}
}

func updateDockerHost(deps Dependencies) gin.HandlerFunc {
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
		existing, err := deps.Repos.DockerHost.GetHostDecrypted(c.Request.Context(), id, tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "docker host not found"})
			return
		}

		var req struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			HostType    string `json:"host_type"`
			HostAddress string `json:"host_address"`
			TLSCACert   string `json:"tls_ca_cert"`
			TLSCert     string `json:"tls_cert"`
			TLSKey      string `json:"tls_key"`
			SSHKey      string `json:"ssh_key"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if req.Name != "" {
			existing.Name = req.Name
		}
		if req.Description != "" {
			existing.Description = req.Description
		}
		if req.HostType != "" {
			existing.HostType = req.HostType
		}
		if req.HostAddress != "" {
			existing.HostAddress = req.HostAddress
		}
		// TLS/SSH fields: update if provided (allow empty to clear)
		if req.TLSCACert != "" {
			existing.TLSCACert = req.TLSCACert
		}
		if req.TLSCert != "" {
			existing.TLSCert = req.TLSCert
		}
		if req.TLSKey != "" {
			existing.TLSKey = req.TLSKey
		}
		if req.SSHKey != "" {
			existing.SSHKey = req.SSHKey
		}

		if err := deps.Repos.DockerHost.UpdateHost(c.Request.Context(), existing); err != nil {
			respondInternalError(c, err)
			return
		}
		existing.TLSCACert = ""
		existing.TLSCert = ""
		existing.TLSKey = ""
		existing.SSHKey = ""
		logAudit(deps, c, "update", "docker_host", existing.ID.String(), nil, gin.H{"name": existing.Name})
		c.JSON(http.StatusOK, existing)
	}
}

func deleteDockerHost(deps Dependencies) gin.HandlerFunc {
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
		if err := deps.Repos.DockerHost.DeleteHost(c.Request.Context(), id); err != nil {
			respondInternalError(c, err)
			return
		}
		logAudit(deps, c, "delete", "docker_host", id.String(), nil, nil)
		c.JSON(http.StatusOK, gin.H{"status": "deleted"})
	}
}

func testDockerHost(deps Dependencies) gin.HandlerFunc {
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
		host, err := deps.Repos.DockerHost.GetHostDecrypted(c.Request.Context(), id, tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "docker host not found"})
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

		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()

		info, err := client.TestConnection(ctx)
		if err != nil {
			host.Status = "error"
			now := time.Now()
			host.LastCheckedAt = &now
			_ = deps.Repos.DockerHost.UpdateHost(c.Request.Context(), host)
			log.Printf("testDockerHost: connection test failed for host %s: %v", id, err)
			c.JSON(http.StatusOK, gin.H{"status": "error", "error": "connection test failed"})
			return
		}

		host.Status = "connected"
		host.DockerVersion = info.Version
		host.OSArch = info.OS + "/" + info.Arch
		host.ContainersRunning = info.ContainersRunning
		now := time.Now()
		host.LastCheckedAt = &now
		_ = deps.Repos.DockerHost.UpdateHost(c.Request.Context(), host)

		c.JSON(http.StatusOK, gin.H{
			"status":             "connected",
			"docker_version":     info.Version,
			"os_arch":            info.OS + "/" + info.Arch,
			"containers_running": info.ContainersRunning,
			"containers_total":   info.ContainersTotal,
		})
	}
}

func getDockerHostInfo(deps Dependencies) gin.HandlerFunc {
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
		host, err := deps.Repos.DockerHost.GetHost(c.Request.Context(), id, tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "docker host not found"})
			return
		}
		host.TLSCACert = ""
		host.TLSCert = ""
		host.TLSKey = ""
		host.SSHKey = ""
		c.JSON(http.StatusOK, host)
	}
}

// listDockerHostContainers discovers running containers on a Docker host via `docker ps`.
func listDockerHostContainers(deps Dependencies) gin.HandlerFunc {
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
		host, err := deps.Repos.DockerHost.GetHostDecrypted(c.Request.Context(), id, tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "docker host not found"})
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

		all := c.Query("all") == "true"
		containers, err := client.ListContainers(ctx, all)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"containers": containers,
			"total":      len(containers),
			"host_id":    id,
			"host_name":  host.Name,
		})
	}
}
