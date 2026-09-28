package providerplugin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	pluginprotocol "github.com/orz-i/Mender/sdk/providerplugin/go/protocol"
	providerv1 "github.com/orz-i/Mender/sdk/providerplugin/go/protocol/v1"
	plugintransport "github.com/orz-i/Mender/sdk/providerplugin/go/transport"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Serve starts a Provider plugin process server using the supported Mender protocol versions.
func Serve(plugin Plugin) error {
	if plugin == nil {
		return fmt.Errorf("provider plugin sdk: plugin must not be nil")
	}
	return plugintransport.Serve(
		pluginprotocol.SupportedVersions(),
		&protocolServer{plugin: plugin},
	)
}

type protocolServer struct {
	providerv1.UnimplementedProviderPluginServiceServer
	plugin Plugin
}

func (s *protocolServer) GetDescriptor(
	_ context.Context,
	_ *providerv1.GetDescriptorRequest,
) (*providerv1.GetDescriptorResponse, error) {
	descriptor := s.plugin.Descriptor()
	if err := validateDescriptor(descriptor); err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}
	return &providerv1.GetDescriptorResponse{
		Descriptor_: &providerv1.Descriptor{
			PluginId:         descriptor.PluginID,
			PackageVersion:   descriptor.PackageVersion,
			DisplayName:      descriptor.DisplayName,
			Description:      descriptor.Description,
			ProtocolVersion:  pluginprotocol.Version1,
			ConfigVersion:    descriptor.ConfigVersion,
			ConfigSchemaJson: append([]byte(nil), descriptor.ConfigSchema...),
			UiSchemaJson:     append([]byte(nil), descriptor.UISchema...),
			Capabilities:     append([]string(nil), descriptor.Capabilities...),
		},
	}, nil
}

