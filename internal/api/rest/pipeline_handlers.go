package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/pipeline"
	"github.com/pepa/pepa/pkg/models"
)

func registerPipelineSourceRoutes(r *gin.RouterGroup, deps Dependencies) {
	sources := r.Group("/pipeline-sources")
	{
		sources.GET("", listPipelineSources(deps))
		sources.POST("", createPipelineSource(deps))
		sources.GET("/:id", getPipelineSource(deps))
		sources.PUT("/:id", updatePipelineSource(deps))
		sources.DELETE("/:id", deletePipelineSource(deps))
		sources.POST("/:id/resolve-schema", resolvePipelineSchema(deps))
		sources.POST("/:id/sync-runs", syncPipelineRuns(deps))
		sources.GET("/:id/state", getPipelineState(deps))
		sources.POST("/:id/plan", getPipelinePlan(deps))
		sources.GET("/:id/inspect", inspectPipelineSource(deps))
		sources.POST("/trivy/auto-discover", trivyAutoDiscover(deps))
		sources.POST("/trivy/scan-all", trivyScanAll(deps))
	}
}

func listPipelineSources(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := auth.GetTenantID(c)
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))

		sources, total, err := deps.Repos.PipelineSource.List(c.Request.Context(), tenantID, page, perPage)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		if sources == nil {
			sources = []models.PipelineSource{}
		}

		c.JSON(http.StatusOK, gin.H{
			"sources":  sources,
			"total":    total,
			"page":     page,
			"per_page": perPage,
		})
	}
}

func createPipelineSource(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := auth.GetTenantID(c)
		userID := auth.GetUserID(c)

		var req models.CreatePipelineSourceRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		config := req.Config
		if config == nil {
			config = json.RawMessage("{}")
		}

		source := &models.PipelineSource{
			TenantID:     tenantID,
			Name:         req.Name,
			SourceType:   req.SourceType,
			Description:  req.Description,
			ConnectionID: req.ConnectionID,
			Config:       config,
			CreatedBy:    userID,
			Status:       "active",
		}

		if err := deps.Repos.PipelineSource.Create(c.Request.Context(), source); err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "create", "pipeline_source", source.ID.String(), nil, source)
		c.JSON(http.StatusCreated, source)
	}
}

func getPipelineSource(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid pipeline source ID"})
			return
		}

		source, err := deps.Repos.PipelineSource.Get(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, source)
	}
}

func updatePipelineSource(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid pipeline source ID"})
			return
		}

		var req models.CreatePipelineSourceRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		existing, err := deps.Repos.PipelineSource.Get(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		config := req.Config
		if config == nil {
			config = existing.Config
		}

		existing.Name = req.Name
		existing.SourceType = req.SourceType
		existing.Description = req.Description
		existing.ConnectionID = req.ConnectionID
		existing.Config = config

		if err := deps.Repos.PipelineSource.Update(c.Request.Context(), id, existing); err != nil {
			respondInternalError(c, err)
			return
		}

		updated, _ := deps.Repos.PipelineSource.Get(c.Request.Context(), id)
		logAudit(deps, c, "update", "pipeline_source", id.String(), nil, updated)
		c.JSON(http.StatusOK, updated)
	}
}

func deletePipelineSource(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid pipeline source ID"})
			return
		}

		if err := deps.Repos.PipelineSource.Delete(c.Request.Context(), id); err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "delete", "pipeline_source", id.String(), nil, nil)
		c.JSON(http.StatusOK, gin.H{"message": "pipeline source deleted", "id": id})
	}
}

