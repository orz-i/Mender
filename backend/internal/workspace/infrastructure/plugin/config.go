package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

const ID providerplugin.ID = "io.mender.provider.workspace"
const (
	ConfigVersion = 2
	pluginVersion = "2.0.0"
)

// ConfigVersion 2 is the ONLY runtime configuration accepted after the cut.
// Legacy root paths are handled exclusively by the forward-only DB migrator.
type Config struct {
	SandboxID string `json:"sandboxId"`
}

func Descriptor() providerplugin.Descriptor {
	return providerplugin.Descriptor{
		ID: ID, Version: pluginVersion, DisplayName: "Sandbox Workspace Tools", Description: "Read, search, edit and execute within one governed Sandbox", ConfigVersion: ConfigVersion, ConfigSchema: append(json.RawMessage(nil), workspaceConfigSchema...), Capabilities: []string{"workspace.read", "workspace.search", "workspace.edit", "workspace.shell"},
	}
}
func DecodeConfig(raw json.RawMessage) (Config, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var c Config
	if err := decoder.Decode(&c); err != nil {
		return Config{}, fmt.Errorf("decode Sandbox Workspace config: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Config{}, fmt.Errorf("sandbox Workspace config: multiple JSON values")
		}
		return Config{}, fmt.Errorf("sandbox Workspace config: trailing data: %w", err)
	}
	if _, err := sandboxdomain.ParseID(c.SandboxID); err != nil {
		return Config{}, fmt.Errorf("sandbox Workspace config: %w", err)
	}
	return c, nil
}
func ValidateConfig(_ context.Context, raw json.RawMessage) error {
	_, err := DecodeConfig(raw)
	return err
}
func InspectConfig(_ context.Context, raw json.RawMessage) (providerplugin.ConfigMetadata, error) {
	c, err := DecodeConfig(raw)
	if err != nil {
		return providerplugin.ConfigMetadata{}, err
	}
	return providerplugin.NewConfigMetadataWithSummary(nil, "sandbox:"+c.SandboxID)
}
