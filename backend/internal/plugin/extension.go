package plugin

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// Kind defines a host-owned contract, not a user-supplied privilege.
type Kind string

const (
	ProviderTools   Kind = "provider.tools"
	SandboxBackend  Kind = "sandbox.backend"
	TunnelTransport Kind = "tunnel.transport"
	AgentBackend    Kind = "agent.backend"
)

func (k Kind) Validate() error {
	switch k {
	case ProviderTools, SandboxBackend, TunnelTransport, AgentBackend:
		return nil
	default:
		return fmt.Errorf("plugin extension kind: unsupported %q", k)
	}
}

// Extension declares the contract of a package contribution.
// ProtocolVersion belongs to this extension kind, never to the whole package.
type Extension struct {
	ID              ExtensionID `json:"id"`
	Kind            Kind        `json:"kind"`
	ProtocolVersion uint32      `json:"protocolVersion"`
}

func (e Extension) Validate() error {
	if _, err := ParseExtensionID(e.ID.String()); err != nil {
		return err
	}
	if err := e.Kind.Validate(); err != nil {
		return err
	}
	if e.ProtocolVersion < 1 {
		return fmt.Errorf("plugin extension protocol version: must be positive")
	}
	return nil
}

// Descriptor is bounded, process-free, management-safe data.
// A claim of sandbox isolation or tunnel readiness is not proof of enforcement.
type Descriptor struct {
	PluginID       ID
	PackageVersion string
	Extension      Extension
	DisplayName    string
	Description    string
	ConfigVersion  uint32
	ConfigSchema   json.RawMessage
	UISchema       json.RawMessage
	Capabilities   []string
}

func (d Descriptor) Validate() error {
	if _, err := ParseID(d.PluginID.String()); err != nil {
		return err
	}
	if err := d.Extension.Validate(); err != nil {
		return err
	}
	if d.PackageVersion == "" || len(d.PackageVersion) > 128 || d.PackageVersion != strings.TrimSpace(d.PackageVersion) {
		return fmt.Errorf("plugin package version: invalid")
	}
	if d.DisplayName == "" || !utf8.ValidString(d.DisplayName) || len(d.DisplayName) > 160 || d.DisplayName != strings.TrimSpace(d.DisplayName) {
		return fmt.Errorf("plugin descriptor: invalid display name")
	}
	if len(d.Description) > 4096 || !utf8.ValidString(d.Description) {
		return fmt.Errorf("plugin descriptor: invalid description")
	}
	if d.ConfigVersion == 0 {
		return fmt.Errorf("plugin descriptor: config version must be positive")
	}
	for _, schema := range []json.RawMessage{d.ConfigSchema, d.UISchema} {
		if len(schema) > 1024*1024 || (len(schema) != 0 && !json.Valid(schema)) {
			return fmt.Errorf("plugin descriptor: invalid or oversized schema")
		}
	}
	if len(d.Capabilities) > 64 {
		return fmt.Errorf("plugin descriptor: too many capabilities")
	}
	seen := map[string]bool{}
	for _, c := range d.Capabilities {
		if c == "" || c != strings.TrimSpace(c) || len(c) > 128 || !utf8.ValidString(c) || seen[c] {
			return fmt.Errorf("plugin descriptor: invalid or duplicate capability %q", c)
		}
		seen[c] = true
	}
	return nil
}

func (d Descriptor) Clone() Descriptor {
	d.ConfigSchema = slices.Clone(d.ConfigSchema)
	d.UISchema = slices.Clone(d.UISchema)
	d.Capabilities = slices.Clone(d.Capabilities)
	return d
}
