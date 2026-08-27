package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	pepacrypto "github.com/pepa/pepa/internal/crypto"
	vaultclient "github.com/pepa/pepa/internal/vault"
)

// VaultConfig holds the vault backend configuration stored in settings.
type VaultConfig struct {
	Mode      string `json:"mode"`       // "local" or "remote"
	Address   string `json:"address"`    // remote Vault address
	Token     string `json:"token"`      // remote Vault token
	MountPath string `json:"mount_path"` // KV engine mount, default "secret"
}

// getVaultConfig reads the vault configuration from settings.
func getVaultConfig(deps Dependencies, ctx context.Context) VaultConfig {
	cfg := VaultConfig{Mode: "local", MountPath: "secret"}
	if deps.Repos.Settings == nil {
		return cfg
	}
	raw, err := deps.Repos.Settings.Get(ctx, "vault")
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(raw, &cfg)
	// The remote Vault token is stored encrypted at rest; decrypt for use.
	if cfg.Token != "" {
		if dec, derr := pepacrypto.Decrypt(cfg.Token); derr == nil {
			cfg.Token = dec
		} else {
			cfg.Token = ""
		}
	}
	if cfg.Mode == "" {
		cfg.Mode = "local"
	}
	if cfg.MountPath == "" {
		cfg.MountPath = "secret"
	}
	return cfg
}

// newRemoteClient creates a HashiCorp Vault client from config. Returns nil if not configured.
func newRemoteClient(cfg VaultConfig) *vaultclient.Client {
	if cfg.Mode != "remote" || cfg.Address == "" {
		return nil
	}
	client, err := vaultclient.NewClient(vaultclient.Config{
		Address:   cfg.Address,
		Token:     cfg.Token,
		MountPath: cfg.MountPath,
	})
	if err != nil {
		return nil
	}
	return client
}

