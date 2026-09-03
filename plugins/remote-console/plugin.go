package main

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/pepa/pepa/internal/plugin/sdk-go"
	"github.com/pepa/pepa/internal/provider"
	"golang.org/x/crypto/ssh"
)

// RemoteConsolePlugin implements provider.Provider for SSH remote console.
// The plugin handles SSH connectivity testing; host CRUD and WebSocket terminal
// are managed directly by the API server (embedded routes).
type RemoteConsolePlugin struct{}

var _ provider.Provider = (*RemoteConsolePlugin)(nil)

func (p *RemoteConsolePlugin) Name() string        { return "remote-console" }
func (p *RemoteConsolePlugin) Version() string     { return "1.0.0" }
func (p *RemoteConsolePlugin) Description() string { return "SSH remote console — test connectivity to hosts via SSH" }
func (p *RemoteConsolePlugin) PluginType() string  { return "infrastructure" }

func (p *RemoteConsolePlugin) Actions() []string {
	return []string{"test_connection"}
}

func (p *RemoteConsolePlugin) Execute(ctx context.Context, action string, params []byte, config map[string]string) ([]byte, error) {
	switch action {
	case "test_connection":
		return p.testConnection(ctx, params, config)
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

func (p *RemoteConsolePlugin) HealthCheck(ctx context.Context) (*provider.HealthStatus, error) {
	return &provider.HealthStatus{
		Status:  "healthy",
		Message: "Remote Console plugin ready — SSH connectivity testing available",
	}, nil
}

// testConnection attempts an SSH connection to verify reachability.
//
// Params (JSON):
//
//	{
//	  "hostname": "192.168.1.10",
//	  "port": 22,              // optional, default 22
//	  "username": "root",      // optional, default "root"
//	  "auth_method": "none",   // "none", "password", "key"
//	  "password": "...",       // required when auth_method=password
//	  "ssh_key": "...",        // required when auth_method=key
//	  "timeout": 10            // optional, seconds, default from config or 30
//	}
func (p *RemoteConsolePlugin) testConnection(ctx context.Context, params []byte, config map[string]string) ([]byte, error) {
	var req struct {
		Hostname   string `json:"hostname"`
		Port       int    `json:"port"`
		Username   string `json:"username"`
		AuthMethod string `json:"auth_method"`
		Password   string `json:"password"`
		SSHKey     string `json:"ssh_key"`
		Timeout    int    `json:"timeout"`
	}
	if err := sdk.JSONUnmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("parse params: %w", err)
	}
	if req.Hostname == "" {
		return nil, fmt.Errorf("hostname is required")
	}

	if req.Port == 0 {
		if v, err := strconv.Atoi(config["default_ssh_port"]); err == nil && v > 0 {
			req.Port = v
		} else {
			req.Port = 22
		}
	}
	if req.Username == "" {
		req.Username = "root"
	}
	if req.Timeout == 0 {
		if v, err := strconv.Atoi(config["connection_timeout"]); err == nil && v > 0 {
			req.Timeout = v
		} else {
			req.Timeout = 30
		}
	}

	sshConfig := &ssh.ClientConfig{
		User:            req.Username,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // #nosec // G302: user-configured target host
		Timeout:         time.Duration(req.Timeout) * time.Second,
	}

	switch req.AuthMethod {
	case "password":
		if req.Password == "" {
			return nil, fmt.Errorf("password is required for password auth")
		}
		sshConfig.Auth = []ssh.AuthMethod{ssh.Password(req.Password)}
	case "key":
		if req.SSHKey == "" {
			return nil, fmt.Errorf("ssh_key is required for key auth")
		}
		signer, err := ssh.ParsePrivateKey([]byte(req.SSHKey))
		if err != nil {
			return nil, fmt.Errorf("invalid SSH key: %w", err)
		}
		sshConfig.Auth = []ssh.AuthMethod{ssh.PublicKeys(signer)}
	case "none", "":
		// No authentication — just test TCP reachability + SSH handshake
	default:
		return nil, fmt.Errorf("unsupported auth_method: %s", req.AuthMethod)
	}

	addr := net.JoinHostPort(req.Hostname, strconv.Itoa(req.Port))

	start := time.Now()
	client, err := ssh.Dial("tcp", addr, sshConfig)
	latency := time.Since(start)

	if err != nil {
		// Return a structured error result (not a plugin error — the connection
		// attempt itself succeeded, but the SSH handshake failed).
		return sdk.ActionOutput(map[string]interface{}{
			"status":  "error",
			"message": fmt.Sprintf("SSH connection failed: %v", err),
			"latency": latency.Milliseconds(),
		})
	}
	_ = client.Close()

	return sdk.ActionOutput(map[string]interface{}{
		"status":  "connected",
		"message": fmt.Sprintf("Successfully connected to %s@%s:%d", req.Username, req.Hostname, req.Port),
		"latency": latency.Milliseconds(),
	})
}
