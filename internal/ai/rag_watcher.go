package ai

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/events"
)

// RAGWatcher subscribes to platform events and triggers RAG re-ingestion.
type RAGWatcher struct {
	eventBus *events.Bus
	engine   *IngestionEngine
	tenantID uuid.UUID

	// Debounce: avoid re-ingesting the same source type too frequently.
	mu         sync.Mutex
	lastIngest map[string]time.Time
	debounce   time.Duration

	// stopCh signals background goroutines to exit.
	stopCh chan struct{}
	// ctx is the parent context for all ingestion operations.
	ctx context.Context
}

// NewRAGWatcher creates a new RAG event watcher.
func NewRAGWatcher(eventBus *events.Bus, engine *IngestionEngine, tenantID uuid.UUID, ctx context.Context) *RAGWatcher {
	if ctx == nil {
		ctx = context.Background()
	}
	return &RAGWatcher{
		eventBus:   eventBus,
		engine:     engine,
		tenantID:   tenantID,
		lastIngest: make(map[string]time.Time),
		debounce:   30 * time.Second,
		stopCh:     make(chan struct{}),
		ctx:        ctx,
	}
}

// Start registers event handlers and begins watching for changes.
//
// Each handler re-indexes the workspace that emitted the event. Before, every
// event triggered a re-index of the single bootstrap tenant, so a change in
// another workspace refreshed the wrong (or nothing's) documents.
func (w *RAGWatcher) Start() {
	// Service events → re-ingest service catalog
	w.eventBus.On("service.created", func(e events.Event) {
		tenantID := w.eventTenant(e.TenantID)
		w.debouncedIngest("service", tenantID, func(ctx context.Context, tenantID uuid.UUID) error {
			loader := NewServiceDocumentLoader(w.engine.pool, tenantID)
			_, err := w.engine.ReindexAll(ctx, loader, tenantID)
			return err
		})
	})

	w.eventBus.On("service.updated", func(e events.Event) {
		tenantID := w.eventTenant(e.TenantID)
		w.debouncedIngest("service", tenantID, func(ctx context.Context, tenantID uuid.UUID) error {
			loader := NewServiceDocumentLoader(w.engine.pool, tenantID)
			_, err := w.engine.ReindexAll(ctx, loader, tenantID)
			return err
		})
	})

	// Entity events → re-ingest entity graph
	w.eventBus.On("entity.created", func(e events.Event) {
		tenantID := w.eventTenant(e.TenantID)
		w.debouncedIngest("entity", tenantID, func(ctx context.Context, tenantID uuid.UUID) error {
			loader := NewEntityDocumentLoader(w.engine.pool, tenantID)
			_, err := w.engine.ReindexAll(ctx, loader, tenantID)
			return err
		})
	})

	w.eventBus.On("entity.updated", func(e events.Event) {
		tenantID := w.eventTenant(e.TenantID)
		w.debouncedIngest("entity", tenantID, func(ctx context.Context, tenantID uuid.UUID) error {
			loader := NewEntityDocumentLoader(w.engine.pool, tenantID)
			_, err := w.engine.ReindexAll(ctx, loader, tenantID)
			return err
		})
	})

	// Pipeline run events → re-ingest pipeline history
	w.eventBus.On("pipeline_run.completed", func(e events.Event) {
		tenantID := w.eventTenant(e.TenantID)
		w.debouncedIngest("pipeline", tenantID, func(ctx context.Context, tenantID uuid.UUID) error {
			loader := NewPipelineDocumentLoader(w.engine.pool, tenantID)
			_, err := w.engine.ReindexAll(ctx, loader, tenantID)
			return err
		})
	})

	// Deployment events → re-ingest pipeline history
	w.eventBus.On("deployment.completed", func(e events.Event) {
		tenantID := w.eventTenant(e.TenantID)
		w.debouncedIngest("pipeline", tenantID, func(ctx context.Context, tenantID uuid.UUID) error {
			loader := NewPipelineDocumentLoader(w.engine.pool, tenantID)
			_, err := w.engine.ReindexAll(ctx, loader, tenantID)
			return err
		})
	})

	slog.Info("RAG watcher started, listening for platform events")
}

// eventTenant resolves the workspace an event came from. Events published
// without a usable tenant fall back to the watcher's default so that the
// long-standing single-tenant deployments keep re-indexing exactly as before.
func (w *RAGWatcher) eventTenant(raw string) uuid.UUID {
	if id, err := uuid.Parse(raw); err == nil && id != uuid.Nil {
		return id
	}
	return w.tenantID
}

