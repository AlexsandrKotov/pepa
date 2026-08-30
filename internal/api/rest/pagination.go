package rest

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// PageParams represents pagination parameters for list endpoints.
type PageParams struct {
	Limit  int    `form:"limit"`
	Cursor string `form:"cursor"`
}

// PaginatedResponse represents a paginated API response.
type PaginatedResponse struct {
	Items      interface{} `json:"items"`
	Total      int         `json:"total,omitempty"`
	NextCursor string      `json:"next_cursor,omitempty"`
	HasMore    bool        `json:"has_more"`
}

// parsePageParams extracts pagination parameters from the request.
func parsePageParams(c *gin.Context) PageParams {
	p := PageParams{Limit: 50} // Default limit
	
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 200 {
			p.Limit = n
		}
	}
	
	p.Cursor = c.Query("cursor")
	return p
}

// Cursor represents a pagination cursor with an ID.
type Cursor struct {
	ID string `json:"id"`
}

// EncodeCursor creates a base64-encoded cursor from an ID.
func EncodeCursor(id string) string {
	if id == "" {
		return ""
	}
	cursor := Cursor{ID: id}
	data, _ := json.Marshal(cursor)
	return base64.URLEncoding.EncodeToString(data)
}

// DecodeCursor parses a base64-encoded cursor and returns the ID.
func DecodeCursor(cursor string) (string, error) {
	if cursor == "" {
		return "", nil
	}
	
	data, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return "", err
	}
	
	var c Cursor
	if err := json.Unmarshal(data, &c); err != nil {
		return "", err
	}
	
	return c.ID, nil
}

// respondPaginated sends a paginated response.
func respondPaginated(c *gin.Context, items interface{}, total int, nextCursor string, hasMore bool) {
	c.JSON(http.StatusOK, PaginatedResponse{
		Items:      items,
		Total:      total,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	})
}
