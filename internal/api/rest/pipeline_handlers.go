package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
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
