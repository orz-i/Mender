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
	pluginID       = "dev.mender.example.echo"
	packageVersion = "0.1.0"
	configVersion  = 1
)

var configSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "prefix": {
      "type": "string",
      "title": "Prefix",
      "default": "echo: "
    },
    "title": {
      "type": "string",
      "title": "Tool title",
      "default": "Echo"
    },
    "credentialRef": {
      "type": "string",
      "title": "Credential",
      "description": "Optional Credential Reference used to demonstrate scoped secret handoff.",
      "x-mender-control": "credential"
    }
  }
}`)

type echoConfig struct {
	Prefix        string `json:"prefix"`
	Title         string `json:"title"`
	CredentialRef string `json:"credentialRef"`
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
		PluginID:       pluginID,
		PackageVersion: packageVersion,
		DisplayName:    "Example Echo",
		Description:    "Example installable Mender Provider plugin built with the public Go SDK.",
		ConfigVersion:  configVersion,
		ConfigSchema:   append(json.RawMessage(nil), configSchema...),
		Capabilities:   []string{"example", "progress"},
	}
}

func (*echoPlugin) ValidateConfig(
	_ context.Context,
	request providerplugin.ConfigRequest,
) ([]providerplugin.Diagnostic, error) {
	if request.ConfigVersion != configVersion {
		return []providerplugin.Diagnostic{{
			Severity: providerplugin.DiagnosticError,
			Path:     "$",
			Code:     "unsupported_config_version",
			Message:  "unsupported config version",
		}}, nil
	}
	config, err := parseConfig(request.Config)
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
	config, err := parseConfig(request.Config)
	if err != nil {
		return providerplugin.ConfigMetadata{}, err
	}
	refs := make([]string, 0, 1)
	if config.CredentialRef != "" {
		refs = append(refs, config.CredentialRef)
	}
	return providerplugin.ConfigMetadata{
		CredentialRefs:    refs,
		ConnectionSummary: "local example echo plugin",
	}, nil
}

func (p *echoPlugin) Open(
	_ context.Context,
	request providerplugin.OpenRequest,
) (string, error) {
	config, err := parseConfig(request.Config)
	if err != nil {
		return "", err
	}
	if config.CredentialRef != "" {
		secret, ok := providerplugin.FindCredential(
			request.DiscoveryCredentials,
			config.CredentialRef,
		)
		if !ok || len(secret) == 0 {
			return "", fmt.Errorf("required discovery credential is unavailable")
		}
		clear(secret)
	}

	sessionID := "example:" + request.InstanceID
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
		return nil, fmt.Errorf("session not found")
	}
	if session.config.CredentialRef != "" {
		secret, found := providerplugin.FindCredential(
			request.DiscoveryCredentials,
			session.config.CredentialRef,
		)
		if !found || len(secret) == 0 {
			return nil, fmt.Errorf("required discovery credential is unavailable")
		}
		clear(secret)
	}

	refs := make([]string, 0, 1)
	if session.config.CredentialRef != "" {
		refs = append(refs, session.config.CredentialRef)
	}
	title := session.config.Title
	if title == "" {
		title = "Echo"
	}
	return []providerplugin.Tool{{
		SourceKey:   "echo",
		Name:        "echo",
		Title:       title,
		Description: "Return the provided value with the configured prefix.",
		InputSchema: json.RawMessage(`{
		  "type": "object",
		  "required": ["value"],
		  "additionalProperties": false,
		  "properties": {
		    "value": {}
		  }
		}`),
		OutputSchema: json.RawMessage(`{
		  "type": "object",
		  "required": ["echo"],
		  "additionalProperties": false,
		  "properties": {
		    "echo": {}
		  }
		}`),
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
		return fmt.Errorf("session not found")
	}
	if request.SourceKey != "echo" {
		return sender.ToolFailure("unknown tool")
	}
	if session.config.CredentialRef != "" {
		secret, found := providerplugin.FindCredential(
			request.Credentials,
			session.config.CredentialRef,
		)
		if !found || len(secret) == 0 {
			return sender.ProviderFailure("required execution credential is unavailable")
		}
		clear(secret)
	}

	var arguments struct {
		Value any `json:"value"`
	}
	if err := json.Unmarshal(request.Arguments, &arguments); err != nil {
		return sender.ToolFailure("arguments must be valid JSON")
	}
	if err := sender.Progress("echoing value"); err != nil {
		return err
	}
	content, err := json.Marshal(map[string]any{
		"echo": session.config.Prefix + fmt.Sprint(arguments.Value),
	})
	if err != nil {
		return err
	}
	return sender.Complete(content)
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

func parseConfig(raw json.RawMessage) (echoConfig, error) {
	if len(raw) == 0 {
		raw = json.RawMessage("{}")
	}
	var config echoConfig
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return echoConfig{}, fmt.Errorf("decode config: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return echoConfig{}, fmt.Errorf("config must contain one JSON document")
		}
		return echoConfig{}, fmt.Errorf("decode trailing config: %w", err)
	}
	config.Title = strings.TrimSpace(config.Title)
	config.CredentialRef = strings.TrimSpace(config.CredentialRef)
	if config.Prefix == "" {
		config.Prefix = "echo: "
	}
	if config.Title == "" {
		config.Title = "Echo"
	}
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
