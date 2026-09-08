package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/k8s"
	"github.com/pepa/pepa/internal/repository"
)

// DeploymentExecutor encapsulates the shared deployment execution logic
// used by both DeploymentService and ServiceDeploymentService.
// It eliminates duplication of kubeconfig resolution, k8s client creation,
// Helm credential lookup, and Helm/Raw deploy routing.
type DeploymentExecutor struct {
	clusterRepo *repository.ClusterRepository
	helmRepo    *repository.HelmRepository
}

// NewDeploymentExecutor creates a new DeploymentExecutor.
func NewDeploymentExecutor(clusterRepo *repository.ClusterRepository, helmRepo *repository.HelmRepository) *DeploymentExecutor {
	return &DeploymentExecutor{
		clusterRepo: clusterRepo,
		helmRepo:    helmRepo,
	}
}

// ResolveKubeconfig retrieves the kubeconfig for a given cluster.
func (e *DeploymentExecutor) ResolveKubeconfig(ctx context.Context, clusterID uuid.UUID) (string, error) {
	kubeconfig, err := e.clusterRepo.GetKubeconfig(ctx, clusterID, uuid.Nil)
	if err != nil {
		return "", fmt.Errorf("get kubeconfig: %w", err)
	}
	if kubeconfig == "" {
		return "", fmt.Errorf("cluster has no kubeconfig configured")
	}
	return kubeconfig, nil
}

// CreateK8sClient creates a Kubernetes client, using the server override from
// the cluster record if available.
func (e *DeploymentExecutor) CreateK8sClient(ctx context.Context, kubeconfig string, clusterID uuid.UUID) (*k8s.Client, error) {
	clusterObj, err := e.clusterRepo.Get(ctx, clusterID, uuid.Nil)
	if err == nil && clusterObj != nil && clusterObj.APIServerURL != "" {
		return k8s.NewClientWithServerOverride(kubeconfig, clusterObj.APIServerURL)
	}
	return k8s.NewClient(kubeconfig)
}

// ResolveHelmCredentials looks up decrypted credentials for a Helm repository URL.
// Returns username, password, token (empty strings if no credentials found).
func (e *DeploymentExecutor) ResolveHelmCredentials(ctx context.Context, chartURL string) (username, password, token string) {
	if e.helmRepo == nil || chartURL == "" {
		return "", "", ""
	}
	helmRepo, err := e.helmRepo.GetByURL(ctx, chartURL, uuid.Nil)
	if err != nil || helmRepo == nil {
		return "", "", ""
	}
	decrypted, err := e.helmRepo.GetDecrypted(ctx, helmRepo.ID, uuid.Nil)
	if err != nil || decrypted == nil {
		return "", "", ""
	}
	return decrypted.Username, decrypted.Password, decrypted.Token
}

// DeployResult holds the outcome of an executeDeploy call.
type DeployResult struct {
	ReleaseName string
	Namespace   string
	Message     string
}

// ExecuteDeploy routes to Helm or Raw deploy based on the parsed spec.
// It returns the deploy result or an error.
func (e *DeploymentExecutor) ExecuteDeploy(
	ctx context.Context,
	client *k8s.Client,
	deploySpec k8s.DeploySpec,
	releaseName, namespace string,
	replicas int32,
	timeoutSeconds int,
) (*DeployResult, error) {
	// Route to Helm deploy if chart info is present
	if deploySpec.Chart != nil && deploySpec.Chart.SourceType != "" && deploySpec.Chart.SourceType != "container" {
		slog.Info("ExecuteDeploy: using Helm deploy", "chart_url", deploySpec.Chart.ChartURL, "chart_name", deploySpec.Chart.ChartName)

		helmSpec := k8s.HelmSpec{
			SourceType:     deploySpec.Chart.SourceType,
			ChartURL:       deploySpec.Chart.ChartURL,
			ChartName:      deploySpec.Chart.ChartName,
			ChartVersion:   deploySpec.Chart.ChartVersion,
			ValuesYAML:     deploySpec.ValuesYAML,
			ReleaseName:    releaseName,
			Namespace:      namespace,
			TimeoutSeconds: timeoutSeconds,
		}

		// Resolve Helm credentials
		username, password, token := e.ResolveHelmCredentials(ctx, deploySpec.Chart.ChartURL)
		helmSpec.Username = username
		helmSpec.Password = password
		helmSpec.Token = token

		// Set image override if containers are specified
		if len(deploySpec.Containers) > 0 && deploySpec.Containers[0].Image != "" {
			if helmSpec.SetValues == nil {
				helmSpec.SetValues = make(map[string]string)
			}
			helmSpec.SetValues["image.repository"] = deploySpec.Containers[0].Image
		}

		result, err := client.HelmDeploy(ctx, helmSpec)
		if err != nil {
			return nil, fmt.Errorf("helm deploy failed: %w", err)
		}
		return &DeployResult{
			ReleaseName: result.ReleaseName,
			Namespace:   result.Namespace,
			Message:     result.Message,
		}, nil
	}

	// Raw K8s deployment
	result, err := client.Deploy(ctx, deploySpec)
	if err != nil {
		return nil, fmt.Errorf("deploy failed: %w", err)
	}
	return &DeployResult{
		ReleaseName: result.ReleaseName,
		Namespace:   result.Namespace,
		Message:     result.Message,
	}, nil
}

// ParseAndPrepareSpec parses a deployment spec JSON and applies defaults.
func ParseAndPrepareSpec(specJSON json.RawMessage, releaseName, namespace string, replicas int32) (k8s.DeploySpec, error) {
	if releaseName == "" {
		releaseName = "pepa-release"
	}
	if namespace == "" {
		namespace = "default"
	}
	return k8s.ParseDeploySpec(specJSON, releaseName, namespace, replicas)
}

// ParseDeploySpec is a convenience method on the executor that parses a
// deployment spec JSON and applies defaults for release name and namespace.
func (e *DeploymentExecutor) ParseDeploySpec(specJSON json.RawMessage, releaseName, namespace string, replicas int32) (k8s.DeploySpec, error) {
	return ParseAndPrepareSpec(specJSON, releaseName, namespace, replicas)
}
