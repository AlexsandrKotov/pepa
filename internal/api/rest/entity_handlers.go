package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/events"
	"github.com/pepa/pepa/internal/service"
	"github.com/pepa/pepa/pkg/models"
)

func registerEntityRoutes(r *gin.RouterGroup, deps Dependencies) {
	entities := r.Group("/entities")
	{
		entities.GET("", listEntities(deps))
		entities.POST("", createEntity(deps))
		entities.GET("/:id", getEntity(deps))
		entities.PUT("/:id", updateEntity(deps))
		entities.DELETE("/:id", deleteEntity(deps))

		// Graph endpoints
		entities.GET("/:id/graph", getEntityGraph(deps))
		entities.GET("/:id/relationships", getEntityRelationships(deps))
		entities.POST("/:id/relationships", createRelationship(deps))
		entities.DELETE("/relationships/:relId", deleteRelationship(deps))

		// Sync / import endpoints
		entities.POST("/sync", syncEntities(deps))
		entities.POST("/import-discovery", importFromDiscovery(deps))
		entities.GET("/sync/status", getSyncStatus(deps))
	}

	// Entity type registry
	types := r.Group("/entity-types")
	{
		types.GET("", listEntityTypes(deps))
		types.POST("", registerEntityType(deps))
	}
}

// entityTenantScope returns the tenant scope to apply to entity queries.
// Platform admins (per the verified JWT) get cross-tenant access (uuid.Nil);
// every other caller is confined to their own tenant.
func entityTenantScope(c *gin.Context) uuid.UUID {
	if auth.IsPlatformAdmin(c) {
		return uuid.Nil
	}
	return auth.GetTenantID(c)
}

func listEntities(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		var filter models.EntityFilter
		if err := c.ShouldBindQuery(&filter); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if filter.Page < 1 {
			filter.Page = 1
		}
		if filter.PerPage < 1 {
			filter.PerPage = 20
		}

		// Scope to the caller's tenant (never taken from query params).
		filter.TenantID = entityTenantScope(c)

		result, err := deps.Repos.Entity.List(c.Request.Context(), filter)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		c.JSON(http.StatusOK, result)
	}
}

func createEntity(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req models.CreateEntityRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		tenantID := auth.GetTenantID(c)
		orgID := auth.GetOrgID(c)
		userID := auth.GetUserID(c)

		entity, err := deps.Repos.Entity.Create(c.Request.Context(), req, tenantID, orgID, userID)
		if err != nil {
			if strings.Contains(err.Error(), "duplicate key") {
				c.JSON(http.StatusConflict, gin.H{"error": "Entity with this type and external ID already exists"})
				return
			}
			respondInternalError(c, err)
			return
		}

		// Emit event
		if deps.EventBus != nil {
			_ = deps.EventBus.Publish(events.Event{
				Type:     "entity.created",
				TenantID: tenantID.String(),
				EntityID: entity.ID.String(),
				Payload: map[string]interface{}{
					"type_key": entity.TypeKey,
					"name":     entity.Name,
				},
			})
		}

		logAudit(deps, c, "create", "entity", entity.ID.String(), nil, entity)

		// Auto-evaluate scorecards for the new entity
		if deps.Services.ScorecardEval != nil {
			go func() {
				if _, err := deps.Services.ScorecardEval.EvaluateEntity(context.Background(), entity.ID, tenantID); err != nil {
					slog.Warn("auto-evaluate on create failed", "entity", entity.ID, "error", err)
				}
			}()
		}

		c.JSON(http.StatusCreated, entity)
	}
}

func getEntity(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid entity ID"})
			return
		}

		entity, err := deps.Repos.Entity.Get(c.Request.Context(), id, entityTenantScope(c))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, entity)
	}
}

