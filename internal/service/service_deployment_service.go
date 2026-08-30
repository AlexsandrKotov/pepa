package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/k8s"
	"github.com/pepa/pepa/internal/repository"
)

// ServiceDeploymentService handles service deployment business logic.
type ServiceDeploymentService struct {
	clusterRepo *repository.ClusterRepository
	serviceRepo *repository.ServiceRepository
	helmRepo    *repository.HelmRepository
}

// NewServiceDeploymentService creates a new ServiceDeploymentService.
func NewServiceDeploymentService(
	clusterRepo *repository.ClusterRepository,
	serviceRepo *repository.ServiceRepository,
	helmRepo *repository.HelmRepository,
) *ServiceDeploymentService {
	return &ServiceDeploymentService{
		clusterRepo: clusterRepo,
		serviceRepo: serviceRepo,
		helmRepo:    helmRepo,
	}
}

// PerformServiceDeployment executes a service deployment to a Kubernetes cluster.
func (s *ServiceDeploymentService) PerformServiceDeployment(
	ctx context.Context,
	deploymentID, serviceID, clusterID uuid.UUID,
	namespace, releaseName string,
	specJSON json.RawMessage,
) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	kubeconfig, err := s.clusterRepo.GetKubeconfig(ctx, clusterID, uuid.Nil)
	if err != nil {
		slog.Info("ERROR: service deployment : get kubeconfig", "id", deploymentID, "error", err)
		if s.serviceRepo != nil {
			_ = s.serviceRepo.UpdateDeployment(ctx, deploymentID, "failed", "pending")
		}
		return fmt.Errorf("get kubeconfig: %w", err)
	}
	if kubeconfig == "" {
		slog.Info("ERROR: service deployment : cluster has no kubeconfig", "id", deploymentID, "id", clusterID)
		if s.serviceRepo != nil {
			_ = s.serviceRepo.UpdateDeployment(ctx, deploymentID, "failed", "pending")
		}
		return fmt.Errorf("cluster has no kubeconfig")
	}

	// Use server override if the cluster has an explicit API server URL
	clusterObj, clusterErr := s.clusterRepo.Get(ctx, clusterID, uuid.Nil)
	var client *k8s.Client
	if clusterErr == nil && clusterObj != nil && clusterObj.APIServerURL != "" {
		client, err = k8s.NewClientWithServerOverride(kubeconfig, clusterObj.APIServerURL)
	} else {
		client, err = k8s.NewClient(kubeconfig)
	}
	if err != nil {
		slog.Info("ERROR: service deployment : create k8s client", "id", deploymentID, "error", err)
		if s.serviceRepo != nil {
			_ = s.serviceRepo.UpdateDeployment(ctx, deploymentID, "failed", "pending")
		}
		return fmt.Errorf("create k8s client: %w", err)
	}

	if releaseName == "" {
		releaseName = "pepa-release"
	}
	if namespace == "" {
		namespace = "default"
	}

	deploySpec, err := k8s.ParseDeploySpec(specJSON, releaseName, namespace, 1)
	if err != nil {
		slog.Info("ERROR: service deployment : parse spec", "id", deploymentID, "error", err)
		if s.serviceRepo != nil {
			_ = s.serviceRepo.UpdateDeployment(ctx, deploymentID, "failed", "pending")
		}
		return fmt.Errorf("parse spec: %w", err)
	}

	// Route to Helm deploy if chart info is present
	if deploySpec.Chart != nil && deploySpec.Chart.SourceType != "" && deploySpec.Chart.SourceType != "container" {
		helmSpec := k8s.HelmSpec{
			SourceType:   deploySpec.Chart.SourceType,
			ChartURL:     deploySpec.Chart.ChartURL,
			ChartName:    deploySpec.Chart.ChartName,
			ChartVersion: deploySpec.Chart.ChartVersion,
			ValuesYAML:   deploySpec.ValuesYAML,
			ReleaseName:  releaseName,
			Namespace:    namespace,
		}
		// Look up Helm repository credentials by URL
		if s.helmRepo != nil && deploySpec.Chart.ChartURL != "" {
			if helmRepo, err := s.helmRepo.GetByURL(ctx, deploySpec.Chart.ChartURL, uuid.Nil); err == nil && helmRepo != nil {
				decrypted, err := s.helmRepo.GetDecrypted(ctx, helmRepo.ID, uuid.Nil)
				if err == nil && decrypted != nil {
					helmSpec.Username = decrypted.Username
					helmSpec.Password = decrypted.Password
					helmSpec.Token = decrypted.Token
				}
			}
		}
		result, err := client.HelmDeploy(ctx, helmSpec)
		if err != nil {
			slog.Info("ERROR: service deployment : helm deploy", "id", deploymentID, "error", err)
			if s.serviceRepo != nil {
				_ = s.serviceRepo.UpdateDeployment(ctx, deploymentID, "failed", "pending")
			}
			return fmt.Errorf("helm deploy: %w", err)
		}
		slog.Info("Service deployment succeeded (Helm)", "id", deploymentID, "arg2", result.Message)
		// Mark deployment as complete
		if s.serviceRepo != nil {
			_ = s.serviceRepo.CompleteDeployment(ctx, deploymentID, serviceID)
		}
		return nil
	}

	// Raw K8s deployment
	result, err := client.Deploy(ctx, deploySpec)
	if err != nil {
		slog.Info("ERROR: service deployment : deploy", "id", deploymentID, "error", err)
		if s.serviceRepo != nil {
			_ = s.serviceRepo.UpdateDeployment(ctx, deploymentID, "failed", "pending")
		}
		return fmt.Errorf("deploy: %w", err)
	}
	slog.Info("Service deployment succeeded", "id", deploymentID, "arg2", result.Message)
	// Mark deployment as complete
	if s.serviceRepo != nil {
		_ = s.serviceRepo.CompleteDeployment(ctx, deploymentID, serviceID)
	}
	return nil
}
