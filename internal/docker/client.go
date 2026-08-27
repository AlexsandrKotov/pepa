// Package docker provides a thin wrapper around the Docker CLI for managing
// Docker hosts and Compose stacks. It uses DOCKER_HOST env var to support
// local, TCP+TLS, and SSH connections uniformly.
package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// HostConfig holds the connection details for a Docker host.
type HostConfig struct {
	HostType    string // "local", "tcp", "ssh"
	HostAddress string // e.g. "unix:///var/run/docker.sock", "tcp://1.2.3.4:2376", "ssh://user@host"
	TLSCACert   string
	TLSCert     string
	TLSKey      string
	SSHKey      string
}

// DockerInfo holds basic info from `docker info`.
type DockerInfo struct {
	Version           string `json:"version"`
	OS                string `json:"operating_system"`
	Arch              string `json:"architecture"`
	ContainersTotal   int    `json:"containers_total"`
	ContainersRunning int    `json:"containers_running"`
}

// ContainerInfo holds info about a single container from `docker compose ps`.
type ContainerInfo struct {
	Name    string `json:"name"`
	Image   string `json:"image"`
	State   string `json:"state"`
	Status  string `json:"status"`
	Ports   string `json:"ports"`
	Service string `json:"service"`
}

// DiscoveredContainer holds info about a running container from `docker ps`.
type DiscoveredContainer struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Image   string            `json:"image"`
	State   string            `json:"state"`
	Status  string            `json:"status"`
	Ports   string            `json:"ports"`
	Labels  map[string]string `json:"labels"`
	Created string            `json:"created"`
	// ComposeProject is set if the container belongs to a compose stack
	ComposeProject string `json:"compose_project,omitempty"`
}

// Client wraps Docker CLI operations for a specific host.
type Client struct {
	cfg HostConfig
}

// NewClient creates a new Docker CLI client for the given host config.
func NewClient(cfg HostConfig) *Client {
	return &Client{cfg: cfg}
}

// env builds the environment variables for Docker CLI commands.
func (c *Client) env() []string {
	env := os.Environ()
	env = append(env, "DOCKER_HOST="+c.cfg.HostAddress)

	if c.cfg.HostType == "tcp" && c.cfg.TLSCert != "" {
		env = append(env, "DOCKER_TLS_VERIFY=1")
		if c.cfg.TLSCACert != "" {
			dir := writeCertsToTempDir(c.cfg.TLSCACert, c.cfg.TLSCert, c.cfg.TLSKey)
			if dir != "" {
				env = append(env, "DOCKER_CERT_PATH="+dir)
			}
		}
	}

	if c.cfg.HostType == "ssh" && c.cfg.SSHKey != "" {
		keyPath := writeSSHKeyToTempFile(c.cfg.SSHKey)
		if keyPath != "" {
			env = append(env, "GIT_SSH_COMMAND=ssh -i "+keyPath)
		}
	}

	return env
}

// run executes a docker CLI command and returns stdout.
func (c *Client) run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", args...) //nolint:gosec // G204: docker is an admin-configured binary
	cmd.Env = c.env()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docker %s: %s: %w", strings.Join(args, " "), stderr.String(), err)
	}
	return stdout.String(), nil
}

// TestConnection runs `docker info` and parses the result.
// Container counts are taken from `docker ps -a` for accuracy (OrbStack's
// `docker info` inflates the counts with internal VM containers).
func (c *Client) TestConnection(ctx context.Context) (*DockerInfo, error) {
	out, err := c.run(ctx, "info", "--format", "{{json .}}")
	if err != nil {
		return nil, err
	}

	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("parsing docker info: %w", err)
	}

	info := &DockerInfo{}
	if v, ok := raw["ServerVersion"].(string); ok {
		info.Version = v
	}
	if v, ok := raw["OperatingSystem"].(string); ok {
		info.OS = v
	}
	if v, ok := raw["Architecture"].(string); ok {
		info.Arch = v
	}

	// Get accurate container counts from `docker ps -a` instead of docker info.
	psOut, err := c.run(ctx, "ps", "-a", "--format", "{{json .}}")
	if err == nil {
		total := 0
		running := 0
		for _, line := range strings.Split(strings.TrimSpace(psOut), "\n") {
			if line == "" {
				continue
			}
			var ctr struct {
				State string `json:"State"`
			}
			if json.Unmarshal([]byte(line), &ctr) == nil {
				total++
				if strings.EqualFold(ctr.State, "running") {
					running++
				}
			}
		}
		info.ContainersTotal = total
		info.ContainersRunning = running
	} else {
		// Fallback to docker info counts if `docker ps` fails.
		containersTotal := raw["ContainersTotal"]
		if containersTotal == nil {
			containersTotal = raw["Containers"]
		}
		if v, ok := containersTotal.(float64); ok {
			info.ContainersTotal = int(v)
		}
		if v, ok := raw["ContainersRunning"].(float64); ok {
			info.ContainersRunning = int(v)
		}
	}

	return info, nil
}

// ComposeUp deploys a compose stack.
func (c *Client) ComposeUp(ctx context.Context, projectName, composeYaml string, envVars map[string]string) error {
	dir, err := os.MkdirTemp("", "pepa-compose-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	composePath := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(composeYaml), 0600); err != nil {
		return fmt.Errorf("writing compose file: %w", err)
	}

	args := []string{"compose", "-f", composePath, "-p", projectName, "up", "-d"}
	cmd := exec.CommandContext(ctx, "docker", args...) //nolint:gosec // G204: docker compose with validated args
	cmd.Env = c.env()
	for k, v := range envVars {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose up: %s: %w", stderr.String(), err)
	}
	return nil
}

