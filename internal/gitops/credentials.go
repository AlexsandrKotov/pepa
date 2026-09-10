// Package gitops credentials resolver provides centralized credential resolution
// for GitOps engines (ArgoCD and FluxCD). This is the single source of truth
// for resolving engine credentials from connections, bindings, and cluster labels.
package gitops

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/repository"
)

// ArgoCredentials holds resolved credentials for ArgoCD.
type ArgoCredentials struct {
	ServerURL    string
	AuthToken    string
	Namespace    string
	Kubeconfig   string
	CABundle     string
	Insecure     bool
	ConnectionID uuid.UUID
}

// FluxCredentials holds resolved credentials for FluxCD (CRD mode only).
type FluxCredentials struct {
	Kubeconfig   string
	ConnectionID uuid.UUID
}

// CredentialResolver resolves GitOps engine credentials from multiple sources.
type CredentialResolver struct {
	connRepo  *repository.ConnectionRepository
	vaultRepo *repository.VaultRepository
}

// NewCredentialResolver creates a new credential resolver.
func NewCredentialResolver(connRepo *repository.ConnectionRepository, vaultRepo *repository.VaultRepository) *CredentialResolver {
	return &CredentialResolver{
		connRepo:  connRepo,
		vaultRepo: vaultRepo,
	}
}

// ResolveOpts specifies options for credential resolution.
type ResolveOpts struct {
	ConnectionID *uuid.UUID // explicit connection ID (highest priority)
	BindingID    *uuid.UUID // binding reference (for app-specific resolution)
	ClusterID    *uuid.UUID // cluster reference (for cluster-scoped resolution)
	TenantID     uuid.UUID  // tenant ID (required)
}

// ResolveArgo resolves ArgoCD credentials from multiple sources in priority order:
// 1. Explicit connection_id
// 2. Binding's argo_connection_id
// 3. Cluster labels argo_connection_id
// 4. GitOps repository's argocd_connection_id
// 5. Tenant's single argocd connection
// 6. Error (no silent fallback)
func (r *CredentialResolver) ResolveArgo(ctx context.Context, opts ResolveOpts) (*ArgoCredentials, error) {
	if r.connRepo == nil {
		return nil, fmt.Errorf("connection repository not available")
	}

	// 1. Explicit connection_id
	if opts.ConnectionID != nil {
		conn, err := r.connRepo.GetDecrypted(ctx, *opts.ConnectionID, opts.TenantID)
		if err != nil {
			return nil, fmt.Errorf("get connection %s: %w", *opts.ConnectionID, err)
		}
		if conn.Type != repository.ConnectionArgoCD && conn.Type != repository.ConnectionKubernetes {
			return nil, fmt.Errorf("connection %s is not an ArgoCD connection (type: %s)", *opts.ConnectionID, conn.Type)
		}
		return r.argocredsFromConnection(ctx, conn)
	}

	// 2-4. TODO: Implement binding/cluster/repo resolution in later stages
	// For now, fall through to step 5

	// 5. Tenant's single argocd connection (tenant-scoped)
	conns, err := r.connRepo.List(ctx, opts.TenantID, string(repository.ConnectionArgoCD))
	if err != nil {
		return nil, fmt.Errorf("list argocd connections for tenant %s: %w", opts.TenantID, err)
	}
	// Filter to connected ones only
	var connected []*repository.Connection
	for i := range conns {
		if conns[i].Status == "connected" {
			connected = append(connected, &conns[i])
		}
	}
	// 5b. Also check kubernetes connections (unified type)
	if len(connected) == 0 {
		kConns, kErr := r.connRepo.List(ctx, opts.TenantID, string(repository.ConnectionKubernetes))
		if kErr == nil {
			for i := range kConns {
				if kConns[i].Status == "connected" {
					connected = append(connected, &kConns[i])
				}
			}
		}
	}
	if len(connected) == 0 {
		return nil, fmt.Errorf("no ArgoCD connection configured for tenant %s", opts.TenantID)
	}
	if len(connected) > 1 {
		return nil, fmt.Errorf("multiple ArgoCD connections found for tenant %s; specify connection_id explicitly", opts.TenantID)
	}
	decrypted, err := r.connRepo.GetDecrypted(ctx, connected[0].ID, opts.TenantID)
	if err != nil {
		return nil, fmt.Errorf("decrypt argocd connection %s: %w", connected[0].ID, err)
	}
	return r.argocredsFromConnection(ctx, decrypted)
}