// resolvePipelineConfig merges connection credentials into a pipeline source config
// when the token or base_url is missing from the config but available on the linked connection.
func resolvePipelineConfig(ctx context.Context, deps Dependencies, source *models.PipelineSource, tenantID uuid.UUID) json.RawMessage {
	config := source.Config
	if source.ConnectionID != nil && deps.Repos.Connection != nil {
		conn, err := deps.Repos.Connection.GetDecrypted(ctx, *source.ConnectionID, tenantID)
		if err == nil {
			var cfgMap map[string]interface{}
			if json.Unmarshal(config, &cfgMap) == nil {
				changed := false
				if token, ok := cfgMap["token"]; !ok || token == "" {
					if connToken, ok := conn.Config["token"]; ok && connToken != "" {
						cfgMap["token"] = connToken
						changed = true
					}
				}
				if baseURL, ok := cfgMap["base_url"]; !ok || baseURL == "" {
					if connURL, ok := conn.Config["url"]; ok && connURL != "" {
						cfgMap["base_url"] = connURL
						changed = true
					}
				}
				if changed {
					config, _ = json.Marshal(cfgMap)
				}
			}
		}
	}
	return config
}

func resolvePipelineSchema(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid pipeline source ID"})
			return
		}

		source, err := deps.Repos.PipelineSource.Get(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		if deps.PipelineRegistry == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "pipeline registry not available"})
			return
		}

		provider, err := deps.PipelineRegistry.Get(source.SourceType)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		config := resolvePipelineConfig(c.Request.Context(), deps, source, auth.GetTenantID(c))

		schema, err := provider.ResolveSchema(c.Request.Context(), config)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		schemaJSON, _ := json.Marshal(schema)
		if err := deps.Repos.PipelineSource.UpdateSchema(c.Request.Context(), id, schemaJSON); err != nil {
			respondInternalError(c, err)
			return
		}

		c.JSON(http.StatusOK, schema)
	}
}

// ── Pipeline Runs ────────────────────────────────────────────────────────────

func syncPipelineRuns(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		sourceID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid source ID"})
			return
		}
		tenantID := auth.GetTenantID(c)

		source, err := deps.Repos.PipelineSource.Get(c.Request.Context(), sourceID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		if deps.PipelineRegistry == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "pipeline registry not available"})
			return
		}

		provider, err := deps.PipelineRegistry.Get(source.SourceType)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Check if the provider supports listing remote runs
		listProvider, ok := provider.(pipeline.ListRunsProvider)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "this engine type does not support syncing runs"})
			return
		}

		config := resolvePipelineConfig(c.Request.Context(), deps, source, tenantID)

		perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "30"))
		if perPage > 100 {
			perPage = 100
		}

		remoteRuns, err := listProvider.ListRemoteRuns(c.Request.Context(), config, perPage)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		synced := 0
		skipped := 0
		for _, rs := range remoteRuns {
			// Parse created_at from remote
			var createdAt time.Time
			if rs.CreatedAt != "" {
				createdAt, _ = time.Parse(time.RFC3339, rs.CreatedAt)
			}
			if createdAt.IsZero() {
				createdAt = time.Now().UTC()
			}

			run := &models.PipelineRun{
				TenantID:       tenantID,
				SourceID:       sourceID,
				ExternalRunID:  rs.ExternalRunID,
				ExternalURL:    rs.ExternalURL,
				Status:         models.PipelineRunStatus(rs.Status),
				ExternalStatus: rs.Status,
				DurationMs:     rs.DurationMs,
				TriggerType:    "sync",
				CreatedAt:      createdAt,
				UpdatedAt:      time.Now().UTC(),
			}

			// Check if this run already exists and is in a terminal state.
			// Completed runs' jobs don't change, so we can skip the expensive job re-sync.
			existing, _ := deps.Repos.PipelineRun.FindByExternalRunID(c.Request.Context(), sourceID, rs.ExternalRunID)
			isTerminal := run.Status == models.PipelineRunSuccess || run.Status == models.PipelineRunFailed ||
				run.Status == models.PipelineRunCancelled || run.Status == models.PipelineRunTimeout
			if existing != nil && existing.Status == run.Status && isTerminal {
				skipped++
				continue
			}

			if err := deps.Repos.PipelineRun.UpsertByExternalRunID(c.Request.Context(), run); err != nil {
				continue
			}
			synced++

			// Sync jobs: delete old, insert new.
			// run.ID is now populated by UpsertByExternalRunID (deterministic UUID v5).
			if len(rs.Jobs) > 0 {
				_ = deps.Repos.PipelineRun.DeleteJobsByRunID(c.Request.Context(), run.ID)
				for _, j := range rs.Jobs {
					stepsJSON, _ := json.Marshal(j.Steps)
					if stepsJSON == nil {
						stepsJSON = json.RawMessage("[]")
					}
					job := &models.PipelineRunJob{
						RunID:         run.ID,
						ExternalJobID: j.ExternalJobID,
						Name:          j.Name,
						Stage:         j.Stage,
						Status:        j.Status,
						LogURL:        j.LogURL,
						RunnerName:    j.RunnerName,
						Steps:         stepsJSON,
					}
					_ = deps.Repos.PipelineRun.CreateJob(c.Request.Context(), job)
				}
			}
		}

		logAudit(deps, c, "sync", "pipeline_runs", sourceID.String(), nil, gin.H{"synced": synced, "skipped": skipped})
		c.JSON(http.StatusOK, gin.H{"synced": synced, "skipped": skipped, "total_remote": len(remoteRuns)})
	}
}

