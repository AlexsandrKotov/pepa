package k8s

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"k8s.io/client-go/tools/clientcmd"
)

// HelmSpec describes a Helm chart deployment.
type HelmSpec struct {
	SourceType     string // "helm_http", "helm_oci", "helm_git"
	ChartURL       string // repo URL or OCI ref
	ChartName      string // chart name (for repo-based installs)
	ChartVersion   string
	ValuesYAML     string
	ReleaseName    string
	Namespace      string
	SetValues      map[string]string // --set key=value pairs
	TimeoutSeconds int               // --wait timeout in seconds (default 300)
	// Credentials for private repositories
	Username string
	Password string
	Token    string
}

// HelmDeploy deploys a Helm chart using the helm CLI.
func (c *Client) HelmDeploy(ctx context.Context, spec HelmSpec) (*DeployResult, error) {
	if spec.ReleaseName == "" {
		return nil, fmt.Errorf("release_name is required for helm deploy")
	}
	if spec.Namespace == "" {
		spec.Namespace = "default"
	}

	// Ensure namespace exists before Helm deploy
	if err := c.ensureNamespace(ctx, spec.Namespace); err != nil {
		slog.Info("WARN: ensure namespace for helm deploy", "name", spec.Namespace, "error", err)
		// Continue anyway — namespace might already exist or be auto-created by Helm
	}

	// Write kubeconfig to temp file (with server override if set)
	kubeconfigData := c.kubeconfig
	if c.serverOverride != "" {
		kubeconfigData = overrideKubeconfigServer(kubeconfigData, c.serverOverride)
	}
	kubeconfigFile, err := os.CreateTemp("", "pepa-kubeconfig-*.yaml")
	if err != nil {
		return nil, fmt.Errorf("create kubeconfig temp file: %w", err)
	}
	defer func() { _ = os.Remove(kubeconfigFile.Name()) }()
	if _, err := kubeconfigFile.WriteString(kubeconfigData); err != nil {
		_ = kubeconfigFile.Close()
		return nil, fmt.Errorf("write kubeconfig: %w", err)
	}
	_ = kubeconfigFile.Close()

	// Write values.yaml to temp file if provided
	var valuesFile string
	if spec.ValuesYAML != "" {
		vf, err := os.CreateTemp("", "pepa-values-*.yaml")
		if err != nil {
			return nil, fmt.Errorf("create values temp file: %w", err)
		}
		defer func() { _ = os.Remove(vf.Name()) }()
		if _, err := vf.WriteString(spec.ValuesYAML); err != nil {
			_ = vf.Close()
			return nil, fmt.Errorf("write values: %w", err)
		}
		_ = vf.Close()
		valuesFile = vf.Name()
	}

	// Determine chart reference
	chartRef := ""
	switch spec.SourceType {
	case "helm_oci":
		// OCI: oci://registry/chart-name
		chartRef = spec.ChartURL
		if spec.ChartName != "" && !strings.Contains(spec.ChartURL, spec.ChartName) {
			chartRef = strings.TrimSuffix(spec.ChartURL, "/") + "/" + spec.ChartName
		}
	case "helm_http", "helm_git":
		// For HTTP/Git repos, add repo first then use repo/chartName
		repoName := "pepa-" + strings.ReplaceAll(spec.ReleaseName, "/", "-")
		if spec.ChartURL == "" {
			return nil, fmt.Errorf("chart_url is required for %s source type", spec.SourceType)
		}
		if spec.ChartName == "" {
			return nil, fmt.Errorf("chart_name is required for %s source type (the Helm chart name within the repository, e.g. 'my-app', 'nginx', 'postgresql')", spec.SourceType)
		}
		slog.Info("Helm deploy: repo= chart= version= release=", "arg1", spec.ChartURL, "name", spec.ChartName, "version", spec.ChartVersion, "name", spec.ReleaseName)
		// Add repo with credentials if provided
		addArgs := []string{"repo", "add", repoName, spec.ChartURL, "--force-update",
			"--kubeconfig", kubeconfigFile.Name()}
		if spec.Username != "" && spec.Password != "" {
			addArgs = append(addArgs, "--username", spec.Username, "--password", spec.Password)
		} else if spec.Token != "" {
			// For token-based auth (e.g., GitLab), use token as username with empty password
			addArgs = append(addArgs, "--username", "gitlab-ci-token", "--password", spec.Token)
		}
		if err := c.runHelm(ctx, addArgs...); err != nil {
			return nil, fmt.Errorf("helm repo add: %w", err)
		}
		// Update repo
		updateArgs := []string{"repo", "update",
			"--kubeconfig", kubeconfigFile.Name()}
		if err := c.runHelm(ctx, updateArgs...); err != nil {
			slog.Info("WARN: helm repo update", "error", err)
			// Continue anyway — chart might be cached
		}
		chartRef = repoName + "/" + spec.ChartName
	default:
		// Container type or unknown — use raw deploy instead
		return nil, fmt.Errorf("unsupported helm source type: %s", spec.SourceType)
	}

	// Build upgrade --install command
	waitTimeout := spec.TimeoutSeconds
	if waitTimeout <= 0 {
		waitTimeout = 300
	}
	args := []string{"upgrade", "--install", spec.ReleaseName, chartRef,
		"--namespace", spec.Namespace,
		"--kubeconfig", kubeconfigFile.Name(),
		"--wait", "--timeout", fmt.Sprintf("%ds", waitTimeout),
	}

	if spec.ChartVersion != "" {
		args = append(args, "--version", spec.ChartVersion)
	}
	if valuesFile != "" {
		args = append(args, "--values", valuesFile)
	}
	for k, v := range spec.SetValues {
		args = append(args, "--set", fmt.Sprintf("%s=%s", k, v))
	}

	if err := c.runHelm(ctx, args...); err != nil {
		return nil, fmt.Errorf("helm upgrade --install: %w", err)
	}

	return &DeployResult{
		ReleaseName: spec.ReleaseName,
		Namespace:   spec.Namespace,
		Status:      "deployed",
		Message:     fmt.Sprintf("Successfully deployed Helm chart %s as %s to %s", chartRef, spec.ReleaseName, spec.Namespace),
	}, nil
}

// runHelm executes a helm CLI command.
func (c *Client) runHelm(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "helm", args...) // #nosec //nolint:gosec // G204: helm is an admin-configured binary
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// Build a detailed error message including helm output
		detail := stderr.String()
		if stdout.Len() > 0 {
			detail = stdout.String() + "\n" + detail
		}
		cmdDesc := strings.Join(args[:min(len(args), 3)], " ")
		return fmt.Errorf("helm %s: %s: %w", cmdDesc, strings.TrimSpace(detail), err)
	}
	if stdout.Len() > 0 {
		slog.Info("helm output", "arg1", strings.TrimSpace(stdout.String()))
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// overrideKubeconfigServer replaces the server URL in a kubeconfig YAML string
// so that the Helm CLI uses the correct API server address.
func overrideKubeconfigServer(kubeconfig, serverURL string) string {
	cfg, err := clientcmd.Load([]byte(kubeconfig))
	if err != nil {
		return kubeconfig
	}
	for _, cluster := range cfg.Clusters {
		if cluster != nil {
			cluster.Server = serverURL
		}
	}
	modified, err := clientcmd.Write(*cfg)
	if err != nil {
		return kubeconfig
	}
	return string(modified)
}
