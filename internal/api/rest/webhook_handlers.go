package rest

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/database"
	"github.com/pepa/pepa/internal/repository"
	"github.com/pepa/pepa/internal/service"
)

// WebhookHandlers handles incoming webhooks from external CI/CD systems.
type WebhookHandlers struct {
	deps Dependencies
}

// NewWebhookHandlers creates new webhook handlers.
func NewWebhookHandlers(deps Dependencies) *WebhookHandlers {
	return &WebhookHandlers{deps: deps}
}

// gitlabPushEvent is the subset of a GitLab Push webhook payload we need.
type gitlabPushEvent struct {
	ObjectKind   string `json:"object_kind"`
	Ref          string `json:"ref"` // "refs/heads/main"
	Branch       string `json:"-"`   // extracted from Ref
	CheckoutSHA  string `json:"checkout_sha"`
	UserName     string `json:"user_name"`
	UserUsername string `json:"user_username"`
	Project      struct {
		ID        int    `json:"id"`
		PathWithNamespace string `json:"path_with_namespace"`
		WebURL    string `json:"web_url"`
	} `json:"project"`
	Commits []struct {
		ID      string `json:"id"`
		Message string `json:"message"`
	} `json:"commits"`
}

// handleGitLabPush processes a GitLab push webhook.
func (h *WebhookHandlers) handleGitLabPush() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		// Read body
		body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20)) // 1MB limit
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read body"})
			return
		}

		// Verify webhook secret if configured
		secret := c.GetHeader("X-Gitlab-Token")
		if !h.verifyWebhookSecret(secret) {
			slog.Warn("webhook: invalid secret", "remote", c.ClientIP())
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid webhook secret"})
			return
		}

		// Parse payload
		var event gitlabPushEvent
		if err := json.Unmarshal(body, &event); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
			return
		}

		// Extract branch from ref
		event.Branch = strings.TrimPrefix(event.Ref, "refs/heads/")
		if event.Branch == "" || event.Branch == event.Ref {
			c.JSON(http.StatusOK, gin.H{"message": "not a branch push, ignoring"})
			return
		}

		slog.Info("webhook: GitLab push received",
			"project", event.Project.PathWithNamespace,
			"project_id", event.Project.ID,
			"branch", event.Branch,
			"sha", event.CheckoutSHA,
			"user", event.UserName,
		)

		// Use default tenant
		tenantID := uuid.MustParse(database.DefaultTenantID)

		// Find matching auto-deploy rules
		if h.deps.Repos.AutoDeployRule == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "auto-deploy rules not configured"})
			return
		}

		projectID := fmt.Sprintf("%d", event.Project.ID)
		rules, err := h.deps.Repos.AutoDeployRule.FindMatchingRules(ctx, tenantID, event.Branch, projectID, event.Project.PathWithNamespace)
		if err != nil {
			slog.Error("webhook: failed to find matching rules", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process rules"})
			return
		}

		if len(rules) == 0 {
			c.JSON(http.StatusOK, gin.H{
				"message":    "no matching auto-deploy rules",
				"branch":     event.Branch,
				"project_id": projectID,
			})
			return
		}

		// Determine image tag
		imageTag := event.CheckoutSHA
		if len(imageTag) > 12 {
			imageTag = imageTag[:12] // short SHA
		}

		// Process rules asynchronously to avoid blocking the webhook response
		go h.processRules(context.Background(), tenantID, rules, event, imageTag)

		c.JSON(http.StatusOK, gin.H{
			"message":      "webhook accepted",
			"branch":       event.Branch,
			"matched_rules": len(rules),
			"image_tag":    imageTag,
		})
	}
}

// processRules executes auto-deploy write-backs for matched rules.
func (h *WebhookHandlers) processRules(ctx context.Context, tenantID uuid.UUID, rules []*repository.AutoDeployRule, event gitlabPushEvent, imageTag string) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	for _, rule := range rules {
		// Extract image tag based on rule configuration
		tag := resolveImageTag(imageTag, event.Branch, rule)

		// Find bindings for this environment
		if h.deps.Repos.GitOpsBinding == nil {
			slog.Warn("webhook: binding repo not available, skipping rule", "rule_id", rule.ID)
			continue
		}

		bindings, err := h.deps.Repos.GitOpsBinding.FindByEnvironment(ctx, rule.EnvironmentID, tenantID)
		if err != nil {
			slog.Error("webhook: failed to find bindings", "rule_id", rule.ID, "error", err)
			continue
		}

		if len(bindings) == 0 {
			slog.Info("webhook: no bindings for environment, skipping", "rule_id", rule.ID, "env_id", rule.EnvironmentID)
			continue
		}

		// Write back image tag for each binding
		writer := service.NewManifestWriter(h.deps.Repos.GitopsRepo, h.deps.Repos.GitOpsBinding)
		for _, binding := range bindings {
			if binding.RepoID == nil {
				continue
			}
			imageName := ""
			if rule.ImageName != nil {
				imageName = *rule.ImageName
			}

			_, writeErr := writer.WriteBack(ctx, tenantID, &service.WriteBackRequest{
				BindingID:     binding.ID,
				ImageTag:      tag,
				ImageName:     imageName,
				CommitMessage: fmt.Sprintf("auto-deploy: %s → %s (branch: %s, sha: %s)", binding.AppName, tag, event.Branch, event.CheckoutSHA[:8]),
			})
			if writeErr != nil {
				slog.Error("webhook: write-back failed",
					"rule_id", rule.ID,
					"binding_id", binding.ID,
					"app", binding.AppName,
					"error", writeErr,
				)
				continue
			}

			slog.Info("webhook: auto-deploy write-back complete",
				"rule_id", rule.ID,
				"binding_id", binding.ID,
				"app", binding.AppName,
				"image_tag", tag,
				"branch", event.Branch,
			)
		}
	}
}