// ── Pipeline Runs ────────────────────────────────────────────

func registerPipelineRunRoutes(r *gin.RouterGroup, deps Dependencies) {
	runs := r.Group("/pipeline-sources/:id/runs")
	{
		runs.GET("", listPipelineRuns(deps))
		runs.POST("", triggerPipelineRun(deps))
		runs.GET("/:runId", getPipelineRun(deps))
		runs.POST("/:runId/refresh", refreshPipelineRun(deps))
		runs.POST("/:runId/cancel", cancelPipelineRun(deps))
		runs.GET("/:runId/jobs", listPipelineRunJobs(deps))
		runs.GET("/:runId/logs", getPipelineRunLogs(deps))
	}
}

func listPipelineRuns(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		sourceID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid source ID"})
			return
		}
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))

		runs, total, err := deps.Repos.PipelineRun.List(c.Request.Context(), sourceID, page, perPage)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		if runs == nil {
			runs = []models.PipelineRun{}
		}

		c.JSON(http.StatusOK, gin.H{
			"runs":     runs,
			"total":    total,
			"page":     page,
			"per_page": perPage,
		})
	}
}

func triggerPipelineRun(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		sourceID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid source ID"})
			return
		}
		tenantID := auth.GetTenantID(c)
		userID := auth.GetUserID(c)

		source, err := deps.Repos.PipelineSource.Get(c.Request.Context(), sourceID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		var req models.RunPipelineRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			// Body is optional
			req = models.RunPipelineRequest{}
		}

		// If a preset is specified, load and merge its parameters
		if req.PresetID != nil && deps.Repos.PipelinePreset != nil {
			preset, err := deps.Repos.PipelinePreset.Get(c.Request.Context(), *req.PresetID)
			if err == nil {
				var presetParams map[string]any
				if json.Unmarshal(preset.Parameters, &presetParams) == nil {
					if req.Parameters == nil {
						req.Parameters = presetParams
					} else {
						// Merge: explicit params override preset
						for k, v := range presetParams {
							if _, exists := req.Parameters[k]; !exists {
								req.Parameters[k] = v
							}
						}
					}
				}
				// Increment use count
				_ = deps.Repos.PipelinePreset.IncrementUseCount(c.Request.Context(), preset.ID)
			}
		}

		if deps.PipelineRegistry == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "pipeline registry not available"})
			return
		}

		provider, err := deps.PipelineRegistry.Get(source.SourceType)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Create the run record first
		paramsJSON, _ := json.Marshal(req.Parameters)
		run := &models.PipelineRun{
			TenantID:    tenantID,
			SourceID:    sourceID,
			PresetID:    req.PresetID,
			Parameters:  paramsJSON,
			Status:      models.PipelineRunPending,
			TriggeredBy: userID,
			TriggerType: "manual",
		}

		if err := deps.Repos.PipelineRun.Create(c.Request.Context(), run); err != nil {
			respondInternalError(c, err)
			return
		}

		config := resolvePipelineConfig(c.Request.Context(), deps, source, auth.GetTenantID(c))

		// For GitLab CI sources, separate spec.inputs (marked with is_input in the
		// parameter schema) from regular CI variables. The inputs are embedded in
		// the params map so the adapter can pass them via the "inputs" field in
		// the GitLab API request body (supported since GitLab 17.10+).
		if source.SourceType == "gitlab_ci" && len(req.Parameters) > 0 {
			schemaJSON := source.ParameterSchema
			// If no stored schema, resolve it on-the-fly to identify inputs.
			if len(schemaJSON) == 0 {
				if resolved, err := provider.ResolveSchema(c.Request.Context(), config); err == nil && resolved != nil {
					schemaJSON, _ = json.Marshal(resolved)
				}
			}
			if len(schemaJSON) > 0 {
				var schema pipeline.ParameterSchema
				if json.Unmarshal(schemaJSON, &schema) == nil {
					inputs := make(map[string]any)
					for k, v := range req.Parameters {
						if prop, ok := schema.Properties[k]; ok && prop.IsInput {
							inputs[k] = v
						}
					}
					if len(inputs) > 0 {
						req.Parameters["__gitlab_inputs__"] = inputs
					}
				}
			}
		}

		// Trigger via the adapter
		result, err := provider.Trigger(c.Request.Context(), config, req.Parameters)
		if err != nil {
			run.Status = models.PipelineRunError
			run.ErrorMessage = err.Error()
			_ = deps.Repos.PipelineRun.Update(c.Request.Context(), run.ID, run)
			respondInternalError(c, err)
			return
		}

		run.ExternalRunID = result.ExternalRunID
		run.ExternalURL = result.ExternalURL
		run.Status = models.PipelineRunRunning
		_ = deps.Repos.PipelineRun.Update(c.Request.Context(), run.ID, run)

		logAudit(deps, c, "trigger", "pipeline_run", run.ID.String(), nil, run)

		c.JSON(http.StatusAccepted, run)
	}
}

