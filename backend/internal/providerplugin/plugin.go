package providerplugin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
)

const (
	maxIDLength                   = 128
	maxVersionLength              = 128
	maxDisplayNameLength          = 160
	maxDescriptionLength          = 4096
	maxDescriptorSchemaBytes      = 1024 * 1024
	maxDescriptorCapabilities     = 64
	maxDescriptorCapabilityLength = 128
	maxConnectionSummaryBytes     = 4096
)

var idPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*(?:\.[a-z][a-z0-9-]*)+$`)

// ID is the stable identity of one Provider plugin implementation.
//
// IDs are namespaced and intentionally are not normalized silently.
type ID string

// ParseID validates and returns a Provider plugin ID.
func ParseID(value string) (ID, error) {
	if value == "" {
		return "", fmt.Errorf("provider plugin id: must not be empty")
	}
	if len(value) > maxIDLength {
		return "", fmt.Errorf("provider plugin id: must be at most %d characters", maxIDLength)
	}
	if value != strings.TrimSpace(value) {
		return "", fmt.Errorf("provider plugin id: must not contain surrounding whitespace")
	}
	if !idPattern.MatchString(value) {
		return "", fmt.Errorf(
			"provider plugin id: must be a lowercase namespaced identifier such as io.mender.provider.mcp",
		)
	}
	return ID(value), nil
}

func (id ID) String() string {
	return string(id)
}

// Descriptor is safe Provider plugin metadata exposed by Runtime/management.
//
// Reading a Descriptor must not activate a Provider or perform network/process
// work.
type Descriptor struct {
	ID              ID
	Version         string
	DisplayName     string
	Description     string
	ConfigVersion   int
	ProtocolVersion uint32
	ConfigSchema    json.RawMessage
	UISchema        json.RawMessage
	Capabilities    []string
}

// Validate checks descriptor invariants.
func (d Descriptor) Validate() error {
	if _, err := ParseID(d.ID.String()); err != nil {
		return err
	}
	if value := strings.TrimSpace(d.Version); value == "" {
		return fmt.Errorf("provider plugin version: must not be empty")
	} else if len(value) > maxVersionLength {
		return fmt.Errorf(
			"provider plugin version: must be at most %d characters",
			maxVersionLength,
		)
	}
	if value := strings.TrimSpace(d.DisplayName); value == "" {
		return fmt.Errorf("provider plugin display name: must not be empty")
	} else if len(value) > maxDisplayNameLength {
		return fmt.Errorf(
			"provider plugin display name: must be at most %d characters",
			maxDisplayNameLength,
		)
	}
	if len(strings.TrimSpace(d.Description)) > maxDescriptionLength {
		return fmt.Errorf(
			"provider plugin description: must be at most %d characters",
			maxDescriptionLength,
		)
	}
	if d.ConfigVersion < 1 {
		return fmt.Errorf("provider plugin config version: must be at least 1")
	}
	if len(d.ConfigSchema) > maxDescriptorSchemaBytes {
		return fmt.Errorf(
			"provider plugin config schema: must be at most %d bytes",
			maxDescriptorSchemaBytes,
		)
	}
	if len(d.ConfigSchema) != 0 {
		if _, err := catalogdomain.NewSchema(d.ConfigSchema); err != nil {
			return fmt.Errorf("provider plugin config schema: %w", err)
		}
	}
	if len(d.UISchema) > maxDescriptorSchemaBytes {
		return fmt.Errorf(
			"provider plugin UI schema: must be at most %d bytes",
			maxDescriptorSchemaBytes,
		)
	}
	if len(d.UISchema) != 0 && !json.Valid(d.UISchema) {
		return fmt.Errorf("provider plugin UI schema: must be valid JSON")
	}
	if len(d.Capabilities) > maxDescriptorCapabilities {
		return fmt.Errorf(
			"provider plugin capabilities: must contain at most %d entries",
			maxDescriptorCapabilities,
		)
	}
	seenCapabilities := make(map[string]struct{}, len(d.Capabilities))
	for _, rawCapability := range d.Capabilities {
		capability := strings.TrimSpace(rawCapability)
		if capability == "" {
			return fmt.Errorf("provider plugin capability: must not be empty")
		}
		if len(capability) > maxDescriptorCapabilityLength {
			return fmt.Errorf(
				"provider plugin capability: must be at most %d characters",
				maxDescriptorCapabilityLength,
			)
		}
		if _, duplicate := seenCapabilities[capability]; duplicate {
			return fmt.Errorf("provider plugin capability: duplicate %q", capability)
		}
		seenCapabilities[capability] = struct{}{}
	}
	return nil
}

// Clone returns a deep copy of descriptor metadata safe for management surfaces.
func (d Descriptor) Clone() Descriptor {
	d.ConfigSchema = append(json.RawMessage(nil), d.ConfigSchema...)
	d.UISchema = append(json.RawMessage(nil), d.UISchema...)
	d.Capabilities = append([]string(nil), d.Capabilities...)
	return d
}

// OpenRequest contains the Mender-owned capabilities needed to open one
// configured Provider instance.
//
// Config is plugin-owned non-secret persisted configuration. Credentials is a
// capability, not resolved secret material.
type OpenRequest struct {
	ProviderID    catalogdomain.ProviderID
	ConfigVersion int
	Config        json.RawMessage
	Credentials   invocationapplication.CredentialResolver
}

// ConfigMetadata is bounded platform-governed metadata extracted from
// plugin-owned configuration without activating the Provider.
type ConfigMetadata struct {
	CredentialRefs    []credentialdomain.Reference
	ConnectionSummary string
}

// NewConfigMetadata validates, de-duplicates, sorts, and clones Credential
// Reference metadata returned across the plugin boundary.
func NewConfigMetadata(
	refs []credentialdomain.Reference,
) (ConfigMetadata, error) {
	return NewConfigMetadataWithSummary(refs, "")
}

// NewConfigMetadataWithSummary additionally validates bounded management-safe
// connection summary metadata.
func NewConfigMetadataWithSummary(
	refs []credentialdomain.Reference,
	connectionSummary string,
) (ConfigMetadata, error) {
	result := make([]credentialdomain.Reference, 0, len(refs))
	seen := make(map[credentialdomain.Reference]struct{}, len(refs))
	for _, ref := range refs {
		parsed, err := credentialdomain.ParseReference(ref.String())
		if err != nil {
			return ConfigMetadata{}, fmt.Errorf("provider plugin config metadata: %w", err)
		}
		if _, exists := seen[parsed]; exists {
			continue
		}
		seen[parsed] = struct{}{}
		result = append(result, parsed)
	}
	slices.SortFunc(result, func(a, b credentialdomain.Reference) int {
		return strings.Compare(a.String(), b.String())
	})
	if !utf8.ValidString(connectionSummary) {
		return ConfigMetadata{}, fmt.Errorf(
			"provider plugin config metadata: connection summary must be valid UTF-8",
		)
	}
	if len(connectionSummary) > maxConnectionSummaryBytes {
		return ConfigMetadata{}, fmt.Errorf(
			"provider plugin config metadata: connection summary must be at most %d bytes",
			maxConnectionSummaryBytes,
		)
	}
	return ConfigMetadata{
		CredentialRefs:    result,
		ConnectionSummary: connectionSummary,
	}, nil
}

// Clone returns a defensive copy of normalized config metadata.
func (m ConfigMetadata) Clone() ConfigMetadata {
	m.CredentialRefs = append([]credentialdomain.Reference(nil), m.CredentialRefs...)
	return m
}

// Plugin validates/inspects configuration and opens runtime Provider
// instances.
type Plugin interface {
	Descriptor() Descriptor
	ValidateConfig(context.Context, json.RawMessage) error
	InspectConfig(context.Context, json.RawMessage) (ConfigMetadata, error)
	Open(context.Context, OpenRequest) (Instance, error)
}

// GovernanceFactProvider is an optional, host-reviewed built-in plugin capability
// for deriving bounded Policy facts from persisted non-secret Provider config and
// validated Tool arguments without activating the Provider.
//
// External plugin protocol implementations are intentionally not trusted as
// GovernanceFactProvider inputs merely because they are installable Providers.
type GovernanceFactProvider interface {
	ExtractGovernanceFacts(
		context.Context,
		json.RawMessage,
		catalogdomain.Tool,
		invocationdomain.JSONValue,
	) (invocationdomain.GovernanceFacts, error)
}

// Instance is one active Provider implementation behind canonical Mender ports.
type Instance interface {
	catalogapplication.ToolSource
	invocationapplication.ToolExecutor
	io.Closer
}

// ReusableInstance optionally reports whether an active Instance can still be
// reused for the same persisted Provider configuration. Manager preserves the
// historical reuse behavior for Instances that do not implement this capability.
//
// Reusable must be a local, side-effect-free liveness check. It must not issue
// network requests, launch processes, or perform plugin RPCs.
type ReusableInstance interface {
	Reusable() bool
}