func (s *protocolServer) ValidateConfig(
	ctx context.Context,
	request *providerv1.ValidateConfigRequest,
) (*providerv1.ValidateConfigResponse, error) {
	diagnostics, err := s.plugin.ValidateConfig(ctx, ConfigRequest{
		ConfigVersion: request.GetConfigVersion(),
		Config:        append(json.RawMessage(nil), request.GetConfigJson()...),
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	response := &providerv1.ValidateConfigResponse{
		Valid:       true,
		Diagnostics: make([]*providerv1.Diagnostic, 0, len(diagnostics)),
	}
	for _, diagnostic := range diagnostics {
		wire, mapErr := diagnosticToProtocol(diagnostic)
		if mapErr != nil {
			return nil, status.Error(codes.FailedPrecondition, mapErr.Error())
		}
		if diagnostic.Severity == DiagnosticError {
			response.Valid = false
		}
		response.Diagnostics = append(response.Diagnostics, wire)
	}
	return response, nil
}

func (s *protocolServer) InspectConfig(
	ctx context.Context,
	request *providerv1.InspectConfigRequest,
) (*providerv1.InspectConfigResponse, error) {
	metadata, err := s.plugin.InspectConfig(ctx, ConfigRequest{
		ConfigVersion: request.GetConfigVersion(),
		Config:        append(json.RawMessage(nil), request.GetConfigJson()...),
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &providerv1.InspectConfigResponse{
		CredentialRefs:    append([]string(nil), metadata.CredentialRefs...),
		ConnectionSummary: metadata.ConnectionSummary,
	}, nil
}

func (s *protocolServer) Open(
	ctx context.Context,
	request *providerv1.OpenRequest,
) (*providerv1.OpenResponse, error) {
	credentials := credentialsFromProtocol(request.GetDiscoveryCredentials())
	defer ZeroCredentials(credentials)
	defer zeroProtocolCredentials(request.GetDiscoveryCredentials())

	sessionID, err := s.plugin.Open(ctx, OpenRequest{
		InstanceID:           request.GetInstanceId(),
		ConfigVersion:        request.GetConfigVersion(),
		Config:               append(json.RawMessage(nil), request.GetConfigJson()...),
		DiscoveryCredentials: credentials,
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if strings.TrimSpace(sessionID) == "" {
		return nil, status.Error(codes.FailedPrecondition, "plugin returned empty session id")
	}
	return &providerv1.OpenResponse{SessionId: sessionID}, nil
}

func (s *protocolServer) ListTools(
	ctx context.Context,
	request *providerv1.ListToolsRequest,
) (*providerv1.ListToolsResponse, error) {
	credentials := credentialsFromProtocol(request.GetDiscoveryCredentials())
	defer ZeroCredentials(credentials)
	defer zeroProtocolCredentials(request.GetDiscoveryCredentials())

	tools, err := s.plugin.ListTools(ctx, ListToolsRequest{
		SessionID:            request.GetSessionId(),
		DiscoveryCredentials: credentials,
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	response := &providerv1.ListToolsResponse{
		Tools: make([]*providerv1.ToolDescriptor, 0, len(tools)),
	}
	for _, tool := range tools {
		if err := validateTool(tool); err != nil {
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		}
		response.Tools = append(response.Tools, &providerv1.ToolDescriptor{
			SourceKey:        tool.SourceKey,
			Name:             tool.Name,
			Title:            tool.Title,
			Description:      tool.Description,
			InputSchemaJson:  append([]byte(nil), tool.InputSchema...),
			OutputSchemaJson: append([]byte(nil), tool.OutputSchema...),
			Tags:             append([]string(nil), tool.Tags...),
			CredentialRefs:   append([]string(nil), tool.CredentialRefs...),
		})
	}
	return response, nil
}

func (s *protocolServer) ExecuteTool(
	request *providerv1.ExecuteToolRequest,
	stream grpc.ServerStreamingServer[providerv1.ExecuteToolResponse],
) error {
	credentials := credentialsFromProtocol(request.GetCredentials())
	defer ZeroCredentials(credentials)
	defer zeroProtocolCredentials(request.GetCredentials())

	sender := &streamSender{stream: stream}
	err := s.plugin.ExecuteTool(stream.Context(), ExecuteRequest{
		SessionID:    request.GetSessionId(),
		InvocationID: request.GetInvocationId(),
		SourceKey:    request.GetSourceKey(),
		Arguments:    append(json.RawMessage(nil), request.GetArgumentsJson()...),
		Credentials:  credentials,
	}, sender)
	if err != nil {
		return status.Error(codes.Internal, err.Error())
	}
	if !sender.terminalSent() {
		return status.Error(
			codes.FailedPrecondition,
			"plugin execution completed without a terminal event",
		)
	}
	return nil
}

func (s *protocolServer) Close(
	ctx context.Context,
	request *providerv1.CloseRequest,
) (*providerv1.CloseResponse, error) {
	if err := s.plugin.Close(ctx, request.GetSessionId()); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &providerv1.CloseResponse{}, nil
}

type streamSender struct {
	stream grpc.ServerStreamingServer[providerv1.ExecuteToolResponse]

	mu       sync.Mutex
	terminal bool
}

func (s *streamSender) Progress(message string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.terminal {
		return fmt.Errorf("provider plugin sdk: progress after terminal event")
	}
	return s.stream.Send(&providerv1.ExecuteToolResponse{
		Event: &providerv1.ExecuteToolResponse_Progress{
			Progress: &providerv1.Progress{Message: message},
		},
	})
}

func (s *streamSender) Complete(content json.RawMessage) error {
	return s.sendTerminal(&providerv1.ExecuteToolResponse{
		Event: &providerv1.ExecuteToolResponse_Complete{
			Complete: &providerv1.Complete{
				ContentJson: append([]byte(nil), content...),
			},
		},
	})
}

func (s *streamSender) InputRequired(
	requestID string,
	message string,
	inputSchema json.RawMessage,
) error {
	return s.sendTerminal(&providerv1.ExecuteToolResponse{
		Event: &providerv1.ExecuteToolResponse_InputRequired{
			InputRequired: &providerv1.InputRequired{
				RequestId:       requestID,
				Message:         message,
				InputSchemaJson: append([]byte(nil), inputSchema...),
			},
		},
	})
}

func (s *streamSender) Task(taskID, taskStatus, message string) error {
	return s.sendTerminal(&providerv1.ExecuteToolResponse{
		Event: &providerv1.ExecuteToolResponse_Task{
			Task: &providerv1.Task{
				TaskId:  taskID,
				Status:  taskStatus,
				Message: message,
			},
		},
	})
}

func (s *streamSender) ToolFailure(message string) error {
	return s.sendFailure(providerv1.Failure_KIND_TOOL_ERROR, message)
}

func (s *streamSender) ProviderFailure(message string) error {
	return s.sendFailure(providerv1.Failure_KIND_PROVIDER_ERROR, message)
}

func (s *streamSender) sendFailure(
	kind providerv1.Failure_Kind,
	message string,
) error {
	return s.sendTerminal(&providerv1.ExecuteToolResponse{
		Event: &providerv1.ExecuteToolResponse_Failure{
			Failure: &providerv1.Failure{
				Kind:    kind,
				Message: message,
			},
		},
	})
}

func (s *streamSender) sendTerminal(
	response *providerv1.ExecuteToolResponse,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.terminal {
		return fmt.Errorf("provider plugin sdk: multiple terminal events")
	}
	if err := s.stream.Send(response); err != nil {
		return err
	}
	s.terminal = true
	return nil
}

func (s *streamSender) terminalSent() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.terminal
}

func credentialsFromProtocol(values []*providerv1.Credential) []Credential {
	result := make([]Credential, 0, len(values))
	for _, value := range values {
		if value == nil {
			continue
		}
		result = append(result, Credential{
			Reference: value.GetReference(),
			Value:     append([]byte(nil), value.GetValue()...),
		})
	}
	return result
}

func zeroProtocolCredentials(values []*providerv1.Credential) {
	for _, value := range values {
		if value == nil {
			continue
		}
		for index := range value.Value {
			value.Value[index] = 0
		}
		value.Value = nil
	}
}

func diagnosticToProtocol(
	diagnostic Diagnostic,
) (*providerv1.Diagnostic, error) {
	var severity providerv1.Diagnostic_Severity
	switch diagnostic.Severity {
	case DiagnosticError:
		severity = providerv1.Diagnostic_SEVERITY_ERROR
	case DiagnosticWarning:
		severity = providerv1.Diagnostic_SEVERITY_WARNING
	default:
		return nil, fmt.Errorf(
			"provider plugin sdk: invalid diagnostic severity %d",
			diagnostic.Severity,
		)
	}
	return &providerv1.Diagnostic{
		Severity: severity,
		Path:     diagnostic.Path,
		Code:     diagnostic.Code,
		Message:  diagnostic.Message,
	}, nil
}

func validateDescriptor(descriptor Descriptor) error {
	if strings.TrimSpace(descriptor.PluginID) == "" {
		return fmt.Errorf("provider plugin sdk: descriptor plugin id is required")
	}
	if strings.TrimSpace(descriptor.PackageVersion) == "" {
		return fmt.Errorf("provider plugin sdk: descriptor package version is required")
	}
	if strings.TrimSpace(descriptor.DisplayName) == "" {
		return fmt.Errorf("provider plugin sdk: descriptor display name is required")
	}
	if descriptor.ConfigVersion == 0 {
		return fmt.Errorf("provider plugin sdk: descriptor config version must be positive")
	}
	if len(descriptor.ConfigSchema) != 0 && !json.Valid(descriptor.ConfigSchema) {
		return fmt.Errorf("provider plugin sdk: descriptor config schema must be valid JSON")
	}
	if len(descriptor.UISchema) != 0 && !json.Valid(descriptor.UISchema) {
		return fmt.Errorf("provider plugin sdk: descriptor UI schema must be valid JSON")
	}
	return nil
}

func validateTool(tool Tool) error {
	if strings.TrimSpace(tool.SourceKey) == "" {
		return fmt.Errorf("provider plugin sdk: tool source key is required")
	}
	if strings.TrimSpace(tool.Name) == "" {
		return fmt.Errorf("provider plugin sdk: tool name is required")
	}
	if len(tool.InputSchema) == 0 || !json.Valid(tool.InputSchema) {
		return fmt.Errorf(
			"provider plugin sdk: tool %s input schema must be valid JSON",
			tool.SourceKey,
		)
	}
	if len(tool.OutputSchema) != 0 && !json.Valid(tool.OutputSchema) {
		return fmt.Errorf(
			"provider plugin sdk: tool %s output schema must be valid JSON",
			tool.SourceKey,
		)
	}
	return nil
}