func getPipelineRun(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		runID, err := uuid.Parse(c.Param("runId"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid run ID"})
			return
		}

		run, err := deps.Repos.PipelineRun.Get(c.Request.Context(), runID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		// Also load jobs
		jobs, _ := deps.Repos.PipelineRun.ListJobs(c.Request.Context(), runID)

		c.JSON(http.StatusOK, gin.H{
			"run":  run,
			"jobs": jobs,
		})
	}
}

func refreshPipelineRun(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		sourceID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid source ID"})
			return
		}
		runID, err := uuid.Parse(c.Param("runId"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid run ID"})
			return
		}

		source, err := deps.Repos.PipelineSource.Get(c.Request.Context(), sourceID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		run, err := deps.Repos.PipelineRun.Get(c.Request.Context(), runID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		if run.ExternalRunID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "no external run ID"})
			return
		}

		if deps.PipelineRegistry == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "pipeline registry not available"})
			return
		}

		provider, err := deps.PipelineRegistry.Get(source.SourceType)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		status, err := provider.Status(c.Request.Context(), resolvePipelineConfig(c.Request.Context(), deps, source, auth.GetTenantID(c)), run.ExternalRunID)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		now := time.Now().UTC()
		run.Status = models.PipelineRunStatus(status.Status)
		run.ExternalStatus = status.Status
		if status.DurationMs != nil {
			run.DurationMs = status.DurationMs
		}
		if status.LogsURL != "" {
			run.LogsURL = status.LogsURL
		}

		// Set started_at when the run begins executing
		if run.Status == models.PipelineRunRunning && run.StartedAt == nil {
			run.StartedAt = &now
		}
		// Set completed_at when the run reaches a terminal state
		switch run.Status {
		case models.PipelineRunSuccess, models.PipelineRunFailed,
			models.PipelineRunCancelled, models.PipelineRunTimeout, models.PipelineRunError:
			if run.CompletedAt == nil {
				run.CompletedAt = &now
			}
			// Calculate duration if not provided by the provider
			if run.DurationMs == nil && run.StartedAt != nil {
				dur := int(now.Sub(*run.StartedAt).Milliseconds())
				if dur > 0 {
					run.DurationMs = &dur
				}
			}
		}

		_ = deps.Repos.PipelineRun.Update(c.Request.Context(), run.ID, run)

		// Refresh jobs too (upsert to avoid duplicates on repeated refreshes)
		jobs, err := provider.Jobs(c.Request.Context(), resolvePipelineConfig(c.Request.Context(), deps, source, auth.GetTenantID(c)), run.ExternalRunID)
		if err == nil {
			for _, j := range jobs {
				job := &models.PipelineRunJob{
					RunID:         run.ID,
					ExternalJobID: j.ExternalJobID,
					Name:          j.Name,
					Stage:         j.Stage,
					Status:        j.Status,
					LogURL:        j.LogURL,
					RunnerName:    j.RunnerName,
					AllowFailure:  j.AllowFailure,
				}
				_ = deps.Repos.PipelineRun.UpsertJob(c.Request.Context(), job)
			}
		}

		c.JSON(http.StatusOK, run)
	}
}

