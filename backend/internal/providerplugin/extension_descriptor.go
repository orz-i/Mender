package providerplugin

import (
	"fmt"
	"math"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
)

// PlatformDescriptor maps the Provider-specific public protocol snapshot to
// the platform's typed extension metadata; no Provider lifecycle is involved.
func PlatformDescriptor(d Descriptor) (platformplugin.Descriptor, error) {
	if err := d.Validate(); err != nil {
		return platformplugin.Descriptor{}, err
	}
	if uint64(d.ConfigVersion) > math.MaxUint32 { // #nosec G115 -- Descriptor.Validate ensures positive config version.
		return platformplugin.Descriptor{}, fmt.Errorf("provider descriptor config version exceeds uint32")
	}
	protocolVersion := d.ProtocolVersion
	if protocolVersion == 0 {
		protocolVersion = 1
	} // statically linked built-ins
	result := platformplugin.Descriptor{
		PluginID: d.ID, PackageVersion: d.Version,
		Extension:   platformplugin.Extension{ID: "main", Kind: platformplugin.ProviderTools, ProtocolVersion: protocolVersion},
		DisplayName: d.DisplayName, Description: d.Description, ConfigVersion: uint32(d.ConfigVersion), // #nosec G115 -- positive, checked against MaxUint32 above.
		ConfigSchema: d.ConfigSchema, UISchema: d.UISchema, Capabilities: d.Capabilities,
	}
	return result.Clone(), result.Validate()
}

// ProviderDescriptor rejects a wrong-kind platform descriptor before it reaches
// the Provider plugin process/runtime boundary.
func ProviderDescriptor(d platformplugin.Descriptor) (Descriptor, error) {
	if err := d.Validate(); err != nil {
		return Descriptor{}, err
	}
	if d.Extension.Kind != platformplugin.ProviderTools || d.Extension.ID != "main" {
		return Descriptor{}, fmt.Errorf("provider plugin: installed extension is not provider.tools/main")
	}
	if d.ConfigVersion > math.MaxInt32 {
		return Descriptor{}, fmt.Errorf("provider plugin: config version exceeds max int32")
	}
	out := Descriptor{
		ID: d.PluginID, Version: d.PackageVersion, DisplayName: d.DisplayName, Description: d.Description,
		ConfigVersion: int(d.ConfigVersion), ProtocolVersion: d.Extension.ProtocolVersion,
		ConfigSchema: d.ConfigSchema, UISchema: d.UISchema, Capabilities: d.Capabilities,
	}
	return out.Clone(), out.Validate()
}
