package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pepa/pepa/internal/repository"
)

// OutboxRelayConfig configures the outbox relay behavior.
type OutboxRelayConfig struct {
	PollInterval    time.Duration
	BatchSize       int
	RetentionPeriod time.Duration
	CleanupInterval time.Duration
}

// DefaultOutboxRelayConfig returns sensible defaults for the relay.
func DefaultOutboxRelayConfig() OutboxRelayConfig {
	return OutboxRelayConfig{
		PollInterval:    1 * time.Second,
		BatchSize:       100,
		RetentionPeriod: 7 * 24 * time.Hour,
		CleanupInterval: 1 * time.Hour,
	}
}

// OutboxRelay reads events from the outbox table and publishes them to the event bus.
type OutboxRelay struct {
	repo   *repository.OutboxRepository
	bus    *Bus
	config OutboxRelayConfig
	cancel context.CancelFunc
}

// NewOutboxRelay creates a new OutboxRelay.
func NewOutboxRelay(repo *repository.OutboxRepository, bus *Bus, config OutboxRelayConfig) *OutboxRelay {
	return &OutboxRelay{
		repo:   repo,
		bus:    bus,
		config: config,
	}
}

// Start begins the relay loop in a background goroutine.
func (r *OutboxRelay) Start(ctx context.Context) {
	ctx, r.cancel = context.WithCancel(ctx)
	go r.relayLoop(ctx)
	go r.cleanupLoop(ctx)
	slog.Info("outbox relay started",
		"poll_interval", r.config.PollInterval,
		"batch_size", r.config.BatchSize,
		"retention", r.config.RetentionPeriod,
	)
}

// Stop gracefully stops the relay.
func (r *OutboxRelay) Stop() {
	if r.cancel != nil {
		r.cancel()
	}
	slog.Info("outbox relay stopped")
}

func (r *OutboxRelay) relayLoop(ctx context.Context) {
	ticker := time.NewTicker(r.config.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.processBatch(ctx)
		}
	}
}

func (r *OutboxRelay) processBatch(ctx context.Context) {
	events, err := r.repo.FetchUnpublished(ctx, r.config.BatchSize)
	if err != nil {
		slog.Error("fetch outbox events failed", "error", err)
		return
	}
	if len(events) == 0 {
		return
	}
	slog.Debug("processing outbox batch", "count", len(events))

	var publishedIDs []uuid.UUID
	for _, e := range events {
		if err := r.publishEvent(ctx, e); err != nil {
			slog.Error("publish outbox event failed",
				"event_id", e.ID, "event_type", e.EventType, "error", err)
			if markErr := r.repo.MarkFailed(ctx, e.ID, err.Error()); markErr != nil {
				slog.Error("mark failed failed", "error", markErr)
			}
			continue
		}
		publishedIDs = append(publishedIDs, e.ID)
	}
	if len(publishedIDs) > 0 {
		if err := r.repo.BatchMarkPublished(ctx, publishedIDs); err != nil {
			slog.Error("batch mark published failed", "error", err)
		} else {
			slog.Debug("published outbox events", "count", len(publishedIDs))
		}
	}
}

func (r *OutboxRelay) publishEvent(_ context.Context, e repository.OutboxEvent) error {
	busEvent := Event{
		Type:     e.EventType,
		TenantID: e.TenantID.String(),
		EntityID: e.AggregateID.String(),
		Payload:  e.Payload,
	}
	return r.bus.Publish(busEvent)
}

func (r *OutboxRelay) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(r.config.CleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.runCleanup(ctx)
		}
	}
}

func (r *OutboxRelay) runCleanup(ctx context.Context) {
	deleted, err := r.repo.Cleanup(ctx, r.config.RetentionPeriod)
	if err != nil {
		slog.Error("outbox cleanup failed", "error", err)
		return
	}
	if deleted > 0 {
		slog.Info("cleaned up outbox events", "deleted", deleted)
	}
}

// Stats returns current outbox statistics.
func (r *OutboxRelay) Stats(ctx context.Context) (pending int64, failed int64, err error) {
	return r.repo.Stats(ctx)
}

// InsertOutboxEvent inserts an outbox event in the SAME transaction as the business operation.
// The tx parameter MUST NOT be nil — the outbox pattern requires the event and domain
// changes to commit or roll back together.
func InsertOutboxEvent(
	ctx context.Context,
	repo *repository.OutboxRepository,
	tx pgx.Tx,
	aggregateType string,
	aggregateID uuid.UUID,
	eventType string,
	data map[string]any,
	tenantID uuid.UUID,
) error {
	if tx == nil {
		return fmt.Errorf("InsertOutboxEvent: tx must not be nil — use a transaction from the business operation")
	}
	payload := map[string]any{
		"id":        uuid.New().String(),
		"type":      eventType,
		"tenant_id": tenantID.String(),
		"timestamp": time.Now().Format(time.RFC3339Nano),
		"data":      data,
	}
	return repo.Insert(ctx, tx, aggregateType, aggregateID, eventType, payload, tenantID)
}

// OutboxEventToJSON converts an outbox event to JSON for the event bus.
func OutboxEventToJSON(e repository.OutboxEvent) ([]byte, error) {
	return json.Marshal(map[string]any{
		"id":        e.ID,
		"type":      e.EventType,
		"tenant_id": e.TenantID,
		"timestamp": e.CreatedAt,
		"data":      e.Payload,
	})
}
