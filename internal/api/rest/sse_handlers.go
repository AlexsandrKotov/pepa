package rest

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/events"
)

// sseClient represents a single SSE connection.
type sseClient struct {
	id       string
	tenantID uuid.UUID
	events   chan string
	done     chan struct{}
}

// sseHub manages all SSE clients.
type sseHub struct {
	mu      sync.RWMutex
	clients map[*sseClient]bool
}

var hub = &sseHub{
	clients: make(map[*sseClient]bool),
}

func (h *sseHub) add(c *sseClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[c] = true
}

func (h *sseHub) remove(c *sseClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.done)
	}
}

// broadcast sends an event to all connected SSE clients that belong to the
// same tenant. Events with an empty TenantID are broadcast to everyone (e.g.
// system-level notifications).
func (h *sseHub) broadcast(event events.Event) {
	data := fmt.Sprintf("event: %s\ndata: %s\n\n", event.Type, marshalEvent(event))

	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		// Tenant isolation: only deliver events that match the client's tenant.
		// Events with no tenant_id (system broadcasts) are delivered to all.
		if event.TenantID != "" && event.TenantID != c.tenantID.String() {
			continue
		}
		select {
		case c.events <- data:
		default:
			// Client too slow, skip
		}
	}
}

func marshalEvent(e events.Event) string {
	// Simple JSON-like serialization for SSE
	return fmt.Sprintf(`{"type":%q,"tenant_id":%q,"entity_id":%q,"plugin_name":%q}`,
		e.Type, e.TenantID, e.EntityID, e.PluginName)
}

// registerSSERoutes adds the Server-Sent Events endpoint.
// The event bus wildcard handler is registered exactly once here — not per
// connection — to prevent a memory leak from accumulating handlers.
func registerSSERoutes(r *gin.RouterGroup, eventBus *events.Bus) {
	// Register the event bus → hub bridge once at startup.
	if eventBus != nil {
		eventBus.On("*", func(e events.Event) {
			hub.broadcast(e)
		})
	}

	r.GET("/stream", func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")

		// Capture the tenant ID from the JWT so we can filter events.
		tenantID := auth.GetTenantID(c)

		client := &sseClient{
			id:       fmt.Sprintf("%p", c),
			tenantID: tenantID,
			events:   make(chan string, 64),
			done:     make(chan struct{}),
		}
		hub.add(client)
		defer hub.remove(client)

		// Send initial connected event
		c.Writer.WriteString("event: connected\ndata: {\"message\":\"stream started\"}\n\n")
		c.Writer.Flush()

		// Keep-alive ticker
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		c.Stream(func(w io.Writer) bool {
			select {
			case data := <-client.events:
				c.Writer.WriteString(data)
				c.Writer.Flush()
				return true
			case <-ticker.C:
				c.Writer.WriteString(": keepalive\n\n")
				c.Writer.Flush()
				return true
			case <-c.Request.Context().Done():
				return false
			case <-client.done:
				return false
			}
		})
	})
}
