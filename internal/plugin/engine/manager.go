package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	hcplugin "github.com/hashicorp/go-plugin"
	"github.com/pepa/pepa/internal/config"
	"github.com/pepa/pepa/internal/database"
	"github.com/pepa/pepa/internal/plugin/signature"
	"github.com/pepa/pepa/pkg/models"

	pb "github.com/pepa/pepa/internal/plugin/proto"
)

// loadedPlugin holds the go-plugin client and derived metadata for a running plugin.
type loadedPlugin struct {
	client     *hcplugin.Client
	grpc       *grpcClient
	info       *pb.InfoResponse
	model      *models.Plugin
	binaryPath string
}

// Manager handles plugin lifecycle: load, unload, execute, health checks.
type Manager struct {
	mu      sync.RWMutex
	cfg     config.PluginConfig
	db      *database.DB
	plugins map[string]*loadedPlugin
}

// NewManager creates a new plugin manager.
func NewManager(cfg config.PluginConfig, db *database.DB) *Manager {
	return &Manager{
		cfg:     cfg,
		db:      db,
		plugins: make(map[string]*loadedPlugin),
	}
}

// LoadPlugin spawns a plugin binary as a subprocess via go-plugin.
func (m *Manager) LoadPlugin(name string, binaryPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.plugins[name]; exists {
		return fmt.Errorf("plugin %q already loaded", name)
	}

	// Verify binary exists and is within the plugin directory
	absPath, err := filepath.Abs(binaryPath)
	if err != nil {
		return fmt.Errorf("invalid binary path: %w", err)
	}
	// Security: ensure the resolved path is within the configured plugin directory
	pluginDir, _ := filepath.Abs(m.cfg.Dir)
	if pluginDir != "" {
		rel, err := filepath.Rel(pluginDir, absPath)
		if err != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("plugin binary path %q escapes plugin directory", absPath)
		}
	}
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return fmt.Errorf("plugin binary not found: %s", absPath)
	}

	// Signature verification — check before spawning the subprocess.
	if m.cfg.SignatureVerify {
		pubKey, keyErr := signature.EmbeddedPublicKey()
		if keyErr != nil {
			return fmt.Errorf("cannot load embedded public key: %w", keyErr)
		}
		if sigErr := signature.VerifyPluginBinary(absPath, pubKey); sigErr != nil {
			if m.cfg.SignatureEnforce {
				log.Printf("[SECURITY] rejecting unsigned plugin %s: %v", name, sigErr)
				return fmt.Errorf("plugin %s rejected: %w", name, sigErr)
			}
			log.Printf("[SECURITY] WARNING: plugin %s failed signature check: %v (loading anyway — enforce mode off)", name, sigErr)
		} else {
			log.Printf("[plugin-manager] plugin %s signature verified", name)
		}
	}

	// Create go-plugin client
	client := hcplugin.NewClient(&hcplugin.ClientConfig{
		HandshakeConfig: Handshake,
		Plugins: map[string]hcplugin.Plugin{
			"pepa-plugin": &GRPCPlugin{},
		},
		AllowedProtocols: []hcplugin.Protocol{hcplugin.ProtocolGRPC},
		Cmd:              exec.Command(absPath),
		Logger: hclog.New(&hclog.LoggerOptions{
			Name:   fmt.Sprintf("plugin:%s", name),
			Output: os.Stderr,
			Level:  hclog.Info,
		}),
	})

	// Connect via gRPC
	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return fmt.Errorf("failed to connect to plugin %s: %w", name, err)
	}

	raw, err := rpcClient.Dispense("pepa-plugin")
	if err != nil {
		client.Kill()
		return fmt.Errorf("failed to dispense plugin %s: %w", name, err)
	}

	grpcCli := raw.(*grpcClient)

	// Fetch plugin info
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	info, err := grpcCli.Info(ctx)
	if err != nil {
		client.Kill()
		return fmt.Errorf("failed to get info from plugin %s: %w", name, err)
	}

	m.plugins[name] = &loadedPlugin{
		client:     client,
		grpc:       grpcCli,
		info:       info,
		binaryPath: absPath,
		model: &models.Plugin{
			Name:    info.Name,
			Version: info.Version,
			Type:    models.PluginType(info.PluginType),
			Status:  models.PluginStatusRunning,
			Enabled: true,
		},
	}

	log.Printf("[plugin-manager] loaded plugin %s@%s type=%s actions=%v",
		info.Name, info.Version, info.PluginType, actionNames(info.Actions))

	return nil
}

