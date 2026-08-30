package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/k8s"
	"github.com/pepa/pepa/internal/repository"
)

// DeploymentService handles deployment business logic.
type DeploymentService struct {
	clusterRepo    *repository.ClusterRepository
	deploymentRepo *repository.DeploymentRepository
	helmRepo       *repository.HelmRepository
}

// NewDeploymentService creates a new DeploymentService.
func NewDeploymentService(
	clusterRepo *repository.ClusterRepository,
	deploymentRepo *repository.DeploymentRepository,
	helmRepo *repository.HelmRepository,
) *DeploymentService {
	return &DeploymentService{
		clusterRepo:    clusterRepo,
		deploymentRepo: deploymentRepo,
		helmRepo:       helmRepo,
	}
}

// DeploymentResult represents the result of a deployment operation.
type DeploymentResult struct {
	Success bool
	Message string
	Logs    string
}

// updateStatusWithLogs updates deployment status with logs.
func (s *DeploymentService) updateStatusWithLogs(deploymentID uuid.UUID, status, logs string) {
	deployment, err := s.deploymentRepo.Get(context.Background(), deploymentID)
	if err != nil {
		slog.Info("ERROR: deployment : get for status update", "id", deploymentID, "error", err)
		return
	}
	deployment.Status = status
	deployment.Logs = logs
	if err := s.deploymentRepo.Update(context.Background(), deployment); err != nil {
		slog.Info("ERROR: deployment : update status", "id", deploymentID, "error", err)
	}
}

// updateStatusWithError updates deployment status with error message and logs.
func (s *DeploymentService) updateStatusWithError(deploymentID uuid.UUID, status, errorMsg, logs string) {
	deployment, err := s.deploymentRepo.Get(context.Background(), deploymentID)
	if err != nil {
		slog.Info("ERROR: deployment : get for status update", "id", deploymentID, "error", err)
		return
	}
	deployment.Status = status
	deployment.ErrorMessage = errorMsg
	deployment.Logs = logs
	if err := s.deploymentRepo.Update(context.Background(), deployment); err != nil {
		slog.Info("ERROR: deployment : update status", "id", deploymentID, "error", err)
	}
}

