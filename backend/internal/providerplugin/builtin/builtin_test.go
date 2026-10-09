package builtin

import (
	"context"
	"errors"
	"slices"
	"testing"

	governance "github.com/orz-i/Mender/backend/internal/governance/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	sandboxapp "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

func TestPluginsReturnStableBuiltInSet(t *testing.T) {
	t.Parallel()

	plugins := Plugins(unusedSandboxRouter{})
	if got, want := len(plugins), 4; got != want {
		t.Fatalf("Plugins(unusedSandboxRouter{}) count = %d, want %d", got, want)
	}

	wantIDs := []providerplugin.ID{
		"io.mender.provider.mcp",
		"io.mender.provider.openapi",
		"io.mender.provider.model-tools",
		"io.mender.provider.workspace",
	}
	gotIDs := make([]providerplugin.ID, 0, len(plugins))
	seen := make(map[providerplugin.ID]struct{}, len(plugins))
	for _, plugin := range plugins {
		if plugin == nil {
			t.Fatal("Plugins(unusedSandboxRouter{}) returned nil plugin")
		}
		descriptor := plugin.Descriptor()
		if err := descriptor.Validate(); err != nil {
			t.Fatalf("descriptor %q invalid: %v", descriptor.ID, err)
		}
		if _, duplicate := seen[descriptor.ID]; duplicate {
			t.Fatalf("Plugins(unusedSandboxRouter{}) duplicate PluginID %q", descriptor.ID)
		}
		seen[descriptor.ID] = struct{}{}
		gotIDs = append(gotIDs, descriptor.ID)
	}
	if !slices.Equal(gotIDs, wantIDs) {
		t.Fatalf("Plugins(unusedSandboxRouter{}) IDs = %v, want %v", gotIDs, wantIDs)
	}

	second := Plugins(unusedSandboxRouter{})
	for index := range plugins {
		if plugins[index] == second[index] {
			t.Fatalf("Plugins(unusedSandboxRouter{}) reused plugin instance at index %d", index)
		}
	}
}

func TestRegisterAddsCompleteBuiltInSet(t *testing.T) {
	t.Parallel()

	registry := providerplugin.NewRegistry()
	if err := Register(registry, unusedSandboxRouter{}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	descriptors := registry.Descriptors()
	if got, want := len(descriptors), 4; got != want {
		t.Fatalf("Descriptors() count = %d, want %d", got, want)
	}

	wantIDs := []providerplugin.ID{
		"io.mender.provider.mcp",
		"io.mender.provider.model-tools",
		"io.mender.provider.openapi",
		"io.mender.provider.workspace",
	}
	gotIDs := make([]providerplugin.ID, 0, len(descriptors))
	for _, descriptor := range descriptors {
		gotIDs = append(gotIDs, descriptor.ID)
	}
	if !slices.Equal(gotIDs, wantIDs) {
		t.Fatalf("Descriptors() IDs = %v, want %v", gotIDs, wantIDs)
	}
}

func TestRegisterRejectsInvalidRegistryAndDuplicateRegistration(t *testing.T) {
	t.Parallel()

	if err := Register(nil, unusedSandboxRouter{}); err == nil {
		t.Fatal("Register(nil, unusedSandboxRouter{}) error = nil, want error")
	}

	registry := providerplugin.NewRegistry()
	if err := Register(registry, unusedSandboxRouter{}); err != nil {
		t.Fatalf("first Register() error = %v", err)
	}
	err := Register(registry, unusedSandboxRouter{})
	if !errors.Is(err, providerplugin.ErrDuplicate) {
		t.Fatalf("second Register() error = %v, want ErrDuplicate", err)
	}
}

// Built-in descriptor tests never activate a Sandbox.
type unusedSandboxRouter struct{}

func (unusedSandboxRouter) Resolve(context.Context, sandboxdomain.ID, sandboxdomain.Capability) (sandboxapp.Placement, error) {
	return sandboxapp.Placement{}, sandboxdomain.ErrNotFound
}
func (unusedSandboxRouter) Acquire(context.Context, governance.Context, sandboxapp.Placement) (*sandboxapp.Lease, error) {
	return nil, sandboxdomain.ErrBackendUnavailable
}