func cancelPipelineRun(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		sourceID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid source ID"})
			return
		}
		runID, err := uuid.Parse(c.Param("runId"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid run ID"})
			return
		}

		source, err := deps.Repos.PipelineSource.Get(c.Request.Context(), sourceID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		run, err := deps.Repos.PipelineRun.Get(c.Request.Context(), runID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		// Only allow cancelling active runs (pending or running)
		if run.Status != models.PipelineRunPending && run.Status != models.PipelineRunRunning {
			c.JSON(http.StatusConflict, gin.H{"error": "pipeline run is already " + string(run.Status)})
			return
		}

		if run.ExternalRunID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "no external run ID"})
			return
		}

		if deps.PipelineRegistry == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "pipeline registry not available"})
			return
		}

		provider, err := deps.PipelineRegistry.Get(source.SourceType)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := provider.Cancel(c.Request.Context(), resolvePipelineConfig(c.Request.Context(), deps, source, auth.GetTenantID(c)), run.ExternalRunID); err != nil {
			respondInternalError(c, err)
			return
		}

		run.Status = models.PipelineRunCancelled
		_ = deps.Repos.PipelineRun.Update(c.Request.Context(), run.ID, run)

		logAudit(deps, c, "cancel", "pipeline_run", run.ID.String(), nil, nil)
		c.JSON(http.StatusOK, run)
	}
}

func listPipelineRunJobs(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		runID, err := uuid.Parse(c.Param("runId"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid run ID"})
			return
		}

		jobs, err := deps.Repos.PipelineRun.ListJobs(c.Request.Context(), runID)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		if jobs == nil {
			jobs = []models.PipelineRunJob{}
		}

		c.JSON(http.StatusOK, gin.H{"jobs": jobs})
	}
}

func getPipelineRunLogs(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		sourceID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid source ID"})
			return
		}
		runID, err := uuid.Parse(c.Param("runId"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid run ID"})
			return
		}

		jobID := c.Query("job_id")

		source, err := deps.Repos.PipelineSource.Get(c.Request.Context(), sourceID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		run, err := deps.Repos.PipelineRun.Get(c.Request.Context(), runID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		if deps.PipelineRegistry == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "pipeline registry not available"})
			return
		}

		provider, err := deps.PipelineRegistry.Get(source.SourceType)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		logs, err := provider.Logs(c.Request.Context(), resolvePipelineConfig(c.Request.Context(), deps, source, auth.GetTenantID(c)), run.ExternalRunID, jobID)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{"logs": logs, "run_id": runID})
	}
}