// PerformDeployment executes a deployment to a Kubernetes cluster.
// This is the main business logic extracted from the HTTP handler.
func (s *DeploymentService) PerformDeployment(
	ctx context.Context,
	deploymentID, clusterID uuid.UUID,
	namespace, releaseName string,
	replicas int32,
	specJSON []byte,
	timeoutSeconds int,
) *DeploymentResult {
	// Use a timeout context to prevent hanging on unreachable clusters
	if timeoutSeconds <= 0 {
		timeoutSeconds = 300
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	var logsBuilder strings.Builder

	// Helper to update deployment status
	updateStatus := func(status, errorMsg, logs string) {
		if errorMsg != "" {
			s.updateStatusWithError(deploymentID, status, errorMsg, logs)
		} else {
			s.updateStatusWithLogs(deploymentID, status, logs)
		}
	}

	// Helper to check if deployment was cancelled
	isCancelled := func() bool {
		d, err := s.deploymentRepo.Get(context.Background(), deploymentID)
		return err == nil && d.Status == "cancelled"
	}

	// Mark as syncing to show progress
	updateStatus("syncing", "", "Starting deployment...\n")

	// Get kubeconfig for the target cluster
	logsBuilder.WriteString(fmt.Sprintf("Getting kubeconfig for cluster %s...\n", clusterID))
	kubeconfig, err := s.clusterRepo.GetKubeconfig(ctx, clusterID, uuid.Nil)
	if err != nil {
		slog.Info("ERROR: deployment : get kubeconfig", "id", deploymentID, "error", err)
		updateStatus("failed", fmt.Sprintf("Failed to get kubeconfig: %v", err), logsBuilder.String())
		return &DeploymentResult{Success: false, Message: err.Error(), Logs: logsBuilder.String()}
	}
	if kubeconfig == "" {
		slog.Info("ERROR: deployment : cluster has no kubeconfig", "id", deploymentID, "id", clusterID)
		updateStatus("failed", "Cluster has no kubeconfig configured", logsBuilder.String())
		return &DeploymentResult{Success: false, Message: "Cluster has no kubeconfig configured", Logs: logsBuilder.String()}
	}

	logsBuilder.WriteString("Kubeconfig obtained.\n")
	logsBuilder.WriteString("Creating Kubernetes client...\n")

	// Check for cancellation before proceeding
	if isCancelled() {
		logsBuilder.WriteString("Deployment cancelled by user.\n")
		updateStatus("cancelled", "", logsBuilder.String())
		return &DeploymentResult{Success: false, Message: "Deployment cancelled by user", Logs: logsBuilder.String()}
	}

	// Create k8s client (use server override if cluster has explicit API server URL)
	var client *k8s.Client
	clusterObj, clusterErr := s.clusterRepo.Get(ctx, clusterID, uuid.Nil)
	if clusterErr == nil && clusterObj != nil && clusterObj.APIServerURL != "" {
		client, err = k8s.NewClientWithServerOverride(kubeconfig, clusterObj.APIServerURL)
	} else {
		client, err = k8s.NewClient(kubeconfig)
	}
	if err != nil {
		slog.Info("ERROR: deployment : create k8s client", "id", deploymentID, "error", err)
		updateStatus("failed", fmt.Sprintf("Failed to create k8s client: %v", err), logsBuilder.String())
		return &DeploymentResult{Success: false, Message: err.Error(), Logs: logsBuilder.String()}
	}

	logsBuilder.WriteString("Kubernetes client created. Parsing deployment spec...\n")

	// Parse the deployment spec
	if releaseName == "" {
		releaseName = "pepa-release"
	}
	if namespace == "" {
		namespace = "default"
	}
	deploySpec, err := k8s.ParseDeploySpec(specJSON, releaseName, namespace, replicas)
	if err != nil {
		slog.Info("ERROR: deployment : parse spec", "id", deploymentID, "error", err)
		updateStatus("failed", fmt.Sprintf("Failed to parse deployment spec: %v", err), logsBuilder.String())
		return &DeploymentResult{Success: false, Message: err.Error(), Logs: logsBuilder.String()}
	}

	// Route to Helm deploy if chart info is present
	if deploySpec.Chart != nil && deploySpec.Chart.SourceType != "" && deploySpec.Chart.SourceType != "container" {
		// Check for cancellation before deploying
		if isCancelled() {
			logsBuilder.WriteString("Deployment cancelled by user.\n")
			updateStatus("cancelled", "", logsBuilder.String())
			return &DeploymentResult{Success: false, Message: "Deployment cancelled by user", Logs: logsBuilder.String()}
		}
		slog.Info("Deployment : using Helm deploy for chart /", "id", deploymentID, "arg2", deploySpec.Chart.ChartURL, "name", deploySpec.Chart.ChartName)
		logsBuilder.WriteString(fmt.Sprintf("Deploying Helm chart: %s/%s (version: %s)\n", deploySpec.Chart.ChartURL, deploySpec.Chart.ChartName, deploySpec.Chart.ChartVersion))
		logsBuilder.WriteString(fmt.Sprintf("Release: %s, Namespace: %s, Timeout: %ds\n", releaseName, namespace, timeoutSeconds))
		
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
		
		// Look up Helm repository credentials by URL
		if s.helmRepo != nil && deploySpec.Chart.ChartURL != "" {
			if helmRepo, err := s.helmRepo.GetByURL(ctx, deploySpec.Chart.ChartURL, uuid.Nil); err == nil && helmRepo != nil {
				// Get decrypted credentials
				decrypted, err := s.helmRepo.GetDecrypted(ctx, helmRepo.ID, uuid.Nil)
				if err == nil && decrypted != nil {
					helmSpec.Username = decrypted.Username
					helmSpec.Password = decrypted.Password
					helmSpec.Token = decrypted.Token
				}
			}
		}
		
		// Set image override if containers are specified
		if len(deploySpec.Containers) > 0 && deploySpec.Containers[0].Image != "" {
			if helmSpec.SetValues == nil {
				helmSpec.SetValues = make(map[string]string)
			}
			helmSpec.SetValues["image.repository"] = deploySpec.Containers[0].Image
		}
		
		logsBuilder.WriteString("Helm deploy initiated...\n")
		result, err := client.HelmDeploy(ctx, helmSpec)
		if err != nil {
			errMsg := fmt.Sprintf("Helm deploy failed: %v", err)
			slog.Info("ERROR: deployment ", "id", deploymentID, "error", errMsg)
			logsBuilder.WriteString(fmt.Sprintf("ERROR: %s\n", errMsg))
			updateStatus("failed", errMsg, logsBuilder.String())
			return &DeploymentResult{Success: false, Message: errMsg, Logs: logsBuilder.String()}
		}
		slog.Info("Deployment succeeded (Helm)", "id", deploymentID, "arg2", result.Message)
		logsBuilder.WriteString(fmt.Sprintf("SUCCESS: %s\n", result.Message))
		updateStatus("deployed", "", logsBuilder.String())
		return &DeploymentResult{Success: true, Message: result.Message, Logs: logsBuilder.String()}
	}

	// Raw K8s deployment (for container source type)
	// Check for cancellation before deploying
	if isCancelled() {
		logsBuilder.WriteString("Deployment cancelled by user.\n")
		updateStatus("cancelled", "", logsBuilder.String())
		return &DeploymentResult{Success: false, Message: "Deployment cancelled by user", Logs: logsBuilder.String()}
	}
	
	logsBuilder.WriteString(fmt.Sprintf("Deploying container to namespace %s (timeout: %ds)\n", namespace, timeoutSeconds))
	result, err := client.Deploy(ctx, deploySpec)
	if err != nil {
		errMsg := fmt.Sprintf("Deploy failed: %v", err)
		slog.Info("ERROR: deployment ", "id", deploymentID, "error", errMsg)
		logsBuilder.WriteString(fmt.Sprintf("ERROR: %s\n", errMsg))
		updateStatus("failed", errMsg, logsBuilder.String())
		return &DeploymentResult{Success: false, Message: errMsg, Logs: logsBuilder.String()}
	}

	slog.Info("Deployment succeeded", "id", deploymentID, "arg2", result.Message)
	logsBuilder.WriteString(fmt.Sprintf("SUCCESS: %s\n", result.Message))
	updateStatus("deployed", "", logsBuilder.String())
	return &DeploymentResult{Success: true, Message: result.Message, Logs: logsBuilder.String()}
}
