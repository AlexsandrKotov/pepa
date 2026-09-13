//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/testenv"
	"github.com/pepa/pepa/pkg/models"
)

// TestTenantIsolation_Services verifies that tenant A cannot see tenant B's services.
// This is critical because RLS is inert in PEPA — isolation relies on WHERE tenant_id filters.
func TestTenantIsolation_Services(t *testing.T) {
	env := testenv.NewTestEnv(t)
	defer env.Cleanup(t)

	ctx := context.Background()
	tenantA := uuid.New()
	tenantB := uuid.New()

	repo := NewServiceRepository(env.DB())

	// Create service for tenant A.
	reqA := models.CreateServiceRequest{
		Name:        "service-a",
		Slug:        "service-a",
		Description: "Tenant A service",
		Namespace:   "default",
	}
	_, err := repo.Create(ctx, reqA, tenantA, nil)
	if err != nil {
		t.Fatalf("create service A: %v", err)
	}

	// Create service for tenant B.
	reqB := models.CreateServiceRequest{
		Name:        "service-b",
		Slug:        "service-b",
		Description: "Tenant B service",
		Namespace:   "default",
	}
	_, err = repo.Create(ctx, reqB, tenantB, nil)
	if err != nil {
		t.Fatalf("create service B: %v", err)
	}

	// List services for tenant A — should only see service A.
	filterA := models.ServiceFilter{TenantID: tenantA}
	servicesA, err := repo.List(ctx, filterA)
	if err != nil {
		t.Fatalf("list services A: %v", err)
	}
	if servicesA.Total != 1 {
		t.Fatalf("tenant A should see 1 service, got %d", servicesA.Total)
	}
	if servicesA.Items[0].TenantID != tenantA {
		t.Fatalf("tenant A should see only their own service")
	}

	// List services for tenant B — should only see service B.
	filterB := models.ServiceFilter{TenantID: tenantB}
	servicesB, err := repo.List(ctx, filterB)
	if err != nil {
		t.Fatalf("list services B: %v", err)
	}
	if servicesB.Total != 1 {
		t.Fatalf("tenant B should see 1 service, got %d", servicesB.Total)
	}
	if servicesB.Items[0].TenantID != tenantB {
		t.Fatalf("tenant B should see only their own service")
	}
}

// TestTenantIsolation_Environments verifies environment isolation.
func TestTenantIsolation_Environments(t *testing.T) {
	env := testenv.NewTestEnv(t)
	defer env.Cleanup(t)

	ctx := context.Background()
	tenantA := uuid.New()
	tenantB := uuid.New()

	repo := NewEnvironmentRepository(env.DB())

	// Create environment for tenant A.
	envA := &Environment{
		ID:       uuid.New(),
		TenantID: tenantA,
		Name:     "Dev A",
		Slug:     "dev-a",
		Type:     "development",
		Status:   "active",
	}
	err := repo.Create(ctx, envA)
	if err != nil {
		t.Fatalf("create environment A: %v", err)
	}

	// Create environment for tenant B.
	envB := &Environment{
		ID:       uuid.New(),
		TenantID: tenantB,
		Name:     "Dev B",
		Slug:     "dev-b",
		Type:     "development",
		Status:   "active",
	}
	err = repo.Create(ctx, envB)
	if err != nil {
		t.Fatalf("create environment B: %v", err)
	}

	// List for tenant A.
	envsA, err := repo.List(ctx, tenantA)
	if err != nil {
		t.Fatalf("list environments A: %v", err)
	}
	if len(envsA) != 1 {
		t.Fatalf("tenant A should see 1 environment, got %d", len(envsA))
	}
	if envsA[0].TenantID != tenantA {
		t.Fatalf("tenant A should see only their own environment")
	}

	// List for tenant B.
	envsB, err := repo.List(ctx, tenantB)
	if err != nil {
		t.Fatalf("list environments B: %v", err)
	}
	if len(envsB) != 1 {
		t.Fatalf("tenant B should see 1 environment, got %d", len(envsB))
	}
	if envsB[0].TenantID != tenantB {
		t.Fatalf("tenant B should see only their own environment")
	}
}

// TestTenantIsolation_Deployments verifies deployment isolation.
func TestTenantIsolation_Deployments(t *testing.T) {
	env := testenv.NewTestEnv(t)
	defer env.Cleanup(t)

	ctx := context.Background()
	tenantA := uuid.New()
	tenantB := uuid.New()

	deployRepo := NewDeploymentRepository(env.DB())

	// Create deployment for tenant A.
	deployA := &Deployment{
		ID:            uuid.New(),
		TenantID:      tenantA,
		ImageTag:      "v1.0.0",
		ImageRepository: "registry/service-a",
		DeployType:    "helm",
		Status:        "deployed",
	}
	err := deployRepo.Create(ctx, deployA)
	if err != nil {
		t.Fatalf("create deployment A: %v", err)
	}

	// Create deployment for tenant B.
	deployB := &Deployment{
		ID:            uuid.New(),
		TenantID:      tenantB,
		ImageTag:      "v1.0.0",
		ImageRepository: "registry/service-b",
		DeployType:    "helm",
		Status:        "deployed",
	}
	err = deployRepo.Create(ctx, deployB)
	if err != nil {
		t.Fatalf("create deployment B: %v", err)
	}

	// List for tenant A.
	deploysA, err := deployRepo.List(ctx, tenantA)
	if err != nil {
		t.Fatalf("list deployments A: %v", err)
	}
	if len(deploysA) != 1 {
		t.Fatalf("tenant A should see 1 deployment, got %d", len(deploysA))
	}
	if deploysA[0].TenantID != tenantA {
		t.Fatalf("tenant A should see only their own deployment")
	}

	// List for tenant B.
	deploysB, err := deployRepo.List(ctx, tenantB)
	if err != nil {
		t.Fatalf("list deployments B: %v", err)
	}
	if len(deploysB) != 1 {
		t.Fatalf("tenant B should see 1 deployment, got %d", len(deploysB))
	}
	if deploysB[0].TenantID != tenantB {
		t.Fatalf("tenant B should see only their own deployment")
	}
}
