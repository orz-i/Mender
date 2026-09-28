package providerplugin

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
)

const (
	maxConfigBytes    = 4 * 1024 * 1024
	maxRevisionLength = 256
)

// Configuration binds one canonical Provider to a Provider plugin and an
// opaque plugin-owned configuration revision.
type Configuration struct {
	ProviderID     catalogdomain.ProviderID
	PluginID       ID
	InstallationID *InstallationID
	ConfigVersion  int
	Revision       string
	Config         json.RawMessage
}

// NewConfiguration constructs a validated configuration with a deterministic
// revision derived from the plugin-owned JSON bytes.
func NewConfiguration(
	providerID catalogdomain.ProviderID,
	pluginID ID,
	configVersion int,
	raw json.RawMessage,
) (Configuration, error) {
	return newConfiguration(providerID, pluginID, nil, configVersion, raw)
}

// NewInstalledConfiguration binds one Provider configuration to an exact
// external plugin installation.
func NewInstalledConfiguration(
	providerID catalogdomain.ProviderID,
	pluginID ID,
	installationID InstallationID,
	configVersion int,
	raw json.RawMessage,
) (Configuration, error) {
	id := installationID
	return newConfiguration(providerID, pluginID, &id, configVersion, raw)
}

func newConfiguration(
	providerID catalogdomain.ProviderID,
	pluginID ID,
	installationID *InstallationID,
	configVersion int,
	raw json.RawMessage,
) (Configuration, error) {
	config := cloneConfig(raw)
	sum := sha256.Sum256(config)
	result := Configuration{
		ProviderID:     providerID,
		PluginID:       pluginID,
		InstallationID: cloneInstallationID(installationID),
		ConfigVersion:  configVersion,
		Revision:       "sha256:" + hex.EncodeToString(sum[:]),
		Config:         config,
	}
	if err := result.Validate(); err != nil {
		return Configuration{}, err
	}
	return result, nil
}

// Validate checks the generic Provider configuration envelope.
//
// Plugin-specific configuration validation is delegated to Plugin.
func (c Configuration) Validate() error {
	if _, err := catalogdomain.ParseProviderID(c.ProviderID.String()); err != nil {
		return fmt.Errorf("provider configuration: %w", err)
	}
	if _, err := ParseID(c.PluginID.String()); err != nil {
		return fmt.Errorf("provider configuration: %w", err)
	}
	if c.InstallationID != nil {
		if _, err := ParseInstallationID(c.InstallationID.String()); err != nil {
			return fmt.Errorf("provider configuration: %w", err)
		}
	}
	if c.ConfigVersion < 1 {
		return fmt.Errorf("provider configuration: config version must be at least 1")
	}
	if c.Revision == "" {
		return fmt.Errorf("provider configuration: revision must not be empty")
	}
	if c.Revision != strings.TrimSpace(c.Revision) {
		return fmt.Errorf("provider configuration: revision must not contain surrounding whitespace")
	}
	if len(c.Revision) > maxRevisionLength {
		return fmt.Errorf(
			"provider configuration: revision must be at most %d characters",
			maxRevisionLength,
		)
	}
	if len(c.Config) == 0 {
		return fmt.Errorf("provider configuration: config must not be empty")
	}
	if len(c.Config) > maxConfigBytes {
		return fmt.Errorf(
			"provider configuration: config must be at most %d bytes",
			maxConfigBytes,
		)
	}
	if !json.Valid(c.Config) {
		return fmt.Errorf("provider configuration: config must be valid JSON")
	}
	return nil
}

func cloneInstallationID(id *InstallationID) *InstallationID {
	if id == nil {
		return nil
	}
	copy := *id
	return &copy
}

func cloneConfig(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}
