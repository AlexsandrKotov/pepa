package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// DeployDockerRequest is the payload for deploying a Docker workload into a
// freshly created LXC container (Docker-in-LXC with nesting enabled).
type DeployDockerRequest struct {
	Node          string            `json:"node"`
	VMID          int               `json:"vmid,omitempty"`
	Hostname      string            `json:"hostname"`
	Template      string            `json:"template"` // OS template volid, e.g. local:vztmpl/debian-12...
	Storage       string            `json:"storage,omitempty"`
	Cores         int               `json:"cores,omitempty"`
	Memory        int               `json:"memory_mb,omitempty"`
	DiskSize      string            `json:"disk_size,omitempty"`
	Network       string            `json:"network,omitempty"`
	SourceType    string            `json:"source_type"` // registry | folder | docker_local
	Image         string            `json:"image,omitempty"`
	FolderPath    string            `json:"folder_path,omitempty"`
	ContainerName string            `json:"container_name,omitempty"`
	Ports         []string          `json:"ports,omitempty"` // "8080:80"
	Env           map[string]string `json:"env,omitempty"`
}

// deployDocker creates an LXC container with nesting, waits for its IP, then
// provisions Docker inside over SSH and deploys the requested workload.
func (p *ProxmoxPlugin) deployDocker(client *Client, config map[string]string, params []byte) ([]byte, error) {
	var req DeployDockerRequest
	if err := actionInput(params, &req); err != nil {
		return nil, fmt.Errorf("deploy_docker: parse params: %w", err)
	}
	if req.Node == "" || req.Hostname == "" || req.Template == "" {
		return nil, fmt.Errorf("deploy_docker requires 'node', 'hostname', and 'template'")
	}
	switch req.SourceType {
	case "registry", "folder", "docker_local":
	default:
		return nil, fmt.Errorf("deploy_docker: unknown source_type %q (expected registry, folder, or docker_local)", req.SourceType)
	}
	if req.SourceType == "registry" || req.SourceType == "docker_local" {
		if req.Image == "" {
			return nil, fmt.Errorf("deploy_docker: 'image' is required for source_type %q", req.SourceType)
		}
	}
	if req.SourceType == "folder" {
		if req.FolderPath == "" {
			return nil, fmt.Errorf("deploy_docker: 'folder_path' is required for source_type folder")
		}
		if st, err := os.Stat(req.FolderPath); err != nil || !st.IsDir() {
			return nil, fmt.Errorf("deploy_docker: folder %q is not accessible from the PEPA server", req.FolderPath)
		}
	}

	// SSH key for provisioning: PEPA injects the public key into the container.
	privateKey := config["ssh_private_key"]
	if privateKey == "" {
		return nil, fmt.Errorf("deploy_docker: 'ssh_private_key' is not set in the Proxmox connection — generate a key (ssh-keygen -t ed25519) and paste the private key into the connection settings")
	}
	signer, err := ssh.ParsePrivateKey([]byte(privateKey))
	if err != nil {
		return nil, fmt.Errorf("deploy_docker: invalid ssh_private_key: %w", err)
	}
	pubKey := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey())))
	sshUser := config["ssh_user"]
	if sshUser == "" {
		sshUser = "root"
	}

	// Allocate VMID upfront so we can poll status and connect later.
	vmid := req.VMID
	if vmid == 0 {
		nextRaw, err := client.NextID()
		if err != nil {
			return nil, fmt.Errorf("deploy_docker: allocate VMID: %w", err)
		}
		vmid, err = strconv.Atoi(strings.Trim(strings.TrimSpace(string(nextRaw)), `"`))
		if err != nil {
			return nil, fmt.Errorf("deploy_docker: parse next VMID %q: %w", string(nextRaw), err)
		}
	}

	containerName := req.ContainerName
	if containerName == "" {
		containerName = req.Hostname
	}

	// 1. Create the LXC container with nesting enabled and the SSH key injected.
	storage := req.Storage
	if storage == "" {
		storage = "local-lvm"
	}
	diskSize := req.DiskSize
	if diskSize == "" {
		diskSize = "8G"
	}
	network := req.Network
	if network == "" {
		network = "vmbr0"
	}
	cores := req.Cores
	if cores == 0 {
		cores = 1
	}
	memory := req.Memory
	if memory == 0 {
		memory = 1024
	}

	form := url.Values{}
	form.Set("vmid", strconv.Itoa(vmid))
	form.Set("hostname", req.Hostname)
	form.Set("ostemplate", req.Template)
	form.Set("cores", strconv.Itoa(cores))
	form.Set("memory", strconv.Itoa(memory))
	form.Set("rootfs", fmt.Sprintf("%s:%s", storage, diskSize))
	form.Set("net0", fmt.Sprintf("name=eth0,bridge=%s,ip=dhcp", network))
	form.Set("features", `{"nesting":1,"keyctl":1}`)
	form.Set("ssh-public-keys", pubKey)
	form.Set("start", "1")
	form.Set("onboot", "1")

	if _, err := client.CreateContainer(req.Node, form); err != nil {
		return nil, fmt.Errorf("deploy_docker: create container: %w", err)
	}

	logLines := []string{fmt.Sprintf("Created LXC container %d (%s)", vmid, req.Hostname)}

	// 2. Wait for the container to get an IP address.
	ip, err := waitForContainerIP(client, req.Node, vmid, 120*time.Second)
	if err != nil {
		return nil, fmt.Errorf("deploy_docker: %w (container %d was created; check it in Proxmox)", err, vmid)
	}
	logLines = append(logLines, "Container got IP: "+ip)

	// 3. Connect over SSH (sshd may need a moment to come up).
	sshClient, err := dialSSH(ip, sshUser, signer, 120*time.Second)
	if err != nil {
		return nil, fmt.Errorf("deploy_docker: %w (container %d is running at %s)", err, vmid, ip)
	}
	defer func() { _ = sshClient.Close() }()
	logLines = append(logLines, "SSH connected")

	// 4. Ensure Docker is installed.
	if out, err := runSSH(sshClient, `sh -c 'command -v docker >/dev/null 2>&1 || (curl -fsSL https://get.docker.com || wget -qO- https://get.docker.com) | sh'`, 5*time.Minute); err != nil {
		return nil, fmt.Errorf("deploy_docker: install docker failed: %v\n%s", err, tail(out, 2000))
	}
	logLines = append(logLines, "Docker engine ready")

	// 5. Deploy the workload depending on the source.
	switch req.SourceType {
	case "registry":
		if out, err := runSSH(sshClient, fmt.Sprintf("docker pull %s", shellQuote(req.Image)), 5*time.Minute); err != nil {
			return nil, fmt.Errorf("deploy_docker: docker pull failed: %v\n%s", err, tail(out, 2000))
		}
		out, err := runSSH(sshClient, dockerRunCommand(containerName, req.Ports, req.Env, req.Image), 2*time.Minute)
		if err != nil {
			return nil, fmt.Errorf("deploy_docker: docker run failed: %v\n%s", err, tail(out, 2000))
		}
		logLines = append(logLines, "Pulled and started image "+req.Image)
	case "docker_local":
		// Stream the image from the PEPA host's docker daemon into the container.
		if err := streamDockerImage(sshClient, req.Image); err != nil {
			return nil, fmt.Errorf("deploy_docker: transfer local image: %w", err)
		}
		logLines = append(logLines, "Transferred local image "+req.Image)
		out, err := runSSH(sshClient, dockerRunCommand(containerName, req.Ports, req.Env, req.Image), 2*time.Minute)
		if err != nil {
			return nil, fmt.Errorf("deploy_docker: docker run failed: %v\n%s", err, tail(out, 2000))
		}
		logLines = append(logLines, "Started container "+containerName)
	case "folder":
		remoteDir := "/opt/pepa/" + req.Hostname
		if err := streamFolder(sshClient, req.FolderPath, remoteDir); err != nil {
			return nil, fmt.Errorf("deploy_docker: transfer folder: %w", err)
		}
		logLines = append(logLines, fmt.Sprintf("Transferred %s to %s", req.FolderPath, remoteDir))
		script := fmt.Sprintf(`set -e; cd %s
if ls docker-compose.yml docker-compose.yaml compose.yml compose.yaml >/dev/null 2>&1; then
  docker compose up -d
elif [ -f Dockerfile ]; then
  docker build -t %s .
  %s
else
  echo "folder contains neither a compose file nor a Dockerfile" >&2; exit 1
fi`, shellQuote(remoteDir), shellQuote(containerName), dockerRunCommand(containerName, req.Ports, req.Env, containerName))
		out, err := runSSH(sshClient, script, 10*time.Minute)
		if err != nil {
			return nil, fmt.Errorf("deploy_docker: deploy from folder failed: %v\n%s", err, tail(out, 2000))
		}
		logLines = append(logLines, "Deployed from folder")
	}

	return actionOutput(map[string]interface{}{
		"status":         "deployed",
		"vmid":           vmid,
		"ip":             ip,
		"container_name": containerName,
		"log":            strings.Join(logLines, "\n"),
	})
}

