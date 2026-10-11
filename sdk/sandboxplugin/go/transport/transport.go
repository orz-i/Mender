package transport

import (
	"context"
	"fmt"
	"math"

	plugin "github.com/hashicorp/go-plugin"
	sandboxv1 "github.com/orz-i/Mender/sdk/sandboxplugin/go/protocol/v1"
	"google.golang.org/grpc"
)

const (
	PluginName = "sandbox"
	// MaxMessageBytes bounds one Sandbox Plugin Protocol gRPC message in either direction.
	MaxMessageBytes = 16 * 1024 * 1024
)

var Handshake = plugin.HandshakeConfig{
	MagicCookieKey:   "MENDER_SANDBOX_PLUGIN",
	MagicCookieValue: "sandbox-plugin",
}

// GRPCPlugin bridges go-plugin process transport to Mender's public gRPC service.
type GRPCPlugin struct {
	plugin.NetRPCUnsupportedPlugin
	Service sandboxv1.SandboxPluginServiceServer
}

func (p *GRPCPlugin) GRPCServer(
	_ *plugin.GRPCBroker,
	server *grpc.Server,
) error {
	if p.Service == nil {
		return fmt.Errorf("sandbox plugin transport: server implementation is required")
	}
	sandboxv1.RegisterSandboxPluginServiceServer(server, p.Service)
	return nil
}

func (*GRPCPlugin) GRPCClient(
	_ context.Context,
	_ *plugin.GRPCBroker,
	connection *grpc.ClientConn,
) (interface{}, error) {
	return sandboxv1.NewSandboxPluginServiceClient(connection), nil
}

// VersionedPluginSets builds go-plugin protocol sets for Mender protocol versions.
func VersionedPluginSets(
	versions []uint32,
	server sandboxv1.SandboxPluginServiceServer,
) (map[int]plugin.PluginSet, error) {
	if len(versions) == 0 {
		return nil, fmt.Errorf("sandbox plugin transport: versions must not be empty")
	}
	result := make(map[int]plugin.PluginSet, len(versions))
	for _, version := range versions {
		if version == 0 || version > math.MaxInt32 {
			return nil, fmt.Errorf("sandbox plugin transport: invalid version %d", version)
		}
		converted := int(version)
		if _, exists := result[converted]; exists {
			return nil, fmt.Errorf("sandbox plugin transport: duplicate version %d", version)
		}
		result[converted] = plugin.PluginSet{
			PluginName: &GRPCPlugin{Service: server},
		}
	}
	return result, nil
}

// Serve starts the canonical Sandbox Plugin Protocol server and blocks until shutdown.
func Serve(
	versions []uint32,
	server sandboxv1.SandboxPluginServiceServer,
) error {
	config, err := ServerConfig(versions, server)
	if err != nil {
		return err
	}
	plugin.Serve(config)
	return nil
}

// ServerConfig returns the canonical go-plugin server configuration.
func ServerConfig(
	versions []uint32,
	server sandboxv1.SandboxPluginServiceServer,
) (*plugin.ServeConfig, error) {
	sets, err := VersionedPluginSets(versions, server)
	if err != nil {
		return nil, err
	}
	if server == nil {
		return nil, fmt.Errorf("sandbox plugin transport: server implementation is required")
	}
	return &plugin.ServeConfig{
		HandshakeConfig:  Handshake,
		VersionedPlugins: sets,
		GRPCServer:       boundedGRPCServer,
	}, nil
}

func boundedGRPCServer(options []grpc.ServerOption) *grpc.Server {
	options = append(
		options,
		grpc.MaxRecvMsgSize(MaxMessageBytes),
		grpc.MaxSendMsgSize(MaxMessageBytes),
	)
	return grpc.NewServer(options...)
}
