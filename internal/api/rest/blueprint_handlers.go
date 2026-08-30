package rest

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
)

// registerServiceBlueprintRoutes registers CRUD routes for service blueprints.
func registerServiceBlueprintRoutes(r *gin.RouterGroup, deps Dependencies) {
	blueprints := r.Group("/blueprints")
	{
		blueprints.GET("", listServiceBlueprints(deps))
		blueprints.POST("", createServiceBlueprint(deps))
		blueprints.GET("/:id", getServiceBlueprint(deps))
		blueprints.PUT("/:id", updateServiceBlueprint(deps))
		blueprints.DELETE("/:id", deleteServiceBlueprint(deps))
	}
}

type serviceBlueprintRow struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	SourceType    string    `json:"source_type"`
	HelmRepoID    *string   `json:"helm_repo_id"`
	Image         string    `json:"image"`
	ChartURL      string    `json:"chart_url"`
	ChartName     string    `json:"chart_name"`
	ChartVersion  string    `json:"chart_version"`
	ChartPath     string    `json:"chart_path"`
	Namespace     string    `json:"namespace"`
	ValuesYAML    string    `json:"values_yaml"`
	CPU           string    `json:"cpu"`
	Memory        string    `json:"memory"`
	Replicas      int       `json:"replicas"`
	Ports         []int     `json:"ports"`
	Category      string    `json:"category"`
	GroupIDs      []string  `json:"group_ids"`
	ComposeYAML   string    `json:"compose_yaml"`
	CreatedAt     time.Time `json:"created_at"`
}

// selectBlueprintCols returns the SQL column list for selecting a blueprint.
// Pass a table alias (e.g. "sb") for JOIN queries to avoid ambiguous columns;
// pass "" for single-table queries.
func selectBlueprintCols(alias ...string) string {
	p := ""
	if len(alias) > 0 && alias[0] != "" {
		p = alias[0] + "."
	}
	return p + `id, ` + p + `name, COALESCE(` + p + `description,''), ` + p + `source_type,
	       ` + p + `helm_repo_id, COALESCE(` + p + `image,''), COALESCE(` + p + `chart_url,''),
	       COALESCE(` + p + `chart_name,''), COALESCE(` + p + `chart_version,''),
	       COALESCE(` + p + `chart_path,''), COALESCE(` + p + `namespace,'default'),
	       COALESCE(` + p + `values_yaml,''), COALESCE(` + p + `cpu,'100m'),
	       COALESCE(` + p + `memory,'128Mi'), COALESCE(` + p + `replicas,1),
	       COALESCE(` + p + `ports,'{}'), COALESCE(` + p + `category,'general'),
	       COALESCE(` + p + `compose_yaml,''),
	       ` + p + `created_at`
}

// scanBlueprintRow scans a blueprint row (must use selectBlueprintCols order).
func scanBlueprintRow(rows interface{ Scan(...interface{}) error }) (*serviceBlueprintRow, error) {
	var bp serviceBlueprintRow
	var helmRepoID *string
	if err := rows.Scan(
		&bp.ID, &bp.Name, &bp.Description, &bp.SourceType,
		&helmRepoID, &bp.Image, &bp.ChartURL,
		&bp.ChartName, &bp.ChartVersion, &bp.ChartPath,
		&bp.Namespace, &bp.ValuesYAML, &bp.CPU,
		&bp.Memory, &bp.Replicas, &bp.Ports, &bp.Category,
		&bp.ComposeYAML,
		&bp.CreatedAt,
	); err != nil {
		return nil, err
	}
	if helmRepoID != nil {
		bp.HelmRepoID = helmRepoID
	}
	if bp.Ports == nil {
		bp.Ports = []int{}
	}
	bp.GroupIDs = []string{}
	return &bp, nil
}

func listServiceBlueprints(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := deps.DB.Pool.Query(c.Request.Context(), `
			SELECT `+selectBlueprintCols()+`
			FROM service_blueprints
			ORDER BY created_at DESC
		`)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		defer rows.Close()

		var blueprints []serviceBlueprintRow
		for rows.Next() {
			bp, err := scanBlueprintRow(rows)
			if err != nil {
				respondInternalError(c, err)
				return
			}
			blueprints = append(blueprints, *bp)
		}
		if blueprints == nil {
			blueprints = []serviceBlueprintRow{}
		}

		// Populate group_ids from junction table
		for i, bp := range blueprints {
			gRows, err := deps.DB.Pool.Query(c.Request.Context(),
				`SELECT group_id FROM blueprint_group_members WHERE blueprint_id = $1 ORDER BY position ASC`, bp.ID)
			if err != nil {
				respondInternalError(c, err)
				return
			}
			var gids []string
			for gRows.Next() {
				var gid string
				if err := gRows.Scan(&gid); err == nil {
					gids = append(gids, gid)
				}
			}
			gRows.Close()
			if gids == nil {
				gids = []string{}
			}
			blueprints[i].GroupIDs = gids
		}

		c.JSON(http.StatusOK, gin.H{"blueprints": blueprints})
	}
}