// waitForContainerIP polls the container status until an IP appears.
func waitForContainerIP(client *Client, node string, vmid int, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if raw, err := client.GetContainerStatus(node, vmid); err == nil {
			var status struct {
				Status string `json:"status"`
				IP     string `json:"ip"`
			}
			if json.Unmarshal(raw, &status) == nil && status.IP != "" {
				return status.IP, nil
			}
		}
		time.Sleep(3 * time.Second)
	}
	return "", fmt.Errorf("timed out waiting for container IP (is a DHCP server available on %s?)", "the network bridge")
}

// dialSSH retries SSH connections until sshd in the container is ready.
func dialSSH(ip, user string, signer ssh.Signer, timeout time.Duration) (*ssh.Client, error) {
	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // #nosec //nolint:gosec // freshly created container with a PEPA-injected key
		Timeout:         10 * time.Second,
	}
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		conn, err := ssh.Dial("tcp", net.JoinHostPort(ip, "22"), cfg)
		if err == nil {
			return conn, nil
		}
		lastErr = err
		time.Sleep(5 * time.Second)
	}
	return nil, fmt.Errorf("SSH to %s never became available: %w", ip, lastErr)
}

// runSSH executes a command and returns combined output, bounded by timeout.
func runSSH(client *ssh.Client, cmd string, timeout time.Duration) (string, error) {
	sess, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer func() { _ = sess.Close() }()
	var out strings.Builder
	sess.Stdout = &out
	sess.Stderr = &out
	done := make(chan error, 1)
	go func() { done <- sess.Run(cmd) }()
	select {
	case err := <-done:
		return out.String(), err
	case <-time.After(timeout):
		return out.String(), fmt.Errorf("command timed out after %s", timeout)
	}
}

