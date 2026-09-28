package providerplugin

import (
	"context"
	"encoding/json"
)

// Descriptor describes one external Provider plugin package implementation.
type Descriptor struct {
	PluginID       string
	PackageVersion string
	DisplayName    string
	Description    string
	ConfigVersion  uint32
	ConfigSchema   json.RawMessage
	UISchema       json.RawMessage
	Capabilities   []string
}

// ConfigRequest carries plugin-owned persisted configuration.
type ConfigRequest struct {
	ConfigVersion uint32
	Config        json.RawMessage
}

// DiagnosticSeverity classifies configuration diagnostics.
type DiagnosticSeverity uint8

const (
	DiagnosticError DiagnosticSeverity = iota + 1
	DiagnosticWarning
)

// Diagnostic is one bounded plugin-owned configuration diagnostic.
type Diagnostic struct {
	Severity DiagnosticSeverity
	Path     string
	Code     string
	Message  string
}

// ConfigMetadata is non-secret metadata extracted from Provider configuration.
type ConfigMetadata struct {
	CredentialRefs    []string
	ConnectionSummary string
}

// Credential is ephemeral secret material scoped by the Mender host.
type Credential struct {
	Reference string
	Value     []byte
}

// OpenRequest opens one configured Provider session.
type OpenRequest struct {
	InstanceID           string
	ConfigVersion        uint32
	Config               json.RawMessage
	DiscoveryCredentials []Credential
}

// ListToolsRequest discovers tools for one open Provider session.
type ListToolsRequest struct {
	SessionID            string
	DiscoveryCredentials []Credential
}

// Tool describes one plugin-owned Tool before Mender assigns canonical ToolID.
type Tool struct {
	SourceKey      string
	Name           string
	Title          string
	Description    string
	InputSchema    json.RawMessage
	OutputSchema   json.RawMessage
	Tags           []string
	CredentialRefs []string
}

// ExecuteRequest executes one discovered Tool inside an open session.
type ExecuteRequest struct {
	SessionID    string
	InvocationID string
	SourceKey    string
	Arguments    json.RawMessage
	Credentials  []Credential
}

// EventSender emits bounded progress and exactly one terminal outcome.
type EventSender interface {
	Progress(message string) error
	Complete(content json.RawMessage) error
	InputRequired(requestID, message string, inputSchema json.RawMessage) error
	Task(taskID, status, message string) error
	ToolFailure(message string) error
	ProviderFailure(message string) error
}

// Plugin is the public Go SDK contract for one Provider plugin implementation.
type Plugin interface {
	Descriptor() Descriptor
	ValidateConfig(context.Context, ConfigRequest) ([]Diagnostic, error)
	InspectConfig(context.Context, ConfigRequest) (ConfigMetadata, error)
	Open(context.Context, OpenRequest) (sessionID string, err error)
	ListTools(context.Context, ListToolsRequest) ([]Tool, error)
	ExecuteTool(context.Context, ExecuteRequest, EventSender) error
	Close(context.Context, string) error
}

// FindCredential returns a copy of one credential value by reference.
func FindCredential(credentials []Credential, reference string) ([]byte, bool) {
	for _, credential := range credentials {
		if credential.Reference == reference {
			return append([]byte(nil), credential.Value...), true
		}
	}
	return nil, false
}

// ZeroCredentials best-effort clears credential values owned by the caller.
func ZeroCredentials(credentials []Credential) {
	for len(credentials) != 0 {
		clear(credentials[0].Value)
		credentials[0].Value = nil
		credentials = credentials[1:]
	}
}
