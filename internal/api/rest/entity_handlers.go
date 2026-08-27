package rest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/events"
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
