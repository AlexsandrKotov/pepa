package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/pepa/pepa/internal/auth"
	pepacrypto "github.com/pepa/pepa/internal/crypto"
	"github.com/pepa/pepa/internal/repository"
	"golang.org/x/crypto/ssh"
)

// registerRemoteConsoleRoutes registers SSH host management and terminal WebSocket routes.
func registerRemoteConsoleRoutes(r *gin.RouterGroup, deps Dependencies) {
	hosts := r.Group("/ssh-hosts")
	{
		hosts.GET("", listSSHHosts(deps))
		hosts.POST("", createSSHHost(deps))
		hosts.GET("/:id", getSSHHost(deps))
		hosts.PUT("/:id", updateSSHHost(deps))
		hosts.DELETE("/:id", deleteSSHHost(deps))
		hosts.POST("/:id/test", testSSHConnection(deps))
		hosts.PUT("/:id/groups", setHostGroups(deps))
	}

	// SSH host groups
	groups := r.Group("/ssh-host-groups")
	{
		groups.GET("", listSSHHostGroups(deps))
		groups.POST("", createSSHHostGroup(deps))
		groups.GET("/:id", getSSHHostGroup(deps))
		groups.PUT("/:id", updateSSHHostGroup(deps))
		groups.DELETE("/:id", deleteSSHHostGroup(deps))
	}

	// WebSocket terminal endpoint — uses a separate upgrader
	r.GET("/ssh-terminal/:host_id", sshTerminalHandler(deps))
}

// ─── CRUD Handlers ────────────────────────────────────────────────────────────

func listSSHHosts(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := auth.GetTenantID(c)
		ctx := c.Request.Context()

		if deps.Repos.SSHHost == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "SSH host repository not available"})
			return
		}

		hosts, err := deps.Repos.SSHHost.List(ctx, tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		// Batch fetch all group memberships in a single query (fixes N+1 problem)
		hostGroupMap := make(map[uuid.UUID][]uuid.UUID)
		if deps.Repos.SSHHostGroup != nil && len(hosts) > 0 {
			hostIDs := make([]uuid.UUID, len(hosts))
			for i, h := range hosts {
				hostIDs[i] = h.ID
			}
			batchMap, err := deps.Repos.SSHHostGroup.GetHostGroupIDsBatch(ctx, hostIDs)
			if err == nil {
				hostGroupMap = batchMap
			}
		}

		// Mask sensitive fields for the response
		result := make([]gin.H, len(hosts))
		for i, h := range hosts {
			groupIDs := hostGroupMap[h.ID]
			if groupIDs == nil {
				groupIDs = []uuid.UUID{}
			}
			result[i] = gin.H{
				"id":           h.ID,
				"name":         h.Name,
				"hostname":     h.Hostname,
				"port":         h.Port,
				"username":     h.Username,
				"auth_method":  h.AuthMethod,
				"has_ssh_key":  h.SSHEncryptedKey != "",
				"has_password": h.PasswordEnc != "",
				"tags":         h.Tags,
				"description":  h.Description,
				"group_ids":    groupIDs,
				"created_by":   h.CreatedBy,
				"created_at":   h.CreatedAt,
				"updated_at":   h.UpdatedAt,
			}
		}

		c.JSON(http.StatusOK, gin.H{"hosts": result, "total": len(result)})
	}
}

