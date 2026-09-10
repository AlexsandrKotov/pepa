package rest

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/pepa/pepa/internal/repository"
)

// registerEnvironmentOverviewRoutes registers the environment overview endpoints.
func registerEnvironmentOverviewRoutes(v1 *gin.RouterGroup, deps Dependencies) {
	if deps.Repos.EnvironmentOverview == nil {
		return
	}

	envOverview := v1.Group("/environments")
	{
		// GET /environments/overview - Get the full environment overview matrix
		envOverview.GET("/overview", func(c *gin.Context) {
			tenantID := getTenantID(c)

			overview, err := deps.Repos.EnvironmentOverview.GetOverview(c.Request.Context(), tenantID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			c.JSON(http.StatusOK, overview)
		})

		// GET /environments/overview/problems - Get all problems across environments
		envOverview.GET("/overview/problems", func(c *gin.Context) {
			tenantID := getTenantID(c)

			overview, err := deps.Repos.EnvironmentOverview.GetOverview(c.Request.Context(), tenantID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			// Filter problems by severity if requested
			severity := c.Query("severity")
			problems := overview.Problems
			if severity != "" {
				filtered := make([]repository.EnvironmentProblem, 0)
				for _, p := range problems {
					if p.Severity == severity {
						filtered = append(filtered, p)
					}
				}
				problems = filtered
			}

			c.JSON(http.StatusOK, gin.H{
				"problems": problems,
				"total":    len(problems),
			})
		})

		// GET /environments/overview/summary - Get just the summary counts
		envOverview.GET("/overview/summary", func(c *gin.Context) {
			tenantID := getTenantID(c)

			overview, err := deps.Repos.EnvironmentOverview.GetOverview(c.Request.Context(), tenantID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			c.JSON(http.StatusOK, overview.Summary)
		})
	}
}
