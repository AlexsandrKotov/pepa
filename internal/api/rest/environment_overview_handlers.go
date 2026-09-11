package rest

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/repository"
)

// registerEnvironmentOverviewRoutes registers the tenant-scoped environment
// overview endpoints (services x environments matrix, problems, summary).
//
// The static "/overview" paths are registered before the CRUD routes so they
// take precedence over the "/environments/:id" wildcard in Gin's routing tree.
func registerEnvironmentOverviewRoutes(v1 *gin.RouterGroup, deps Dependencies) {
	if deps.Repos.EnvironmentOverview == nil {
		return
	}

	envOverview := v1.Group("/environments")
	{
		// GET /environments/overview - full environment overview matrix
		envOverview.GET("/overview", func(c *gin.Context) {
			tenantID := auth.GetTenantID(c)

			overview, err := deps.Repos.EnvironmentOverview.GetOverview(c.Request.Context(), tenantID)
			if err != nil {
				respondInternalError(c, err)
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"environments": nonNilEnvironments(overview.Environments),
				"services":     nonNilOverviewRows(overview.Services),
				"problems":     nonNilProblems(overview.Problems),
				"summary":      overview.Summary,
			})
		})

		// GET /environments/overview/problems - problems across environments,
		// optionally filtered by severity
		envOverview.GET("/overview/problems", func(c *gin.Context) {
			tenantID := auth.GetTenantID(c)

			overview, err := deps.Repos.EnvironmentOverview.GetOverview(c.Request.Context(), tenantID)
			if err != nil {
				respondInternalError(c, err)
				return
			}

			problems := overview.Problems
			if severity := c.Query("severity"); severity != "" {
				filtered := make([]repository.EnvironmentProblem, 0, len(problems))
				for _, p := range problems {
					if p.Severity == severity {
						filtered = append(filtered, p)
					}
				}
				problems = filtered
			}

			c.JSON(http.StatusOK, gin.H{
				"problems": nonNilProblems(problems),
				"total":    len(problems),
			})
		})

		// GET /environments/overview/summary - aggregate counts only
		envOverview.GET("/overview/summary", func(c *gin.Context) {
			tenantID := auth.GetTenantID(c)

			overview, err := deps.Repos.EnvironmentOverview.GetOverview(c.Request.Context(), tenantID)
			if err != nil {
				respondInternalError(c, err)
				return
			}

			c.JSON(http.StatusOK, overview.Summary)
		})
	}
}

// The overview page indexes into these collections directly, so they must
// marshal as [] rather than null when a tenant has no data yet.
func nonNilEnvironments(items []repository.Environment) []repository.Environment {
	if items == nil {
		return []repository.Environment{}
	}
	return items
}

func nonNilOverviewRows(items []repository.EnvironmentOverviewRow) []repository.EnvironmentOverviewRow {
	if items == nil {
		return []repository.EnvironmentOverviewRow{}
	}
	return items
}

func nonNilProblems(items []repository.EnvironmentProblem) []repository.EnvironmentProblem {
	if items == nil {
		return []repository.EnvironmentProblem{}
	}
	return items
}
