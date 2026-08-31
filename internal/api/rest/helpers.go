package rest

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

// respondInternalError logs the full error server-side and returns a generic
// error message to the client. The request ID is included so users can
// correlate the error with server logs when reporting issues.
func respondInternalError(c *gin.Context, err error) {
	if err == nil {
		return
	}
	reqID, _ := c.Get("request_id")
	slog.Info("internal error", "request_id", reqID, "path", c.FullPath(), "error", err)
	c.JSON(http.StatusInternalServerError, gin.H{
		"error":      "internal server error",
		"request_id": reqID,
	})
}
