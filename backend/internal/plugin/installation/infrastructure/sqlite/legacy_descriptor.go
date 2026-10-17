package sqlite

import (
	"encoding/json"
	"fmt"
	"math"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
)

// Kept exclusively for the forward-only legacy installation migration.
type legacyProviderDescriptorRecord struct {
	PluginID        string   `json:"pluginId"`
	PackageVersion  string   `json:"packageVersion"`
	ProtocolVersion uint32   `json:"protocolVersion"`
	ConfigVersion   int      `json:"configVersion"`
	DisplayName     string   `json:"displayName"`
	Description     string   `json:"description,omitempty"`
	ConfigSchema    string   `json:"configSchema,omitempty"`
	UISchema        string   `json:"uiSchema,omitempty"`
	Capabilities    []string `json:"capabilities,omitempty"`
}

func decodeLegacyProviderDescriptor(raw string, extension platformplugin.Extension) (platformplugin.Descriptor, error) {
	var legacy legacyProviderDescriptorRecord
	if err := json.Unmarshal([]byte(raw), &legacy); err != nil {
		return platformplugin.Descriptor{}, err
	}
	id, err := platformplugin.ParseID(legacy.PluginID)
	if err != nil {
		return platformplugin.Descriptor{}, err
	}
	if legacy.ConfigVersion <= 0 || uint64(legacy.ConfigVersion) > math.MaxUint32 || legacy.ProtocolVersion != extension.ProtocolVersion {
		return platformplugin.Descriptor{}, fmt.Errorf("incompatible legacy Provider descriptor version")
	}
	d := platformplugin.Descriptor{
		PluginID: id, PackageVersion: legacy.PackageVersion, Extension: extension,
		DisplayName: legacy.DisplayName, Description: legacy.Description, ConfigVersion: uint32(legacy.ConfigVersion), // #nosec G115 -- migration checks positive and <= MaxUint32.
		ConfigSchema: nil, UISchema: nil,
		Capabilities: legacy.Capabilities,
	}
	if legacy.ConfigSchema != "" {
		d.ConfigSchema = json.RawMessage(legacy.ConfigSchema)
	}
	if legacy.UISchema != "" {
		d.UISchema = json.RawMessage(legacy.UISchema)
	}
	if err := d.Validate(); err != nil {
		return platformplugin.Descriptor{}, err
	}
	return d, nil
}
