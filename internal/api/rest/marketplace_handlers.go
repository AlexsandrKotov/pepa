package rest

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/provider"
	"github.com/pepa/pepa/internal/repository"
	"gopkg.in/yaml.v3"
)

// validPluginID matches safe plugin identifiers (alphanumeric, underscore, hyphen, 1-64 chars).
var validPluginID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// MarketplacePlugin represents a plugin available in the marketplace
type MarketplacePlugin struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	DisplayName     string         `json:"display_name"`
	Description     string         `json:"description"`
	Version         string         `json:"version"`
	Type            string         `json:"type"`
	Category        string         `json:"category"`
	Author          string         `json:"author"`
	License         string         `json:"license"`
	Installed       bool           `json:"installed"`
	Running         bool           `json:"running"`
	BinaryAvailable bool           `json:"binary_available"`
	Embedded        bool           `json:"embedded"`
	Actions         []ActionDef    `json:"actions"`
	ConfigSchema    map[string]any `json:"config_schema,omitempty"`
	RequiresConfig  []string       `json:"requires_config,omitempty"`
}

// ActionDef describes a single plugin action.
type ActionDef struct {
	Name        string         `json:"name" yaml:"name"`
	Description string         `json:"description" yaml:"description"`
	Parameters  map[string]any `json:"parameters,omitempty" yaml:"parameters,omitempty"`
}

// pluginYAML is the on-disk plugin definition.
type pluginYAML struct {
	Name         string       `yaml:"name"`
	Version      string       `yaml:"version"`
	DisplayName  string       `yaml:"display_name"`
	Description  string       `yaml:"description"`
	Category     string       `yaml:"category"`
	Type         string       `yaml:"type"`
	Author       string       `yaml:"author"`
	License      string       `yaml:"license"`
	ConfigSchema configSchema `yaml:"config_schema"`
	Actions      []ActionDef  `yaml:"actions"`
}

type configSchema struct {
	Type       string                `yaml:"type"`
	Properties map[string]schemaProp `yaml:"properties"`
	Required   []string              `yaml:"required"`
}

type schemaProp struct {
	Type        string `yaml:"type"`
	Description string `yaml:"description"`
	Default     any    `yaml:"default"`
}

var (
	marketplaceCache []MarketplacePlugin
	marketplaceMu    sync.RWMutex
)

// loadMarketplacePlugins reads real plugin definitions from plugins/builtin/*/plugin.yaml
func loadMarketplacePlugins(pluginDir string) []MarketplacePlugin {
	marketplaceMu.RLock()
	if marketplaceCache != nil {
		defer marketplaceMu.RUnlock()
		return marketplaceCache
	}
	marketplaceMu.RUnlock()

	marketplaceMu.Lock()
	defer marketplaceMu.Unlock()

	// Double-check after acquiring write lock
	if marketplaceCache != nil {
		return marketplaceCache
	}

	builtinDir := filepath.Join(pluginDir, "builtin")
	entries, err := os.ReadDir(builtinDir)
	if err != nil {
		slog.Info("warning: cannot read builtin plugins dir ", "path", builtinDir, "error", err)
		marketplaceCache = []MarketplacePlugin{}
		return marketplaceCache
	}

	var plugins []MarketplacePlugin
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		yamlPath := filepath.Join(builtinDir, entry.Name(), "plugin.yaml")
		data, err := os.ReadFile(yamlPath) //nolint:gosec // G304: yamlPath is from a controlled builtin plugins directory
		if err != nil {
			slog.Info("skip : no plugin.yaml", "name", entry.Name())
			continue
		}

		// Parse multi-document YAML (some files have --- separators)
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		for {
			var def pluginYAML
			if err := decoder.Decode(&def); err != nil {
				if err == io.EOF {
					break
				}
				slog.Info("skip document in ", "path", yamlPath, "error", err)
				continue
			}
			if def.Name == "" {
				continue
			}

			// Check if binary exists (built by `make plugins` into bin/builtin/<name>/<name>)
			binPath := filepath.Join(pluginDir, "bin", "builtin", def.Name, def.Name)
			binaryAvailable := false
			if info, err := os.Stat(binPath); err == nil && !info.IsDir() {
				binaryAvailable = true
			}

			// Detect embedded plugins: logic runs inside the API server,
			// no separate gRPC binary needed. These have a plugin.yaml in
			// plugins/builtin/ but no Go source in plugins/<name>/.
			srcDir := filepath.Join(pluginDir, def.Name)
			embedded := false
			if entries, err := os.ReadDir(srcDir); err != nil || len(entries) == 0 {
				// No source directory or empty — plugin is embedded in the API server
				embedded = true
				binaryAvailable = true // API server is the "binary"
			}

			// Extract required config fields
			var requiresConfig []string
			if def.ConfigSchema.Required != nil {
				requiresConfig = def.ConfigSchema.Required
			}

			// Convert config schema to generic map for JSON
			var configSchemaMap map[string]any
			if schemaJSON, err := yaml.Marshal(def.ConfigSchema); err == nil {
				_ = yaml.Unmarshal(schemaJSON, &configSchemaMap)
			}

			// Convert actions to have descriptions
			actions := make([]ActionDef, len(def.Actions))
			copy(actions, def.Actions)

			displayName := def.DisplayName
			if displayName == "" {
				displayName = def.Name
			}

			plugins = append(plugins, MarketplacePlugin{
				ID:              def.Name,
				Name:            displayName,
				DisplayName:     displayName,
				Description:     def.Description,
				Version:         def.Version,
				Type:            def.Category,
				Category:        categoryLabel(def.Category),
				Author:          def.Author,
				License:         def.License,
				Installed:       false,
				Running:         false,
				BinaryAvailable: binaryAvailable,
				Embedded:        embedded,
				Actions:         actions,
				ConfigSchema:    configSchemaMap,
				RequiresConfig:  requiresConfig,
			})
		}
	}

	slog.Info("loaded real plugin definitions from builtin dir", "count", len(plugins), "dir", builtinDir)
	marketplaceCache = plugins
	return marketplaceCache
}

