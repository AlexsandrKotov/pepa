package rest

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
)

// registerBlueprintGroupRoutes registers CRUD routes for blueprint groups.
func registerBlueprintGroupRoutes(r *gin.RouterGroup, deps Dependencies) {
	groups := r.Group("/blueprint-groups")
	{
		groups.GET("", listBlueprintGroups(deps))
		groups.POST("", createBlueprintGroup(deps))
		groups.PUT("/:id", updateBlueprintGroup(deps))
		groups.DELETE("/:id", deleteBlueprintGroup(deps))
		groups.PUT("/:id/reorder", reorderBlueprintGroup(deps))
	}
}

type blueprintGroupRow struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Position    int                   `json:"position"`
	Blueprints  []serviceBlueprintRow `json:"blueprints"`
	CreatedAt   time.Time             `json:"created_at"`
}

func listBlueprintGroups(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := auth.GetTenantID(c)

		// Fetch groups
		rows, err := deps.DB.Pool.Query(c.Request.Context(), `
			SELECT id, name, COALESCE(description,''), COALESCE(position,0), created_at
			FROM blueprint_groups
			WHERE tenant_id = $1
			ORDER BY position ASC, created_at ASC
		`, tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		defer rows.Close()

		var groups []blueprintGroupRow
		for rows.Next() {
			var g blueprintGroupRow
			if err := rows.Scan(&g.ID, &g.Name, &g.Description, &g.Position, &g.CreatedAt); err != nil {
				respondInternalError(c, err)
				return
			}
			g.Blueprints = []serviceBlueprintRow{}
			groups = append(groups, g)
		}
		if groups == nil {
			groups = []blueprintGroupRow{}
		}

		// Fetch member blueprints for each group
		for i, g := range groups {
			bpRows, err := deps.DB.Pool.Query(c.Request.Context(), `
				SELECT id, name, COALESCE(description,''), source_type,
				       helm_repo_id, COALESCE(image,''), COALESCE(chart_url,''),
				       COALESCE(chart_name,''), COALESCE(chart_version,''),
				       COALESCE(chart_path,''), COALESCE(namespace,'default'),
				       COALESCE(values_yaml,''), COALESCE(cpu,'100m'),
				       COALESCE(memory,'128Mi'), COALESCE(replicas,1),
				       COALESCE(ports,'{}'), COALESCE(category,'general'),
				       group_id, COALESCE(group_position,0),
				       COALESCE(compose_yaml,''),
				       created_at
				FROM service_blueprints
				WHERE group_id = $1
				ORDER BY group_position ASC, created_at ASC
			`, g.ID)
			if err != nil {
				respondInternalError(c, err)
				return
			}
			var bps []serviceBlueprintRow
			for bpRows.Next() {
				var bp serviceBlueprintRow
				var helmRepoID *string
				if err := bpRows.Scan(
					&bp.ID, &bp.Name, &bp.Description, &bp.SourceType,
					&helmRepoID, &bp.Image, &bp.ChartURL,
					&bp.ChartName, &bp.ChartVersion, &bp.ChartPath,
					&bp.Namespace, &bp.ValuesYAML, &bp.CPU,
					&bp.Memory, &bp.Replicas, &bp.Ports, &bp.Category,
					&bp.GroupID, &bp.GroupPosition, &bp.ComposeYAML,
					&bp.CreatedAt,
				); err != nil {
					bpRows.Close()
					respondInternalError(c, err)
					return
				}
				if helmRepoID != nil {
					bp.HelmRepoID = helmRepoID
				}
				if bp.Ports == nil {
					bp.Ports = []int{}
				}
				bps = append(bps, bp)
			}
			bpRows.Close()
			if bps == nil {
				bps = []serviceBlueprintRow{}
			}
			groups[i].Blueprints = bps
		}

		c.JSON(http.StatusOK, gin.H{"groups": groups})
	}
}

func createBlueprintGroup(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := auth.GetTenantID(c)
		userID := auth.GetUserID(c)

		var req struct {
			Name        string `json:"name" binding:"required"`
			Description string `json:"description"`
			Position    int    `json:"position"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		var g blueprintGroupRow
		err := deps.DB.Pool.QueryRow(c.Request.Context(), `
			INSERT INTO blueprint_groups (tenant_id, name, description, position, created_by)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id, name, COALESCE(description,''), COALESCE(position,0), created_at
		`, tenantID, req.Name, req.Description, req.Position, userID).Scan(
			&g.ID, &g.Name, &g.Description, &g.Position, &g.CreatedAt,
		)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		g.Blueprints = []serviceBlueprintRow{}

		logAudit(deps, c, "create", "blueprint_group", g.ID, nil, g)
		c.JSON(http.StatusCreated, g)
	}
}

func updateBlueprintGroup(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group ID"})
			return
		}

		var req struct {
			Name        string `json:"name" binding:"required"`
			Description string `json:"description"`
			Position    int    `json:"position"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		var g blueprintGroupRow
		err = deps.DB.Pool.QueryRow(c.Request.Context(), `
			UPDATE blueprint_groups SET
				name=$2, description=$3, position=$4, updated_at=NOW()
			WHERE id=$1
			RETURNING id, name, COALESCE(description,''), COALESCE(position,0), created_at
		`, id, req.Name, req.Description, req.Position).Scan(
			&g.ID, &g.Name, &g.Description, &g.Position, &g.CreatedAt,
		)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		g.Blueprints = []serviceBlueprintRow{}

		logAudit(deps, c, "update", "blueprint_group", g.ID, nil, g)
		c.JSON(http.StatusOK, g)
	}
}

func deleteBlueprintGroup(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group ID"})
			return
		}

		// Unlink member blueprints (set group_id to NULL)
		_, _ = deps.DB.Pool.Exec(c.Request.Context(),
			`UPDATE service_blueprints SET group_id = NULL, group_position = 0 WHERE group_id = $1`, id)

		_, err = deps.DB.Pool.Exec(c.Request.Context(), `DELETE FROM blueprint_groups WHERE id=$1`, id)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "delete", "blueprint_group", id.String(), nil, nil)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

func reorderBlueprintGroup(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		groupID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group ID"})
			return
		}

		var req struct {
			// Ordered list of blueprint IDs representing the new order
			BlueprintIDs []string `json:"blueprint_ids" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Update each blueprint's group_position
		for i, bpIDStr := range req.BlueprintIDs {
			bpID, parseErr := uuid.Parse(bpIDStr)
			if parseErr != nil {
				continue
			}
			_, _ = deps.DB.Pool.Exec(c.Request.Context(),
				`UPDATE service_blueprints SET group_position=$1 WHERE id=$2 AND group_id=$3`,
				i, bpID, groupID)
		}

		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}
