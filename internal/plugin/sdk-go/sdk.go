// Package sdk provides the Go SDK for building PEPA plugins.
//
// A PEPA plugin implements the provider.Provider interface and calls
// sdk.Serve() to start as a go-plugin subprocess. The PEPA host
// spawns the plugin binary and communicates via gRPC.
//
// Quick Start:
//
//	func main() {
//	    sdk.Serve(&MyPlugin{})
//	}
//
// The plugin binary must be placed in the configured plugin directory.
package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	hcplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"

	pb "github.com/pepa/pepa/internal/plugin/proto"
	"github.com/pepa/pepa/internal/provider"
)

// Handshake is the shared handshake config for all PEPA plugins.
// Must match the host-side Handshake in internal/plugin/engine.
var Handshake = hcplugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "PEPA_PLUGIN",
	MagicCookieValue: "pepa",
}

// Serve starts the plugin as a go-plugin subprocess and blocks until shutdown.
// The provided Provider implementation handles all actions dispatched by the host.
func Serve(p provider.Provider) {
	slog.Info("starting plugin @ type=", "name", p.Name(), p.Version(), p.PluginType())

	hcplugin.Serve(&hcplugin.ServeConfig{
		HandshakeConfig: Handshake,
		Plugins: map[string]hcplugin.Plugin{
			"pepa-plugin": &GRPCPlugin{Impl: p},
		},
		GRPCServer: hcplugin.DefaultGRPCServer,
	})
}

// GRPCPlugin implements plugin.GRPCPlugin for the server (plugin) side.
type GRPCPlugin struct {
	hcplugin.Plugin
	Impl provider.Provider
}

func (p *GRPCPlugin) GRPCServer(broker *hcplugin.GRPCBroker, s *grpc.Server) error {
	pb.RegisterPEPAPluginServer(s, &grpcServerImpl{Impl: p.Impl})
	return nil
}

func (p *GRPCPlugin) GRPCClient(ctx context.Context, broker *hcplugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return nil, fmt.Errorf("GRPCClient not implemented on plugin side")
}

// grpcServerImpl wraps a provider.Provider and serves it via gRPC.
type grpcServerImpl struct {
	pb.UnimplementedPEPAPluginServer
	Impl provider.Provider
}

func (s *grpcServerImpl) Info(ctx context.Context, _ *pb.InfoRequest) (*pb.InfoResponse, error) {
	actions := s.Impl.Actions()
	actionInfos := make([]*pb.ActionInfo, len(actions))
	for i, a := range actions {
		actionInfos[i] = &pb.ActionInfo{
			Name:        a,
			Description: "Action: " + a,
		}
	}
	return &pb.InfoResponse{
		Name:        s.Impl.Name(),
		Version:     s.Impl.Version(),
		Description: s.Impl.Description(),
		PluginType:  s.Impl.PluginType(),
		Actions:     actionInfos,
	}, nil
}

func (s *grpcServerImpl) Execute(ctx context.Context, req *pb.ExecuteRequest) (*pb.ExecuteResponse, error) {
	output, err := s.Impl.Execute(ctx, req.Action, req.Params, req.ConnectionConfig)
	if err != nil {
		return &pb.ExecuteResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}
	return &pb.ExecuteResponse{
		Success: true,
		Output:  output,
	}, nil
}

func (s *grpcServerImpl) HealthCheck(ctx context.Context, _ *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	start := time.Now()
	status, err := s.Impl.HealthCheck(ctx)
	if err != nil {
		return &pb.HealthCheckResponse{
			Status:  "unhealthy",
			Message: err.Error(),
		}, nil
	}
	return &pb.HealthCheckResponse{
		Status:    status.Status,
		Message:   status.Message,
		LatencyMs: time.Since(start).Milliseconds(),
	}, nil
}

// ── Convenience helpers for plugin authors ────────────────────

// JSONMarshal is a helper to marshal action output.
func JSONMarshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

// JSONUnmarshal is a helper to unmarshal action params.
func JSONUnmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

// ActionResult creates a successful JSON-encoded action result.
func ActionOutput(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}
