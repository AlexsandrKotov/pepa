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
}

// NewRAGWatcher creates a new RAG event watcher.
func NewRAGWatcher(eventBus *events.Bus, engine *IngestionEngine, tenantID uuid.UUID) *RAGWatcher {
	return &RAGWatcher{
		eventBus:   eventBus,
		engine:     engine,
		tenantID:   tenantID,
		lastIngest: make(map[string]time.Time),
		debounce:   30 * time.Second,
		stopCh:     make(chan struct{}),
	}
}

// Start registers event handlers and begins watching for changes.
func (w *RAGWatcher) Start() {
	// Service events → re-ingest service catalog
	w.eventBus.On("service.created", func(e events.Event) {
		w.debouncedIngest("service", func(ctx context.Context) error {
			loader := NewServiceDocumentLoader(w.engine.pool, w.tenantID)
			_, err := w.engine.ReindexAll(ctx, loader, w.tenantID)
			return err
		})
	})

	w.eventBus.On("service.updated", func(e events.Event) {
		w.debouncedIngest("service", func(ctx context.Context) error {
			loader := NewServiceDocumentLoader(w.engine.pool, w.tenantID)
			_, err := w.engine.ReindexAll(ctx, loader, w.tenantID)
			return err
		})
	})

	// Entity events → re-ingest entity graph
	w.eventBus.On("entity.created", func(e events.Event) {
		w.debouncedIngest("entity", func(ctx context.Context) error {
			loader := NewEntityDocumentLoader(w.engine.pool, w.tenantID)
			_, err := w.engine.ReindexAll(ctx, loader, w.tenantID)
			return err
		})
	})

	w.eventBus.On("entity.updated", func(e events.Event) {
		w.debouncedIngest("entity", func(ctx context.Context) error {
			loader := NewEntityDocumentLoader(w.engine.pool, w.tenantID)
			_, err := w.engine.ReindexAll(ctx, loader, w.tenantID)
			return err
		})
	})

	// Pipeline run events → re-ingest pipeline history
	w.eventBus.On("pipeline_run.completed", func(e events.Event) {
		w.debouncedIngest("pipeline", func(ctx context.Context) error {
			loader := NewPipelineDocumentLoader(w.engine.pool, w.tenantID)
			_, err := w.engine.ReindexAll(ctx, loader, w.tenantID)
			return err
		})
	})

	// Deployment events → re-ingest pipeline history
	w.eventBus.On("deployment.completed", func(e events.Event) {
		w.debouncedIngest("pipeline", func(ctx context.Context) error {
			loader := NewPipelineDocumentLoader(w.engine.pool, w.tenantID)
			_, err := w.engine.ReindexAll(ctx, loader, w.tenantID)
			return err
		})
	})

	slog.Info("RAG watcher started, listening for platform events")
}

// debouncedIngest runs an ingestion function with debouncing.
func (w *RAGWatcher) debouncedIngest(sourceType string, fn func(context.Context) error) {
	w.mu.Lock()
	if last, ok := w.lastIngest[sourceType]; ok && time.Since(last) < w.debounce {
		w.mu.Unlock()
		slog.Debug("RAG: skipping re-ingestion (debounced)", "source", sourceType)
		return
	}
	w.lastIngest[sourceType] = time.Now()
	w.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := fn(ctx); err != nil {
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
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

				loaders := []namedLoader{
					{"service", NewServiceDocumentLoader(w.engine.pool, w.tenantID)},
					{"entity", NewEntityDocumentLoader(w.engine.pool, w.tenantID)},
					{"pipeline", NewPipelineDocumentLoader(w.engine.pool, w.tenantID)},
				}

				for _, item := range loaders {
					count, err := w.engine.ReindexAll(ctx, item.loader, w.tenantID)
					if err != nil {
						slog.Warn("RAG: periodic re-index failed", "source", item.name, "error", err)
					} else {
						slog.Info("RAG: periodic re-index complete", "source", item.name, "documents", count)
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
