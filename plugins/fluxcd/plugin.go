package main

import (
	"context"
	"fmt"

	"github.com/pepa/pepa/internal/plugin/sdk-go"
	"github.com/pepa/pepa/internal/provider"
)

// FluxCDPlugin implements provider.Provider for FluxCD GitOps.
type FluxCDPlugin struct{}

var _ provider.Provider = (*FluxCDPlugin)(nil)

func (p *FluxCDPlugin) Name() string    { return "fluxcd" }
func (p *FluxCDPlugin) Version() string { return "0.1.0" }
func (p *FluxCDPlugin) Description() string {
	return "FluxCD GitOps — Kustomizations, HelmReleases, reconcile, suspend/resume"
}
func (p *FluxCDPlugin) PluginType() string { return "cd_engine" }

func (p *FluxCDPlugin) Actions() []string {
	return []string{
		"list_kustomizations",
		"get_kustomization",
		"reconcile_kustomization",
		"list_helmreleases",
		"get_helmrelease",
		"reconcile_helmrelease",
		"suspend",
		"resume",
		"get_health",
	}
}

func (p *FluxCDPlugin) Execute(ctx context.Context, action string, params []byte, config map[string]string) ([]byte, error) {
	kubeconfig := config["kubeconfig"]
	if kubeconfig == "" {
		return nil, fmt.Errorf("fluxcd plugin requires 'kubeconfig' in connection config")
	}

	controller, err := NewFluxController([]byte(kubeconfig))
	if err != nil {
		return nil, fmt.Errorf("create flux controller: %w", err)
	}

	switch action {
	case "list_kustomizations":
		return p.listKustomizations(ctx, controller, params)
	case "get_kustomization":
		return p.getKustomization(ctx, controller, params)
	case "reconcile_kustomization":
		return p.reconcileKustomization(ctx, controller, params)
	case "list_helmreleases":
		return p.listHelmReleases(ctx, controller, params)
	case "get_helmrelease":
		return p.getHelmRelease(ctx, controller, params)
	case "reconcile_helmrelease":
		return p.reconcileHelmRelease(ctx, controller, params)
	case "suspend":
		return p.suspend(ctx, controller, params)
	case "resume":
		return p.resume(ctx, controller, params)
	case "get_health":
		return p.getHealth(ctx, controller, params)
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

func (p *FluxCDPlugin) HealthCheck(ctx context.Context) (*provider.HealthStatus, error) {
	return &provider.HealthStatus{
		Status:  "healthy",
		Message: "FluxCD plugin ready — requires connection config (kubeconfig)",
	}, nil
}

// actionOutput is a helper to encode action results.
func actionOutput(v interface{}) ([]byte, error) {
	return sdk.JSONMarshal(v)
}

// actionInput is a helper to decode action params.
func actionInput(data []byte, v interface{}) error {
	return sdk.JSONUnmarshal(data, v)
}
