package events

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func setupTestBus(t *testing.T) (*Bus, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return NewBus(client), mr
}

func TestOn_RegistersHandler(t *testing.T) {
	bus, mr := setupTestBus(t)
	defer mr.Close()

	called := false
	bus.On("test.event", func(e Event) {
		called = true
	})

	// Dispatch directly (without Redis pub/sub)
	bus.dispatch(Event{Type: "test.event", TenantID: "t1"})

	if !called {
		t.Error("expected handler to be called")
	}
}

func TestDispatch_MultipleHandlers(t *testing.T) {
	bus, mr := setupTestBus(t)
	defer mr.Close()

	var count int32
	bus.On("multi.event", func(e Event) { atomic.AddInt32(&count, 1) })
	bus.On("multi.event", func(e Event) { atomic.AddInt32(&count, 1) })

	bus.dispatch(Event{Type: "multi.event"})

	if atomic.LoadInt32(&count) != 2 {
		t.Errorf("expected 2 handlers called, got %d", count)
	}
}

func TestDispatch_WildcardHandler(t *testing.T) {
	bus, mr := setupTestBus(t)
	defer mr.Close()

	var received []string
	var mu sync.Mutex
	bus.On("*", func(e Event) {
		mu.Lock()
		received = append(received, e.Type)
		mu.Unlock()
	})

	bus.dispatch(Event{Type: "event.a"})
	bus.dispatch(Event{Type: "event.b"})

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 2 {
		t.Errorf("expected 2 events via wildcard, got %d", len(received))
	}
}

func TestDispatch_TypeSpecificAndWildcard(t *testing.T) {
	bus, mr := setupTestBus(t)
	defer mr.Close()

	var specific, wildcard int32
	bus.On("order.created", func(e Event) { atomic.AddInt32(&specific, 1) })
	bus.On("*", func(e Event) { atomic.AddInt32(&wildcard, 1) })

	bus.dispatch(Event{Type: "order.created"})

	if atomic.LoadInt32(&specific) != 1 {
		t.Errorf("expected specific handler called once, got %d", specific)
	}
	if atomic.LoadInt32(&wildcard) != 1 {
		t.Errorf("expected wildcard handler called once, got %d", wildcard)
	}
}

func TestDispatch_NoMatchingHandler(t *testing.T) {
	bus, mr := setupTestBus(t)
	defer mr.Close()

	// Should not panic
	bus.dispatch(Event{Type: "unhandled.event"})
}

func TestPublish(t *testing.T) {
	bus, mr := setupTestBus(t)
	defer mr.Close()

	err := bus.Publish(Event{
		Type:     "test.publish",
		TenantID: "tenant-1",
		Payload:  map[string]interface{}{"key": "value"},
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
}

func TestStartStop(t *testing.T) {
	bus, mr := setupTestBus(t)
	defer mr.Close()

	bus.Start()

	// Give workers time to start
	time.Sleep(50 * time.Millisecond)

	bus.Stop()

	// Should not panic on double stop
	bus.Stop()
}

func TestEventStruct(t *testing.T) {
	e := Event{
		Type:       "entity.created",
		TenantID:   "tenant-1",
		EntityID:   "entity-1",
		PluginName: "kubernetes",
		Payload:    map[string]interface{}{"name": "test"},
	}
	if e.Type != "entity.created" {
		t.Errorf("unexpected type: %s", e.Type)
	}
	if e.TenantID != "tenant-1" {
		t.Errorf("unexpected tenant: %s", e.TenantID)
	}
}
