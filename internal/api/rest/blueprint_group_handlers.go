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
		groups.POST("/:id/blueprints", addBlueprintsToGroup(deps))
		groups.DELETE("/:id/blueprints/:bpId", removeBlueprintFromGroup(deps))
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

// fetchGroupBlueprints queries blueprints that are members of the given group
// via the junction table, ordered by junction position.
func fetchGroupBlueprints(c *gin.Context, deps Dependencies, groupID string) ([]serviceBlueprintRow, error) {
	rows, err := deps.DB.Pool.Query(c.Request.Context(), `
		SELECT `+selectBlueprintCols("sb")+`
		FROM service_blueprints sb
		JOIN blueprint_group_members bgm ON bgm.blueprint_id = sb.id
		WHERE bgm.group_id = $1
		ORDER BY bgm.position ASC, sb.created_at ASC
	`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bps []serviceBlueprintRow
	for rows.Next() {
		bp, err := scanBlueprintRow(rows)
		if err != nil {
			return nil, err
		}
		bps = append(bps, *bp)
	}
	if bps == nil {
		bps = []serviceBlueprintRow{}
	}
	return bps, nil
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

		// Fetch member blueprints for each group via junction table
		for i, g := range groups {
			bps, err := fetchGroupBlueprints(c, deps, g.ID)
			if err != nil {
				respondInternalError(c, err)
				return
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

		// Junction entries are deleted automatically via ON DELETE CASCADE
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

		// Update each membership's position in the junction table
		for i, bpIDStr := range req.BlueprintIDs {
			bpID, parseErr := uuid.Parse(bpIDStr)
			if parseErr != nil {
				continue
			}
			_, _ = deps.DB.Pool.Exec(c.Request.Context(),
				`UPDATE blueprint_group_members SET position=$1 WHERE group_id=$2 AND blueprint_id=$3`,
				i, groupID, bpID)
		}

		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

func addBlueprintsToGroup(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		groupID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group ID"})
			return
		}

		var req struct {
			BlueprintIDs []string `json:"blueprint_ids" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Find current max position in this group
		var maxPos int
		_ = deps.DB.Pool.QueryRow(c.Request.Context(),
			`SELECT COALESCE(MAX(position), -1) FROM blueprint_group_members WHERE group_id = $1`,
			groupID).Scan(&maxPos)

		added := 0
		for i, bpIDStr := range req.BlueprintIDs {
			bpID, parseErr := uuid.Parse(bpIDStr)
			if parseErr != nil {
				continue
			}
			_, err := deps.DB.Pool.Exec(c.Request.Context(),
				`INSERT INTO blueprint_group_members (group_id, blueprint_id, position)
				 VALUES ($1, $2, $3)
				 ON CONFLICT (group_id, blueprint_id) DO NOTHING`,
				groupID, bpID, maxPos+1+i)
			if err == nil {
				added++
			}
		}

		logAudit(deps, c, "add_blueprints", "blueprint_group", groupID.String(), nil,
			gin.H{"blueprint_ids": req.BlueprintIDs, "added": added})
		c.JSON(http.StatusOK, gin.H{"ok": true, "added": added})
	}
}

func removeBlueprintFromGroup(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		groupID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group ID"})
			return
		}

		bpID, err := uuid.Parse(c.Param("bpId"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid blueprint ID"})
			return
		}

		_, err = deps.DB.Pool.Exec(c.Request.Context(),
			`DELETE FROM blueprint_group_members WHERE group_id = $1 AND blueprint_id = $2`,
			groupID, bpID)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "remove_blueprint", "blueprint_group", groupID.String(), nil,
			gin.H{"blueprint_id": bpID.String()})
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}