func updateEntity(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid entity ID"})
			return
		}

		var req models.UpdateEntityRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		entity, err := deps.Repos.Entity.Update(c.Request.Context(), id, req, auth.GetUserID(c), entityTenantScope(c))
		if err != nil {
			respondInternalError(c, err)
			return
		}

		if deps.EventBus != nil {
			_ = deps.EventBus.Publish(events.Event{
				Type:     "entity.updated",
				EntityID: entity.ID.String(),
				Payload: map[string]interface{}{
					"name": entity.Name,
				},
			})
		}

		logAudit(deps, c, "update", "entity", entity.ID.String(), nil, entity)

		c.JSON(http.StatusOK, entity)
	}
}

func deleteEntity(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid entity ID"})
			return
		}

		if err := deps.Repos.Entity.Delete(c.Request.Context(), id, entityTenantScope(c)); err != nil {
			respondInternalError(c, err)
			return
		}

		if deps.EventBus != nil {
			_ = deps.EventBus.Publish(events.Event{
				Type:     "entity.deleted",
				EntityID: id.String(),
			})
		}

		logAudit(deps, c, "delete", "entity", id.String(), nil, nil)

		c.JSON(http.StatusOK, gin.H{"message": "entity deleted", "id": id})
	}
}

func getEntityGraph(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid entity ID"})
			return
		}

		depth, _ := strconv.Atoi(c.DefaultQuery("depth", "2"))
		if depth < 1 {
			depth = 1
		}
		if depth > 5 {
			depth = 5
		}

		result, err := deps.Repos.Entity.GetSubgraph(c.Request.Context(), id, depth, entityTenantScope(c))
		if err != nil {
			respondInternalError(c, err)
			return
		}

		c.JSON(http.StatusOK, result)
	}
}

func getEntityRelationships(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid entity ID"})
			return
		}

		rels, err := deps.Repos.Entity.GetRelationships(c.Request.Context(), id, entityTenantScope(c))
		if err != nil {
			respondInternalError(c, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{"relationships": rels})
	}
}

func listEntityTypes(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		types, err := deps.Repos.Entity.ListEntityTypes(c.Request.Context())
		if err != nil {
			respondInternalError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"entity_types": types})
	}
}

func registerEntityType(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		var et models.EntityType
		if err := c.ShouldBindJSON(&et); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if et.TypeKey == "" || et.DisplayName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "type_key and display_name are required"})
			return
		}

		if err := deps.Repos.Entity.CreateEntityType(c.Request.Context(), &et); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("create entity type: %v", err)})
			return
		}

		c.JSON(http.StatusCreated, et)
	}
}