func createSSHHost(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserID(c)
		tenantID := auth.GetTenantID(c)
		ctx := c.Request.Context()

		var req struct {
			Name        string   `json:"name" binding:"required"`
			Hostname    string   `json:"hostname" binding:"required"`
			Port        int      `json:"port"`
			Username    string   `json:"username"`
			AuthMethod  string   `json:"auth_method"`
			SSHKey      string   `json:"ssh_key"`
			Password    string   `json:"password"`
			Tags        []string `json:"tags"`
			Description string   `json:"description"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "name and hostname are required"})
			return
		}

		if req.Port == 0 {
			req.Port = 22
		}
		if req.Username == "" {
			req.Username = "root"
		}
		if req.AuthMethod == "" {
			req.AuthMethod = "password"
		}
		// Validate auth_method
		switch req.AuthMethod {
		case "password", "key", "ldap_passthrough":
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid auth_method, must be password, key, or ldap_passthrough"})
			return
		}

		// Encrypt sensitive fields
		var sshKeyEnc, passwordEnc string
		var err error
		if req.SSHKey != "" {
			sshKeyEnc, err = pepacrypto.Encrypt(req.SSHKey)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to encrypt SSH key"})
				return
			}
		}
		if req.Password != "" {
			passwordEnc, err = pepacrypto.Encrypt(req.Password)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to encrypt password"})
				return
			}
		}

		host := &repository.SSHHost{
			ID:              uuid.New(),
			TenantID:        tenantID,
			Name:            req.Name,
			Hostname:        req.Hostname,
			Port:            req.Port,
			Username:        req.Username,
			AuthMethod:      req.AuthMethod,
			SSHEncryptedKey: sshKeyEnc,
			PasswordEnc:     passwordEnc,
			Tags:            req.Tags,
			Description:     req.Description,
			CreatedBy:       userID,
		}

		if deps.Repos.SSHHost == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "SSH host repository not available"})
			return
		}

		if err := deps.Repos.SSHHost.Create(ctx, host); err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "create", "ssh_host", host.ID.String(), nil, gin.H{
			"name": host.Name, "hostname": host.Hostname,
		})

		c.JSON(http.StatusCreated, gin.H{
			"id":      host.ID,
			"message": "SSH host added",
		})
	}
}

func getSSHHost(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid host ID"})
			return
		}

		ctx := c.Request.Context()
		if deps.Repos.SSHHost == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "SSH host repository not available"})
			return
		}

		host, err := deps.Repos.SSHHost.GetByID(ctx, id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "host not found"})
			return
		}

		// Verify tenant isolation
		tenantID := auth.GetTenantID(c)
		if host.TenantID != tenantID {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"id":           host.ID,
			"name":         host.Name,
			"hostname":     host.Hostname,
			"port":         host.Port,
			"username":     host.Username,
			"auth_method":  host.AuthMethod,
			"has_ssh_key":  host.SSHEncryptedKey != "",
			"has_password": host.PasswordEnc != "",
			"tags":         host.Tags,
			"description":  host.Description,
			"created_by":   host.CreatedBy,
			"created_at":   host.CreatedAt,
			"updated_at":   host.UpdatedAt,
		})
	}
}

func updateSSHHost(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid host ID"})
			return
		}

		ctx := c.Request.Context()
		if deps.Repos.SSHHost == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "SSH host repository not available"})
			return
		}

		host, err := deps.Repos.SSHHost.GetByID(ctx, id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "host not found"})
			return
		}

		// Verify tenant isolation
		tenantID := auth.GetTenantID(c)
		if host.TenantID != tenantID {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
			return
		}

		var req struct {
			Name        string   `json:"name"`
			Hostname    string   `json:"hostname"`
			Port        int      `json:"port"`
			Username    string   `json:"username"`
			AuthMethod  string   `json:"auth_method"`
			SSHKey      string   `json:"ssh_key"`
			Password    string   `json:"password"`
			Tags        []string `json:"tags"`
			Description *string  `json:"description"` // pointer to allow clearing
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if req.Name != "" {
			host.Name = req.Name
		}
		if req.Hostname != "" {
			host.Hostname = req.Hostname
		}
		if req.Port > 0 {
			host.Port = req.Port
		}
		if req.Username != "" {
			host.Username = req.Username
		}
		if req.AuthMethod != "" {
			switch req.AuthMethod {
			case "password", "key", "ldap_passthrough":
				host.AuthMethod = req.AuthMethod
			default:
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid auth_method, must be password, key, or ldap_passthrough"})
				return
			}
		}
		if req.Tags != nil {
			host.Tags = req.Tags
		}
		if req.Description != nil {
			host.Description = *req.Description
		}

		// Re-encrypt if new credentials provided
		if req.SSHKey != "" {
			enc, err := pepacrypto.Encrypt(req.SSHKey)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to encrypt SSH key"})
				return
			}
			host.SSHEncryptedKey = enc
		}
		if req.Password != "" {
			enc, err := pepacrypto.Encrypt(req.Password)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to encrypt password"})
				return
			}
			host.PasswordEnc = enc
		}

		if err := deps.Repos.SSHHost.Update(ctx, host); err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "update", "ssh_host", id.String(), nil, nil)
		c.JSON(http.StatusOK, gin.H{"message": "host updated"})
	}
}

func deleteSSHHost(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid host ID"})
			return
		}

		ctx := c.Request.Context()
		if deps.Repos.SSHHost == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "SSH host repository not available"})
			return
		}

		// Verify tenant isolation before deletion
		tenantID := auth.GetTenantID(c)
		host, err := deps.Repos.SSHHost.GetByID(ctx, id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "host not found"})
			return
		}
		if host.TenantID != tenantID {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
			return
		}

		if err := deps.Repos.SSHHost.Delete(ctx, id); err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "delete", "ssh_host", id.String(), nil, nil)
		c.JSON(http.StatusOK, gin.H{"message": "host deleted"})
	}
}

// ─── Host Groups ──────────────────────────────────────────────────────────────

func setHostGroups(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		hostID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid host ID"})
			return
		}

		var req struct {
			GroupIDs []uuid.UUID `json:"group_ids"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "group_ids array required"})
			return
		}

		if deps.Repos.SSHHostGroup == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "SSH host group repository not available"})
			return
		}

		// CRITICAL FIX: Verify host belongs to current tenant
		ctx := c.Request.Context()
		if deps.Repos.SSHHost == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "SSH host repository not available"})
			return
		}
		host, err := deps.Repos.SSHHost.GetByID(ctx, hostID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "host not found"})
			return
		}
		tenantID := auth.GetTenantID(c)
		if host.TenantID != tenantID {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
			return
		}

		// Repository validates that all groups exist and belong to the same tenant
		if err := deps.Repos.SSHHostGroup.SetHostGroups(ctx, hostID, req.GroupIDs, tenantID); err != nil {
			if strings.Contains(err.Error(), "do not exist or belong to a different tenant") {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "update", "ssh_host_groups", hostID.String(), nil, gin.H{"group_ids": req.GroupIDs})
		c.JSON(http.StatusOK, gin.H{"message": "host groups updated"})
	}
}

