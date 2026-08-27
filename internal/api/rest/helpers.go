package rest

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// respondInternalError logs the full error server-side and returns a generic
// message to the client. This prevents leaking internal details (SQL queries,
// connection strings, stack traces) to external callers.
func respondInternalError(c *gin.Context, err error) {
	if err == nil {
		return
	}
	reqID, _ := c.Get("request_id")
	log.Printf("[ERROR] request_id=%v path=%s error=%v", reqID, c.FullPath(), err)
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error, please try again later"})
}
