package builtin

import (
	"fmt"

	mcpplugin "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/plugin"
	modeltoolsplugin "github.com/orz-i/Mender/backend/internal/modeltools/infrastructure/plugin"
	openapiplugin "github.com/orz-i/Mender/backend/internal/openapi/infrastructure/plugin"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

// Plugins constructs the statically linked first-party Provider plugins.
//
// The returned order is deterministic and intentionally follows the historical
// Runtime registration order. Callers receive fresh plugin values so tests or
// future implementation-local state cannot leak across Runtime instances.
func Plugins() []providerplugin.Plugin {
	return []providerplugin.Plugin{
		mcpplugin.New(),
		openapiplugin.New(),
		modeltoolsplugin.New(),
	}
}

// Register adds the complete statically linked built-in Provider plugin set to
// registry. It fails closed when any built-in descriptor is invalid or a
// PluginID is already registered.
func Register(registry *providerplugin.Registry) error {
	if registry == nil {
		return fmt.Errorf("register built-in provider plugins: registry must not be nil")
	}

	for _, plugin := range Plugins() {
		descriptor := plugin.Descriptor()
		if err := registry.Register(plugin); err != nil {
			return fmt.Errorf(
				"register built-in provider plugin %s: %w",
				descriptor.ID,
				err,
			)
		}
	}
	return nil
}