// categoryLabel maps internal category to display label.
func categoryLabel(cat string) string {
	labels := map[string]string{
		"git_provider":   "Source Control",
		"cd_engine":      "CD / GitOps",
		"task_tracker":   "Project Management",
		"notification":   "Notifications",
		"monitoring":     "Observability",
		"secret_manager": "Security",
		"cloud_provider": "Infrastructure",
		"ci_engine":      "CI / Automation",
		"virtualization": "Virtualization",
		"storage":        "Object Storage",
	}
	if label, ok := labels[cat]; ok {
		return label
	}
	return cat
}

// reloadMarketplaceCache forces a cache refresh (called after install/uninstall).
func reloadMarketplaceCache() {
	marketplaceMu.Lock()
	marketplaceCache = nil
	marketplaceMu.Unlock()
}

func registerMarketplaceRoutes(r *gin.RouterGroup, deps Dependencies) {
	marketplace := r.Group("/marketplace")
	{
		marketplace.GET("", listMarketplacePlugins(deps))
		marketplace.GET("/:id", getMarketplacePlugin(deps))
		marketplace.POST("/:id/install", installMarketplacePlugin(deps))
		marketplace.POST("/:id/uninstall", uninstallMarketplacePlugin(deps))
	}
}

func listMarketplacePlugins(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		pluginDir := "./plugins"
		if deps.Config != nil && deps.Config.Plugin.Dir != "" {
			pluginDir = deps.Config.Plugin.Dir
		}

		plugins := loadMarketplacePlugins(pluginDir)

		// Build result with live status
		result := make([]MarketplacePlugin, len(plugins))
		copy(result, plugins)

		// Check which plugins are installed in the database.
		// Any enabled row that is not explicitly uninstalled counts as installed
		// (covers both marketplace installs and legacy "running" rows).
		installedMap := make(map[string]bool)
		if deps.Repos.Plugin != nil {
			installedPlugins, err := deps.Repos.Plugin.List(c.Request.Context())
			if err == nil {
				for _, p := range installedPlugins {
					if p.Enabled && p.Status != "uninstalled" {
						installedMap[p.Name] = true
					}
				}
			}
		}

		// Check which plugins are running in the engine
		runningMap := make(map[string]bool)
		if deps.PluginMgr != nil {
			loadedPlugins := deps.PluginMgr.ListPlugins()
			for _, p := range loadedPlugins {
				if p.Status == "running" {
					runningMap[p.Name] = true
				}
			}
		}

		// Update live status
		for i := range result {
			if installedMap[result[i].ID] {
				result[i].Installed = true
			}
			if runningMap[result[i].ID] {
				result[i].Running = true
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"plugins": result,
			"total":   len(result),
		})
	}
}

