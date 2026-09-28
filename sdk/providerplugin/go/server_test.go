package providerplugin

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	providerv1 "github.com/orz-i/Mender/sdk/providerplugin/go/protocol/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestProtocolServerMapsDescriptorAndValidation(t *testing.T) {
	t.Parallel()

	plugin := &testPlugin{}
	server := &protocolServer{plugin: plugin}

	descriptor, err := server.GetDescriptor(
		context.Background(),
		&providerv1.GetDescriptorRequest{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if descriptor.GetDescriptor_().GetPluginId() != "dev.example.echo" {
		t.Fatalf("descriptor = %+v", descriptor.GetDescriptor_())
	}

	validated, err := server.ValidateConfig(
		context.Background(),
		&providerv1.ValidateConfigRequest{
			ConfigVersion: 1,
			ConfigJson:    []byte("{}"),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if validated.GetValid() {
		t.Fatal("ValidateConfig() valid = true")
	}
	if len(validated.GetDiagnostics()) != 1 ||
		validated.GetDiagnostics()[0].GetSeverity() != providerv1.Diagnostic_SEVERITY_ERROR {
		t.Fatalf("diagnostics = %+v", validated.GetDiagnostics())
	}
}

func TestCredentialHelpersCopyAndZero(t *testing.T) {
	t.Parallel()

	credentials := []Credential{{
		Reference: "credential://demo/token",
		Value:     []byte("secret"),
	}}
	value, ok := FindCredential(credentials, "credential://demo/token")
	if !ok || string(value) != "secret" {
		t.Fatalf("FindCredential() = %q, %v", value, ok)
	}
	value[0] = 'X'
	if string(credentials[0].Value) != "secret" {
		t.Fatal("FindCredential() exposed mutable credential bytes")
	}
	ZeroCredentials(credentials)
	if credentials[0].Value != nil {
		t.Fatalf("ZeroCredentials() value = %v", credentials[0].Value)
	}
}

func TestEventSenderEnforcesExactlyOneTerminalEvent(t *testing.T) {
	t.Parallel()

	stream := &testExecutionStream{ctx: context.Background()}
	sender := &streamSender{stream: stream}

	if err := sender.Progress("working"); err != nil {
		t.Fatal(err)
	}
	if err := sender.Complete(json.RawMessage(`{"ok":true}`)); err != nil {
		t.Fatal(err)
	}
	if !sender.terminalSent() {
		t.Fatal("terminalSent() = false")
	}
	if err := sender.ToolFailure("second terminal"); err == nil {
		t.Fatal("second terminal event error = nil")
	}
	if err := sender.Progress("too late"); err == nil {
		t.Fatal("progress after terminal error = nil")
	}
	if len(stream.sent) != 2 {
		t.Fatalf("sent event count = %d, want 2", len(stream.sent))
	}
}

func TestProtocolServerRejectsExecutionWithoutTerminalEvent(t *testing.T) {
	t.Parallel()

	server := &protocolServer{
		plugin: &noTerminalPlugin{testPlugin: &testPlugin{}},
	}
	stream := &testExecutionStream{ctx: context.Background()}
	err := server.ExecuteTool(
		&providerv1.ExecuteToolRequest{
			SessionId:     "session",
			InvocationId:  "invocation",
			SourceKey:     "echo",
			ArgumentsJson: []byte("{}"),
		},
		stream,
	)
	if err == nil || !strings.Contains(err.Error(), "without a terminal event") {
		t.Fatalf("ExecuteTool() error = %v", err)
	}
}

func TestValidateDescriptorRejectsInvalidSchema(t *testing.T) {
	t.Parallel()

	descriptor := testPlugin{}.Descriptor()
	descriptor.ConfigSchema = json.RawMessage("{")
	if err := validateDescriptor(descriptor); err == nil {
		t.Fatal("validateDescriptor() error = nil")
	}
}

type testExecutionStream struct {
	ctx  context.Context
	sent []*providerv1.ExecuteToolResponse
}

func (s *testExecutionStream) Send(
	response *providerv1.ExecuteToolResponse,
) error {
	s.sent = append(s.sent, response)
	return nil
}

func (*testExecutionStream) SetHeader(metadata.MD) error {
	return nil
}

func (*testExecutionStream) SendHeader(metadata.MD) error {
	return nil
}

func (*testExecutionStream) SetTrailer(metadata.MD) {}

func (s *testExecutionStream) Context() context.Context {
	return s.ctx
}

func (*testExecutionStream) SendMsg(any) error {
	return nil
}

func (*testExecutionStream) RecvMsg(any) error {
	return nil
}

var _ grpc.ServerStreamingServer[providerv1.ExecuteToolResponse] = (*testExecutionStream)(nil)

type noTerminalPlugin struct {
	*testPlugin
}

func (*noTerminalPlugin) ExecuteTool(
	context.Context,
	ExecuteRequest,
	EventSender,
) error {
	return nil
}

type testPlugin struct{}

func (testPlugin) Descriptor() Descriptor {
	return Descriptor{
		PluginID:       "dev.example.echo",
		PackageVersion: "1.0.0",
		DisplayName:    "Echo",
		Description:    "Test plugin",
		ConfigVersion:  1,
		ConfigSchema:   json.RawMessage("{}"),
	}
}

func (*testPlugin) ValidateConfig(
	context.Context,
	ConfigRequest,
) ([]Diagnostic, error) {
	return []Diagnostic{{
		Severity: DiagnosticError,
		Path:     "name",
		Code:     "required",
		Message:  "name is required",
	}}, nil
}

func (*testPlugin) InspectConfig(
	context.Context,
	ConfigRequest,
) (ConfigMetadata, error) {
	return ConfigMetadata{}, nil
}

func (*testPlugin) Open(
	context.Context,
	OpenRequest,
) (string, error) {
	return "session", nil
}

func (*testPlugin) ListTools(
	context.Context,
	ListToolsRequest,
) ([]Tool, error) {
	return nil, nil
}

func (*testPlugin) ExecuteTool(
	context.Context,
	ExecuteRequest,
	EventSender,
) error {
	return errors.New("not implemented")
}

func (*testPlugin) Close(context.Context, string) error {
	return nil
}
