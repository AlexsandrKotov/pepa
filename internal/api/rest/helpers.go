package rest

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

// respondInternalError logs the full error server-side and returns the error
// message to the client so users can diagnose issues (e.g. terraform not
// installed, repo clone failed). PEPA is an internal developer portal, so
// leaking error details to authenticated users is acceptable.
func respondInternalError(c *gin.Context, err error) {
	if err == nil {
		return
	}
	reqID, _ := c.Get("request_id")
	slog.Info("internal error", "request_id", reqID, "path", c.FullPath(), "error", err)
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}
