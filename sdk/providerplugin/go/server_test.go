package providerplugin

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	providerv1 "github.com/orz-i/Mender/sdk/providerplugin/go/protocol/v1"
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

func TestValidateDescriptorRejectsInvalidSchema(t *testing.T) {
	t.Parallel()

	descriptor := testPlugin{}.Descriptor()
	descriptor.ConfigSchema = json.RawMessage("{")
	if err := validateDescriptor(descriptor); err == nil {
		t.Fatal("validateDescriptor() error = nil")
	}
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