// resolveImageTag determines the image tag based on the rule's image_tag_source.
func resolveImageTag(shaTag, branch string, rule *repository.AutoDeployRule) string {
	switch rule.ImageTagSource {
	case "branch_name":
		// Use branch name as tag (sanitize)
		tag := strings.ReplaceAll(branch, "/", "-")
		tag = strings.ReplaceAll(tag, "_", "-")
		return tag
	case "regex":
		if rule.ImageTagRegex != nil {
			re, err := regexp.Compile(*rule.ImageTagRegex)
			if err == nil {
				matches := re.FindStringSubmatch(branch)
				if len(matches) > 1 {
					return matches[1] // first capture group
				}
			}
		}
		return shaTag
	default:
		return shaTag
	}
}

// verifyWebhookSecret checks the webhook secret against configured values.
// If a secret is stored in settings, ALL requests must present a matching token.
// If no secret is configured yet, requests are allowed (first-run setup).
func (h *WebhookHandlers) verifyWebhookSecret(provided string) bool {
	if h.deps.Repos.Settings == nil {
		// Settings unavailable — fail closed to prevent unauthenticated deploys
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stored, err := h.deps.Repos.Settings.Get(ctx, "webhook_secret")
	if err != nil || len(stored) == 0 {
		// No secret configured — allow (first-run setup)
		return true
	}

	// A secret IS configured: require a non-empty matching token
	if provided == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), stored) == 1
}

// handleListRules returns all auto-deploy rules.
func (h *WebhookHandlers) handleListRules() gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := uuid.MustParse(database.DefaultTenantID)
		ctx := c.Request.Context()

		if h.deps.Repos.AutoDeployRule == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "auto-deploy rules not configured"})
			return
		}

		rules, err := h.deps.Repos.AutoDeployRule.List(ctx, tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"rules": rules,
			"total": len(rules),
		})
	}
}

// handleCreateRule creates a new auto-deploy rule.
func (h *WebhookHandlers) handleCreateRule() gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := uuid.MustParse(database.DefaultTenantID)
		ctx := c.Request.Context()

		if h.deps.Repos.AutoDeployRule == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "auto-deploy rules not configured"})
			return
		}

		var rule repository.AutoDeployRule
		if err := c.ShouldBindJSON(&rule); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		rule.TenantID = tenantID
		rule.Enabled = true

		if err := h.deps.Repos.AutoDeployRule.Create(ctx, &rule); err != nil {
			respondInternalError(c, err)
			return
		}

		c.JSON(http.StatusCreated, gin.H{"rule": rule})
	}
}

// handleUpdateRule updates an existing auto-deploy rule.
func (h *WebhookHandlers) handleUpdateRule() gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := uuid.MustParse(database.DefaultTenantID)
		ctx := c.Request.Context()

		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rule ID"})
			return
		}

		if h.deps.Repos.AutoDeployRule == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "auto-deploy rules not configured"})
			return
		}

		existing, err := h.deps.Repos.AutoDeployRule.Get(ctx, id, tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "rule not found"})
			return
		}

		var updates repository.AutoDeployRule
		if err := c.ShouldBindJSON(&updates); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Merge updates
		updates.ID = existing.ID
		updates.TenantID = existing.TenantID
		updates.CreatedAt = existing.CreatedAt
		updates.CreatedBy = existing.CreatedBy

		if err := h.deps.Repos.AutoDeployRule.Update(ctx, &updates); err != nil {
			respondInternalError(c, err)
			return
		}

		updated, _ := h.deps.Repos.AutoDeployRule.Get(ctx, id, tenantID)
		c.JSON(http.StatusOK, gin.H{"rule": updated})
	}
}

// handleDeleteRule deletes an auto-deploy rule.
func (h *WebhookHandlers) handleDeleteRule() gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := uuid.MustParse(database.DefaultTenantID)
		ctx := c.Request.Context()

		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rule ID"})
			return
		}

		if h.deps.Repos.AutoDeployRule == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "auto-deploy rules not configured"})
			return
		}

		if err := h.deps.Repos.AutoDeployRule.Delete(ctx, id, tenantID); err != nil {
			respondInternalError(c, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "rule deleted"})
	}
}

// registerAutoDeployRoutes registers authenticated auto-deploy rule CRUD routes.
func registerAutoDeployRoutes(v1 *gin.RouterGroup, deps Dependencies) {
	h := NewWebhookHandlers(deps)
	rules := v1.Group("/auto-deploy-rules")
	{
		rules.GET("", h.handleListRules())
		rules.POST("", h.handleCreateRule())
		rules.PUT("/:id", h.handleUpdateRule())
		rules.DELETE("/:id", h.handleDeleteRule())
	}
}