// ── Pipeline Presets ─────────────────────────────────────────

func registerPipelinePresetRoutes(r *gin.RouterGroup, deps Dependencies) {
	presets := r.Group("/pipeline-sources/:id/presets")
	{
		presets.GET("", listPipelinePresets(deps))
		presets.POST("", createPipelinePreset(deps))
		presets.GET("/:presetId", getPipelinePreset(deps))
		presets.PUT("/:presetId", updatePipelinePreset(deps))
		presets.DELETE("/:presetId", deletePipelinePreset(deps))
	}
}

func listPipelinePresets(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		sourceID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid source ID"})
			return
		}

		presets, err := deps.Repos.PipelinePreset.List(c.Request.Context(), sourceID)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		if presets == nil {
			presets = []models.PipelinePreset{}
		}

		c.JSON(http.StatusOK, gin.H{"presets": presets})
	}
}

func createPipelinePreset(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		sourceID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid source ID"})
			return
		}
		tenantID := auth.GetTenantID(c)
		userID := auth.GetUserID(c)

		var req models.CreatePipelinePresetRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		params := req.Parameters
		if params == nil {
			params = json.RawMessage("{}")
		}

		preset := &models.PipelinePreset{
			TenantID:    tenantID,
			SourceID:    sourceID,
			Name:        req.Name,
			Description: req.Description,
			Parameters:  params,
			CreatedBy:   userID,
		}

		if err := deps.Repos.PipelinePreset.Create(c.Request.Context(), preset); err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "create", "pipeline_preset", preset.ID.String(), nil, preset)
		c.JSON(http.StatusCreated, preset)
	}
}

func getPipelinePreset(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		presetID, err := uuid.Parse(c.Param("presetId"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid preset ID"})
			return
		}

		preset, err := deps.Repos.PipelinePreset.Get(c.Request.Context(), presetID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, preset)
	}
}

func updatePipelinePreset(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		presetID, err := uuid.Parse(c.Param("presetId"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid preset ID"})
			return
		}

		var req models.CreatePipelinePresetRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		existing, err := deps.Repos.PipelinePreset.Get(c.Request.Context(), presetID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		params := req.Parameters
		if params == nil {
			params = existing.Parameters
		}

		existing.Name = req.Name
		existing.Description = req.Description
		existing.Parameters = params

		if err := deps.Repos.PipelinePreset.Update(c.Request.Context(), presetID, existing); err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "update", "pipeline_preset", presetID.String(), nil, gin.H{"name": existing.Name})

		updated, _ := deps.Repos.PipelinePreset.Get(c.Request.Context(), presetID)
		c.JSON(http.StatusOK, updated)
	}
}

func deletePipelinePreset(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		presetID, err := uuid.Parse(c.Param("presetId"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid preset ID"})
			return
		}

		if err := deps.Repos.PipelinePreset.Delete(c.Request.Context(), presetID); err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "delete", "pipeline_preset", presetID.String(), nil, nil)
		c.JSON(http.StatusOK, gin.H{"message": "preset deleted", "id": presetID})
	}
}

// ── Pipeline State & Plan (EnhancedProvider) ────────────────────────────────

// getPipelineState returns the current infrastructure state for a pipeline source
// (e.g. Terraform resources, Ansible host inventory).
func getPipelineState(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid pipeline source ID"})
			return
		}

		source, err := deps.Repos.PipelineSource.Get(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		if deps.PipelineRegistry == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "pipeline registry not available"})
			return
		}

		provider, err := deps.PipelineRegistry.Get(source.SourceType)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Check if the provider supports State (EnhancedProvider)
		enhanced, ok := provider.(pipeline.EnhancedProvider)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("%s does not support state browsing", source.SourceType)})
			return
		}

		// Collect params for backend initialization:
		// 1. From request body (POST), or
		// 2. From the last successful run's stored parameters
		var params map[string]any
		if c.Request.ContentLength > 0 {
			if err := c.ShouldBindJSON(&params); err != nil {
				params = make(map[string]any)
			}
		}
		if len(params) == 0 && deps.Repos.PipelineRun != nil {
			runs, _, listErr := deps.Repos.PipelineRun.List(c.Request.Context(), id, 1, 1)
			if listErr == nil {
				for _, run := range runs {
					if run.Status == "success" && len(run.Parameters) > 0 {
						_ = json.Unmarshal(run.Parameters, &params)
					}
					break
				}
			}
		}

		config := resolvePipelineConfig(c.Request.Context(), deps, source, auth.GetTenantID(c))
		state, err := enhanced.State(c.Request.Context(), config, params)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		c.JSON(http.StatusOK, state)
	}
}

