package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"

	providerplugin "github.com/orz-i/Mender/sdk/providerplugin/go"
)

const (
	examplePluginID       = "dev.mender.example.echo"
	examplePackageVersion = "0.1.0"
	exampleConfigVersion  = 1
)

var (
	exampleConfigSchema = json.RawMessage(`{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type": "object",
		"additionalProperties": false,
		"properties": {
			"prefix": {
				"type": "string",
				"title": "Prefix",
				"description": "Optional string prepended to string values before they are returned."
			},
			"credentialRef": {
				"type": "string",
				"title": "Credential",
				"description": "Optional inventory Credential proving host-scoped secret handoff.",
				"x-mender-control": "credential"
			}
		}
	}`)
	exampleInputSchema = json.RawMessage(`{
		"type": "object",
		"additionalProperties": true
	}`)
	exampleOutputSchema = json.RawMessage(`{
		"type": "object",
		"additionalProperties": true
	}`)
)

type echoConfig struct {
	Prefix        string `json:"prefix,omitempty"`
	CredentialRef string `json:"credentialRef,omitempty"`
}

type echoSession struct {
	config echoConfig
}

type echoPlugin struct {
	mu       sync.RWMutex
	sessions map[string]echoSession
}

func newEchoPlugin() *echoPlugin {
	return &echoPlugin{sessions: make(map[string]echoSession)}
}

func (*echoPlugin) Descriptor() providerplugin.Descriptor {
	return providerplugin.Descriptor{
		PluginID:       examplePluginID,
		PackageVersion: examplePackageVersion,
		DisplayName:    "Example Echo",
		Description:    "Standalone external Provider plugin built with the public Mender Go SDK.",
		ConfigVersion:  exampleConfigVersion,
		ConfigSchema:   append(json.RawMessage(nil), exampleConfigSchema...),
		Capabilities:   []string{"progress"},
	}
}

func (*echoPlugin) ValidateConfig(
	_ context.Context,
	request providerplugin.ConfigRequest,
) ([]providerplugin.Diagnostic, error) {
	if request.ConfigVersion != exampleConfigVersion {
		return []providerplugin.Diagnostic{{
			Severity: providerplugin.DiagnosticError,
			Path:     "configVersion",
			Code:     "unsupported",
			Message:  "unsupported configuration version",
		}}, nil
	}
	config, err := decodeEchoConfig(request.Config)
	if err != nil {
		return []providerplugin.Diagnostic{{
			Severity: providerplugin.DiagnosticError,
			Path:     "$",
			Code:     "invalid_config",
			Message:  err.Error(),
		}}, nil
	}
	if config.CredentialRef != "" && !validCredentialReference(config.CredentialRef) {
		return []providerplugin.Diagnostic{{
			Severity: providerplugin.DiagnosticError,
			Path:     "credentialRef",
			Code:     "invalid_credential_ref",
			Message:  "credentialRef must use credential://namespace/name",
		}}, nil
	}
	return nil, nil
}

func (*echoPlugin) InspectConfig(
	_ context.Context,
	request providerplugin.ConfigRequest,
) (providerplugin.ConfigMetadata, error) {
	config, err := decodeEchoConfig(request.Config)
	if err != nil {
		return providerplugin.ConfigMetadata{}, err
	}
	refs := make([]string, 0, 1)
	if config.CredentialRef != "" {
		refs = append(refs, config.CredentialRef)
	}
	return providerplugin.ConfigMetadata{
		CredentialRefs:    refs,
		ConnectionSummary: "in-process echo example",
	}, nil
}

func (p *echoPlugin) Open(
	_ context.Context,
	request providerplugin.OpenRequest,
) (string, error) {
	config, err := decodeEchoConfig(request.Config)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(request.InstanceID) == "" {
		return "", fmt.Errorf("instance id is required")
	}
	if config.CredentialRef != "" {
		value, ok := providerplugin.FindCredential(
			request.DiscoveryCredentials,
			config.CredentialRef,
		)
		if !ok || len(value) == 0 {
			return "", fmt.Errorf(
				"discovery credential %s is required",
				config.CredentialRef,
			)
		}
		clear(value)
	}
	sessionID := "echo:" + request.InstanceID
	p.mu.Lock()
	p.sessions[sessionID] = echoSession{config: config}
	p.mu.Unlock()
	return sessionID, nil
}

