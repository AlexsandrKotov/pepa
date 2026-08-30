package rest

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/pepa/pepa/internal/ai"
)

// MultiAgentHandlers handles multi-agent coordination endpoints.
type MultiAgentHandlers struct {
	coordinator *ai.AgentCoordinator
	specialists *ai.SpecialistRegistry
}

// NewMultiAgentHandlers creates new multi-agent handlers.
func NewMultiAgentHandlers(coordinator *ai.AgentCoordinator, specialists *ai.SpecialistRegistry) *MultiAgentHandlers {
	return &MultiAgentHandlers{
		coordinator: coordinator,
		specialists: specialists,
	}
}

// Route sends a query to the most appropriate specialist agent.
func (h *MultiAgentHandlers) Route(c *gin.Context) {
	if h.coordinator == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Agent coordinator not initialized"})
		return
	}

	var req struct {
		Query string `json:"query" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.coordinator.Route(c.Request.Context(), req.Query)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// Coordinate sends a query to multiple specialists and synthesizes results.
func (h *MultiAgentHandlers) Coordinate(c *gin.Context) {
	if h.coordinator == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Agent coordinator not initialized"})
		return
	}

	var req struct {
		Query       string   `json:"query" binding:"required"`
		Specialists []string `json:"specialists"` // optional: limit to specific specialists
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Parse specialist types
	var specs []ai.SpecialistType
	for _, s := range req.Specialists {
		switch strings.ToLower(s) {
		case "sre":
			specs = append(specs, ai.SpecialistSRE)
		case "devops":
			specs = append(specs, ai.SpecialistDevOps)
		case "security":
			specs = append(specs, ai.SpecialistSecurity)
		case "doc":
			specs = append(specs, ai.SpecialistDoc)
		case "cost":
			specs = append(specs, ai.SpecialistCost)
		case "general":
			specs = append(specs, ai.SpecialistGeneral)
		}
	}

	result, err := h.coordinator.Coordinate(c.Request.Context(), req.Query, specs)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// ListSpecialists returns all available specialist agents.
func (h *MultiAgentHandlers) ListSpecialists(c *gin.Context) {
	if h.specialists == nil {
		c.JSON(http.StatusOK, gin.H{"specialists": []interface{}{}, "total": 0})
		return
	}

	types := h.specialists.List()
	specialists := make([]map[string]string, 0, len(types))
	for _, t := range types {
		specialists = append(specialists, map[string]string{
			"type":        string(t),
			"description": specialistDescription(t),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"specialists": specialists,
		"total":       len(specialists),
	})
}

// specialistDescription returns a human-readable description of a specialist.
func specialistDescription(t ai.SpecialistType) string {
	switch t {
	case ai.SpecialistSRE:
		return "Site Reliability Engineering: monitoring, incidents, health, metrics, debugging"
	case ai.SpecialistDevOps:
		return "DevOps: deployments, pipelines, CI/CD, Kubernetes, Docker, infrastructure"
	case ai.SpecialistSecurity:
		return "Security: vulnerabilities, RBAC, secrets, compliance, audit"
	case ai.SpecialistDoc:
		return "Documentation: technical writing, knowledge base, service docs, runbooks"
	case ai.SpecialistCost:
		return "Cost Optimization: resource analysis, right-sizing, budget, idle resources"
	case ai.SpecialistGeneral:
		return "General purpose: handles queries that span multiple domains"
	default:
		return fmt.Sprintf("Unknown specialist: %s", t)
	}
}