// ResolveFlux resolves FluxCD credentials (CRD mode only).
func (r *CredentialResolver) ResolveFlux(ctx context.Context, opts ResolveOpts) (*FluxCredentials, error) {
	if r.connRepo == nil {
		return nil, fmt.Errorf("connection repository not available")
	}

	// 1. Explicit connection_id
	if opts.ConnectionID != nil {
		conn, err := r.connRepo.GetDecrypted(ctx, *opts.ConnectionID, opts.TenantID)
		if err != nil {
			return nil, fmt.Errorf("get connection %s: %w", *opts.ConnectionID, err)
		}
		if conn.Type != repository.ConnectionFluxCD && conn.Type != repository.ConnectionKubernetes {
			return nil, fmt.Errorf("connection %s is not a FluxCD connection (type: %s)", *opts.ConnectionID, conn.Type)
		}
		return r.fluxcredsFromConnection(ctx, conn)
	}

	// 2-4. TODO: Implement binding/cluster/repo resolution in later stages

	// 5. Tenant's single fluxcd connection (tenant-scoped)
	conns, err := r.connRepo.List(ctx, opts.TenantID, string(repository.ConnectionFluxCD))
	if err != nil {
		return nil, fmt.Errorf("list fluxcd connections for tenant %s: %w", opts.TenantID, err)
	}
	var connected []*repository.Connection
	for i := range conns {
		if conns[i].Status == "connected" {
			connected = append(connected, &conns[i])
		}
	}
	// 5b. Also check kubernetes connections (unified type)
	if len(connected) == 0 {
		kConns, kErr := r.connRepo.List(ctx, opts.TenantID, string(repository.ConnectionKubernetes))
		if kErr == nil {
			for i := range kConns {
				if kConns[i].Status == "connected" {
					connected = append(connected, &kConns[i])
				}
			}
		}
	}
	if len(connected) == 0 {
		return nil, fmt.Errorf("no FluxCD connection configured for tenant %s", opts.TenantID)
	}
	if len(connected) > 1 {
		return nil, fmt.Errorf("multiple FluxCD connections found for tenant %s; specify connection_id explicitly", opts.TenantID)
	}
	decrypted, err := r.connRepo.GetDecrypted(ctx, connected[0].ID, opts.TenantID)
	if err != nil {
		return nil, fmt.Errorf("decrypt fluxcd connection %s: %w", connected[0].ID, err)
	}
	return r.fluxcredsFromConnection(ctx, decrypted)
}

// argocredsFromConnection extracts ArgoCD credentials from a connection.
func (r *CredentialResolver) argocredsFromConnection(ctx context.Context, conn *repository.Connection) (*ArgoCredentials, error) {
	if conn == nil {
		return nil, fmt.Errorf("connection is nil")
	}

	creds := &ArgoCredentials{
		ConnectionID: conn.ID,
	}

	// Extract fields from config
	if serverURL, ok := conn.Config["server_url"].(string); ok {
		creds.ServerURL = serverURL
	}
	if authToken, ok := conn.Config["auth_token"].(string); ok {
		creds.AuthToken = r.resolveVaultRef(ctx, conn.TenantID, authToken)
	}
	if kubeconfig, ok := conn.Config["kubeconfig"].(string); ok {
		creds.Kubeconfig = r.resolveVaultRef(ctx, conn.TenantID, kubeconfig)
	}
	if caBundle, ok := conn.Config["ca_bundle"].(string); ok {
		creds.CABundle = caBundle
	}
	if insecure, ok := conn.Config["insecure"].(bool); ok {
		creds.Insecure = insecure
	}
	if namespace, ok := conn.Config["core_namespace"].(string); ok {
		creds.Namespace = namespace
	}

	// Validate: must have either server_url+auth_token or kubeconfig
	if creds.ServerURL == "" && creds.Kubeconfig == "" {
		return nil, fmt.Errorf("ArgoCD connection %s must have either server_url or kubeconfig", conn.ID)
	}
	if creds.ServerURL != "" && creds.AuthToken == "" {
		return nil, fmt.Errorf("ArgoCD connection %s has server_url but no auth_token", conn.ID)
	}

	return creds, nil
}

