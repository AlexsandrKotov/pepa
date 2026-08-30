package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	dockerpkg "github.com/pepa/pepa/internal/docker"
	"github.com/pepa/pepa/internal/repository"
)

// registerBlueprintDeployRoutes registers deploy routes for blueprints.
func registerBlueprintDeployRoutes(r *gin.RouterGroup, deps Dependencies) {
	r.POST("/blueprints/:id/deploy-docker", deployBlueprintToDocker(deps))
	r.POST("/blueprint-groups/:id/deploy-docker", deployBlueprintGroupToDocker(deps))
	r.POST("/blueprint-groups/:id/deploy-kubernetes", deployBlueprintGroupToKubernetes(deps))
}

func deployBlueprintToDocker(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.DockerHost == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "docker host repository not available"})
			return
		}

		bpID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid blueprint ID"})
			return
		}

		var req struct {
			DockerHostID uuid.UUID         `json:"docker_host_id" binding:"required"`
			EnvVars      map[string]string `json:"env_vars"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Fetch blueprint
		var bp serviceBlueprintRow
		var helmRepoID *string
		err = deps.DB.Pool.QueryRow(c.Request.Context(), `
			SELECT `+selectBlueprintCols()+`
			FROM service_blueprints WHERE id = $1
		`, bpID).Scan(
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

		if bp.SourceType != "docker_compose" || bp.ComposeYAML == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "blueprint must be of type docker_compose with compose_yaml"})
			return
		}

		// Verify Docker host
		tenantID := auth.GetTenantID(c)
		host, err := deps.Repos.DockerHost.GetHostDecrypted(c.Request.Context(), req.DockerHostID, tenantID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "docker host not found"})
			return
		}

		envVars := req.EnvVars
		if envVars == nil {
			envVars = make(map[string]string)
		}
		envJSON, _ := json.Marshal(envVars)

		svc := &repository.DockerService{
			TenantID:     tenantID,
			DockerHostID: req.DockerHostID,
			Name:         bp.Name,
			ComposeYaml:  bp.ComposeYAML,
			EnvVars:      envJSON,
			Status:       "deploying",
			Containers:   json.RawMessage("[]"),
		}

		if err := deps.Repos.DockerHost.CreateService(c.Request.Context(), svc); err != nil {
			respondInternalError(c, err)
			return
		}

		// Deploy via Docker CLI
		cfg := dockerpkg.HostConfig{
			HostType:    host.HostType,
			HostAddress: host.HostAddress,
			TLSCACert:   host.TLSCACert,
			TLSCert:     host.TLSCert,
			TLSKey:      host.TLSKey,
			SSHKey:      host.SSHKey,
		}
		client := dockerpkg.NewClient(cfg)

		ctx, cancel := context.WithTimeout(c.Request.Context(), 120*time.Second)
		defer cancel()

		if err := client.ComposeUp(ctx, svc.Name, svc.ComposeYaml, envVars); err != nil {
			svc.Status = "error"
			_ = deps.Repos.DockerHost.UpdateService(c.Request.Context(), svc)
			respondInternalError(c, err)
			return
		}

		containers, err := client.ComposePs(ctx, svc.Name)
		if err == nil {
			cJSON, _ := json.Marshal(containers)
			svc.Containers = cJSON
		}
		svc.Status = "running"
		_ = deps.Repos.DockerHost.UpdateService(c.Request.Context(), svc)

		logAudit(deps, c, "deploy_docker", "blueprint", bp.ID, nil, gin.H{"docker_host": req.DockerHostID.String()})
		c.JSON(http.StatusCreated, svc)
	}
}

func deployBlueprintGroupToDocker(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.DockerHost == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "docker host repository not available"})
			return
		}

		groupID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group ID"})
			return
		}

		var req struct {
			DockerHostID uuid.UUID         `json:"docker_host_id" binding:"required"`
			EnvVars      map[string]string `json:"env_vars"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		tenantID := auth.GetTenantID(c)

		// Verify Docker host
		host, err := deps.Repos.DockerHost.GetHostDecrypted(c.Request.Context(), req.DockerHostID, tenantID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "docker host not found"})
			return
		}

		// Fetch group member blueprints via junction table
		rows, err := deps.DB.Pool.Query(c.Request.Context(), `
			SELECT `+selectBlueprintCols("sb")+`
			FROM service_blueprints sb
			JOIN blueprint_group_members bgm ON bgm.blueprint_id = sb.id
			WHERE bgm.group_id = $1 AND sb.source_type = 'docker_compose' AND sb.compose_yaml != ''
			ORDER BY bgm.position ASC
		`, groupID)
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

		if len(blueprints) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "no docker_compose blueprints found in this group"})
			return
		}

		envVars := req.EnvVars
		if envVars == nil {
			envVars = make(map[string]string)
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

		type deployResult struct {
			BlueprintID   string `json:"blueprint_id"`
			BlueprintName string `json:"blueprint_name"`
			Status        string `json:"status"`
			Error         string `json:"error,omitempty"`
		}
		var results []deployResult

		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Minute)
		defer cancel()

		for _, bp := range blueprints {
			envJSON, _ := json.Marshal(envVars)
			svc := &repository.DockerService{
				TenantID:     tenantID,
				DockerHostID: req.DockerHostID,
				Name:         bp.Name,
				ComposeYaml:  bp.ComposeYAML,
				EnvVars:      envJSON,
				Status:       "deploying",
				Containers:   json.RawMessage("[]"),
			}

			if err := deps.Repos.DockerHost.CreateService(ctx, svc); err != nil {
				results = append(results, deployResult{
					BlueprintID:   bp.ID,
					BlueprintName: bp.Name,
					Status:        "error",
					Error:         fmt.Sprintf("create service: %v", err),
				})
				continue
			}

			svcCtx, svcCancel := context.WithTimeout(ctx, 120*time.Second)
			if err := client.ComposeUp(svcCtx, svc.Name, svc.ComposeYaml, envVars); err != nil {
				svcCancel()
				svc.Status = "error"
				_ = deps.Repos.DockerHost.UpdateService(ctx, svc)
				results = append(results, deployResult{
					BlueprintID:   bp.ID,
					BlueprintName: bp.Name,
					Status:        "error",
					Error:         err.Error(),
				})
				continue
			}
			svcCancel()

			containers, _ := client.ComposePs(ctx, svc.Name)
			if containers != nil {
				cJSON, _ := json.Marshal(containers)
				svc.Containers = cJSON
			}
			svc.Status = "running"
			_ = deps.Repos.DockerHost.UpdateService(ctx, svc)

			results = append(results, deployResult{
				BlueprintID:   bp.ID,
				BlueprintName: bp.Name,
				Status:        "running",
			})
		}

		logAudit(deps, c, "deploy_group_docker", "blueprint_group", groupID.String(), nil,
			gin.H{"docker_host": req.DockerHostID.String(), "count": len(blueprints)})
		c.JSON(http.StatusCreated, gin.H{
			"results": results,
			"total":   len(results),
		})
	}
}

