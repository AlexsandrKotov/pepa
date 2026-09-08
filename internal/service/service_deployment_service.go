package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/repository"
)

// ServiceDeploymentService handles service deployment business logic.
type ServiceDeploymentService struct {
	clusterRepo *repository.ClusterRepository
	serviceRepo *repository.ServiceRepository
	helmRepo    *repository.HelmRepository
	executor    *DeploymentExecutor
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
		executor:    NewDeploymentExecutor(clusterRepo, helmRepo),
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

	kubeconfig, err := s.executor.ResolveKubeconfig(ctx, clusterID)
	if err != nil {
		slog.Info("ERROR: service deployment : get kubeconfig", "id", deploymentID, "error", err)
		if s.serviceRepo != nil {
			_ = s.serviceRepo.UpdateDeployment(ctx, deploymentID, "failed", "pending")
		}
		return fmt.Errorf("get kubeconfig: %w", err)
	}

	client, err := s.executor.CreateK8sClient(ctx, kubeconfig, clusterID)
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

	deploySpec, err := s.executor.ParseDeploySpec(specJSON, releaseName, namespace, 1)
	if err != nil {
		slog.Info("ERROR: service deployment : parse spec", "id", deploymentID, "error", err)
		if s.serviceRepo != nil {
			_ = s.serviceRepo.UpdateDeployment(ctx, deploymentID, "failed", "pending")
		}
		return fmt.Errorf("parse spec: %w", err)
	}

	// Execute deploy via shared executor (handles Helm vs Raw routing + credentials)
	result, err := s.executor.ExecuteDeploy(ctx, client, deploySpec, releaseName, namespace, 1, 300)
	if err != nil {
		slog.Info("ERROR: service deployment", "id", deploymentID, "error", err)
		if s.serviceRepo != nil {
			_ = s.serviceRepo.UpdateDeployment(ctx, deploymentID, "failed", "pending")
		}
		return fmt.Errorf("deploy: %w", err)
	}

	slog.Info("Service deployment succeeded", "id", deploymentID, "message", result.Message)
	// Mark deployment as complete
	if s.serviceRepo != nil {
		_ = s.serviceRepo.CompleteDeployment(ctx, deploymentID, serviceID)
	}
	return nil
}
