package engine

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"

	pb "github.com/pepa/pepa/internal/plugin/proto"
	"github.com/pepa/pepa/internal/provider"
)

// Handshake is the shared handshake config for all PEPA plugins.
var Handshake = plugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "PEPA_PLUGIN",
	MagicCookieValue: "pepa",
}

// PEPAPluginInterface is the interface go-plugin uses to create client/server.
type PEPAPluginInterface interface {
	PEPAPluginClient() pb.PEPAPluginClient
}

// GRPCPlugin implements plugin.GRPCPlugin for go-plugin.
type GRPCPlugin struct {
	plugin.Plugin
	Impl provider.Provider
}

func (p *GRPCPlugin) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	pb.RegisterPEPAPluginServer(s, &grpcServer{Impl: p.Impl})
	return nil
}

func (p *GRPCPlugin) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return &grpcClient{client: pb.NewPEPAPluginClient(c)}, nil
}

// ── gRPC Client (host side) ───────────────────────────────────

type grpcClient struct {
	client pb.PEPAPluginClient
}

func (c *grpcClient) Info(ctx context.Context) (*pb.InfoResponse, error) {
	return c.client.Info(ctx, &pb.InfoRequest{})
}

func (c *grpcClient) Execute(ctx context.Context, action string, params []byte, tenantID string, config map[string]string) (*pb.ExecuteResponse, error) {
	return c.client.Execute(ctx, &pb.ExecuteRequest{
		Action:           action,
		Params:           params,
		TenantId:         tenantID,
		ConnectionConfig: config,
	})
}

func (c *grpcClient) HealthCheck(ctx context.Context) (*pb.HealthCheckResponse, error) {
	return c.client.HealthCheck(ctx, &pb.HealthCheckRequest{})
}

// ── gRPC Server (plugin side) ─────────────────────────────────

type grpcServer struct {
	pb.UnimplementedPEPAPluginServer
	Impl provider.Provider
}

func (s *grpcServer) Info(ctx context.Context, _ *pb.InfoRequest) (*pb.InfoResponse, error) {
	impl := s.Impl
	actions := impl.Actions()
	actionInfos := make([]*pb.ActionInfo, len(actions))
	for i, a := range actions {
		actionInfos[i] = &pb.ActionInfo{
			Name:        a,
			Description: "Action: " + a,
		}
	}
	return &pb.InfoResponse{
		Name:        impl.Name(),
		Version:     impl.Version(),
		Description: impl.Description(),
		PluginType:  impl.PluginType(),
		Actions:     actionInfos,
	}, nil
}

func (s *grpcServer) Execute(ctx context.Context, req *pb.ExecuteRequest) (*pb.ExecuteResponse, error) {
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

func (s *grpcServer) HealthCheck(ctx context.Context, _ *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
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

// ── Helper: convert ExecuteResponse to provider.ActionResult ──

func ExecuteResponseToActionResult(resp *pb.ExecuteResponse) *provider.ActionResult {
	result := &provider.ActionResult{
		Success: resp.Success,
		Output:  json.RawMessage(resp.Output),
		Error:   resp.Error,
	}
	return result
}