func createServiceBlueprint(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserID(c)

		var req struct {
			Name          string   `json:"name" binding:"required"`
			Description   string   `json:"description"`
			SourceType    string   `json:"source_type" binding:"required"`
			HelmRepoID    *string  `json:"helm_repo_id"`
			Image         string   `json:"image"`
			ChartURL      string   `json:"chart_url"`
			ChartName     string   `json:"chart_name"`
			ChartVersion  string   `json:"chart_version"`
			ChartPath     string   `json:"chart_path"`
			Namespace     string   `json:"namespace"`
			ValuesYAML    string   `json:"values_yaml"`
			CPU           string   `json:"cpu"`
			Memory        string   `json:"memory"`
			Replicas      int      `json:"replicas"`
			Ports         []int    `json:"ports"`
			Category      string   `json:"category"`
			GroupIDs      []string `json:"group_ids"`
			ComposeYAML   string   `json:"compose_yaml"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if req.Namespace == "" {
			req.Namespace = "default"
		}
		if req.CPU == "" {
			req.CPU = "100m"
		}
		if req.Memory == "" {
			req.Memory = "128Mi"
		}
		if req.Replicas < 1 {
			req.Replicas = 1
		}
		if req.Category == "" {
			req.Category = "general"
		}
		if req.Ports == nil {
			req.Ports = []int{}
		}

		var bp serviceBlueprintRow
		var helmRepoID *string
		err := deps.DB.Pool.QueryRow(c.Request.Context(), `
			INSERT INTO service_blueprints
				(name, description, source_type, helm_repo_id, image, chart_url,
				 chart_name, chart_version, chart_path, namespace, values_yaml,
				 cpu, memory, replicas, ports, category, compose_yaml, created_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
			RETURNING `+selectBlueprintCols(),
			req.Name, req.Description, req.SourceType, req.HelmRepoID,
			req.Image, req.ChartURL, req.ChartName, req.ChartVersion,
			req.ChartPath, req.Namespace, req.ValuesYAML,
			req.CPU, req.Memory, req.Replicas, req.Ports,
			req.Category, req.ComposeYAML, userID,
		).Scan(
			&bp.ID, &bp.Name, &bp.Description, &bp.SourceType,
			&helmRepoID, &bp.Image, &bp.ChartURL,
			&bp.ChartName, &bp.ChartVersion, &bp.ChartPath,
			&bp.Namespace, &bp.ValuesYAML, &bp.CPU,
			&bp.Memory, &bp.Replicas, &bp.Ports, &bp.Category,
			&bp.ComposeYAML,
			&bp.CreatedAt,
		)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		if helmRepoID != nil {
			bp.HelmRepoID = helmRepoID
		}
		if bp.Ports == nil {
			bp.Ports = []int{}
		}
		bp.GroupIDs = []string{}

		// Insert group memberships
		for i, gid := range req.GroupIDs {
			_, _ = deps.DB.Pool.Exec(c.Request.Context(),
				`INSERT INTO blueprint_group_members (group_id, blueprint_id, position) VALUES ($1,$2,$3)
				 ON CONFLICT (group_id, blueprint_id) DO UPDATE SET position = $3`,
				gid, bp.ID, i)
			bp.GroupIDs = append(bp.GroupIDs, gid)
		}

		logAudit(deps, c, "create", "service_blueprint", bp.ID, nil, bp)
		c.JSON(http.StatusCreated, bp)
	}
}

func getServiceBlueprint(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid blueprint ID"})
			return
		}

		var bp serviceBlueprintRow
		var helmRepoID *string
		err = deps.DB.Pool.QueryRow(c.Request.Context(), `
			SELECT `+selectBlueprintCols()+`
			FROM service_blueprints WHERE id = $1
		`, id).Scan(
			&bp.ID, &bp.Name, &bp.Description, &bp.SourceType,
			&helmRepoID, &bp.Image, &bp.ChartURL,
			&bp.ChartName, &bp.ChartVersion, &bp.ChartPath,
			&bp.Namespace, &bp.ValuesYAML, &bp.CPU,
			&bp.Memory, &bp.Replicas, &bp.Ports, &bp.Category,
			&bp.ComposeYAML,
			&bp.CreatedAt,
		)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "blueprint not found"})
			return
		}
		if helmRepoID != nil {
			bp.HelmRepoID = helmRepoID
		}
		if bp.Ports == nil {
			bp.Ports = []int{}
		}

		// Populate group_ids from junction table
		gRows, err := deps.DB.Pool.Query(c.Request.Context(),
			`SELECT group_id FROM blueprint_group_members WHERE blueprint_id = $1 ORDER BY position ASC`, bp.ID)
		if err == nil {
			var gids []string
			for gRows.Next() {
				var gid string
				if err := gRows.Scan(&gid); err == nil {
					gids = append(gids, gid)
				}
			}
			gRows.Close()
			if gids == nil {
				gids = []string{}
			}
			bp.GroupIDs = gids
		}

		c.JSON(http.StatusOK, bp)
	}
}

func updateServiceBlueprint(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid blueprint ID"})
			return
		}

		var req struct {
			Name          string   `json:"name" binding:"required"`
			Description   string   `json:"description"`
			SourceType    string   `json:"source_type" binding:"required"`
			HelmRepoID    *string  `json:"helm_repo_id"`
			Image         string   `json:"image"`
			ChartURL      string   `json:"chart_url"`
			ChartName     string   `json:"chart_name"`
			ChartVersion  string   `json:"chart_version"`
			ChartPath     string   `json:"chart_path"`
			Namespace     string   `json:"namespace"`
			ValuesYAML    string   `json:"values_yaml"`
			CPU           string   `json:"cpu"`
			Memory        string   `json:"memory"`
			Replicas      int      `json:"replicas"`
			Ports         []int    `json:"ports"`
			Category      string   `json:"category"`
			GroupIDs      []string `json:"group_ids"`
			ComposeYAML   string   `json:"compose_yaml"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if req.Namespace == "" {
			req.Namespace = "default"
		}
		if req.CPU == "" {
			req.CPU = "100m"
		}
		if req.Memory == "" {
			req.Memory = "128Mi"
		}
		if req.Replicas < 1 {
			req.Replicas = 1
		}
		if req.Category == "" {
			req.Category = "general"
		}
		if req.Ports == nil {
			req.Ports = []int{}
		}

		var bp serviceBlueprintRow
		var helmRepoID *string
		err = deps.DB.Pool.QueryRow(c.Request.Context(), `
			UPDATE service_blueprints SET
				name=$2, description=$3, source_type=$4, helm_repo_id=$5,
				image=$6, chart_url=$7, chart_name=$8, chart_version=$9,
				chart_path=$10, namespace=$11, values_yaml=$12,
				cpu=$13, memory=$14, replicas=$15, ports=$16,
				category=$17, compose_yaml=$18, updated_at=NOW()
			WHERE id=$1
			RETURNING `+selectBlueprintCols(),
			id, req.Name, req.Description, req.SourceType, req.HelmRepoID,
			req.Image, req.ChartURL, req.ChartName, req.ChartVersion,
			req.ChartPath, req.Namespace, req.ValuesYAML,
			req.CPU, req.Memory, req.Replicas, req.Ports,
			req.Category, req.ComposeYAML,
		).Scan(
			&bp.ID, &bp.Name, &bp.Description, &bp.SourceType,
			&helmRepoID, &bp.Image, &bp.ChartURL,
			&bp.ChartName, &bp.ChartVersion, &bp.ChartPath,
			&bp.Namespace, &bp.ValuesYAML, &bp.CPU,
			&bp.Memory, &bp.Replicas, &bp.Ports, &bp.Category,
			&bp.ComposeYAML,
			&bp.CreatedAt,
		)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		if helmRepoID != nil {
			bp.HelmRepoID = helmRepoID
		}
		if bp.Ports == nil {
			bp.Ports = []int{}
		}

		// Sync group memberships: remove old, insert new
		_, _ = deps.DB.Pool.Exec(c.Request.Context(),
			`DELETE FROM blueprint_group_members WHERE blueprint_id = $1`, id)
		bp.GroupIDs = []string{}
		for i, gid := range req.GroupIDs {
			_, _ = deps.DB.Pool.Exec(c.Request.Context(),
				`INSERT INTO blueprint_group_members (group_id, blueprint_id, position) VALUES ($1,$2,$3)
				 ON CONFLICT (group_id, blueprint_id) DO UPDATE SET position = $3`,
				gid, id, i)
			bp.GroupIDs = append(bp.GroupIDs, gid)
		}

		logAudit(deps, c, "update", "service_blueprint", bp.ID, nil, bp)
		c.JSON(http.StatusOK, bp)
	}
}

func deleteServiceBlueprint(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid blueprint ID"})
			return
		}

		_, err = deps.DB.Pool.Exec(c.Request.Context(), `DELETE FROM service_blueprints WHERE id=$1`, id)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "delete", "service_blueprint", id.String(), nil, nil)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}
