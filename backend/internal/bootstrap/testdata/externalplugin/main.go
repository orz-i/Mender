package main

import (
	"context"

	pluginprotocol "github.com/orz-i/Mender/sdk/providerplugin/go/protocol"
	providerv1 "github.com/orz-i/Mender/sdk/providerplugin/go/protocol/v1"
	plugintransport "github.com/orz-i/Mender/sdk/providerplugin/go/transport"
	"google.golang.org/grpc"
)

func main() {
	if err := plugintransport.Serve(
		pluginprotocol.SupportedVersions(),
		&server{},
	); err != nil {
		panic(err)
	}
}

type server struct {
	providerv1.UnimplementedProviderPluginServiceServer
}

func (*server) GetDescriptor(
	context.Context,
	*providerv1.GetDescriptorRequest,
) (*providerv1.GetDescriptorResponse, error) {
	return &providerv1.GetDescriptorResponse{
		Descriptor_: &providerv1.Descriptor{
			PluginId:         "dev.mender.test.echo",
			PackageVersion:   "1.0.0",
			DisplayName:      "External Echo",
			Description:      "External runtime integration fixture",
			ProtocolVersion:  pluginprotocol.Version1,
			ConfigVersion:    1,
			ConfigSchemaJson: []byte(`{"type":"object","additionalProperties":false}`),
		},
	}, nil
}

func (*server) ValidateConfig(
	context.Context,
	*providerv1.ValidateConfigRequest,
) (*providerv1.ValidateConfigResponse, error) {
	return &providerv1.ValidateConfigResponse{Valid: true}, nil
}

func (*server) InspectConfig(
	context.Context,
	*providerv1.InspectConfigRequest,
) (*providerv1.InspectConfigResponse, error) {
	return &providerv1.InspectConfigResponse{
		ConnectionSummary: "external test fixture",
	}, nil
}

func (*server) Open(
	context.Context,
	*providerv1.OpenRequest,
) (*providerv1.OpenResponse, error) {
	return &providerv1.OpenResponse{SessionId: "fixture-session"}, nil
}

func (*server) ListTools(
	context.Context,
	*providerv1.ListToolsRequest,
) (*providerv1.ListToolsResponse, error) {
	return &providerv1.ListToolsResponse{
		Tools: []*providerv1.ToolDescriptor{{
			SourceKey:        "echo",
			Name:             "echo",
			Title:            "External Echo",
			Description:      "Return JSON arguments unchanged",
			InputSchemaJson:  []byte(`{"type":"object"}`),
			OutputSchemaJson: []byte(`{"type":"object"}`),
			Tags:             []string{"external", "fixture"},
		}},
	}, nil
}

func (*server) ExecuteTool(
	request *providerv1.ExecuteToolRequest,
	stream grpc.ServerStreamingServer[providerv1.ExecuteToolResponse],
) error {
	if err := stream.Send(&providerv1.ExecuteToolResponse{
		Event: &providerv1.ExecuteToolResponse_Progress{
			Progress: &providerv1.Progress{Message: "external fixture executing"},
		},
	}); err != nil {
		return err
	}
	return stream.Send(&providerv1.ExecuteToolResponse{
		Event: &providerv1.ExecuteToolResponse_Complete{
			Complete: &providerv1.Complete{
				ContentJson: append([]byte(nil), request.GetArgumentsJson()...),
			},
		},
	})
}

func (*server) Close(
	context.Context,
	*providerv1.CloseRequest,
) (*providerv1.CloseResponse, error) {
	return &providerv1.CloseResponse{}, nil
}