func listSSHHostGroups(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := auth.GetTenantID(c)
		ctx := c.Request.Context()

		if deps.Repos.SSHHostGroup == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "SSH host group repository not available"})
			return
		}

		groups, err := deps.Repos.SSHHostGroup.List(ctx, tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{"groups": groups, "total": len(groups)})
	}
}

func createSSHHostGroup(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserID(c)
		tenantID := auth.GetTenantID(c)
		ctx := c.Request.Context()

		var req struct {
			Name        string `json:"name" binding:"required"`
			Description string `json:"description"`
			Color       string `json:"color"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
			return
		}

		// Validate color format (hex color)
		if req.Color == "" {
			req.Color = "#7aa2f7"
		} else if len(req.Color) != 7 || req.Color[0] != '#' {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid color format, use #RRGGBB"})
			return
		} else {
			// Validate hex characters
			for i := 1; i < 7; i++ {
				ch := req.Color[i]
				if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')) {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid color format, use #RRGGBB"})
					return
				}
			}
		}

		if deps.Repos.SSHHostGroup == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "SSH host group repository not available"})
			return
		}

		group := &repository.SSHHostGroup{
			ID:          uuid.New(),
			TenantID:    tenantID,
			Name:        req.Name,
			Description: req.Description,
			Color:       req.Color,
			CreatedBy:   userID,
		}

		if err := deps.Repos.SSHHostGroup.Create(ctx, group); err != nil {
			if strings.Contains(err.Error(), "unique index") || strings.Contains(err.Error(), "duplicate") {
				c.JSON(http.StatusConflict, gin.H{"error": "a group with this name already exists"})
				return
			}
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "create", "ssh_host_group", group.ID.String(), nil, gin.H{"name": group.Name})
		c.JSON(http.StatusCreated, gin.H{"id": group.ID, "message": "group created"})
	}
}

func getSSHHostGroup(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group ID"})
			return
		}

		if deps.Repos.SSHHostGroup == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "SSH host group repository not available"})
			return
		}

		// Use tenant-scoped query
		tenantID := auth.GetTenantID(c)
		group, err := deps.Repos.SSHHostGroup.GetByIDAndTenant(c.Request.Context(), id, tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "group not found"})
			return
		}

		// Include host IDs in this group
		hostIDs, _ := deps.Repos.SSHHostGroup.GetGroupHostIDs(c.Request.Context(), id)
		if hostIDs == nil {
			hostIDs = []uuid.UUID{}
		}

		c.JSON(http.StatusOK, gin.H{
			"id":          group.ID,
			"name":        group.Name,
			"description": group.Description,
			"color":       group.Color,
			"host_count":  group.HostCount,
			"host_ids":    hostIDs,
			"created_by":  group.CreatedBy,
			"created_at":  group.CreatedAt,
			"updated_at":  group.UpdatedAt,
		})
	}
}

func updateSSHHostGroup(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group ID"})
			return
		}

		if deps.Repos.SSHHostGroup == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "SSH host group repository not available"})
			return
		}

		// Use tenant-scoped query
		tenantID := auth.GetTenantID(c)
		group, err := deps.Repos.SSHHostGroup.GetByIDAndTenant(c.Request.Context(), id, tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "group not found"})
			return
		}

		var req struct {
			Name        string  `json:"name"`
			Description *string `json:"description"` // pointer to allow clearing
			Color       string  `json:"color"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if req.Name != "" {
			group.Name = req.Name
		}
		if req.Description != nil {
			group.Description = *req.Description
		}
		// Validate color format if provided
		if req.Color != "" {
			if len(req.Color) != 7 || req.Color[0] != '#' {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid color format, use #RRGGBB"})
				return
			}
			for i := 1; i < 7; i++ {
				ch := req.Color[i]
				if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')) {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid color format, use #RRGGBB"})
					return
				}
			}
			group.Color = req.Color
		}

		if err := deps.Repos.SSHHostGroup.Update(c.Request.Context(), group); err != nil {
			if strings.Contains(err.Error(), "unique index") || strings.Contains(err.Error(), "duplicate") {
				c.JSON(http.StatusConflict, gin.H{"error": "a group with this name already exists"})
				return
			}
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "update", "ssh_host_group", id.String(), nil, nil)
		c.JSON(http.StatusOK, gin.H{"message": "group updated"})
	}
}