func createRelationship(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		sourceID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid entity ID"})
			return
		}

		var req struct {
			TargetID uuid.UUID       `json:"target_id" binding:"required"`
			TypeKey  string          `json:"type_key" binding:"required"`
			Metadata json.RawMessage `json:"metadata,omitempty"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Both endpoints must be visible in the caller's tenant scope, otherwise
		// a user could link entities that belong to another tenant.
		scope := entityTenantScope(c)
		if _, err := deps.Repos.Entity.Get(c.Request.Context(), sourceID, scope); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "source entity not found"})
			return
		}
		if _, err := deps.Repos.Entity.Get(c.Request.Context(), req.TargetID, scope); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "target entity not found"})
			return
		}

		tenantID := auth.GetTenantID(c)

		rel, err := deps.Repos.Entity.CreateRelationship(c.Request.Context(), sourceID, req.TargetID, req.TypeKey, tenantID, req.Metadata)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		if deps.EventBus != nil {
			_ = deps.EventBus.Publish(events.Event{
				Type:     "relationship.created",
				TenantID: tenantID.String(),
				EntityID: sourceID.String(),
				Payload: map[string]interface{}{
					"type_key":  rel.TypeKey,
					"target_id": rel.TargetID.String(),
				},
			})
		}

		// Enqueue entity sync job
		if deps.JobQueue != nil {
			_ = deps.JobQueue.Enqueue("entity.sync", tenantID.String(), map[string]interface{}{
				"entity_id": sourceID.String(),
			})
		}

		logAudit(deps, c, "create", "relationship", sourceID.String(), nil, gin.H{"target_id": req.TargetID.String(), "type_key": req.TypeKey})

		c.JSON(http.StatusCreated, rel)
	}
}

func deleteRelationship(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		relID, err := uuid.Parse(c.Param("relId"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid relationship ID"})
			return
		}

		if err := deps.Repos.Entity.DeleteRelationship(c.Request.Context(), relID, entityTenantScope(c)); err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "delete", "relationship", relID.String(), nil, nil)

		c.JSON(http.StatusOK, gin.H{"message": "relationship deleted", "id": relID})
	}
}

// ── Sync / Import handlers ─────────────────────────────────────

// syncEntities triggers a full sync from discovery to entities.
// It discovers all services from connected clusters and creates/updates entities.
func syncEntities(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Services.EntitySync == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "entity sync service not available"})
			return
		}

		tenantID := auth.GetTenantID(c)
		orgID := auth.GetOrgID(c)

		// Discover services from all sources using the discovery cache
		discovered := discoverAllServices(c.Request.Context(), deps)

		// Convert to sync items
		items := make([]service.DiscoveredItem, 0, len(discovered))
		for _, d := range discovered {
			items = append(items, service.DiscoveredItem{
				Name:      d.Name,
				Namespace: d.Namespace,
				Cluster:   d.Cluster,
				Source:    d.Source,
				Status:    d.Status,
				Health:    d.Health,
				Image:     d.Image,
			})
		}

		result, err := deps.Services.EntitySync.SyncDiscoveryToEntities(c.Request.Context(), tenantID, orgID, items)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "sync", "entities", "", nil, gin.H{
			"created": result.Created,
			"updated": result.Updated,
			"total":   result.Total,
		})

		c.JSON(http.StatusOK, gin.H{
			"message": fmt.Sprintf("synced %d entities", result.Created+result.Updated),
			"synced":  result.Created + result.Updated,
			"created": result.Created,
			"updated": result.Updated,
		})
	}
}

// importFromDiscovery imports selected discovered services as entities.
func importFromDiscovery(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Services.EntitySync == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "entity sync service not available"})
			return
		}

		var req struct {
			Items []struct {
				Name      string `json:"name" binding:"required"`
				Namespace string `json:"namespace"`
				Cluster   string `json:"cluster"`
				Source    string `json:"source"`
			} `json:"items" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		tenantID := auth.GetTenantID(c)
		orgID := auth.GetOrgID(c)

		items := make([]service.DiscoveredItem, 0, len(req.Items))
		for _, item := range req.Items {
			items = append(items, service.DiscoveredItem{
				Name:      item.Name,
				Namespace: item.Namespace,
				Cluster:   item.Cluster,
				Source:    item.Source,
			})
		}

		result, err := deps.Services.EntitySync.SyncDiscoveryToEntities(c.Request.Context(), tenantID, orgID, items)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "import", "entities", "", nil, gin.H{
			"imported": result.Created,
		})

		c.JSON(http.StatusOK, gin.H{
			"message":  fmt.Sprintf("imported %d entities", result.Created),
			"imported": result.Created,
		})
	}
}

// getSyncStatus returns the current sync status.
func getSyncStatus(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Services.EntitySync == nil {
			c.JSON(http.StatusOK, nil)
			return
		}

		tenantID := auth.GetTenantID(c)
		status, err := deps.Services.EntitySync.GetSyncStatus(c.Request.Context(), tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		c.JSON(http.StatusOK, status)
	}
}

// discoverAllServices collects services from all discovery sources.
// This reuses the same discovery logic as the /discovery/services endpoint.
func discoverAllServices(_ context.Context, _ Dependencies) []DiscoveredService {
	// Use the discovery cache if fresh
	discoveryCacheMu.RLock()
	if time.Since(discoveryCacheTime) < discoveryCacheTTL && len(discoveryCache) > 0 {
		cached := make([]DiscoveredService, len(discoveryCache))
		copy(cached, discoveryCache)
		discoveryCacheMu.RUnlock()
		return cached
	}
	discoveryCacheMu.RUnlock()

	// If cache is stale, return empty — the frontend should call /discovery/sync first
	// or the sync handler will populate the cache via the discovery endpoint
	return nil
}