// UnloadPlugin kills the plugin subprocess and removes it.
func (m *Manager) UnloadPlugin(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	lp, ok := m.plugins[name]
	if !ok {
		return ErrPluginNotFound(name)
	}

	lp.client.Kill()
	delete(m.plugins, name)
	log.Printf("[plugin-manager] unloaded plugin %s", name)
	return nil
}

// Execute dispatches an action to a loaded plugin via gRPC.
func (m *Manager) Execute(ctx context.Context, name string, action string, params []byte, config map[string]string) (*pb.ExecuteResponse, error) {
	m.mu.RLock()
	lp, ok := m.plugins[name]
	m.mu.RUnlock()

	if !ok {
		return nil, ErrPluginNotFound(name)
	}

	return lp.grpc.Execute(ctx, action, params, "", config)
}

// ExecuteAction is a convenience wrapper that returns raw output bytes.
func (m *Manager) ExecuteAction(ctx context.Context, name string, action string, params []byte, config map[string]string) ([]byte, error) {
	resp, err := m.Execute(ctx, name, action, params, config)
	if err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf("plugin action failed: %s", resp.Error)
	}
	return resp.Output, nil
}

// ListPlugins returns all loaded plugins as models.Plugin.
func (m *Manager) ListPlugins() []*models.Plugin {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*models.Plugin, 0, len(m.plugins))
	for _, lp := range m.plugins {
		result = append(result, lp.model)
	}
	return result
}

// ListLoadedPlugins returns extended info about loaded plugins.
func (m *Manager) ListLoadedPlugins() map[string]*pb.InfoResponse {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*pb.InfoResponse, len(m.plugins))
	for name, lp := range m.plugins {
		result[name] = lp.info
	}
	return result
}

// GetPlugin returns a plugin model by name.
func (m *Manager) GetPlugin(name string) *models.Plugin {
	m.mu.RLock()
	defer m.mu.RUnlock()
	lp, ok := m.plugins[name]
	if !ok {
		return nil
	}
	return lp.model
}

// GetPluginInfo returns the gRPC InfoResponse for a plugin.
func (m *Manager) GetPluginInfo(name string) *pb.InfoResponse {
	m.mu.RLock()
	defer m.mu.RUnlock()
	lp, ok := m.plugins[name]
	if !ok {
		return nil
	}
	return lp.info
}

// Enable activates a plugin (marks it as running).
func (m *Manager) Enable(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	lp, ok := m.plugins[name]
	if !ok {
		return ErrPluginNotFound(name)
	}
	lp.model.Enabled = true
	lp.model.Status = models.PluginStatusRunning
	return nil
}

// Disable deactivates a plugin (marks it as disabled but keeps it loaded).
func (m *Manager) Disable(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	lp, ok := m.plugins[name]
	if !ok {
		return ErrPluginNotFound(name)
	}
	lp.model.Enabled = false
	lp.model.Status = models.PluginStatusDisabled
	return nil
}