func (p *echoPlugin) ListTools(
	_ context.Context,
	request providerplugin.ListToolsRequest,
) ([]providerplugin.Tool, error) {
	session, ok := p.session(request.SessionID)
	if !ok {
		return nil, fmt.Errorf("session %q was not found", request.SessionID)
	}
	if session.config.CredentialRef != "" {
		value, exists := providerplugin.FindCredential(
			request.DiscoveryCredentials,
			session.config.CredentialRef,
		)
		if !exists || len(value) == 0 {
			return nil, fmt.Errorf(
				"discovery credential %s is required",
				session.config.CredentialRef,
			)
		}
		clear(value)
	}
	refs := make([]string, 0, 1)
	if session.config.CredentialRef != "" {
		refs = append(refs, session.config.CredentialRef)
	}
	return []providerplugin.Tool{{
		SourceKey:      "echo",
		Name:           "echo",
		Title:          "Echo",
		Description:    "Return the supplied JSON object, optionally prefixing string values.",
		InputSchema:    append(json.RawMessage(nil), exampleInputSchema...),
		OutputSchema:   append(json.RawMessage(nil), exampleOutputSchema...),
		Tags:           []string{"example", "echo"},
		CredentialRefs: refs,
	}}, nil
}

func (p *echoPlugin) ExecuteTool(
	_ context.Context,
	request providerplugin.ExecuteRequest,
	sender providerplugin.EventSender,
) error {
	session, ok := p.session(request.SessionID)
	if !ok {
		return fmt.Errorf("session %q was not found", request.SessionID)
	}
	if request.SourceKey != "echo" {
		return sender.ToolFailure("unknown Tool SourceKey")
	}
	if session.config.CredentialRef != "" {
		value, exists := providerplugin.FindCredential(
			request.Credentials,
			session.config.CredentialRef,
		)
		if !exists || len(value) == 0 {
			return sender.ProviderFailure("required execution Credential is missing")
		}
		clear(value)
	}
	if err := sender.Progress("echoing input"); err != nil {
		return err
	}

	var input map[string]any
	if err := json.Unmarshal(request.Arguments, &input); err != nil {
		return sender.ToolFailure("arguments must be a JSON object")
	}
	if session.config.Prefix != "" {
		for name, value := range input {
			if text, ok := value.(string); ok {
				input[name] = session.config.Prefix + text
			}
		}
	}
	output, err := json.Marshal(input)
	if err != nil {
		return err
	}
	return sender.Complete(output)
}

func (p *echoPlugin) Close(
	_ context.Context,
	sessionID string,
) error {
	p.mu.Lock()
	delete(p.sessions, sessionID)
	p.mu.Unlock()
	return nil
}

func (p *echoPlugin) session(sessionID string) (echoSession, bool) {
	p.mu.RLock()
	session, ok := p.sessions[sessionID]
	p.mu.RUnlock()
	return session, ok
}

func decodeEchoConfig(raw json.RawMessage) (echoConfig, error) {
	if len(raw) == 0 {
		raw = json.RawMessage("{}")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()

	var config echoConfig
	if err := decoder.Decode(&config); err != nil {
		return echoConfig{}, fmt.Errorf("decode configuration: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return echoConfig{}, fmt.Errorf("configuration must contain one JSON document")
		}
		return echoConfig{}, fmt.Errorf("decode trailing configuration: %w", err)
	}
	config.Prefix = strings.TrimSpace(config.Prefix)
	config.CredentialRef = strings.TrimSpace(config.CredentialRef)
	return config, nil
}

func validCredentialReference(value string) bool {
	if !strings.HasPrefix(value, "credential://") {
		return false
	}
	remainder := strings.TrimPrefix(value, "credential://")
	namespace, name, ok := strings.Cut(remainder, "/")
	return ok &&
		strings.TrimSpace(namespace) != "" &&
		strings.TrimSpace(name) != "" &&
		!strings.Contains(name, "/")
}