// streamDockerImage saves an image from the local docker daemon and streams
// it into the container via `docker load`.
func streamDockerImage(client *ssh.Client, image string) error {
	saveCmd := exec.Command("docker", "save", image) // #nosec //nolint:gosec // image name validated by caller context
	saveOut, err := saveCmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := saveCmd.Start(); err != nil {
		return fmt.Errorf("is the docker CLI available on the PEPA server with access to the local daemon? %w", err)
	}

	sess, err := client.NewSession()
	if err != nil {
		_ = saveCmd.Process.Kill()
		return err
	}
	defer func() { _ = sess.Close() }()
	stdin, err := sess.StdinPipe()
	if err != nil {
		_ = saveCmd.Process.Kill()
		return err
	}
	var out strings.Builder
	sess.Stdout = &out
	sess.Stderr = &out
	if err := sess.Start("docker load"); err != nil {
		_ = saveCmd.Process.Kill()
		return err
	}
	if _, err := io.Copy(stdin, saveOut); err != nil {
		return err
	}
	_ = stdin.Close()
	if err := saveCmd.Wait(); err != nil {
		return fmt.Errorf("docker save %s: %w (does the image exist locally?)", image, err)
	}
	if err := sess.Wait(); err != nil {
		return fmt.Errorf("docker load in container: %v\n%s", err, tail(out.String(), 1000))
	}
	return nil
}

// streamFolder tars a local directory and extracts it into remoteDir via SSH.
func streamFolder(client *ssh.Client, folderPath, remoteDir string) error {
	abs, err := filepath.Abs(folderPath)
	if err != nil {
		return err
	}
	tarCmd := exec.Command("tar", "-czf", "-", "-C", abs, ".") // #nosec //nolint:gosec // path validated by caller
	tarOut, err := tarCmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := tarCmd.Start(); err != nil {
		return err
	}

	sess, err := client.NewSession()
	if err != nil {
		_ = tarCmd.Process.Kill()
		return err
	}
	defer func() { _ = sess.Close() }()
	stdin, err := sess.StdinPipe()
	if err != nil {
		_ = tarCmd.Process.Kill()
		return err
	}
	var out strings.Builder
	sess.Stdout = &out
	sess.Stderr = &out
	if err := sess.Start(fmt.Sprintf("mkdir -p %s && tar -xzf - -C %s", shellQuote(remoteDir), shellQuote(remoteDir))); err != nil {
		_ = tarCmd.Process.Kill()
		return err
	}
	if _, err := io.Copy(stdin, tarOut); err != nil {
		return err
	}
	_ = stdin.Close()
	if err := tarCmd.Wait(); err != nil {
		return fmt.Errorf("tar %s: %w", folderPath, err)
	}
	if err := sess.Wait(); err != nil {
		return fmt.Errorf("extract in container: %v\n%s", err, tail(out.String(), 1000))
	}
	return nil
}

// dockerRunCommand builds a `docker run` command line.
func dockerRunCommand(name string, ports []string, env map[string]string, image string) string {
	parts := []string{"docker", "run", "-d", "--restart", "unless-stopped", "--name", shellQuote(name)}
	for _, p := range ports {
		p = strings.TrimSpace(p)
		if p != "" {
			parts = append(parts, "-p", shellQuote(p))
		}
	}
	for k, v := range env {
		parts = append(parts, "-e", shellQuote(k+"="+v))
	}
	parts = append(parts, shellQuote(image))
	return strings.Join(parts, " ")
}

// shellQuote wraps a value in single quotes for safe shell usage.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// tail returns the last n characters of s.
func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "..." + s[len(s)-n:]
}