// getPipelinePlan runs a plan/preview (e.g. terraform plan, ansible --check)
// and returns the structured result.
func getPipelinePlan(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid pipeline source ID"})
			return
		}

		source, err := deps.Repos.PipelineSource.Get(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		if deps.PipelineRegistry == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "pipeline registry not available"})
			return
		}

		provider, err := deps.PipelineRegistry.Get(source.SourceType)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Check if the provider supports Plan (EnhancedProvider)
		enhanced, ok := provider.(pipeline.EnhancedProvider)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("%s does not support plan preview", source.SourceType)})
			return
		}

		// Optional parameters from request body
		var params map[string]any
		if c.Request.ContentLength > 0 {
			if err := c.ShouldBindJSON(&params); err != nil {
				params = make(map[string]any)
			}
		}

		config := resolvePipelineConfig(c.Request.Context(), deps, source, auth.GetTenantID(c))
		plan, err := enhanced.Plan(c.Request.Context(), config, params)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		c.JSON(http.StatusOK, plan)
	}
}

// inspectPipelineSource returns structured metadata about a pipeline source
// (e.g. Ansible playbooks/roles, Terraform modules/resources).
func inspectPipelineSource(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid pipeline source ID"})
			return
		}

		source, err := deps.Repos.PipelineSource.Get(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		if deps.PipelineRegistry == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "pipeline registry not available"})
			return
		}

		provider, err := deps.PipelineRegistry.Get(source.SourceType)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		inspectable, ok := provider.(pipeline.InspectableProvider)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("%s does not support inspection", source.SourceType)})
			return
		}

		config := resolvePipelineConfig(c.Request.Context(), deps, source, auth.GetTenantID(c))
		result, err := inspectable.Inspect(c.Request.Context(), config)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		c.Data(http.StatusOK, "application/json", result)
	}
}