// debouncedIngest runs an ingestion function with debouncing. The debounce
// window is tracked per source type *and* tenant, so a burst of activity in one
// workspace cannot suppress another workspace's re-index.
func (w *RAGWatcher) debouncedIngest(sourceType string, tenantID uuid.UUID, fn func(context.Context, uuid.UUID) error) {
	key := sourceType + "|" + tenantID.String()

	w.mu.Lock()
	if last, ok := w.lastIngest[key]; ok && time.Since(last) < w.debounce {
		w.mu.Unlock()
		slog.Debug("RAG: skipping re-ingestion (debounced)", "source", sourceType)
		return
	}
	w.lastIngest[key] = time.Now()
	w.mu.Unlock()

	ctx, cancel := context.WithTimeout(w.ctx, 60*time.Second)
	defer cancel()

	if err := fn(ctx, tenantID); err != nil {
		slog.Warn("RAG: event-driven re-ingestion failed", "source", sourceType, "error", err)
	} else {
		slog.Info("RAG: re-ingested after event", "source", sourceType)
	}
}

// Stop shuts down background goroutines started by PeriodicReindex.
func (w *RAGWatcher) Stop() {
	select {
	case <-w.stopCh:
		// already closed
	default:
		close(w.stopCh)
	}
}

// PeriodicReindex runs a full re-index on a schedule.
func (w *RAGWatcher) PeriodicReindex(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// Use an ordered slice instead of a map to avoid non-deterministic iteration.
		type namedLoader struct {
			name   string
			loader DocumentLoader
		}

		for {
			select {
			case <-w.stopCh:
				return
			case <-ticker.C:
				slog.Info("RAG: starting periodic re-index")
				ctx, cancel := context.WithTimeout(w.ctx, 5*time.Minute)

				tenants, err := w.knownTenants(ctx)
				if err != nil {
					slog.Warn("RAG: cannot list tenants for periodic re-index, using default", "error", err)
					tenants = []uuid.UUID{w.tenantID}
				}
				if len(tenants) == 0 {
					tenants = []uuid.UUID{w.tenantID}
				}

				for _, tenantID := range tenants {
					if ctx.Err() != nil {
						break
					}
					loaders := []namedLoader{
						{"service", NewServiceDocumentLoader(w.engine.pool, tenantID)},
						{"entity", NewEntityDocumentLoader(w.engine.pool, tenantID)},
						{"pipeline", NewPipelineDocumentLoader(w.engine.pool, tenantID)},
					}

					for _, item := range loaders {
						count, err := w.engine.ReindexAll(ctx, item.loader, tenantID)
						if err != nil {
							slog.Warn("RAG: periodic re-index failed", "source", item.name, "tenant_id", tenantID.String(), "error", err)
						} else {
							slog.Info("RAG: periodic re-index complete", "source", item.name, "tenant_id", tenantID.String(), "documents", count)
						}
					}
				}

				// Expire old documents
				expired, err := w.engine.ExpireOld(ctx)
				if err != nil {
					slog.Warn("RAG: failed to expire old documents", "error", err)
				} else if expired > 0 {
					slog.Info("RAG: expired old documents", "count", expired)
				}

				cancel()
			}
		}
	}()
}

// knownTenants lists every workspace whose knowledge base should be refreshed by
// the periodic re-index. Results are ordered so the pass is deterministic.
func (w *RAGWatcher) knownTenants(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := w.engine.pool.Query(ctx, `SELECT id FROM tenants ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tenants []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		tenants = append(tenants, id)
	}
	return tenants, rows.Err()
}

// IngestAll performs a one-time full re-index of all sources.
func IngestAll(ctx context.Context, engine *IngestionEngine, tenantID uuid.UUID) error {
	type namedLoader struct {
		name   string
		loader DocumentLoader
	}

	loaders := []namedLoader{
		{"service", NewServiceDocumentLoader(engine.pool, tenantID)},
		{"entity", NewEntityDocumentLoader(engine.pool, tenantID)},
		{"pipeline", NewPipelineDocumentLoader(engine.pool, tenantID)},
	}

	for _, item := range loaders {
		count, err := engine.ReindexAll(ctx, item.loader, tenantID)
		if err != nil {
			slog.Warn("RAG: full re-index failed", "source", item.name, "error", err)
			continue
		}
		slog.Info("RAG: full re-index complete", "source", item.name, "documents", count)
	}
	return nil
}
