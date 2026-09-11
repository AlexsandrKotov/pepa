package ai

import (
	"context"

	"github.com/google/uuid"
)

// tenantContextKey is the private key under which the request's tenant is stored
// in a context.Context. A dedicated unexported type makes collisions with other
// packages' context keys impossible.
type tenantContextKey struct{}

// WithTenantContext returns a context carrying the tenant a tool call must be
// scoped to. Handlers derive it from the verified JWT before starting a
// generation, so a tool can never be steered into another workspace by the
// model or by request-body parameters.
func WithTenantContext(ctx context.Context, tenantID uuid.UUID) context.Context {
	if tenantID == uuid.Nil {
		return ctx
	}
	return context.WithValue(ctx, tenantContextKey{}, tenantID)
}

// TenantFromContext returns the tenant bound by WithTenantContext, if any.
func TenantFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(tenantContextKey{}).(uuid.UUID)
	if !ok || id == uuid.Nil {
		return uuid.Nil, false
	}
	return id, true
}

// resolveTenant returns the tenant bound to ctx, falling back to the component's
// own workspace for calls that have no request scope (background re-indexes,
// startup seeding).
func resolveTenant(ctx context.Context, fallback uuid.UUID) uuid.UUID {
	if id, ok := TenantFromContext(ctx); ok {
		return id
	}
	return fallback
}