// fluxcredsFromConnection extracts FluxCD credentials from a connection.
func (r *CredentialResolver) fluxcredsFromConnection(ctx context.Context, conn *repository.Connection) (*FluxCredentials, error) {
	if conn == nil {
		return nil, fmt.Errorf("connection is nil")
	}

	creds := &FluxCredentials{
		ConnectionID: conn.ID,
	}

	// Extract kubeconfig from config
	if kubeconfig, ok := conn.Config["kubeconfig"].(string); ok {
		creds.Kubeconfig = r.resolveVaultRef(ctx, conn.TenantID, kubeconfig)
	}

	// Validate: must have kubeconfig
	if creds.Kubeconfig == "" {
		return nil, fmt.Errorf("FluxCD connection %s must have kubeconfig", conn.ID)
	}

	return creds, nil
}

// ResolveAllFlux returns FluxCD credentials for every matching connection in the
// tenant.  It collects both dedicated fluxcd connections and kubernetes-type
// connections (the unified replacement).  This is used by the engine List path
// where we need to query ALL clusters, not just one.
func (r *CredentialResolver) ResolveAllFlux(ctx context.Context, opts ResolveOpts) ([]*FluxCredentials, error) {
	if r.connRepo == nil {
		return nil, fmt.Errorf("connection repository not available")
	}

	var allCreds []*FluxCredentials
	seen := map[uuid.UUID]bool{}

	// 1. Explicit connection_id (highest priority)
	if opts.ConnectionID != nil {
		conn, err := r.connRepo.GetDecrypted(ctx, *opts.ConnectionID, opts.TenantID)
		if err != nil {
			return nil, fmt.Errorf("get connection %s: %w", *opts.ConnectionID, err)
		}
		if conn.Type != repository.ConnectionFluxCD && conn.Type != repository.ConnectionKubernetes {
			return nil, fmt.Errorf("connection %s is not a FluxCD connection (type: %s)", *opts.ConnectionID, conn.Type)
		}
		creds, err := r.fluxcredsFromConnection(ctx, conn)
		if err == nil {
			allCreds = append(allCreds, creds)
			seen[conn.ID] = true
		}
		return allCreds, nil
	}

	// 2. Dedicated fluxcd connections
	conns, err := r.connRepo.List(ctx, opts.TenantID, string(repository.ConnectionFluxCD))
	if err == nil {
		for i := range conns {
			if conns[i].Status != "connected" || seen[conns[i].ID] {
				continue
			}
			decrypted, derr := r.connRepo.GetDecrypted(ctx, conns[i].ID, opts.TenantID)
			if derr != nil {
				continue
			}
			creds, cerr := r.fluxcredsFromConnection(ctx, decrypted)
			if cerr == nil {
				allCreds = append(allCreds, creds)
				seen[conns[i].ID] = true
			}
		}
	}

	// 3. Kubernetes-type connections (unified fallback)
	kConns, kErr := r.connRepo.List(ctx, opts.TenantID, string(repository.ConnectionKubernetes))
	if kErr == nil {
		for i := range kConns {
			if kConns[i].Status != "connected" || seen[kConns[i].ID] {
				continue
			}
			decrypted, derr := r.connRepo.GetDecrypted(ctx, kConns[i].ID, opts.TenantID)
			if derr != nil {
				continue
			}
			creds, cerr := r.fluxcredsFromConnection(ctx, decrypted)
			if cerr == nil {
				allCreds = append(allCreds, creds)
				seen[kConns[i].ID] = true
			}
		}
	}

	if len(allCreds) == 0 {
		return nil, fmt.Errorf("no FluxCD connection configured for tenant %s", opts.TenantID)
	}
	return allCreds, nil
}

