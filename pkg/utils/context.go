package utils

import (
	"context"
	"time"
)

// DetachContext prepares a context for work that must outlive the request (or
// parent goroutine) that scheduled it.
//
// A fire-and-forget goroutine that keeps using the request context gets two
// problems at once: the context is cancelled as soon as the handler returns, so
// the work dies half-way and often without a trace, and when no deadline
// replaces it the work can hang forever. DetachContext strips cancellation and
// deadlines from parent while keeping its values and logger fields, then applies
// timeout (skipped when timeout <= 0) as the only bound on the job.
//
// The returned CancelFunc must always be called; it never returns nil.
//
// The parent context must not be read from inside the goroutine after the
// handler has returned (for example via gin.Context.Request), because the
// framework reuses those objects.
func DetachContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	ctx := context.WithoutCancel(parent)
	if timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}