// trivyAutoDiscover discovers git repositories from connections and creates Trivy scan sources.
func trivyAutoDiscover(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := auth.GetTenantID(c)

		if deps.Repos.Connection == nil || deps.Repos.PipelineSource == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "repositories not available"})
			return
		}

		// List git connections
		gitlabConns, _ := deps.Repos.Connection.List(c.Request.Context(), tenantID, "gitlab")
		gitConns, _ := deps.Repos.Connection.List(c.Request.Context(), tenantID, "git")

		type repoInfo struct {
			URL          string
			ConnectionID string
			Name         string
		}

		var discovered []repoInfo

		// Use provider registry to browse repos
		if deps.ProviderRegistry != nil {
			browseConn := func(connID uuid.UUID, pluginName string, connConfig map[string]string) {
				entry, ok := deps.ProviderRegistry.Get(pluginName)
				if !ok || entry == nil || !entry.Enabled || entry.Executor == nil {
					return
				}
				resp, err := entry.Executor.Execute(c.Request.Context(), "list_repos", nil, tenantID.String(), connConfig)
				if err != nil || resp == nil {
					return
				}
				var browseResult struct {
					Repos []struct {
						URL      string `json:"url"`
						Name     string `json:"name"`
						FullName string `json:"full_name"`
					} `json:"repos"`
				}
				if json.Unmarshal(resp.GetOutput(), &browseResult) == nil {
					for _, r := range browseResult.Repos {
						if r.URL != "" {
							discovered = append(discovered, repoInfo{
								URL:          r.URL,
								ConnectionID: connID.String(),
								Name:         r.FullName,
							})
						}
					}
				}
			}

			for _, conn := range gitlabConns {
				cfg := make(map[string]string)
				for k, v := range conn.Config {
					if s, ok := v.(string); ok {
						cfg[k] = s
					}
				}
				browseConn(conn.ID, "gitlab", cfg)
			}
			for _, conn := range gitConns {
				prov, _ := conn.Config["provider"].(string)
				if prov == "" {
					prov = "gitlab"
				}
				cfg := make(map[string]string)
				for k, v := range conn.Config {
					if s, ok := v.(string); ok {
						cfg[k] = s
					}
				}
				browseConn(conn.ID, prov, cfg)
			}
		}

		// Create trivy pipeline sources for discovered repos
		// Fetch all existing sources once to avoid N+1 queries
		allSources, _, _ := deps.Repos.PipelineSource.List(c.Request.Context(), tenantID, 1, 200)
		existingTrivy := make(map[string]bool) // target URL → already exists
		for _, s := range allSources {
			if s.SourceType == "trivy" {
				var cfg map[string]string
				if json.Unmarshal(s.Config, &cfg) == nil {
					if target, ok := cfg["target"]; ok {
						existingTrivy[target] = true
					}
				}
			}
		}

		created := 0
		existing := 0
		var sources []models.PipelineSource

		for _, repo := range discovered {
			if existingTrivy[repo.URL] {
				existing++
				continue
			}

			// Create new trivy source
			connUUID, _ := uuid.Parse(repo.ConnectionID)
			configJSON, _ := json.Marshal(map[string]string{
				"target":    repo.URL,
				"scan_type": "repo",
				"severity":  "HIGH,CRITICAL",
				"format":    "json",
			})
			newSource := &models.PipelineSource{
				TenantID:     tenantID,
				Name:         fmt.Sprintf("Trivy: %s", repo.Name),
				SourceType:   "trivy",
				Description:  fmt.Sprintf("Auto-discovered from connection %s", repo.ConnectionID),
				ConnectionID: &connUUID,
				Config:       configJSON,
				Status:       "active",
			}
			if err := deps.Repos.PipelineSource.Create(c.Request.Context(), newSource); err == nil {
				created++
				sources = append(sources, *newSource)
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"created":  created,
			"existing": existing,
			"sources":  sources,
		})
	}
}

// trivyScanAll triggers vulnerability scans for all Trivy pipeline sources.
func trivyScanAll(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := auth.GetTenantID(c)

		if deps.Repos.PipelineSource == nil || deps.PipelineRegistry == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "services not available"})
			return
		}

		allSources, _, _ := deps.Repos.PipelineSource.List(c.Request.Context(), tenantID, 1, 200)
		trivySources := make([]models.PipelineSource, 0)
		for _, s := range allSources {
			if s.SourceType == "trivy" {
				trivySources = append(trivySources, s)
			}
		}

		provider, err := deps.PipelineRegistry.Get("trivy")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "trivy provider not available"})
			return
		}

		scanned := 0
		var results []map[string]interface{}

		for _, src := range trivySources {
			config := resolvePipelineConfig(c.Request.Context(), deps, &src, tenantID)
			result, triggerErr := provider.Trigger(c.Request.Context(), config, map[string]any{
				"severity": "HIGH,CRITICAL",
			})
			entry := map[string]interface{}{
				"source_id":   src.ID,
				"source_name": src.Name,
			}
			if triggerErr != nil {
				entry["status"] = "error"
				entry["error"] = triggerErr.Error()
			} else {
				entry["status"] = result.Status
				entry["run_id"] = result.ExternalRunID
				scanned++
			}
			results = append(results, entry)
		}

		c.JSON(http.StatusOK, gin.H{
			"scanned": scanned,
			"total":   len(trivySources),
			"results": results,
		})
	}
}
