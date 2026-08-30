package rest

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/ai"
)

// ProactiveAIHandlers handles proactive AI endpoints (risk, docs, cost, stale).
type ProactiveAIHandlers struct {
	riskScorer   *ai.RiskScorer
	docGenerator *ai.DocGenerator
	costAdvisor  *ai.CostAdvisor
	staleDetector *ai.StaleDetector
	tenantID     uuid.UUID
}

// NewProactiveAIHandlers creates new proactive AI handlers.
func NewProactiveAIHandlers(riskScorer *ai.RiskScorer, docGen *ai.DocGenerator, costAdv *ai.CostAdvisor, staleDet *ai.StaleDetector, tenantID uuid.UUID) *ProactiveAIHandlers {
	return &ProactiveAIHandlers{
		riskScorer:    riskScorer,
		docGenerator:  docGen,
		costAdvisor:   costAdv,
		staleDetector: staleDet,
		tenantID:      tenantID,
	}
}

// AssessDeploymentRisk evaluates deployment risk before deploy.
func (h *ProactiveAIHandlers) AssessDeploymentRisk(c *gin.Context) {
	if h.riskScorer == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Risk scorer not initialized"})
		return
	}

	var req struct {
		ServiceName string `json:"service_name" binding:"required"`
		Version     string `json:"version"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	assessment, err := h.riskScorer.AssessDeployment(c.Request.Context(), req.ServiceName, req.Version)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, assessment)
}

// GenerateServiceDocs creates AI-generated documentation for a service.
func (h *ProactiveAIHandlers) GenerateServiceDocs(c *gin.Context) {
	if h.docGenerator == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Doc generator not initialized"})
		return
	}

	serviceName := c.Param("service")
	if serviceName == "" {
		var req struct {
			ServiceName string `json:"service_name" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		serviceName = req.ServiceName
	}

	doc, err := h.docGenerator.GenerateServiceDocs(c.Request.Context(), serviceName)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, doc)
}

// AnalyzeCosts provides cost optimization recommendations.
func (h *ProactiveAIHandlers) AnalyzeCosts(c *gin.Context) {
	if h.costAdvisor == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Cost advisor not initialized"})
		return
	}

	recommendations, err := h.costAdvisor.AnalyzeCosts(c.Request.Context())
	if err != nil {
		respondInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"recommendations": recommendations,
		"total":           len(recommendations),
		"analyzed_at":     time.Now(),
	})
}

// DetectStaleResources finds unused or stale resources.
func (h *ProactiveAIHandlers) DetectStaleResources(c *gin.Context) {
	if h.staleDetector == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Stale detector not initialized"})
		return
	}

	stale, err := h.staleDetector.DetectStaleResources(c.Request.Context())
	if err != nil {
		respondInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"resources":   stale,
		"total":       len(stale),
		"detected_at": time.Now(),
	})
}