func deleteSSHHostGroup(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group ID"})
			return
		}

		if deps.Repos.SSHHostGroup == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "SSH host group repository not available"})
			return
		}

		// Explicit tenant check before deletion
		tenantID := auth.GetTenantID(c)
		_, err = deps.Repos.SSHHostGroup.GetByIDAndTenant(c.Request.Context(), id, tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "group not found"})
			return
		}

		if err := deps.Repos.SSHHostGroup.Delete(c.Request.Context(), id); err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "delete", "ssh_host_group", id.String(), nil, nil)
		c.JSON(http.StatusOK, gin.H{"message": "group deleted"})
	}
}

// ─── Test Connection ──────────────────────────────────────────────────────────

func testSSHConnection(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid host ID"})
			return
		}

		ctx := c.Request.Context()
		if deps.Repos.SSHHost == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "SSH host repository not available"})
			return
		}

		host, err := deps.Repos.SSHHost.GetByID(ctx, id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "host not found"})
			return
		}

		// Build SSH config based on auth method
		config, err := buildSSHConfig(host, deps, c)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// Attempt connection
		addr := net.JoinHostPort(host.Hostname, strconv.Itoa(host.Port))
		client, err := ssh.Dial("tcp", addr, config)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"status":  "error",
				"message": fmt.Sprintf("Connection failed: %v", err),
			})
			return
		}
		_ = client.Close()

		c.JSON(http.StatusOK, gin.H{
			"status":  "connected",
			"message": fmt.Sprintf("Successfully connected to %s", host.Hostname),
		})
	}
}