// ResolveAllArgo returns ArgoCD credentials for every matching connection in the
// tenant.  It collects both dedicated argocd connections and kubernetes-type
// connections.  This is used by the engine List path where we need to query
// ALL clusters, not just one.
func (r *CredentialResolver) ResolveAllArgo(ctx context.Context, opts ResolveOpts) ([]*ArgoCredentials, error) {
	if r.connRepo == nil {
		return nil, fmt.Errorf("connection repository not available")
	}

	var allCreds []*ArgoCredentials
	seen := map[uuid.UUID]bool{}

	// 1. Explicit connection_id (highest priority)
	if opts.ConnectionID != nil {
		conn, err := r.connRepo.GetDecrypted(ctx, *opts.ConnectionID, opts.TenantID)
		if err != nil {
			return nil, fmt.Errorf("get connection %s: %w", *opts.ConnectionID, err)
		}
		if conn.Type != repository.ConnectionArgoCD && conn.Type != repository.ConnectionKubernetes {
			return nil, fmt.Errorf("connection %s is not an ArgoCD connection (type: %s)", *opts.ConnectionID, conn.Type)
		}
		creds, err := r.argocredsFromConnection(ctx, conn)
		if err == nil {
			allCreds = append(allCreds, creds)
			seen[conn.ID] = true
		}
		return allCreds, nil
	}

	// 2. Dedicated argocd connections
	conns, err := r.connRepo.List(ctx, opts.TenantID, string(repository.ConnectionArgoCD))
	if err == nil {
		for i := range conns {
			if conns[i].Status != "connected" || seen[conns[i].ID] {
				continue
			}
			decrypted, derr := r.connRepo.GetDecrypted(ctx, conns[i].ID, opts.TenantID)
			if derr != nil {
				continue
			}
			creds, cerr := r.argocredsFromConnection(ctx, decrypted)
			if cerr == nil {
				allCreds = append(allCreds, creds)
				seen[conns[i].ID] = true
			}
		}
	}

	// 3. Kubernetes-type connections (unified fallback)
	kConns, kErr := r.connRepo.List(ctx, opts.TenantID, string(repository.ConnectionKubernetes))
	if kErr == nil {
		for i := range kConns {
			if kConns[i].Status != "connected" || seen[kConns[i].ID] {
				continue
			}
			decrypted, derr := r.connRepo.GetDecrypted(ctx, kConns[i].ID, opts.TenantID)
			if derr != nil {
				continue
			}
			creds, cerr := r.argocredsFromConnection(ctx, decrypted)
			if cerr == nil {
				allCreds = append(allCreds, creds)
				seen[kConns[i].ID] = true
			}
		}
	}

	if len(allCreds) == 0 {
		return nil, fmt.Errorf("no ArgoCD connection configured for tenant %s", opts.TenantID)
	}
	return allCreds, nil
}

// resolveVaultRef resolves a vault reference (vault:<path>) to the actual secret value.
// If the value is not a vault reference, it returns the value as-is.
// Uses tenant-scoped Vault access to maintain multi-tenant isolation.
func (r *CredentialResolver) resolveVaultRef(ctx context.Context, tenantID uuid.UUID, value string) string {
	if value == "" || !strings.HasPrefix(value, "vault:") {
		return value
	}

	if r.vaultRepo == nil {
		return value // Vault not available, return as-is
	}

	secretPath := strings.TrimPrefix(value, "vault:")
	secret, err := r.vaultRepo.Get(ctx, tenantID, secretPath)
	if err != nil {
		slog.Warn("vault secret resolution failed", "tenant_id", tenantID, "path", secretPath, "error", err)
		return value
	}

	if val, ok := secret.Data["value"]; ok && val != "" {
		return val
	}

	return value
}