func deployBlueprintGroupToKubernetes(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		groupID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group ID"})
			return
		}

		var req struct {
			ClusterID string `json:"cluster_id" binding:"required"`
			Namespace string `json:"namespace"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if req.Namespace == "" {
			req.Namespace = "default"
		}

		// Fetch group member blueprints (non-docker_compose ones for K8s) via junction table
		rows, err := deps.DB.Pool.Query(c.Request.Context(), `
			SELECT `+selectBlueprintCols("sb")+`
			FROM service_blueprints sb
			JOIN blueprint_group_members bgm ON bgm.blueprint_id = sb.id
			WHERE bgm.group_id = $1 AND sb.source_type != 'docker_compose'
			ORDER BY bgm.position ASC
		`, groupID)
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

		if len(blueprints) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "no deployable blueprints found in this group"})
			return
		}

		type deployResult struct {
			BlueprintID   string `json:"blueprint_id"`
			BlueprintName string `json:"blueprint_name"`
			Status        string `json:"status"`
			DeploymentID  string `json:"deployment_id,omitempty"`
			Error         string `json:"error,omitempty"`
		}
		var results []deployResult

		for _, bp := range blueprints {
			// Create a deployment record for each blueprint
			deployType := "helm"
			if bp.SourceType != "container" {
				deployType = bp.SourceType
			}

			spec := map[string]interface{}{
				"values_yaml": bp.ValuesYAML,
				"chart": map[string]interface{}{
					"source_type":  bp.SourceType,
					"chart_url":    bp.ChartURL,
					"chart_name":   bp.ChartName,
					"chart_version": bp.ChartVersion,
					"chart_path":   bp.ChartPath,
				},
				"service": map[string]interface{}{
					"port": func() int {
						if len(bp.Ports) > 0 {
							return bp.Ports[0]
						}
						return 80
					}(),
					"type": "ClusterIP",
				},
			}
			if bp.Image != "" {
				spec["containers"] = []map[string]interface{}{
					{"name": bp.Name, "image": bp.Image, "cpu": bp.CPU, "memory": bp.Memory, "ports": bp.Ports},
				}
			}
			specJSON, _ := json.Marshal(spec)

			d := &repository.Deployment{
				TenantID:        auth.GetTenantID(c),
				GitlabProjectName: bp.Name,
				TargetClusterID: func() *uuid.UUID { id, _ := uuid.Parse(req.ClusterID); return &id }(),
				TargetNamespace: req.Namespace,
				ImageRepository: bp.Image,
				DeployType:      deployType,
				Replicas:        bp.Replicas,
				Strategy:        "rolling",
				Spec:            specJSON,
				Status:          "pending",
				TimeoutSeconds:  300,
			}

			if deps.Repos.Deployment == nil {
				results = append(results, deployResult{
					BlueprintID:   bp.ID,
					BlueprintName: bp.Name,
					Status:        "error",
					Error:         "deployment repository not available",
				})
				continue
			}

			if err := deps.Repos.Deployment.Create(c.Request.Context(), d); err != nil {
				results = append(results, deployResult{
					BlueprintID:   bp.ID,
					BlueprintName: bp.Name,
					Status:        "error",
					Error:         err.Error(),
				})
				continue
			}

			results = append(results, deployResult{
				BlueprintID:   bp.ID,
				BlueprintName: bp.Name,
				Status:        "pending",
				DeploymentID:  d.ID.String(),
			})
		}

		logAudit(deps, c, "deploy_group_k8s", "blueprint_group", groupID.String(), nil,
			gin.H{"cluster_id": req.ClusterID, "count": len(blueprints)})
		c.JSON(http.StatusCreated, gin.H{
			"results": results,
			"total":   len(results),
		})
	}
}
