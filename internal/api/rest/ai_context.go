package rest

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/pepa/pepa/internal/ai"
	"github.com/pepa/pepa/internal/auth"
)

// agentContext returns the request context with the caller's workspace bound to
// it, for every call that can end up executing AI tools.
//
// The AI tool registry is constructed once at startup and shared by all
// requests, so tools cannot carry a per-tenant instance. Passing the tenant
// through the context (which the agent loop hands to each tool verbatim) keeps
// "list my services" and "deploy this" inside the workspace of the token holder
// instead of the bootstrap tenant the registry was built with.
func agentContext(c *gin.Context) context.Context {
	return ai.WithTenantContext(c.Request.Context(), auth.GetTenantID(c))
}
