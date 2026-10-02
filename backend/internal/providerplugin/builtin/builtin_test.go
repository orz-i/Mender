package builtin

import (
	"errors"
	"slices"
	"testing"

	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	workspaceplugin "github.com/orz-i/Mender/backend/internal/workspace/infrastructure/plugin"
)

func TestPluginsReturnStableBuiltInSet(t *testing.T) {
	t.Parallel()

	plugins := Plugins(workspaceplugin.HostShellLauncher{})
	if got, want := len(plugins), 4; got != want {
		t.Fatalf("Plugins(workspaceplugin.HostShellLauncher{}) count = %d, want %d", got, want)
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
			t.Fatal("Plugins(workspaceplugin.HostShellLauncher{}) returned nil plugin")
		}
		descriptor := plugin.Descriptor()
		if err := descriptor.Validate(); err != nil {
			t.Fatalf("descriptor %q invalid: %v", descriptor.ID, err)
		}
		if _, duplicate := seen[descriptor.ID]; duplicate {
			t.Fatalf("Plugins(workspaceplugin.HostShellLauncher{}) duplicate PluginID %q", descriptor.ID)
		}
		seen[descriptor.ID] = struct{}{}
		gotIDs = append(gotIDs, descriptor.ID)
	}
	if !slices.Equal(gotIDs, wantIDs) {
		t.Fatalf("Plugins(workspaceplugin.HostShellLauncher{}) IDs = %v, want %v", gotIDs, wantIDs)
	}

	second := Plugins(workspaceplugin.HostShellLauncher{})
	for index := range plugins {
		if plugins[index] == second[index] {
			t.Fatalf("Plugins(workspaceplugin.HostShellLauncher{}) reused plugin instance at index %d", index)
		}
	}
}

func TestRegisterAddsCompleteBuiltInSet(t *testing.T) {
	t.Parallel()

	registry := providerplugin.NewRegistry()
	if err := Register(registry, workspaceplugin.HostShellLauncher{}); err != nil {
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

	if err := Register(nil, workspaceplugin.HostShellLauncher{}); err == nil {
		t.Fatal("Register(nil, workspaceplugin.HostShellLauncher{}) error = nil, want error")
	}

	registry := providerplugin.NewRegistry()
	if err := Register(registry, workspaceplugin.HostShellLauncher{}); err != nil {
		t.Fatalf("first Register() error = %v", err)
	}
	err := Register(registry, workspaceplugin.HostShellLauncher{})
	if !errors.Is(err, providerplugin.ErrDuplicate) {
		t.Fatalf("second Register() error = %v, want ErrDuplicate", err)
	}
}
