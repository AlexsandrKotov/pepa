package rest

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/pkg/models"
)

func registerScorecardRoutes(r *gin.RouterGroup, deps Dependencies) {
	scorecards := r.Group("/scorecards")
	{
		scorecards.GET("", listScorecards(deps))
		scorecards.POST("", createScorecard(deps))
		scorecards.GET("/:id", getScorecard(deps))
		scorecards.PUT("/:id", updateScorecard(deps))
		scorecards.DELETE("/:id", deleteScorecard(deps))

		// Rules
		scorecards.GET("/:id/rules", listScorecardRules(deps))
		scorecards.POST("/:id/rules", addScorecardRule(deps))
		scorecards.DELETE("/rules/:ruleId", deleteScorecardRule(deps))

		// Evaluation
		scorecards.POST("/:id/evaluate", evaluateScorecard(deps))
		scorecards.GET("/:id/results", listScorecardResults(deps))

		// Entity scores
		scorecards.GET("/entity/:entityId", getEntityScores(deps))
	}
}

func listScorecards(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := auth.GetTenantID(c)
		items, err := deps.Repos.Scorecard.ListScorecards(c.Request.Context(), tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		if items == nil {
			items = []models.Scorecard{}
		}

		// Load rules count for each scorecard
		type scorecardWithRules struct {
			models.Scorecard
			RuleCount int `json:"rule_count"`
		}
		result := make([]scorecardWithRules, len(items))
		for i, sc := range items {
			rules, _ := deps.Repos.Scorecard.ListRules(c.Request.Context(), sc.ID)
			result[i] = scorecardWithRules{Scorecard: sc, RuleCount: len(rules)}
		}

		c.JSON(http.StatusOK, gin.H{"scorecards": result, "total": len(result)})
	}
}

func createScorecard(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := auth.GetTenantID(c)
		userID := auth.GetUserID(c)

		var req models.CreateScorecardRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		sc := &models.Scorecard{
			TenantID:    tenantID,
			Name:        req.Name,
			Description: req.Description,
			Enabled:     req.Enabled,
			Config:      req.Config,
			CreatedBy:   userID,
		}

		if err := deps.Repos.Scorecard.CreateScorecard(c.Request.Context(), sc); err != nil {
			respondInternalError(c, err)
			return
		}

		// Create rules if provided
		for _, ruleReq := range req.Rules {
			rule := &models.ScorecardRule{
				ScorecardID: sc.ID,
				Name:        ruleReq.Name,
				Description: ruleReq.Description,
				Expression:  ruleReq.Expression,
				Weight:      ruleReq.Weight,
				PassMessage: ruleReq.PassMessage,
				FailMessage: ruleReq.FailMessage,
				Severity:    ruleReq.Severity,
				Metadata:    ruleReq.Metadata,
			}
			if err := deps.Repos.Scorecard.CreateRule(c.Request.Context(), rule); err != nil {
				respondInternalError(c, err)
				return
			}
		}

		// Log audit
		logAudit(deps, c, "create", "scorecard", sc.ID.String(), nil, sc)

		c.JSON(http.StatusCreated, sc)
	}
}

func getScorecard(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scorecard ID"})
			return
		}

		sc, err := deps.Repos.Scorecard.GetScorecard(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		rules, _ := deps.Repos.Scorecard.ListRules(c.Request.Context(), id)
		if rules == nil {
			rules = []models.ScorecardRule{}
		}

		c.JSON(http.StatusOK, gin.H{
			"scorecard": sc,
			"rules":     rules,
		})
	}
}

func updateScorecard(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scorecard ID"})
			return
		}

		var req models.UpdateScorecardRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		sc, err := deps.Repos.Scorecard.UpdateScorecard(c.Request.Context(), id, req)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "update", "scorecard", id.String(), nil, sc)
		c.JSON(http.StatusOK, sc)
	}
}

func deleteScorecard(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scorecard ID"})
			return
		}

		if err := deps.Repos.Scorecard.DeleteScorecard(c.Request.Context(), id); err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "delete", "scorecard", id.String(), nil, nil)
		c.JSON(http.StatusOK, gin.H{"message": "scorecard deleted", "id": id})
	}
}

func listScorecardRules(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scorecard ID"})
			return
		}

		rules, err := deps.Repos.Scorecard.ListRules(c.Request.Context(), id)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		if rules == nil {
			rules = []models.ScorecardRule{}
		}

		c.JSON(http.StatusOK, gin.H{"rules": rules, "total": len(rules)})
	}
}

func addScorecardRule(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scorecard ID"})
			return
		}

		var req models.CreateScorecardRuleRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		rule := &models.ScorecardRule{
			ScorecardID: id,
			Name:        req.Name,
			Description: req.Description,
			Expression:  req.Expression,
			Weight:      req.Weight,
			PassMessage: req.PassMessage,
			FailMessage: req.FailMessage,
			Severity:    req.Severity,
			Metadata:    req.Metadata,
		}

		if err := deps.Repos.Scorecard.CreateRule(c.Request.Context(), rule); err != nil {
			respondInternalError(c, err)
			return
		}

		c.JSON(http.StatusCreated, rule)
	}
}

func deleteScorecardRule(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		ruleID, err := uuid.Parse(c.Param("ruleId"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rule ID"})
			return
		}

		if err := deps.Repos.Scorecard.DeleteRule(c.Request.Context(), ruleID); err != nil {
			respondInternalError(c, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "rule deleted", "id": ruleID})
	}
}

func evaluateScorecard(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scorecard ID"})
			return
		}

		tenantID := auth.GetTenantID(c)

		var req struct {
			EntityID string `json:"entity_id"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// "all" (or empty) evaluates every entity in the tenant
		if req.EntityID == "" || req.EntityID == "all" {
			entityIDs, err := deps.Repos.Scorecard.ListTenantEntityIDs(c.Request.Context(), tenantID)
			if err != nil {
				respondInternalError(c, err)
				return
			}

			var results []models.ScorecardResult
			for _, entityID := range entityIDs {
				result, err := deps.Repos.Scorecard.EvaluateEntity(c.Request.Context(), id, entityID, tenantID)
				if err != nil {
					continue // skip entities that fail to evaluate
				}
				results = append(results, *result)
			}
			if results == nil {
				results = []models.ScorecardResult{}
			}

			logAudit(deps, c, "evaluate", "scorecard", id.String(), nil, gin.H{"entities": len(results)})
			c.JSON(http.StatusOK, gin.H{"results": results, "total": len(results)})
			return
		}

		entityID, err := uuid.Parse(req.EntityID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid entity ID"})
			return
		}

		result, err := deps.Repos.Scorecard.EvaluateEntity(c.Request.Context(), id, entityID, tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "evaluate", "scorecard", id.String(), nil, result)
		c.JSON(http.StatusOK, result)
	}
}

func listScorecardResults(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scorecard ID"})
			return
		}

		results, err := deps.Repos.Scorecard.ListResults(c.Request.Context(), id)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		if results == nil {
			results = []models.ScorecardResult{}
		}

		c.JSON(http.StatusOK, gin.H{"results": results, "total": len(results)})
	}
}

func getEntityScores(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		entityID, err := uuid.Parse(c.Param("entityId"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid entity ID"})
			return
		}

		tenantID := auth.GetTenantID(c)

		results, err := deps.Repos.Scorecard.GetEntityScores(c.Request.Context(), entityID, tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		if results == nil {
			results = []models.ScorecardResult{}
		}

		c.JSON(http.StatusOK, gin.H{"scores": results, "total": len(results)})
	}
}
