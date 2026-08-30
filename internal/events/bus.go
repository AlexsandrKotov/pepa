package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// DefaultChannel is the default Redis pub/sub channel.
	DefaultChannel = "pepa:events"
	// workerPoolSize is the number of event dispatch workers.
	// 8 workers is sufficient for single/multi-tenant deployments;
	// each worker dispatches events synchronously to registered handlers.
	workerPoolSize = 8
	// DeadLetterChannel is the Redis key for events that could not be dispatched.
	DeadLetterChannel = "pepa:events:dead"
	// dispatchTimeout is how long to wait before dropping an event to the dead-letter queue.
	dispatchTimeout = 5 * time.Second
)

// Bus handles event publishing and subscription via Redis Pub/Sub.
type Bus struct {
	client   *redis.Client
	channel  string
	mu       sync.RWMutex
	handlers map[string][]func(Event)
	ctx      context.Context
	cancel   context.CancelFunc
	eventCh  chan Event

	// dropped counts — atomic for lock-free reads
	droppedCount atomic.Uint64
}

// NewBus creates a new event bus backed by Redis pub/sub.
func NewBus(client *redis.Client) *Bus {
	ctx, cancel := context.WithCancel(context.Background())
	return &Bus{
		client:   client,
		channel:  DefaultChannel,
		handlers: make(map[string][]func(Event)),
		ctx:      ctx,
		cancel:   cancel,
		eventCh:  make(chan Event, workerPoolSize),
	}
}

// Event represents a platform event.
type Event struct {
	Type       string                 `json:"type"`
	TenantID   string                 `json:"tenant_id"`
	EntityID   string                 `json:"entity_id,omitempty"`
	PluginName string                 `json:"plugin_name,omitempty"`
	Payload    map[string]interface{} `json:"payload"`
}

// Publish sends an event to the bus.
func (b *Bus) Publish(event Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	return b.client.Publish(b.ctx, b.channel, data).Err()
}

// On registers a handler for a specific event type.
func (b *Bus) On(eventType string, handler func(Event)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventType] = append(b.handlers[eventType], handler)
}

// Start begins listening for events from Redis pub/sub.
func (b *Bus) Start() {
	// Start worker pool for event dispatch
	var wg sync.WaitGroup
	for i := 0; i < workerPoolSize; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-b.ctx.Done():
					return
				case event, ok := <-b.eventCh:
					if !ok {
						return
					}
					b.dispatch(event)
				}
			}
		}()
	}

	pubsub := b.client.Subscribe(b.ctx, b.channel)
	ch := pubsub.Channel()

	go func() {
		for {
			select {
			case <-b.ctx.Done():
				_ = pubsub.Close()
				close(b.eventCh)
				wg.Wait()
				return
			case msg, ok := <-ch:
				if !ok {
					close(b.eventCh)
					wg.Wait()
					return
				}
				var event Event
				if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
					slog.Info("events: unmarshal error", "error", err)
					continue
				}
				// Send to worker pool with timeout; fall back to dead-letter queue.
				select {
				case b.eventCh <- event:
				case <-time.After(dispatchTimeout):
					b.droppedCount.Add(1)
					slog.Info("events: worker pool saturated after , sending event type= to dead-letter queue", "arg1", dispatchTimeout, "type", event.Type)
					b.sendToDeadLetter(event)
				}
			}
		}
	}()
}

// dispatch sends an event to all registered handlers synchronously.
// Called from worker pool goroutines.
func (b *Bus) dispatch(event Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// Call type-specific handlers
	if handlers, ok := b.handlers[event.Type]; ok {
		for _, h := range handlers {
			h(event)
		}
	}

	// Call wildcard handlers
	if handlers, ok := b.handlers["*"]; ok {
		for _, h := range handlers {
			h(event)
		}
	}
}

// Stop shuts down the event bus.
func (b *Bus) Stop() {
	b.cancel()
}

// DroppedCount returns the number of events that were sent to the dead-letter
// queue because the worker pool was saturated.
func (b *Bus) DroppedCount() uint64 {
	return b.droppedCount.Load()
}

// sendToDeadLetter persists an event to a Redis list so it can be inspected
// or replayed later.
func (b *Bus) sendToDeadLetter(event Event) {
	data, err := json.Marshal(event)
	if err != nil {
		slog.Info("events: failed to marshal dead-letter event", "error", err)
		return
	}
	if err := b.client.LPush(b.ctx, DeadLetterChannel, data).Err(); err != nil {
		slog.Info("events: failed to push dead-letter event", "error", err)
	}
}