func registerVaultRoutes(r *gin.RouterGroup, deps Dependencies) {
	// Vault write operations get stricter rate limiting (30 req/min per IP)
	vaultWriteLimiter := newRateLimiter(30, time.Minute)
	// Vault read operations get a moderate rate limit (120 req/min per IP)
	vaultReadLimiter := newRateLimiter(120, time.Minute)

	v := r.Group("/vault")
	{
		// Get vault configuration
		v.GET("/config", vaultReadLimiter.Middleware(), func(c *gin.Context) {
			if !vaultCheckRBAC(c, deps, "vault", "read") {
				return
			}
			cfg := getVaultConfig(deps, c.Request.Context())
			// Mask sensitive fields
			masked := cfg
			if masked.Token != "" {
				if len(masked.Token) > 12 {
					masked.Token = masked.Token[:4] + "••••" + masked.Token[len(masked.Token)-4:]
				} else {
					masked.Token = "••••••••"
				}
			}
			c.JSON(http.StatusOK, gin.H{"config": masked})
		})

		// Save vault configuration
		v.POST("/config", vaultWriteLimiter.Middleware(), func(c *gin.Context) {
			if !vaultCheckRBAC(c, deps, "vault", "update") {
				return
			}
			var req VaultConfig
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			if req.Mode != "local" && req.Mode != "remote" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "mode must be 'local' or 'remote'"})
				return
			}

			// If switching to remote, validate the connection
			if req.Mode == "remote" {
				if req.Address == "" {
					c.JSON(http.StatusBadRequest, gin.H{"error": "address is required for remote mode"})
					return
				}
				client, err := vaultclient.NewClient(vaultclient.Config{
					Address:   req.Address,
					Token:     req.Token,
					MountPath: req.MountPath,
				})
				if err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				// Test connectivity
				ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
				defer cancel()
				_, err = client.Health(ctx)
				if err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "cannot connect to Vault: " + err.Error()})
					return
				}
			}

			// Preserve existing token if not provided in update
			existing := getVaultConfig(deps, c.Request.Context())
			if req.Mode == "remote" && req.Token == "" && existing.Token != "" {
				req.Token = existing.Token
			}

			// Encrypt the token before persisting (at-rest protection).
			if req.Token != "" {
				enc, err := pepacrypto.Encrypt(req.Token)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to encrypt vault token"})
					return
				}
				req.Token = enc
			}

			data, _ := json.Marshal(req)
			if err := deps.Repos.Settings.Set(c.Request.Context(), "vault", data); err != nil {
				respondInternalError(c, err)
				return
			}

			logAudit(deps, c, "update", "vault_config", "vault", nil, map[string]string{
				"mode": req.Mode,
			})

			c.JSON(http.StatusOK, gin.H{"message": "vault configuration saved", "mode": req.Mode})
		})

		// List secret paths
		v.GET("/paths", vaultReadLimiter.Middleware(), func(c *gin.Context) {
			if !vaultCheckRBAC(c, deps, "vault", "read") {
				return
			}
			prefix := c.Query("prefix")
			tenantID := auth.GetTenantID(c)
			cfg := getVaultConfig(deps, c.Request.Context())

			if cfg.Mode == "remote" {
				client := newRemoteClient(cfg)
				if client == nil {
					c.JSON(http.StatusServiceUnavailable, gin.H{"error": "remote Vault not configured"})
					return
				}
				keys, err := client.ListSecrets(c.Request.Context(), prefix)
				if err != nil {
					respondInternalError(c, err)
					return
				}
				paths := make([]map[string]interface{}, 0, len(keys))
				for _, k := range keys {
					hasChildren := strings.HasSuffix(k, "/")
					fullPath := k
					if prefix != "" {
						fullPath = strings.TrimSuffix(prefix, "/") + "/" + k
					}
					if hasChildren {
						fullPath = strings.TrimSuffix(fullPath, "/")
					}
					paths = append(paths, map[string]interface{}{
						"path":         fullPath,
						"type":         "kv",
						"has_children": hasChildren,
					})
				}
				// Apply path-based ACL filtering
				paths = filterPathsByACL(deps, c, paths)
				c.JSON(http.StatusOK, gin.H{"paths": paths, "total": len(paths), "mode": "remote"})
				return
			}

			// Local mode
			paths, err := deps.Repos.Vault.ListPaths(c.Request.Context(), tenantID, prefix)
			if err != nil {
				respondInternalError(c, err)
				return
			}
			if paths == nil {
				paths = []map[string]interface{}{}
			}
			// Apply path-based ACL filtering
			paths = filterPathsByACL(deps, c, paths)
			c.JSON(http.StatusOK, gin.H{"paths": paths, "total": len(paths), "mode": "local"})
		})

		// Get a secret
		v.GET("/secrets/*path", vaultReadLimiter.Middleware(), func(c *gin.Context) {
			if !vaultCheckRBAC(c, deps, "vault", "read") {
				return
			}
			path := strings.TrimPrefix(c.Param("path"), "/")
			if path == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
				return
			}
			if err := validateVaultPath(path); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			cfg := getVaultConfig(deps, c.Request.Context())

			if cfg.Mode == "remote" {
				client := newRemoteClient(cfg)
				if client == nil {
					c.JSON(http.StatusServiceUnavailable, gin.H{"error": "remote Vault not configured"})
					return
				}
				// Check path-based ACL
				if !checkVaultPathAccess(deps, c, path, "read") {
					c.JSON(http.StatusForbidden, gin.H{"error": "access denied to this secret path"})
					return
				}
				secret, err := client.GetSecret(c.Request.Context(), path)
				if err != nil {
					if strings.Contains(err.Error(), "not found") {
						c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
						return
					}
					respondInternalError(c, err)
					return
				}
				c.JSON(http.StatusOK, gin.H{
					"path": path,
					"secret": gin.H{
						"data":     secret.Data,
						"metadata": secret.Metadata,
					},
					"mode": "remote",
				})
				return
			}

			// Local mode
			tenantID := auth.GetTenantID(c)
			// Check path-based ACL
			if !checkVaultPathAccess(deps, c, path, "read") {
				c.JSON(http.StatusForbidden, gin.H{"error": "access denied to this secret path"})
				return
			}
			secret, err := deps.Repos.Vault.Get(c.Request.Context(), tenantID, path)
			if err != nil {
				if strings.Contains(err.Error(), "not found") {
					c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
					return
				}
				respondInternalError(c, err)
				return
			}
			// Audit log secret reads (path only, never the value)
			logAudit(deps, c, "read", "vault_secret", path, nil, map[string]string{
				"path": path,
				"mode": "local",
			})
			c.JSON(http.StatusOK, gin.H{
				"path": secret.Path,
				"secret": gin.H{
					"data":     secret.Data,
					"metadata": secret.Metadata,
				},
				"mode": "local",
			})
		})

		// Create/update a secret
		v.POST("/secrets/*path", vaultWriteLimiter.Middleware(), func(c *gin.Context) {
			if !vaultCheckRBAC(c, deps, "vault", "create") {
				return
			}
			path := strings.TrimPrefix(c.Param("path"), "/")
			if path == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
				return
			}

			// Validate secret path
			if err := validateVaultPath(path); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}

			var req struct {
				Data map[string]string `json:"data" binding:"required"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}

			// Validate keys and values
			if err := validateVaultData(req.Data); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}

			cfg := getVaultConfig(deps, c.Request.Context())

			if cfg.Mode == "remote" {
				client := newRemoteClient(cfg)
				if client == nil {
					c.JSON(http.StatusServiceUnavailable, gin.H{"error": "remote Vault not configured"})
					return
				}
				// Check path-based ACL for create
				if !checkVaultPathAccess(deps, c, path, "create") {
					c.JSON(http.StatusForbidden, gin.H{"error": "access denied to write to this secret path"})
					return
				}
				meta, err := client.WriteSecret(c.Request.Context(), path, req.Data)
				if err != nil {
					respondInternalError(c, err)
					return
				}
				logAudit(deps, c, "write", "vault_secret", path, nil, map[string]string{
					"path": path,
					"keys": strings.Join(mapKeys(req.Data), ","),
					"mode": "remote",
				})
				c.JSON(http.StatusOK, gin.H{
					"path":    path,
					"status":  "written",
					"version": meta.Version,
					"mode":    "remote",
				})
				return
			}

			// Local mode
			tenantID := auth.GetTenantID(c)
			createdBy := auth.GetEmail(c)
			ownerID := auth.GetUserID(c)
			// Check path-based ACL for create
			if !checkVaultPathAccess(deps, c, path, "create") {
				c.JSON(http.StatusForbidden, gin.H{"error": "access denied to write to this secret path"})
				return
			}
			secret, err := deps.Repos.Vault.Set(c.Request.Context(), tenantID, path, req.Data, createdBy, ownerID)
			if err != nil {
				respondInternalError(c, err)
				return
			}
			logAudit(deps, c, "write", "vault_secret", path, nil, map[string]string{
				"path": path,
				"keys": strings.Join(mapKeys(req.Data), ","),
				"mode": "local",
			})
			c.JSON(http.StatusOK, gin.H{
				"path":    secret.Path,
				"status":  "written",
				"version": secret.Metadata.Version,
				"mode":    "local",
			})
		})

		// Delete a secret
		v.DELETE("/secrets/*path", vaultWriteLimiter.Middleware(), func(c *gin.Context) {
			if !vaultCheckRBAC(c, deps, "vault", "delete") {
				return
			}
			path := strings.TrimPrefix(c.Param("path"), "/")
			if path == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
				return
			}
			if err := validateVaultPath(path); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}

			cfg := getVaultConfig(deps, c.Request.Context())

			if cfg.Mode == "remote" {
				client := newRemoteClient(cfg)
				if client == nil {
					c.JSON(http.StatusServiceUnavailable, gin.H{"error": "remote Vault not configured"})
					return
				}
				// Check path-based ACL for delete
				if !checkVaultPathAccess(deps, c, path, "delete") {
					c.JSON(http.StatusForbidden, gin.H{"error": "access denied to delete this secret path"})
					return
				}
				if err := client.DeleteSecret(c.Request.Context(), path); err != nil {
					respondInternalError(c, err)
					return
				}
				logAudit(deps, c, "delete", "vault_secret", path, nil, map[string]string{"mode": "remote"})
				c.JSON(http.StatusOK, gin.H{"status": "deleted", "path": path, "mode": "remote"})
				return
			}

			// Local mode
			tenantID := auth.GetTenantID(c)
			// Check path-based ACL for delete
			if !checkVaultPathAccess(deps, c, path, "delete") {
				c.JSON(http.StatusForbidden, gin.H{"error": "access denied to delete this secret path"})
				return
			}
			if err := deps.Repos.Vault.Delete(c.Request.Context(), tenantID, path); err != nil {
				respondInternalError(c, err)
				return
			}
			logAudit(deps, c, "delete", "vault_secret", path, nil, map[string]string{"mode": "local"})
			c.JSON(http.StatusOK, gin.H{"status": "deleted", "path": path, "mode": "local"})
		})

		// List secret engines
		v.GET("/engines", vaultReadLimiter.Middleware(), func(c *gin.Context) {
			if !vaultCheckRBAC(c, deps, "vault", "read") {
				return
			}
			cfg := getVaultConfig(deps, c.Request.Context())

			if cfg.Mode == "remote" {
				client := newRemoteClient(cfg)
				if client == nil {
					c.JSON(http.StatusServiceUnavailable, gin.H{"error": "remote Vault not configured"})
					return
				}
				engines, err := client.ListEngines(c.Request.Context())
				if err != nil {
					respondInternalError(c, err)
					return
				}
				c.JSON(http.StatusOK, gin.H{"engines": engines, "total": len(engines), "mode": "remote"})
				return
			}

			// Local mode
			engines, err := deps.Repos.Vault.Engines(c.Request.Context())
			if err != nil {
				respondInternalError(c, err)
				return
			}
			c.JSON(http.StatusOK, gin.H{"engines": engines, "total": len(engines), "mode": "local"})
		})

		// Test connection
		v.POST("/test-connection", func(c *gin.Context) {
			var req struct {
				Address   string `json:"address"`
				Token     string `json:"token"`
				MountPath string `json:"mount_path"`
			}
			_ = c.ShouldBindJSON(&req)

			// If address provided, test remote Vault
			if req.Address != "" {
				client, err := vaultclient.NewClient(vaultclient.Config{
					Address:   req.Address,
					Token:     req.Token,
					MountPath: req.MountPath,
				})
				if err != nil {
					c.JSON(http.StatusBadRequest, gin.H{
						"status":  "error",
						"message": err.Error(),
						"type":    "remote",
					})
					return
				}
				ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
				defer cancel()
				health, err := client.Health(ctx)
				if err != nil {
					c.JSON(http.StatusOK, gin.H{
						"status":  "error",
						"message": err.Error(),
						"type":    "remote",
						"name":    "hashicorp-vault",
					})
					return
				}
				c.JSON(http.StatusOK, gin.H{
					"status":  "ok",
					"message": "Connected to HashiCorp Vault successfully",
					"type":    "remote",
					"name":    "hashicorp-vault",
					"details": health,
				})
				return
			}

			// Test local
			c.JSON(http.StatusOK, gin.H{
				"status":  "ok",
				"message": "Built-in KV secret engine is operational",
				"type":    "local",
				"name":    "pepa-vault",
				"details": map[string]interface{}{
					"backend":        "postgresql",
					"encryption":     "aes-256-gcm",
					"key_derivation": "argon2id",
				},
			})
		})

		// Vault security status
		v.GET("/status", vaultReadLimiter.Middleware(), func(c *gin.Context) {
			if !vaultCheckRBAC(c, deps, "vault", "read") {
				return
			}
			cfg := getVaultConfig(deps, c.Request.Context())

			if cfg.Mode == "local" {
				tenantID := auth.GetTenantID(c)
				status, err := deps.Repos.Vault.Status(c.Request.Context(), tenantID)
				if err != nil {
					respondInternalError(c, err)
					return
				}
				c.JSON(http.StatusOK, gin.H{
					"status": status,
					"mode":   "local",
				})
				return
			}

			// For remote, return basic info
			c.JSON(http.StatusOK, gin.H{
				"status": map[string]interface{}{
					"encryption_type": "managed by remote",
					"key_derivation":  "managed by remote",
					"per_path_keys":   true,
				},
				"mode": cfg.Mode,
			})
		})

		// Key rotation: re-encrypt all secrets with current key
		v.POST("/rotate", vaultWriteLimiter.Middleware(), func(c *gin.Context) {
			cfg := getVaultConfig(deps, c.Request.Context())

			if cfg.Mode != "local" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "key rotation is only available in local mode"})
				return
			}

			rotated, errs, err := deps.Repos.Vault.RotateAll(c.Request.Context(), auth.GetTenantID(c))
			if err != nil {
				respondInternalError(c, err)
				return
			}

			logAudit(deps, c, "rotate", "vault_keys", "all", nil, map[string]string{
				"rotated": fmt.Sprintf("%d", rotated),
				"errors":  fmt.Sprintf("%d", len(errs)),
			})

			c.JSON(http.StatusOK, gin.H{
				"rotated": rotated,
				"errors":  errs,
				"message": fmt.Sprintf("Re-encrypted %d secrets with Argon2id per-path keys", rotated),
			})
		})
	}

	// Register ACL management routes (uses the same rate limiters)
	registerVaultACLRoutesWithLimiters(v, vaultReadLimiter, vaultWriteLimiter, deps)
}

func mapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// vaultCheckRBAC checks if the current user has permission for a vault operation.
// Vault holds secrets, so this fails CLOSED: if the RBAC engine is unavailable
// or the request has no authenticated user, the operation is denied.
// Returns true if allowed, false if the request was already aborted with a response.
func vaultCheckRBAC(c *gin.Context, deps Dependencies, resource, action string) bool {
	if deps.RBAC == nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "authorization unavailable"})
		return false
	}
	userID := auth.GetUserID(c)
	if userID == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
		return false
	}
	tenantID := auth.GetTenantID(c)
	allowed, err := deps.RBAC.CheckPermission(c.Request.Context(), tenantID, *userID, resource, action)
	if err != nil {
		respondInternalError(c, err)
		return false
	}
	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{"error": fmt.Sprintf("permission denied: %s:%s", resource, action)})
		return false
	}
	return true
}

// validateVaultPath checks a secret path for safety.
func validateVaultPath(path string) error {
	if len(path) > 512 {
		return fmt.Errorf("path too long (max 512 characters)")
	}
	if strings.Contains(path, "..") {
		return fmt.Errorf("path must not contain '..'")
	}
	if strings.HasPrefix(path, "/") || strings.HasSuffix(path, "/") {
		return fmt.Errorf("path must not start or end with '/'")
	}
	for _, r := range path {
		if unicode.IsControl(r) {
			return fmt.Errorf("path must not contain control characters")
		}
	}
	return nil
}

// validateVaultData checks secret keys and values for safety.
func validateVaultData(data map[string]string) error {
	for key, value := range data {
		// Validate key format: alphanumeric, dashes, underscores, dots only
		for _, r := range key {
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' && r != '.' {
				return fmt.Errorf("invalid key %q: only alphanumeric, dash, underscore, and dot allowed", key)
			}
		}
		// Validate value size: max 100KB
		if len(value) > 100*1024 {
			return fmt.Errorf("value for key %q exceeds maximum size (100KB)", key)
		}
	}
	return nil
}

// ── Vault ACL helpers ────────────────────────────────────────────────────────

// VaultACLEntry represents a path-based access control entry.
type VaultACLEntry struct {
	ID         string     `json:"id"`
	PathPrefix string     `json:"path_prefix"`
	UserID     *string    `json:"user_id,omitempty"`
	TeamID     *string    `json:"team_id,omitempty"`
	UserName   string     `json:"user_name,omitempty"`
	TeamName   string     `json:"team_name,omitempty"`
	CanRead    bool       `json:"can_read"`
	CanCreate  bool       `json:"can_create"`
	CanDelete  bool       `json:"can_delete"`
	CreatedBy  *string    `json:"created_by,omitempty"`
	CreatedAt  string     `json:"created_at"`
}

// checkVaultPathAccess returns true if the user may access the given path.
// Access is granted when:
//  1. The user is a Super Admin (bypasses all ACL).
//  2. The user owns at least one secret at the path or a parent prefix (owner has full access).
//     For create, parent-prefix ownership is checked so owners can add new secrets under their paths.
//     For read/delete, exact-path ownership is checked.
//  3. An explicit ACL entry grants the requested action to the user or their team.
//
// Default-deny: if none of the above apply, access is denied.
func checkVaultPathAccess(deps Dependencies, c *gin.Context, path, action string) bool {
	userID := auth.GetUserID(c)
	if userID == nil {
		return false // fail closed if somehow reached without a user
	}
	tenantID := auth.GetTenantID(c)
	ctx := c.Request.Context()

	// Super Admins bypass ACL
	for _, r := range auth.GetRoles(c) {
		if r == "super_admin" {
			return true
		}
	}

	// Collect all prefixes of the path, e.g. "a/b/c" → ["a", "a/b", "a/b/c"]
	parts := strings.Split(path, "/")
	prefixes := make([]string, 0, len(parts))
	for i := range parts {
		prefixes = append(prefixes, strings.Join(parts[:i+1], "/"))
	}

	// For create, check ownership of PARENT prefixes (not the exact path, which
	// doesn't exist yet). For read/delete, check the exact path only.
	// This lets an owner of "team-a/secret1" create "team-a/new-secret".
	var ownerCheckPaths []string
	if action == "create" && len(parts) > 1 {
		ownerCheckPaths = prefixes[:len(prefixes)-1] // parent prefixes only
	} else {
		ownerCheckPaths = prefixes
	}

	if len(ownerCheckPaths) > 0 {
		ownerCount, err := deps.Repos.VaultConfig.CountSecretOwner(ctx, tenantID, *userID, ownerCheckPaths)
		if err != nil {
			log.Printf("[vault-acl] owner count error: %v", err)
		} else if ownerCount > 0 {
			return true // user owns a secret at this path or parent → full access
		}
	}

	// For create: if no ACL entries exist for ANY prefix of this path, allow it.
	// Users should be able to create new secrets freely; restrictions only apply
	// once explicit ACL rules are in place.
	if action == "create" {
		aclCount, err := deps.Repos.VaultConfig.CountACLEntries(ctx, tenantID, prefixes)
		if err != nil {
			log.Printf("[vault-acl] acl count error: %v", err)
			return false
		}
		if aclCount == 0 {
			return true // no ACL rules for this path → allow creation (RBAC already checked)
		}
	}

	// Query ACL entries that match any of these prefixes.
	aclEntries, err := deps.Repos.VaultConfig.ListACLEntriesForPaths(ctx, tenantID, prefixes)
	if err != nil {
		log.Printf("[vault-acl] query error: %v", err)
		return false
	}

	for _, acl := range aclEntries {
		// Check user match
		if acl.UserID != nil && acl.UserID.String() == userID.String() {
			if action == "read" && acl.CanRead {
				return true
			}
			if action == "create" && acl.CanCreate {
				return true
			}
			if action == "delete" && acl.CanDelete {
				return true
			}
		}
	}

	// Check team-based entries
	hasAccess, err := deps.Repos.VaultConfig.CheckTeamACLAccess(ctx, tenantID, *userID, prefixes, action)
	if err != nil {
		log.Printf("[vault-acl] team query error: %v", err)
		return false
	}
	if hasAccess {
		return true
	}

	// No owner match and no ACL grant → denied
	log.Printf("[vault-acl] DENIED: user=%s path=%q action=%s prefixes=%v", userID.String(), path, action, prefixes)
	return false
}

// filterPathsByACL removes paths the user cannot access.
// Access is granted when:
//  1. The user is a Super Admin (sees all paths).
//  2. The user owns the path (owner_id matches).
//  3. An explicit ACL entry grants read access to the user or their team.
//
// Default-deny: paths without an owner match or ACL grant are hidden.
func filterPathsByACL(deps Dependencies, c *gin.Context, paths []map[string]interface{}) []map[string]interface{} {
	userID := auth.GetUserID(c)
	if userID == nil {
		return paths // dev mode: no filtering
	}
	// Super Admins see everything
	for _, r := range auth.GetRoles(c) {
		if r == "super_admin" {
			return paths
		}
	}
	tenantID := auth.GetTenantID(c)
	ctx := c.Request.Context()

	// Load all ACL entries for this tenant once.
	rows, err := deps.DB.Pool.Query(ctx, `
		SELECT path_prefix, user_id, team_id, can_read FROM vault_acl WHERE tenant_id = $1
	`, tenantID)
	if err != nil {
		log.Printf("[vault-acl] filter query error: %v", err)
		return nil
	}
	defer rows.Close()

	var acls []aclRow
	for rows.Next() {
		var r aclRow
		if err := rows.Scan(&r.pathPrefix, &r.userID, &r.teamID, &r.canRead); err != nil {
			continue
		}
		acls = append(acls, r)
	}

	// Load user's team memberships.
	teamRows, err := deps.DB.Pool.Query(ctx, `
		SELECT team_id FROM team_memberships WHERE user_id = $1
	`, *userID)
	if err != nil {
		log.Printf("[vault-acl] team membership query error: %v", err)
		return nil
	}
	defer teamRows.Close()
	userTeams := make(map[string]bool)
	for teamRows.Next() {
		var tid string
		if err := teamRows.Scan(&tid); err == nil {
			userTeams[tid] = true
		}
	}

	filtered := make([]map[string]interface{}, 0, len(paths))
	for _, p := range paths {
		pathStr, _ := p["path"].(string)
		if pathStr == "" {
			continue
		}
		// Check ownership: path entries carry owner_id from ListPaths
		ownerIDStr, _ := p["owner_id"].(string)
		isOwner := ownerIDStr != "" && ownerIDStr == userID.String()
		if isOwner {
			filtered = append(filtered, p)
			continue
		}
		// Check ACL grant
		if pathHasReadAccess(pathStr, userID.String(), userTeams, acls) {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

// pathHasReadAccess checks if a path is readable given the ACL entries.
// Default-deny: returns true only when an explicit ACL entry grants read access.
func pathHasReadAccess(path, uid string, userTeams map[string]bool, acls []aclRow) bool {
	// Check all prefixes of the path.
	parts := strings.Split(path, "/")
	for i := range parts {
		prefix := strings.Join(parts[:i+1], "/")
		for _, a := range acls {
			if a.pathPrefix == prefix {
				if a.userID != nil && *a.userID == uid && a.canRead {
					return true
				}
				if a.teamID != nil && userTeams[*a.teamID] && a.canRead {
					return true
				}
			}
		}
	}
	// Default-deny: no explicit grant → not visible.
	return false
}

type aclRow struct {
	pathPrefix string
	userID     *string
	teamID     *string
	canRead    bool
}

// ── Vault ACL routes ─────────────────────────────────────────────────────────

func registerVaultACLRoutesWithLimiters(r *gin.RouterGroup, readLimiter, writeLimiter *rateLimiter, deps Dependencies) {
	acl := r.Group("/acl")
	{
		// List ACL entries
		acl.GET("", readLimiter.Middleware(), func(c *gin.Context) {
			if !vaultCheckRBAC(c, deps, "vault", "read") {
				return
			}
			tenantID := auth.GetTenantID(c)
			rows, err := deps.DB.Pool.Query(c.Request.Context(), `
				SELECT va.id, va.path_prefix, va.user_id, va.team_id,
				       COALESCE(u.name,''), COALESCE(t.name,''),
				       va.can_read, va.can_create, va.can_delete,
				       va.created_by, va.created_at
				FROM vault_acl va
				LEFT JOIN users u ON u.id = va.user_id
				LEFT JOIN teams t ON t.id = va.team_id
				WHERE va.tenant_id = $1
				ORDER BY va.path_prefix, COALESCE(u.name, t.name)
			`, tenantID)
			if err != nil {
				respondInternalError(c, err)
				return
			}
			defer rows.Close()

			entries := make([]VaultACLEntry, 0)
			for rows.Next() {
				var e VaultACLEntry
				var userID, teamID, createdBy *string
				var createdAt time.Time
				if err := rows.Scan(&e.ID, &e.PathPrefix, &userID, &teamID,
					&e.UserName, &e.TeamName,
					&e.CanRead, &e.CanCreate, &e.CanDelete,
					&createdBy, &createdAt); err != nil {
					respondInternalError(c, err)
					return
				}
				e.UserID = userID
				e.TeamID = teamID
				e.CreatedBy = createdBy
				e.CreatedAt = createdAt.Format("2006-01-02T15:04:05Z")
				entries = append(entries, e)
			}
			c.JSON(http.StatusOK, gin.H{"entries": entries, "total": len(entries)})
		})

		// Create ACL entry
		acl.POST("", writeLimiter.Middleware(), func(c *gin.Context) {
			if !vaultCheckRBAC(c, deps, "vault", "create") {
				return
			}
			var req struct {
				PathPrefix string  `json:"path_prefix" binding:"required"`
				UserID     *string `json:"user_id"`
				TeamID     *string `json:"team_id"`
				CanRead    bool    `json:"can_read"`
				CanCreate  bool    `json:"can_create"`
				CanDelete  bool    `json:"can_delete"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "path_prefix is required"})
				return
			}
			if req.UserID == nil && req.TeamID == nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "user_id or team_id is required"})
				return
			}
			if err := validateVaultPath(req.PathPrefix); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid path_prefix: " + err.Error()})
				return
			}

			tenantID := auth.GetTenantID(c)
			createdBy := auth.GetUserID(c)
			var userID, teamID *uuid.UUID
			if req.UserID != nil {
				if id, err := uuid.Parse(*req.UserID); err == nil {
					userID = &id
				} else {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
					return
				}
			}
			if req.TeamID != nil {
				if id, err := uuid.Parse(*req.TeamID); err == nil {
					teamID = &id
				} else {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid team_id"})
					return
				}
			}

			id := uuid.New()
			_, err := deps.DB.Pool.Exec(c.Request.Context(), `
				INSERT INTO vault_acl (id, tenant_id, path_prefix, user_id, team_id, can_read, can_create, can_delete, created_by)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			`, id, tenantID, req.PathPrefix, userID, teamID, req.CanRead, req.CanCreate, req.CanDelete, createdBy)
			if err != nil {
				respondInternalError(c, err)
				return
			}

			logAudit(deps, c, "create", "vault_acl", id.String(), nil, gin.H{
				"path_prefix": req.PathPrefix,
			})
			c.JSON(http.StatusCreated, gin.H{"id": id, "message": "ACL entry created"})
		})

		// Delete ACL entry — only the original creator or a Super Admin may delete.
		acl.DELETE("/:id", writeLimiter.Middleware(), func(c *gin.Context) {
			if !vaultCheckRBAC(c, deps, "vault", "delete") {
				return
			}
			id, err := uuid.Parse(c.Param("id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ACL entry ID"})
				return
			}
			tenantID := auth.GetTenantID(c)
			userID := auth.GetUserID(c)

			// Look up the entry to check ownership
			var createdBy *string
			err = deps.DB.Pool.QueryRow(c.Request.Context(), `
				SELECT created_by FROM vault_acl WHERE id = $1 AND tenant_id = $2
			`, id, tenantID).Scan(&createdBy)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "ACL entry not found"})
				return
			}

			// Super Admins bypass ownership check
			isSuperAdmin := false
			for _, r := range auth.GetRoles(c) {
				if r == "super_admin" {
					isSuperAdmin = true
					break
				}
			}

			// Only the creator or a Super Admin can delete
			if !isSuperAdmin && (createdBy == nil || *createdBy != userID.String()) {
				c.JSON(http.StatusForbidden, gin.H{"error": "only the rule creator can delete this entry"})
				return
			}

			_, err = deps.DB.Pool.Exec(c.Request.Context(), `
				DELETE FROM vault_acl WHERE id = $1 AND tenant_id = $2
			`, id, tenantID)
			if err != nil {
				respondInternalError(c, err)
				return
			}
			logAudit(deps, c, "delete", "vault_acl", id.String(), nil, nil)
			c.JSON(http.StatusOK, gin.H{"message": "ACL entry deleted"})
		})
	}
}
