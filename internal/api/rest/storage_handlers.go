package rest

import (
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/pepa/pepa/internal/storage"
)

// registerStorageRoutes adds plugin storage endpoints (backed by local or S3 storage).
func registerStorageRoutes(r *gin.RouterGroup, deps Dependencies) {
	plugins := r.Group("/storage/plugins")
	{
		plugins.GET("", listStoredPlugins(deps))
		plugins.POST("/upload", uploadPlugin(deps))
		plugins.GET("/:name/:version/download", downloadPlugin(deps))
		plugins.DELETE("/:name/:version", deletePlugin(deps))
		plugins.GET("/:name/:version", getPluginMeta(deps))
	}
}

func listStoredPlugins(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Storage == nil || !deps.Storage.IsAvailable() {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "plugin storage not available"})
			return
		}

		objects, err := deps.Storage.ListPlugins(c.Request.Context())
		if err != nil {
			respondInternalError(c, err)
			return
		}

		// Enrich with parsed name/version
		type pluginEntry struct {
			Name         string `json:"name"`
			Version      string `json:"version"`
			Size         int64  `json:"size"`
			ContentType  string `json:"content_type"`
			LastModified string `json:"last_modified"`
		}

		entries := make([]pluginEntry, 0, len(objects))
		for _, obj := range objects {
			name, version := storage.ParsePluginKey(obj.Key)
			entries = append(entries, pluginEntry{
				Name:         name,
				Version:      version,
				Size:         obj.Size,
				ContentType:  obj.ContentType,
				LastModified: obj.LastModified.Format("2006-01-02T15:04:05Z"),
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"plugins": entries,
			"total":   len(entries),
		})
	}
}

func uploadPlugin(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Storage == nil || !deps.Storage.IsAvailable() {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "plugin storage not available"})
			return
		}

		// Parse multipart form (max 100MB)
		if err := c.Request.ParseMultipartForm(100 << 20); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid multipart form: " + err.Error()})
			return
		}

		name := c.Request.FormValue("name")
		version := c.Request.FormValue("version")
		if name == "" || version == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "name and version are required"})
			return
		}

		// Sanitize inputs
		name = strings.ReplaceAll(name, "/", "-")
		version = strings.ReplaceAll(version, "/", "-")

		file, header, err := c.Request.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "file field is required: " + err.Error()})
			return
		}
		defer func() { _ = file.Close() }()

		if err := deps.Storage.UploadPlugin(c.Request.Context(), name, version, file, header.Size); err != nil {
			respondInternalError(c, err)
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"message": "plugin uploaded",
			"name":    name,
			"version": version,
			"size":    header.Size,
		})
	}
}

func downloadPlugin(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Storage == nil || !deps.Storage.IsAvailable() {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "plugin storage not available"})
			return
		}

		name := c.Param("name")
		version := c.Param("version")

		reader, err := deps.Storage.DownloadPlugin(c.Request.Context(), name, version)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "plugin not found"})
			return
		}
		defer func() { _ = reader.Close() }()

		c.Header("Content-Disposition", "attachment; filename="+name)
		c.Header("Content-Type", "application/octet-stream")
		c.Status(http.StatusOK)

		if _, err := io.Copy(c.Writer, reader); err != nil {
			slog.Info("download stream error", "error", err)
		}
	}
}

func deletePlugin(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Storage == nil || !deps.Storage.IsAvailable() {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "plugin storage not available"})
			return
		}

		name := c.Param("name")
		version := c.Param("version")

		if err := deps.Storage.DeletePlugin(c.Request.Context(), name, version); err != nil {
			respondInternalError(c, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "plugin deleted",
			"name":    name,
			"version": version,
		})
	}
}

func getPluginMeta(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Storage == nil || !deps.Storage.IsAvailable() {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "plugin storage not available"})
			return
		}

		name := c.Param("name")
		version := c.Param("version")

		meta, err := deps.Storage.StatPlugin(c.Request.Context(), name, version)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "plugin not found"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"name":          name,
			"version":       version,
			"size":          meta.Size,
			"content_type":  meta.ContentType,
			"last_modified": meta.LastModified.Format("2006-01-02T15:04:05Z"),
		})
	}
}
