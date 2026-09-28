package main

import (
	"context"
	"encoding/json"
	"testing"

	providerplugin "github.com/orz-i/Mender/sdk/providerplugin/go"
)

func TestEchoPluginLifecycle(t *testing.T) {
	t.Parallel()

	plugin := newEchoPlugin()
	config := json.RawMessage(
		`{"prefix":"test: ","credentialRef":"credential://example/token"}`,
	)

	diagnostics, err := plugin.ValidateConfig(
		context.Background(),
		providerplugin.ConfigRequest{
			ConfigVersion: configVersion,
			Config:        config,
		},
	)
	if err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("ValidateConfig() diagnostics = %+v", diagnostics)
	}

	metadata, err := plugin.InspectConfig(
		context.Background(),
		providerplugin.ConfigRequest{
			ConfigVersion: configVersion,
			Config:        config,
		},
	)
	if err != nil {
		t.Fatalf("InspectConfig() error = %v", err)
	}
	if len(metadata.CredentialRefs) != 1 ||
		metadata.CredentialRefs[0] != "credential://example/token" {
		t.Fatalf("InspectConfig() = %+v", metadata)
	}

	sessionID, err := plugin.Open(
		context.Background(),
		providerplugin.OpenRequest{
			InstanceID:    "instance-1",
			ConfigVersion: configVersion,
			Config:        config,
			DiscoveryCredentials: []providerplugin.Credential{{
				Reference: "credential://example/token",
				Value:     []byte("secret"),
			}},
		},
	)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	tools, err := plugin.ListTools(
		context.Background(),
		providerplugin.ListToolsRequest{
			SessionID: sessionID,
			DiscoveryCredentials: []providerplugin.Credential{{
				Reference: "credential://example/token",
				Value:     []byte("secret"),
			}},
		},
	)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(tools) != 1 ||
		tools[0].SourceKey != "echo" ||
		len(tools[0].CredentialRefs) != 1 {
		t.Fatalf("ListTools() = %+v", tools)
	}

	sender := &testEventSender{}
	err = plugin.ExecuteTool(
		context.Background(),
		providerplugin.ExecuteRequest{
			SessionID:    sessionID,
			InvocationID: "invocation-1",
			SourceKey:    "echo",
			Arguments:    json.RawMessage(`{"value":"hello"}`),
			Credentials: []providerplugin.Credential{{
				Reference: "credential://example/token",
				Value:     []byte("secret"),
			}},
		},
		sender,
	)
	if err != nil {
		t.Fatalf("ExecuteTool() error = %v", err)
	}
	if len(sender.progress) != 1 || sender.progress[0] != "echoing value" {
		t.Fatalf("progress = %#v", sender.progress)
	}
	if string(sender.complete) != `{"echo":"test: hello"}` {
		t.Fatalf("complete = %s", sender.complete)
	}

	if err := plugin.Close(context.Background(), sessionID); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := plugin.ListTools(
		context.Background(),
		providerplugin.ListToolsRequest{SessionID: sessionID},
	); err == nil {
		t.Fatal("ListTools() after Close error = nil")
	}
}

func TestEchoPluginValidationRejectsInvalidCredentialReference(t *testing.T) {
	t.Parallel()

	plugin := newEchoPlugin()
	diagnostics, err := plugin.ValidateConfig(
		context.Background(),
		providerplugin.ConfigRequest{
			ConfigVersion: configVersion,
			Config: json.RawMessage(
				`{"credentialRef":"not-a-credential-reference"}`,
			),
		},
	)
	if err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}
	if len(diagnostics) != 1 ||
		diagnostics[0].Code != "invalid_credential_ref" {
		t.Fatalf("diagnostics = %+v", diagnostics)
	}
}

func TestEchoPluginValidationRejectsTrailingConfigDocument(t *testing.T) {
	t.Parallel()

	plugin := newEchoPlugin()
	diagnostics, err := plugin.ValidateConfig(
		context.Background(),
		providerplugin.ConfigRequest{
			ConfigVersion: configVersion,
			Config:        json.RawMessage(`{} {}`),
		},
	)
	if err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}
	if len(diagnostics) != 1 ||
		diagnostics[0].Severity != providerplugin.DiagnosticError {
		t.Fatalf("diagnostics = %+v", diagnostics)
	}
}

func TestEchoPluginValidationRejectsUnknownConfig(t *testing.T) {
	t.Parallel()

	plugin := newEchoPlugin()
	diagnostics, err := plugin.ValidateConfig(
		context.Background(),
		providerplugin.ConfigRequest{
			ConfigVersion: configVersion,
			Config:        json.RawMessage(`{"unknown":true}`),
		},
	)
	if err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}
	if len(diagnostics) != 1 ||
		diagnostics[0].Severity != providerplugin.DiagnosticError {
		t.Fatalf("diagnostics = %+v", diagnostics)
	}
}

type testEventSender struct {
	progress []string
	complete json.RawMessage
}

func (s *testEventSender) Progress(message string) error {
	s.progress = append(s.progress, message)
	return nil
}

func (s *testEventSender) Complete(content json.RawMessage) error {
	s.complete = append(json.RawMessage(nil), content...)
	return nil
}

func (*testEventSender) InputRequired(
	string,
	string,
	json.RawMessage,
) error {
	return nil
}

func (*testEventSender) Task(string, string, string) error {
	return nil
}

func (*testEventSender) ToolFailure(string) error {
	return nil
}

func (*testEventSender) ProviderFailure(string) error {
	return nil
}
