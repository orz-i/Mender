package main

import (
	"context"
	"encoding/json"
	"testing"

	providerplugin "github.com/orz-i/Mender/sdk/providerplugin/go"
)

func TestEchoPluginLifecycleWithoutCredentials(t *testing.T) {
	t.Parallel()

	plugin := newEchoPlugin()
	config := json.RawMessage("{}")

	diagnostics, err := plugin.ValidateConfig(
		context.Background(),
		providerplugin.ConfigRequest{
			ConfigVersion: exampleConfigVersion,
			Config:        config,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("ValidateConfig() diagnostics = %+v", diagnostics)
	}

	sessionID, err := plugin.Open(
		context.Background(),
		providerplugin.OpenRequest{
			InstanceID:    "prov_example",
			ConfigVersion: exampleConfigVersion,
			Config:        config,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	tools, err := plugin.ListTools(
		context.Background(),
		providerplugin.ListToolsRequest{SessionID: sessionID},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 || tools[0].SourceKey != "echo" {
		t.Fatalf("ListTools() = %+v", tools)
	}

	sender := &testSender{}
	if err := plugin.ExecuteTool(
		context.Background(),
		providerplugin.ExecuteRequest{
			SessionID: sessionID,
			SourceKey: "echo",
			Arguments: json.RawMessage(`{"value":"hello"}`),
		},
		sender,
	); err != nil {
		t.Fatal(err)
	}
	if sender.progress != "echoing input" {
		t.Fatalf("progress = %q", sender.progress)
	}
	if string(sender.complete) != `{"value":"hello"}` {
		t.Fatalf("complete = %s", sender.complete)
	}
	if err := plugin.Close(context.Background(), sessionID); err != nil {
		t.Fatal(err)
	}
}

func TestEchoPluginCredentialAndPrefix(t *testing.T) {
	t.Parallel()

	plugin := newEchoPlugin()
	config := json.RawMessage(`{
		"prefix": "demo:",
		"credentialRef": "credential://example/token"
	}`)
	credential := providerplugin.Credential{
		Reference: "credential://example/token",
		Value:     []byte("secret"),
	}

	metadata, err := plugin.InspectConfig(
		context.Background(),
		providerplugin.ConfigRequest{
			ConfigVersion: exampleConfigVersion,
			Config:        config,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(metadata.CredentialRefs) != 1 ||
		metadata.CredentialRefs[0] != credential.Reference {
		t.Fatalf("InspectConfig() = %+v", metadata)
	}

	sessionID, err := plugin.Open(
		context.Background(),
		providerplugin.OpenRequest{
			InstanceID:           "prov_example",
			ConfigVersion:        exampleConfigVersion,
			Config:               config,
			DiscoveryCredentials: []providerplugin.Credential{credential},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	tools, err := plugin.ListTools(
		context.Background(),
		providerplugin.ListToolsRequest{
			SessionID:            sessionID,
			DiscoveryCredentials: []providerplugin.Credential{credential},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools[0].CredentialRefs) != 1 ||
		tools[0].CredentialRefs[0] != credential.Reference {
		t.Fatalf("CredentialRefs = %+v", tools[0].CredentialRefs)
	}

	sender := &testSender{}
	if err := plugin.ExecuteTool(
		context.Background(),
		providerplugin.ExecuteRequest{
			SessionID:   sessionID,
			SourceKey:   "echo",
			Arguments:   json.RawMessage(`{"value":"hello","count":2}`),
			Credentials: []providerplugin.Credential{credential},
		},
		sender,
	); err != nil {
		t.Fatal(err)
	}
	if string(sender.complete) != `{"count":2,"value":"demo:hello"}` {
		t.Fatalf("complete = %s", sender.complete)
	}
}

func TestEchoPluginRejectsUnknownConfigField(t *testing.T) {
	t.Parallel()

	plugin := newEchoPlugin()
	diagnostics, err := plugin.ValidateConfig(
		context.Background(),
		providerplugin.ConfigRequest{
			ConfigVersion: exampleConfigVersion,
			Config:        json.RawMessage(`{"unknown":true}`),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 1 ||
		diagnostics[0].Severity != providerplugin.DiagnosticError {
		t.Fatalf("diagnostics = %+v", diagnostics)
	}
}

type testSender struct {
	progress string
	complete json.RawMessage
}

func (s *testSender) Progress(message string) error {
	s.progress = message
	return nil
}

func (s *testSender) Complete(content json.RawMessage) error {
	s.complete = append(json.RawMessage(nil), content...)
	return nil
}

func (*testSender) InputRequired(
	string,
	string,
	json.RawMessage,
) error {
	return nil
}

func (*testSender) Task(string, string, string) error {
	return nil
}

func (*testSender) ToolFailure(string) error {
	return nil
}

func (*testSender) ProviderFailure(string) error {
	return nil
}