func getMarketplacePlugin(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if !validPluginID.MatchString(id) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid plugin id"})
			return
		}

		pluginDir := "./plugins"
		if deps.Config != nil && deps.Config.Plugin.Dir != "" {
			pluginDir = deps.Config.Plugin.Dir
		}

		plugins := loadMarketplacePlugins(pluginDir)

		var found *MarketplacePlugin
		for i := range plugins {
			if plugins[i].ID == id {
				found = &plugins[i]
				break
			}
		}

		if found == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "plugin not found in marketplace"})
			return
		}

		// Check installed status
		if deps.Repos.Plugin != nil {
			plugin, err := deps.Repos.Plugin.GetByName(c.Request.Context(), id)
			if err == nil && plugin != nil && plugin.Enabled && plugin.Status != "uninstalled" {
				found.Installed = true
			}
		}

		// Check running status
		if deps.PluginMgr != nil {
			p := deps.PluginMgr.GetPlugin(id)
			if p != nil && p.Status == "running" {
				found.Running = true
			}
		}

		c.JSON(http.StatusOK, found)
	}
}

// loadPluginBinary ensures a plugin binary is loaded and active in the engine.
// Binaries discovered at startup stay unloaded until explicitly installed, so
// this loads the binary on demand (or re-activates it if it is already loaded).
func loadPluginBinary(deps Dependencies, id string) error {
	if deps.PluginMgr == nil {
		return fmt.Errorf("plugin manager not available")
	}
	if deps.PluginMgr.GetPlugin(id) != nil {
		// Already loaded (e.g. discovered at startup) — just activate it.
		return deps.PluginMgr.Enable(id)
	}

	pluginDir := "./plugins"
	if deps.Config != nil && deps.Config.Plugin.Dir != "" {
		pluginDir = deps.Config.Plugin.Dir
	}

	binPath := filepath.Join(pluginDir, "bin", "builtin", id, id)
	if _, err := os.Stat(binPath); err != nil {
		return fmt.Errorf("plugin binary not found: build it with `make plugins` and place it in plugins/bin/builtin/%s", id)
	}

	// Validate resolved path is within plugin directory
	absBin, err1 := filepath.Abs(binPath)
	absDir, err2 := filepath.Abs(pluginDir)
	if err1 != nil || err2 != nil {
		return fmt.Errorf("resolve plugin binary path: %v / %v", err1, err2)
	}
	rel, relErr := filepath.Rel(absDir, absBin)
	if relErr == nil && strings.HasPrefix(rel, "..") {
		return fmt.Errorf("plugin binary path escapes plugin dir")
	}

	return deps.PluginMgr.LoadPlugin(id, binPath)
}

