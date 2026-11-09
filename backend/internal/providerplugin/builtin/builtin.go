package builtin

import (
	"fmt"

	mcpplugin "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/plugin"
	modeltoolsplugin "github.com/orz-i/Mender/backend/internal/modeltools/infrastructure/plugin"
	openapiplugin "github.com/orz-i/Mender/backend/internal/openapi/infrastructure/plugin"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

// Only independently configured external Provider instances belong in this
// registry. Agent-as-Tool and native Workspace/SSH Jobs use typed SourceRef,
// and the Sandbox Router/Agent Task service own their own execution lifetimes.
func Plugins() []providerplugin.Plugin {
	return []providerplugin.Plugin{
		mcpplugin.New(),
		openapiplugin.New(),
		modeltoolsplugin.New(),
	}
}

func Register(registry *providerplugin.Registry) error {
	if registry == nil {
		return fmt.Errorf("register provider plugins: registry unavailable")
	}
	for _, plugin := range Plugins() {
		descriptor := plugin.Descriptor()
		if err := registry.Register(plugin); err != nil {
			return fmt.Errorf("register provider %s: %w", descriptor.ID, err)
		}
	}
	return nil
}
