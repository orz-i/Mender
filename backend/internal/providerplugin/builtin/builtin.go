package builtin

import (
	"fmt"

	mcpplugin "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/plugin"
	modeltoolsplugin "github.com/orz-i/Mender/backend/internal/modeltools/infrastructure/plugin"
	openapiplugin "github.com/orz-i/Mender/backend/internal/openapi/infrastructure/plugin"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	sandboxapplication "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sshjobsplugin "github.com/orz-i/Mender/backend/internal/sshjobs/infrastructure/plugin"
	workspaceplugin "github.com/orz-i/Mender/backend/internal/workspace/infrastructure/plugin"
)

// Plugins constructs the statically linked first-party Provider plugins.
//
// The returned order is deterministic and intentionally follows the historical
// Runtime registration order. Callers receive fresh plugin values so tests or
// future implementation-local state cannot leak across Runtime instances.
func Plugins(sandboxRouter sandboxapplication.ExecutionRouter, jobs sshjobsplugin.Port, targets ...workspaceplugin.TargetLookup) []providerplugin.Plugin {
	return []providerplugin.Plugin{
		mcpplugin.New(),
		openapiplugin.New(),
		modeltoolsplugin.New(),
		workspaceplugin.New(sandboxRouter, targets...),
		sshjobsplugin.New(jobs),
	}
}

// Register adds the complete statically linked built-in Provider plugin set to
// registry. It fails closed when any built-in descriptor is invalid or a
// PluginID is already registered.
func Register(
	registry *providerplugin.Registry,
	sandboxRouter sandboxapplication.ExecutionRouter,
	jobs sshjobsplugin.Port,
	targets ...workspaceplugin.TargetLookup,
) error {
	if registry == nil {
		return fmt.Errorf("register built-in provider plugins: registry must not be nil")
	}

	if sandboxRouter == nil || jobs == nil {
		return fmt.Errorf("register built-in provider plugins: sandbox Router and SSH Job ports required")
	}

	for _, plugin := range Plugins(sandboxRouter, jobs, targets...) {
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