// HealthCheck returns the health status of a plugin by calling its gRPC HealthCheck.
func (m *Manager) HealthCheck(name string) *models.PluginHealth {
	m.mu.RLock()
	lp, ok := m.plugins[name]
	m.mu.RUnlock()

	if !ok {
		return &models.PluginHealth{
			PluginName: name,
			Status:     "not_found",
			Message:    "plugin not loaded",
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := lp.grpc.HealthCheck(ctx)
	if err != nil {
		return &models.PluginHealth{
			PluginName: name,
			Status:     "unhealthy",
			Message:    err.Error(),
		}
	}

	return &models.PluginHealth{
		PluginName: name,
		Status:     resp.Status,
		Message:    resp.Message,
		Latency:    time.Duration(resp.LatencyMs) * time.Millisecond,
	}
}

// DiscoverAndLoad scans the plugin directory for binaries and loads them.
func (m *Manager) DiscoverAndLoad() error {
	dir := m.cfg.Dir
	if dir == "" {
		dir = "./plugins"
	}

	loaded, err := m.scanDir(dir)
	if err != nil {
		return err
	}

	// Also scan the bin/ sub-directory (output of `make plugins`).
	// Binaries live at <dir>/bin/<name>/<name>.
	binSubdir := filepath.Join(dir, "bin")
	if info, err := os.Stat(binSubdir); err == nil && info.IsDir() {
		binLoaded, err := m.scanDir(binSubdir)
		if err != nil {
			log.Printf("[plugin-manager] warning: bin/ scan failed: %v", err)
		} else {
			loaded += binLoaded
		}
	}

	// Also scan custom plugin directory if configured
	if m.cfg.CustomDir != "" {
		customLoaded, err := m.scanDir(m.cfg.CustomDir)
		if err != nil {
			log.Printf("[plugin-manager] warning: custom plugin dir scan failed: %v", err)
		} else {
			loaded += customLoaded
		}
	}

	log.Printf("[plugin-manager] discovered and loaded %d plugin(s) total", loaded)
	return nil
}

// scanDir scans a single directory for plugin binaries and loads them.
func (m *Manager) scanDir(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("[plugin-manager] plugin directory %s does not exist, skipping discovery", dir)
			return 0, nil
		}
		return 0, fmt.Errorf("failed to read plugin directory %s: %w", dir, err)
	}

	loaded := 0
	for _, entry := range entries {
		if entry.IsDir() {
			// Look for executable inside subdirectory
			binPath := filepath.Join(dir, entry.Name(), entry.Name())
			if _, err := os.Stat(binPath); err == nil {
				if err := m.LoadPlugin(entry.Name(), binPath); err != nil {
					log.Printf("[plugin-manager] failed to load plugin %s: %v", entry.Name(), err)
					continue
				}
				loaded++
			}
		} else if !entry.IsDir() && isExecutable(entry) {
			// Standalone binary
			name := entry.Name()
			binPath := filepath.Join(dir, name)
			if err := m.LoadPlugin(name, binPath); err != nil {
				log.Printf("[plugin-manager] failed to load plugin %s: %v", name, err)
				continue
			}
			loaded++
		}
	}

	log.Printf("[plugin-manager] discovered and loaded %d plugin(s) from %s", loaded, dir)
	return loaded, nil
}

// Shutdown gracefully stops all plugins.
func (m *Manager) Shutdown(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, lp := range m.plugins {
		log.Printf("[plugin-manager] stopping plugin %s", name)
		lp.client.Kill()
		lp.model.Status = models.PluginStatusStopped
	}
}

// GetGRPCClient returns the raw gRPC client for a plugin (used by provider registry).
func (m *Manager) GetGRPCClient(name string) (*grpcClient, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	lp, ok := m.plugins[name]
	if !ok {
		return nil, ErrPluginNotFound(name)
	}
	return lp.grpc, nil
}

// ── helpers ───────────────────────────────────────────────────

func isExecutable(entry os.DirEntry) bool {
	info, err := entry.Info()
	if err != nil {
		return false
	}
	return info.Mode()&0111 != 0
}

func actionNames(actions []*pb.ActionInfo) []string {
	names := make([]string, len(actions))
	for i, a := range actions {
		names[i] = a.Name
	}
	return names
}

// ── JSON helper for Execute params ────────────────────────────

func MarshalParams(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

// ErrPluginNotFound is returned when a plugin is not found.
type PluginNotFoundError struct{ Name string }

func (e PluginNotFoundError) Error() string { return "plugin not found: " + e.Name }

func ErrPluginNotFound(name string) error { return PluginNotFoundError{Name: name} }
