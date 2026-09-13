package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pepa/pepa/internal/database"
)

// OutboxEvent represents an event stored in the transactional outbox.
type OutboxEvent struct {
	ID            uuid.UUID
	AggregateType string
	AggregateID   uuid.UUID
	EventType     string
	Payload       map[string]any
	TenantID      uuid.UUID
	CreatedAt     time.Time
	PublishedAt   *time.Time
	Published     bool
	RetryCount    int
	MaxRetries    int
	LastError     *string
}

// OutboxRepository manages the transactional outbox for reliable event publishing.
type OutboxRepository struct {
	db *database.DB
}

// NewOutboxRepository creates a new OutboxRepository.
func NewOutboxRepository(db *database.DB) *OutboxRepository {
	return &OutboxRepository{db: db}
}

// Insert adds an event to the outbox within the current transaction.
// This should be called within the same transaction as the business operation.
func (r *OutboxRepository) Insert(ctx context.Context, tx pgx.Tx, aggregateType string, aggregateID uuid.UUID, eventType string, payload map[string]any, tenantID uuid.UUID) error {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, payload, tenant_id)
		VALUES ($1, $2, $3, $4, $5)
	`, aggregateType, aggregateID, eventType, payloadJSON, tenantID)
	if err != nil {
		return fmt.Errorf("insert outbox event: %w", err)
	}

	return nil
}

// FetchUnpublished retrieves unpublished events ready for publishing.
// Events are ordered by creation time and limited by batchSize.
func (r *OutboxRepository) FetchUnpublished(ctx context.Context, batchSize int) ([]OutboxEvent, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, aggregate_type, aggregate_id, event_type, payload, tenant_id,
		       created_at, published_at, published, retry_count, max_retries, last_error
		FROM outbox_events
		WHERE published = false AND retry_count < max_retries
		ORDER BY created_at ASC
		LIMIT $1
	`, batchSize)
	if err != nil {
		return nil, fmt.Errorf("fetch unpublished: %w", err)
	}
	defer rows.Close()

	var events []OutboxEvent
	for rows.Next() {
		var e OutboxEvent
		var payloadBytes []byte
		if err := rows.Scan(
			&e.ID, &e.AggregateType, &e.AggregateID, &e.EventType,
			&payloadBytes, &e.TenantID, &e.CreatedAt, &e.PublishedAt,
			&e.Published, &e.RetryCount, &e.MaxRetries, &e.LastError,
		); err != nil {
			return nil, fmt.Errorf("scan outbox event: %w", err)
		}
		if err := json.Unmarshal(payloadBytes, &e.Payload); err != nil {
			return nil, fmt.Errorf("unmarshal payload: %w", err)
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// MarkPublished marks an event as successfully published.
func (r *OutboxRepository) MarkPublished(ctx context.Context, eventID uuid.UUID) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE outbox_events
		SET published = true, published_at = NOW()
		WHERE id = $1
	`, eventID)
	if err != nil {
		return fmt.Errorf("mark published: %w", err)
	}
	return nil
}

// MarkFailed increments the retry count and records the error.
func (r *OutboxRepository) MarkFailed(ctx context.Context, eventID uuid.UUID, errMsg string) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE outbox_events
		SET retry_count = retry_count + 1, last_error = $2
		WHERE id = $1
	`, eventID, errMsg)
	if err != nil {
		return fmt.Errorf("mark failed: %w", err)
	}
	return nil
}

// Cleanup removes old published events (older than retention period).
func (r *OutboxRepository) Cleanup(ctx context.Context, retention time.Duration) (int64, error) {
	tag, err := r.db.Pool.Exec(ctx, `
		DELETE FROM outbox_events
		WHERE published = true AND published_at < $1
	`, time.Now().Add(-retention))
	if err != nil {
		return 0, fmt.Errorf("cleanup outbox: %w", err)
	}
	return tag.RowsAffected(), nil
}

// InsertWithTx is a helper that inserts an outbox event within a transaction.
// If tx is nil, it creates a new transaction.
func (r *OutboxRepository) InsertWithTx(ctx context.Context, tx pgx.Tx, aggregateType string, aggregateID uuid.UUID, eventType string, payload map[string]any, tenantID uuid.UUID) error {
	if tx == nil {
		var err error
		tx, err = r.db.Pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin tx: %w", err)
		}
		defer tx.Rollback(ctx)

		if err := r.Insert(ctx, tx, aggregateType, aggregateID, eventType, payload, tenantID); err != nil {
			return err
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit tx: %w", err)
		}
		return nil
	}

	return r.Insert(ctx, tx, aggregateType, aggregateID, eventType, payload, tenantID)
}

// BatchMarkPublished marks multiple events as published in a single transaction.
func (r *OutboxRepository) BatchMarkPublished(ctx context.Context, eventIDs []uuid.UUID) error {
	if len(eventIDs) == 0 {
		return nil
	}

	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, id := range eventIDs {
		if _, err := tx.Exec(ctx, `
			UPDATE outbox_events
			SET published = true, published_at = NOW()
			WHERE id = $1
		`, id); err != nil {
			return fmt.Errorf("mark published %s: %w", id, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// Stats returns outbox statistics for monitoring.
func (r *OutboxRepository) Stats(ctx context.Context) (pending int64, failed int64, err error) {
	err = r.db.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM outbox_events WHERE published = false AND retry_count < max_retries
	`).Scan(&pending)
	if err != nil {
		return 0, 0, fmt.Errorf("count pending: %w", err)
	}

	err = r.db.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM outbox_events WHERE published = false AND retry_count >= max_retries
	`).Scan(&failed)
	if err != nil {
		return 0, 0, fmt.Errorf("count failed: %w", err)
	}

	return pending, failed, nil
}