// ComposeDown removes a compose stack.
func (c *Client) ComposeDown(ctx context.Context, projectName string) error {
	_, err := c.run(ctx, "compose", "-p", projectName, "down", "--remove-orphans")
	return err
}

// ComposeStop stops a compose stack.
func (c *Client) ComposeStop(ctx context.Context, projectName string) error {
	_, err := c.run(ctx, "compose", "-p", projectName, "stop")
	return err
}

// ComposeStart starts a stopped compose stack.
func (c *Client) ComposeStart(ctx context.Context, projectName string) error {
	_, err := c.run(ctx, "compose", "-p", projectName, "start")
	return err
}

// ComposePs lists containers for a compose stack.
func (c *Client) ComposePs(ctx context.Context, projectName string) ([]ContainerInfo, error) {
	out, err := c.run(ctx, "compose", "-p", projectName, "ps", "--format", "json")
	if err != nil {
		return nil, err
	}

	var containers []ContainerInfo
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		var ci ContainerInfo
		if err := json.Unmarshal([]byte(line), &ci); err != nil {
			continue
		}
		containers = append(containers, ci)
	}
	return containers, nil
}

// ListContainers runs `docker ps` and returns all running containers on the host.
func (c *Client) ListContainers(ctx context.Context, all bool) ([]DiscoveredContainer, error) {
	args := []string{"ps", "--format", "{{json .}}"}
	if all {
		args = []string{"ps", "-a", "--format", "{{json .}}"}
	}
	out, err := c.run(ctx, args...)
	if err != nil {
		return nil, err
	}

	var containers []DiscoveredContainer
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		// docker ps --format json outputs one JSON object per line
		var raw struct {
			ID        string `json:"ID"`
			Names     string `json:"Names"`
			Image     string `json:"Image"`
			State     string `json:"State"`
			Status    string `json:"Status"`
			Ports     string `json:"Ports"`
			Labels    string `json:"Labels"`
			CreatedAt string `json:"CreatedAt"`
		}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}

		// Parse labels into map
		labels := make(map[string]string)
		if raw.Labels != "" {
			for _, pair := range strings.Split(raw.Labels, ",") {
				parts := strings.SplitN(pair, "=", 2)
				if len(parts) == 2 {
					labels[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
				}
			}
		}

		// Detect compose project from labels
		composeProject := ""
		if v, ok := labels["com.docker.compose.project"]; ok {
			composeProject = v
		}

		containers = append(containers, DiscoveredContainer{
			ID:             raw.ID,
			Name:           raw.Names,
			Image:          raw.Image,
			State:          raw.State,
			Status:         raw.Status,
			Ports:          raw.Ports,
			Labels:         labels,
			Created:        raw.CreatedAt,
			ComposeProject: composeProject,
		})
	}
	return containers, nil
}

// ContainerLogs retrieves logs for a specific container by name or ID.
func (c *Client) ContainerLogs(ctx context.Context, containerName string, tail int) (string, error) {
	args := []string{"logs"}
	if tail > 0 {
		args = append(args, "--tail", fmt.Sprintf("%d", tail))
	}
	args = append(args, containerName)
	return c.run(ctx, args...)
}

// ComposeLogs retrieves logs for a compose stack.
func (c *Client) ComposeLogs(ctx context.Context, projectName, serviceName string, tail int) (string, error) {
	args := []string{"compose", "-p", projectName, "logs", "--no-color"}
	if tail > 0 {
		args = append(args, "--tail", fmt.Sprintf("%d", tail))
	}
	if serviceName != "" {
		args = append(args, serviceName)
	}
	return c.run(ctx, args...)
}

// ComposeRestart restarts a compose stack or specific service.
func (c *Client) ComposeRestart(ctx context.Context, projectName, serviceName string) error {
	args := []string{"compose", "-p", projectName, "restart"}
	if serviceName != "" {
		args = append(args, serviceName)
	}
	_, err := c.run(ctx, args...)
	return err
}

// writeCertsToTempDir writes TLS certs to a temp directory and returns the path.
func writeCertsToTempDir(caCert, cert, key string) string {
	dir, err := os.MkdirTemp("", "pepa-docker-tls-*")
	if err != nil {
		return ""
	}
	if caCert != "" {
		_ = os.WriteFile(filepath.Join(dir, "ca.pem"), []byte(caCert), 0600)
	}
	if cert != "" {
		_ = os.WriteFile(filepath.Join(dir, "cert.pem"), []byte(cert), 0600)
	}
	if key != "" {
		_ = os.WriteFile(filepath.Join(dir, "key.pem"), []byte(key), 0600)
	}
	return dir
}

// writeSSHKeyToTempFile writes an SSH key to a temp file and returns the path.
func writeSSHKeyToTempFile(key string) string {
	f, err := os.CreateTemp("", "pepa-docker-ssh-*")
	if err != nil {
		return ""
	}
	_, _ = f.WriteString(key)
	_ = f.Chmod(0600)
	_ = f.Close()
	return f.Name()
}