// ─── WebSocket Terminal ───────────────────────────────────────────────────────

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true // non-browser clients
		}
		// Parse the Origin URL and compare hostnames (works behind reverse proxies
		// where Host may be rewritten by nginx).
		originURL, err := url.Parse(origin)
		if err != nil {
			return false
		}
		originHost, _, _ := net.SplitHostPort(originURL.Host)
		if originHost == "" {
			originHost = originURL.Host // no port in Origin
		}
		// If the request Host is empty, reject the connection to prevent
		// origin validation bypass via empty hostname matching.
		if r.Host == "" {
			return false
		}
		reqHost, _, _ := net.SplitHostPort(r.Host)
		if reqHost == "" {
			reqHost = r.Host // no port in Host
		}
		return originHost == reqHost
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// wsMessage is used to parse structured JSON messages from the WebSocket.
type wsMessage struct {
	Type     string `json:"type"`
	Password string `json:"password"`
	Cols     int    `json:"cols"`
	Rows     int    `json:"rows"`
}

// sessionTracker tracks active SSH sessions per user.
var (
	activeSessions   = make(map[uuid.UUID]int)      // userID -> session count
	activeSessionAge = make(map[uuid.UUID]time.Time) // userID -> last activity
	activeSessionsMu sync.Mutex
)

// sessionCleanupInterval is how often we scan for stale session entries.
const sessionCleanupInterval = 5 * time.Minute

// sessionMaxAge is the maximum time a session entry can persist without activity.
const sessionMaxAge = 24 * time.Hour

func init() {
	// Periodic cleanup of stale session entries to prevent unbounded map growth.
	go func() {
		ticker := time.NewTicker(sessionCleanupInterval)
		defer ticker.Stop()
		for range ticker.C {
			activeSessionsMu.Lock()
			cutoff := time.Now().Add(-sessionMaxAge)
			for uid, lastSeen := range activeSessionAge {
				if lastSeen.Before(cutoff) {
					delete(activeSessions, uid)
					delete(activeSessionAge, uid)
				}
			}
			activeSessionsMu.Unlock()
		}
	}()
}

