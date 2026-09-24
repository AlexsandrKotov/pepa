package rest

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// respondInternalError logs the full error server-side and returns a generic
// error message to the client. The request ID is included so users can
// correlate the error with server logs when reporting issues.
func respondInternalError(c *gin.Context, err error) {
	if err == nil {
		return
	}
	reqID, _ := c.Get("request_id")
	slog.Error("internal error", "request_id", reqID, "path", c.FullPath(), "error", err)
	c.JSON(http.StatusInternalServerError, gin.H{
		"error":      "internal server error",
		"request_id": reqID,
	})
}

// parseUUIDParam extracts a UUID path parameter by name. On parse failure it
// writes a 400 response with a descriptive message and returns false so the
// caller can early-exit.
func parseUUIDParam(c *gin.Context, name, label string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid " + label + " id"})
		return uuid.Nil, false
	}
	return id, true
}

// mapToStringMap converts map[string]any to map[string]string, dropping
// non-string values.
func mapToStringMap(m map[string]any) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}
