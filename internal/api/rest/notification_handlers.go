package rest

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/repository"
	"github.com/pepa/pepa/internal/service"
)

func registerNotificationRoutes(r *gin.RouterGroup, deps Dependencies) {
	notifications := r.Group("/notifications")
	{
		// Rules CRUD
		notifications.GET("/rules", listNotificationRules(deps))
		notifications.POST("/rules", createNotificationRule(deps))
		notifications.PUT("/rules/:id", updateNotificationRule(deps))
		notifications.DELETE("/rules/:id", deleteNotificationRule(deps))
		notifications.POST("/rules/:id/test", testNotificationRule(deps))

		// History
		notifications.GET("/history", listNotificationHistory(deps))
		notifications.GET("/stats", getNotificationStats(deps))

		// Utilities
		notifications.GET("/events", listEventTypes(deps))
		notifications.POST("/preview", previewTemplate(deps))
	}
}

func listNotificationRules(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.NotificationRule == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "notification repository not available"})
			return
		}
		tenantID := auth.GetTenantID(c)
		rules, err := deps.Repos.NotificationRule.FindByTenant(c.Request.Context(), tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		if rules == nil {
			rules = []repository.NotificationRule{}
		}
		c.JSON(http.StatusOK, gin.H{"rules": rules, "total": len(rules)})
	}
}

func createNotificationRule(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.NotificationRule == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "notification repository not available"})
			return
		}
		tenantID := auth.GetTenantID(c)

		var rule repository.NotificationRule
		if err := c.ShouldBindJSON(&rule); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
			return
		}
		rule.TenantID = tenantID
		if rule.Name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
			return
		}
		if rule.ConnectionID == uuid.Nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "connection_id is required"})
			return
		}
		if rule.BodyTemplate == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "body_template is required"})
			return
		}
		if len(rule.EventTypes) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "at least one event type is required"})
			return
		}
		if rule.FormatConfig == nil {
			rule.FormatConfig = map[string]any{}
		}

		if err := deps.Repos.NotificationRule.Create(c.Request.Context(), &rule); err != nil {
			respondInternalError(c, err)
			return
		}
		c.JSON(http.StatusCreated, rule)
	}
}

func updateNotificationRule(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.NotificationRule == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "notification repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rule ID"})
			return
		}
		tenantID := auth.GetTenantID(c)

		existing, err := deps.Repos.NotificationRule.Get(c.Request.Context(), id, tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "notification rule not found"})
			return
		}

		var update repository.NotificationRule
		if err := c.ShouldBindJSON(&update); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
			return
		}

		// Apply partial update
		if update.Name != "" {
			existing.Name = update.Name
		}
		if update.Description != "" || update.Description == "" {
			existing.Description = update.Description
		}
		existing.Enabled = update.Enabled
		if len(update.EventTypes) > 0 {
			existing.EventTypes = update.EventTypes
		}
		if update.ConnectionID != uuid.Nil {
			existing.ConnectionID = update.ConnectionID
		}
		if update.BodyTemplate != "" {
			existing.BodyTemplate = update.BodyTemplate
		}
		existing.SubjectTemplate = update.SubjectTemplate
		if update.FormatConfig != nil {
			existing.FormatConfig = update.FormatConfig
		}

		if err := deps.Repos.NotificationRule.Update(c.Request.Context(), existing); err != nil {
			respondInternalError(c, err)
			return
		}
		c.JSON(http.StatusOK, existing)
	}
}

func deleteNotificationRule(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.NotificationRule == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "notification repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rule ID"})
			return
		}

		if err := deps.Repos.NotificationRule.Delete(c.Request.Context(), id); err != nil {
			respondInternalError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "deleted"})
	}
}

func testNotificationRule(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Services.NotificationDispatcher == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "notification dispatcher not available"})
			return
		}
		if deps.Repos.NotificationRule == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "notification repository not available"})
			return
		}

		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rule ID"})
			return
		}
		tenantID := auth.GetTenantID(c)

		rule, err := deps.Repos.NotificationRule.Get(c.Request.Context(), id, tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "notification rule not found"})
			return
		}

		output, err := deps.Services.NotificationDispatcher.SendTest(c.Request.Context(), rule)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "sent", "response": output})
	}
}

func listNotificationHistory(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.NotificationLog == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "notification repository not available"})
			return
		}
		tenantID := auth.GetTenantID(c)

		filter := repository.LogFilter{
			Status:    c.Query("status"),
			EventType: c.Query("event_type"),
		}
		if connID := c.Query("connection_id"); connID != "" {
			parsed, err := uuid.Parse(connID)
			if err == nil {
				filter.ConnectionID = &parsed
			}
		}
		if p := c.Query("page"); p != "" {
			filter.Page, _ = strconv.Atoi(p)
		}
		if pp := c.Query("per_page"); pp != "" {
			filter.PerPage, _ = strconv.Atoi(pp)
		}

		items, total, err := deps.Repos.NotificationLog.FindByTenant(c.Request.Context(), tenantID, filter)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		if items == nil {
			items = []repository.NotificationLog{}
		}
		c.JSON(http.StatusOK, gin.H{"items": items, "total": total})
	}
}

func getNotificationStats(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.NotificationLog == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "notification repository not available"})
			return
		}
		tenantID := auth.GetTenantID(c)

		stats, err := deps.Repos.NotificationLog.StatsByConnection(c.Request.Context(), tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		if stats == nil {
			stats = []repository.NotificationStat{}
		}
		c.JSON(http.StatusOK, gin.H{"stats": stats})
	}
}

func listEventTypes(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		categories := service.EventCategories()
		// Flatten all event types
		var allTypes []string
		for _, types := range categories {
			allTypes = append(allTypes, types...)
		}
		c.JSON(http.StatusOK, gin.H{
			"types":      allTypes,
			"categories": categories,
		})
	}
}

func previewTemplate(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			BodyTemplate string `json:"body_template"`
			EventType    string `json:"event_type"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
			return
		}
		if req.BodyTemplate == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "body_template is required"})
			return
		}
		if req.EventType == "" {
			req.EventType = "test.notification"
		}

		rendered := service.PreviewTemplate(req.BodyTemplate, req.EventType)
		c.JSON(http.StatusOK, gin.H{"rendered": rendered})
	}
}