func installMarketplacePlugin(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Only platform admins can install plugins (plugins are global, affect all tenants)
		if !auth.IsPlatformAdmin(c) {
			c.JSON(http.StatusForbidden, gin.H{"error": "only platform administrators can install plugins"})
			return
		}

		id := c.Param("id")
		if !validPluginID.MatchString(id) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid plugin id"})
			return
		}

		if deps.Repos.Plugin == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "plugin repository not available"})
			return
		}

		pluginDir := "./plugins"
		if deps.Config != nil && deps.Config.Plugin.Dir != "" {
			pluginDir = deps.Config.Plugin.Dir
		}

		// Find plugin in marketplace definitions
		plugins := loadMarketplacePlugins(pluginDir)
		var found *MarketplacePlugin
		for i := range plugins {
			if plugins[i].ID == id {
				found = &plugins[i]
				break
			}
		}

		if found == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "plugin not found in marketplace"})
			return
		}

		// Block installation when the plugin binary is not built/available.
		// A plugin without a binary cannot run, so registering it would be misleading.
		if !found.BinaryAvailable {
			c.JSON(http.StatusConflict, gin.H{
				"error": "plugin binary is not available — build it with `make plugins` before installing",
			})
			return
		}

		// Check if already installed
		existing, err := deps.Repos.Plugin.GetByName(c.Request.Context(), id)
		if err == nil && existing != nil && existing.Enabled && existing.Status != "uninstalled" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "plugin already installed"})
			return
		}

		// Register plugin in database (preserve config from a previous install)
		plugin := &repository.Plugin{
			Name:    found.ID,
			Version: found.Version,
			Type:    found.Type,
			Status:  "installed",
			Enabled: true,
		}
		if existing != nil {
			plugin.Config = existing.Config
		}

		if err := deps.Repos.Plugin.Register(c.Request.Context(), plugin); err != nil {
			respondInternalError(c, err)
			return
		}

		// Load (or re-activate) the plugin binary. Binaries discovered at startup
		// stay unloaded until explicitly installed, so this is what activates it.
		// Embedded plugins have no separate binary — their logic runs inside the
		// API server, so they are always "running" once registered.
		binaryLoaded := false
		if found.Embedded {
			binaryLoaded = true // embedded plugins are always active
			slog.Info("embedded plugin installed", "id", id)
		} else if found.BinaryAvailable {
			if err := loadPluginBinary(deps, found.ID); err != nil {
				slog.Info("plugin registered but failed to load binary", "id", id, "error", err)
			} else {
				binaryLoaded = true
			}
		}

		// Register in the provider registry so actions are available immediately.
		if binaryLoaded && deps.ProviderRegistry != nil && deps.PluginMgr != nil {
			if info := deps.PluginMgr.GetPluginInfo(found.ID); info != nil {
				if grpcClient, err := deps.PluginMgr.GetGRPCClient(found.ID); err == nil {
					deps.ProviderRegistry.Register(&provider.PluginEntry{
						Name:     found.ID,
						Type:     info.PluginType,
						Info:     info,
						Executor: grpcClient,
						Enabled:  true,
					})
				}
			}
		}

		// Reflect the live state: binary loaded means the plugin is running.
		if binaryLoaded {
			plugin.Status = "running"
			if err := deps.Repos.Plugin.Register(c.Request.Context(), plugin); err != nil {
				slog.Info("plugin loaded but failed to persist status", "id", id, "error", err)
			}
		}

		// Invalidate cache so status updates
		reloadMarketplaceCache()

		logAudit(deps, c, "install", "marketplace_plugin", id, nil, gin.H{"plugin": found.ID, "version": found.Version})

		c.JSON(http.StatusOK, gin.H{
			"message":          "plugin installed successfully",
			"plugin":           plugin,
			"binary_available": found.BinaryAvailable,
			"hint":             getInstallHint(found),
		})
	}
}

func uninstallMarketplacePlugin(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Only platform admins can uninstall plugins (plugins are global, affect all tenants)
		if !auth.IsPlatformAdmin(c) {
			c.JSON(http.StatusForbidden, gin.H{"error": "only platform administrators can uninstall plugins"})
			return
		}

		id := c.Param("id")
		if !validPluginID.MatchString(id) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid plugin id"})
			return
		}

		if deps.Repos.Plugin == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "plugin repository not available"})
			return
		}

		// Get plugin
		plugin, err := deps.Repos.Plugin.GetByName(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "plugin not installed"})
			return
		}

		// Unload from engine if running
		if deps.PluginMgr != nil {
			_ = deps.PluginMgr.UnloadPlugin(id)
		}

		// Remove from provider registry so no further actions are dispatched.
		if deps.ProviderRegistry != nil {
			deps.ProviderRegistry.Unregister(id)
		}

		// Mark as uninstalled in DB
		plugin.Enabled = false
		plugin.Status = "uninstalled"
		if err := deps.Repos.Plugin.Register(c.Request.Context(), plugin); err != nil {
			respondInternalError(c, err)
			return
		}

		// Invalidate cache
		reloadMarketplaceCache()

		logAudit(deps, c, "uninstall", "marketplace_plugin", id, nil, gin.H{"plugin": plugin.Name})

		c.JSON(http.StatusOK, gin.H{
			"message": "plugin uninstalled successfully",
		})
	}
}

// getInstallHint returns a helpful message after installation.
func getInstallHint(p *MarketplacePlugin) string {
	if !p.BinaryAvailable {
		return "Plugin registered but no binary found. Build the plugin binary and place it in plugins/bin/" + p.ID
	}
	if len(p.RequiresConfig) > 0 {
		return "Plugin installed. Configure required settings: " + joinStrings(p.RequiresConfig)
	}
	return "Plugin installed and ready to use."
}

func joinStrings(ss []string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += ", "
		}
		result += s
	}
	return result
}
