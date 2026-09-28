package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

const (
	// ID is the stable built-in Model Tool Bridge Provider plugin identity.
	ID providerplugin.ID = "io.mender.provider.model-tools"

	// ConfigVersion is the current persisted Model Tool Bridge configuration version.
	ConfigVersion = 1

	pluginVersion   = "1.0.0"
	maxModelBytes   = 256
	maxCapabilities = 16
)

// Engine identifies one reviewed hosted-tool model API.
type Engine string

const (
	EngineOpenAI    Engine = "openai"
	EngineAnthropic Engine = "anthropic"
	EngineGemini    Engine = "gemini"
)

// Capability identifies one normalized Mender capability exposed by the bridge.
type Capability string

const (
	CapabilityWebSearch Capability = "web_search"
)

// Config is the non-secret persisted Model Tool Bridge Provider configuration.
type Config struct {
	Engine        Engine                     `json:"engine"`
	Model         string                     `json:"model"`
	CredentialRef credentialdomain.Reference `json:"credentialRef"`
	Capabilities  []Capability               `json:"capabilities"`
}

// Descriptor returns management-safe metadata for the built-in plugin.
func Descriptor() providerplugin.Descriptor {
	return providerplugin.Descriptor{
		ID:            ID,
		Version:       pluginVersion,
		DisplayName:   "Model Tools",
		Description:   "Bridge provider-hosted model tools into governed Mender Tools",
		ConfigVersion: ConfigVersion,
	}
}

// DecodeConfig strict-decodes and validates plugin-owned configuration.
func DecodeConfig(raw json.RawMessage) (Config, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()

	var config Config
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("decode Model Tools plugin config: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Config{}, fmt.Errorf("decode Model Tools plugin config: multiple JSON values")
		}
		return Config{}, fmt.Errorf("decode Model Tools plugin config: trailing data: %w", err)
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

// Validate checks the configuration without provider API access or Credential resolution.
func (c Config) Validate() error {
	switch c.Engine {
	case EngineOpenAI, EngineAnthropic, EngineGemini:
	default:
		return fmt.Errorf("model tools plugin config: unsupported engine %q", c.Engine)
	}

	model := strings.TrimSpace(c.Model)
	if model == "" {
		return fmt.Errorf("model tools plugin config: model must not be empty")
	}
	if model != c.Model {
		return fmt.Errorf("model tools plugin config: model must not contain surrounding whitespace")
	}
	if len(model) > maxModelBytes {
		return fmt.Errorf(
			"model tools plugin config: model must be at most %d bytes",
			maxModelBytes,
		)
	}

	if _, err := credentialdomain.ParseReference(c.CredentialRef.String()); err != nil {
		return fmt.Errorf("model tools plugin config: %w", err)
	}

	if len(c.Capabilities) == 0 {
		return fmt.Errorf("model tools plugin config: at least one capability is required")
	}
	if len(c.Capabilities) > maxCapabilities {
		return fmt.Errorf(
			"model tools plugin config: capabilities must contain at most %d values",
			maxCapabilities,
		)
	}

	seen := make(map[Capability]struct{}, len(c.Capabilities))
	for _, capability := range c.Capabilities {
		if capability != CapabilityWebSearch {
			return fmt.Errorf(
				"model tools plugin config: unsupported capability %q",
				capability,
			)
		}
		if _, exists := seen[capability]; exists {
			return fmt.Errorf(
				"model tools plugin config: duplicate capability %q",
				capability,
			)
		}
		seen[capability] = struct{}{}
	}
	return nil
}

// HasCapability reports whether a reviewed normalized capability is enabled.
func (c Config) HasCapability(capability Capability) bool {
	return slices.Contains(c.Capabilities, capability)
}

// ValidateConfig implements providerplugin.Plugin without network access.
func ValidateConfig(
	_ context.Context,
	raw json.RawMessage,
) error {
	_, err := DecodeConfig(raw)
	return err
}

// InspectConfig exposes bounded generic metadata without activating the Provider.
func InspectConfig(
	_ context.Context,
	raw json.RawMessage,
) (providerplugin.ConfigMetadata, error) {
	config, err := DecodeConfig(raw)
	if err != nil {
		return providerplugin.ConfigMetadata{}, err
	}
	return providerplugin.NewConfigMetadata(
		[]credentialdomain.Reference{config.CredentialRef},
	)
}