func sshTerminalHandler(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		hostID, err := uuid.Parse(c.Param("host_id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid host ID"})
			return
		}

		// Authenticate via httpOnly cookie (browser sends it automatically with WebSocket)
		userID, tenantID, userEmail, err := authenticateWebSocket(c, deps)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		// Check session limit and reserve atomically to prevent TOCTOU race
		activeSessionsMu.Lock()
		sessionCount := activeSessions[*userID]
		maxSessions := 5
		if sessionCount >= maxSessions {
			activeSessionsMu.Unlock()
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "maximum concurrent sessions reached"})
			return
		}
		activeSessions[*userID]++
		activeSessionAge[*userID] = time.Now()
		activeSessionsMu.Unlock()

		defer func() {
			activeSessionsMu.Lock()
			activeSessions[*userID]--
			if activeSessions[*userID] <= 0 {
				delete(activeSessions, *userID)
				delete(activeSessionAge, *userID)
			}
			activeSessionsMu.Unlock()
		}()

		ctx := c.Request.Context()
		if deps.Repos.SSHHost == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "SSH host repository not available"})
			return
		}

		host, err := deps.Repos.SSHHost.GetByID(ctx, hostID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "host not found"})
			return
		}

		// Verify tenant isolation
		if host.TenantID != *tenantID {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
			return
		}

		// Build SSH config
		sshConfig, err := buildSSHConfig(host, deps, c)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// Upgrade to WebSocket
		ws, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			slog.Error("WebSocket upgrade failed", "error", err)
			return
		}
		defer func() { _ = ws.Close() }() //nolint:errcheck // cleanup

		// Write mutex to prevent concurrent WebSocket writes (gorilla/websocket requirement)
		var writeMu sync.Mutex
		wsWrite := func(msgType int, data []byte) error {
			writeMu.Lock()
			defer writeMu.Unlock()
			return ws.WriteMessage(msgType, data)
		}

		// For LDAP passthrough, wait for password via first WebSocket message
		if host.AuthMethod == "ldap_passthrough" {
			_ = ws.SetReadDeadline(time.Now().Add(30 * time.Second)) //nolint:errcheck // timeout setup
			_, msg, readErr := ws.ReadMessage()
			_ = ws.SetReadDeadline(time.Time{}) //nolint:errcheck // clear deadline
			if readErr != nil {
				slog.Warn("WebSocket read timeout waiting for auth", "host", host.Hostname)
				return
			}
			var authMsg wsMessage
			if jsonErr := json.Unmarshal(msg, &authMsg); jsonErr != nil || authMsg.Type != "auth" || authMsg.Password == "" {
				_ = wsWrite(websocket.TextMessage, []byte("\r\n\x1b[31mAuthentication required. Send {\"type\":\"auth\",\"password\":\"...\"} first.\x1b[0m\r\n"))
				return
			}
			sshConfig.Auth = []ssh.AuthMethod{ssh.Password(authMsg.Password)}
		}

		// Connect to SSH
		addr := net.JoinHostPort(host.Hostname, strconv.Itoa(host.Port))
		sshClient, err := ssh.Dial("tcp", addr, sshConfig)
		if err != nil {
			_ = wsWrite(websocket.TextMessage, []byte(fmt.Sprintf("\r\n\x1b[31mConnection failed: %v\x1b[0m\r\n", err)))
			return
		}
		defer func() { _ = sshClient.Close() }() //nolint:errcheck // cleanup

		sshSession, err := sshClient.NewSession()
		if err != nil {
			_ = wsWrite(websocket.TextMessage, []byte(fmt.Sprintf("\r\n\x1b[31mFailed to create session: %v\x1b[0m\r\n", err)))
			return
		}
		defer func() { _ = sshSession.Close() }() //nolint:errcheck // cleanup

		// Request PTY
		modes := ssh.TerminalModes{
			ssh.ECHO:          1,
			ssh.TTY_OP_ISPEED: 14400,
			ssh.TTY_OP_OSPEED: 14400,
		}
		cols, rows := 80, 24
		if w := c.Query("cols"); w != "" {
			if colsVal, parseErr := strconv.Atoi(w); parseErr == nil && colsVal > 0 {
				cols = colsVal
			}
		}
		if h := c.Query("rows"); h != "" {
			if rowsVal, parseErr := strconv.Atoi(h); parseErr == nil && rowsVal > 0 {
				rows = rowsVal
			}
		}
		if err := sshSession.RequestPty("xterm-256color", rows, cols, modes); err != nil {
			_ = wsWrite(websocket.TextMessage, []byte(fmt.Sprintf("\r\n\x1b[31mPTY request failed: %v\x1b[0m\r\n", err)))
			return
		}

		// Pipe SSH stdout/stderr to WebSocket
		stdout, _ := sshSession.StdoutPipe() //nolint:errcheck // pipe setup
		stderr, _ := sshSession.StderrPipe() //nolint:errcheck // pipe setup

		// Track both stdout and stderr goroutines with WaitGroup
		var ioWg sync.WaitGroup
		ioWg.Add(2)

		go func() {
			defer ioWg.Done()
			buf := make([]byte, 4096)
			for {
				n, err := stdout.Read(buf)
				if n > 0 {
					if wsErr := wsWrite(websocket.BinaryMessage, buf[:n]); wsErr != nil {
						return
					}
				}
				if err != nil {
					return
				}
			}
		}()

		go func() {
			defer ioWg.Done()
			buf := make([]byte, 4096)
			for {
				n, err := stderr.Read(buf)
				if n > 0 {
					if wsErr := wsWrite(websocket.BinaryMessage, buf[:n]); wsErr != nil {
						return
					}
				}
				if err != nil {
					return
				}
			}
		}()

		// Pipe WebSocket input to SSH stdin
		stdin, _ := sshSession.StdinPipe() //nolint:errcheck // pipe setup

		// Start shell
		if err := sshSession.Shell(); err != nil {
			_ = wsWrite(websocket.TextMessage, []byte(fmt.Sprintf("\r\n\x1b[31mShell failed: %v\x1b[0m\r\n", err))) //nolint:errcheck // error notification
			return
		}

		// Read from WebSocket and write to SSH stdin
		// Also intercept commands for logging
		go func() {
			var cmdBuf bytes.Buffer
			var expectingPassword bool // Flag to skip logging password input

			// Commands that typically trigger a password prompt
			isPasswordTrigger := func(cmd string) bool {
				cmd = strings.TrimSpace(cmd)
				// sudo (without -S which reads from stdin)
				if strings.HasPrefix(cmd, "sudo ") && !strings.Contains(cmd, " -S") {
					return true
				}
				// su (switch user)
				if cmd == "su" || strings.HasPrefix(cmd, "su ") {
					return true
				}
				// passwd
				if cmd == "passwd" || strings.HasPrefix(cmd, "passwd ") {
					return true
				}
				// ssh to a new host (might prompt for password on first connect)
				if strings.HasPrefix(cmd, "ssh ") && !strings.Contains(cmd, "-i ") {
					return true
				}
				return false
			}

			for {
				// Set read deadline for idle timeout (30 minutes)
				_ = ws.SetReadDeadline(time.Now().Add(30 * time.Minute)) //nolint:errcheck // idle timeout
				_, msg, err := ws.ReadMessage()
				if err != nil {
					// Check if it's a timeout error
					if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
						_ = wsWrite(websocket.TextMessage, []byte("\r\n\x1b[33mSession idle timeout (30 minutes). Disconnecting.\x1b[0m\r\n")) //nolint:errcheck // timeout notification
					}
					return
				}
				// Reset read deadline on activity
				_ = ws.SetReadDeadline(time.Time{}) //nolint:errcheck // clear deadline
				// Handle window resize messages (JSON format)
				if len(msg) > 0 && msg[0] == '{' {
					var resizeMsg wsMessage
					if json.Unmarshal(msg, &resizeMsg) == nil && resizeMsg.Type == "resize" {
						if resizeMsg.Cols > 0 && resizeMsg.Rows > 0 {
							_ = sshSession.WindowChange(resizeMsg.Rows, resizeMsg.Cols) //nolint:errcheck // resize
						}
						continue
					}
				}
				// Intercept keystrokes for command logging
				for _, b := range msg {
					switch b {
					case 0x0d: // Enter (CR)
						cmd := strings.TrimSpace(cmdBuf.String())
						cmdBuf.Reset()
						// Skip logging if we're expecting a password
						if expectingPassword {
							expectingPassword = false
							continue
						}
						if cmd != "" && deps.Repos.PluginActivity != nil {
							// Check if this command will trigger a password prompt
							if isPasswordTrigger(cmd) {
								expectingPassword = true
							}
							entry := &repository.SSHCommandLog{
								TenantID: *tenantID,
								UserID:   userID,
								HostID:   hostID,
								HostName: host.Hostname,
								Username: host.Username,
								Command:  cmd,
							}
							go func(e *repository.SSHCommandLog) {
								logCtx, logCancel := context.WithTimeout(context.Background(), 5*time.Second)
								defer logCancel()
								if logErr := deps.Repos.PluginActivity.LogSSHCommand(logCtx, e); logErr != nil {
									slog.Debug("failed to log SSH command", "error", logErr)
								}
							}(entry)
						}
					case 0x7f: // Backspace
						if cmdBuf.Len() > 0 {
							cmdBuf.Truncate(cmdBuf.Len() - 1)
						}
					case 0x03: // Ctrl+C — clear buffer and reset password expectation
						cmdBuf.Reset()
						expectingPassword = false
					default:
						if b >= 0x20 && b < 0x7f { // printable ASCII
							cmdBuf.WriteByte(b)
						}
					}
				}
				if _, err := stdin.Write(msg); err != nil {
					return
				}
			}
		}()

		// Wait for I/O goroutines to finish or context cancellation
		done := make(chan struct{})
		go func() {
			ioWg.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-c.Request.Context().Done():
		}

		slog.Info("SSH session ended", "host", host.Hostname, "user_id", userID, "user_email", userEmail)
	}
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// buildSSHConfig creates an ssh.ClientConfig based on the host's auth method.
func buildSSHConfig(host *repository.SSHHost, deps Dependencies, c *gin.Context) (*ssh.ClientConfig, error) {
	config := &ssh.ClientConfig{
		User:            host.Username,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // #nosec // G302: user-configured hosts with explicit trust
		Timeout:         30 * time.Second,
	}

	switch host.AuthMethod {
	case "key":
		if host.SSHEncryptedKey == "" {
			return nil, fmt.Errorf("SSH key not configured for this host")
		}
		key, err := pepacrypto.Decrypt(host.SSHEncryptedKey)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt SSH key: %w", err)
		}
		signer, err := ssh.ParsePrivateKey([]byte(key))
		if err != nil {
			return nil, fmt.Errorf("invalid SSH key: %w", err)
		}
		config.Auth = []ssh.AuthMethod{ssh.PublicKeys(signer)}

	case "password":
		if host.PasswordEnc == "" {
			return nil, fmt.Errorf("password not configured for this host")
		}
		password, err := pepacrypto.Decrypt(host.PasswordEnc)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt password: %w", err)
		}
		config.Auth = []ssh.AuthMethod{ssh.Password(password)}

	case "ldap_passthrough":
		// For LDAP passthrough, the password is provided at connection time via WebSocket message.
		// The actual password auth is set in the WebSocket handler after receiving the auth message.
		// If a stored password exists, use it as fallback.
		if host.PasswordEnc != "" {
			password, err := pepacrypto.Decrypt(host.PasswordEnc)
			if err == nil {
				config.Auth = []ssh.AuthMethod{ssh.Password(password)}
			}
		}

	default:
		return nil, fmt.Errorf("unsupported auth method: %s", host.AuthMethod)
	}

	return config, nil
}

// authenticateWebSocket validates JWT from the httpOnly cookie.
// The browser sends the cookie automatically with the WebSocket request.
func authenticateWebSocket(c *gin.Context, deps Dependencies) (userID *uuid.UUID, tenantID *uuid.UUID, email string, err error) {
	cookie, cookieErr := c.Request.Cookie("pepa_token")
	if cookieErr != nil {
		return nil, nil, "", fmt.Errorf("no authentication token")
	}

	tokenStr := cookie.Value
	if tokenStr == "" {
		return nil, nil, "", fmt.Errorf("no authentication token")
	}

	// Validate JWT
	claims, validateErr := auth.ValidateJWT(tokenStr, deps.Config.Auth.JWTSecret)
	if validateErr != nil {
		return nil, nil, "", validateErr
	}

	return &claims.UserID, &claims.TenantID, claims.Email, nil
}
