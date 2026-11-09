package builtin

import (
	"errors"
	"slices"
	"testing"

	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

func TestPluginsReturnStableIndependentProviderSet(t *testing.T) {
	t.Parallel()
	plugins := Plugins()
	if len(plugins) != 3 {
		t.Fatalf("plugins count = %d", len(plugins))
	}
	want := []providerplugin.ID{"io.mender.provider.mcp", "io.mender.provider.openapi", "io.mender.provider.model-tools"}
	got := make([]providerplugin.ID, 0, len(plugins))
	for _, p := range plugins {
		if p == nil {
			t.Fatal("nil plugin")
		}
		d := p.Descriptor()
		if err := d.Validate(); err != nil {
			t.Fatal(err)
		}
		got = append(got, d.ID)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("provider IDs: %v != %v", got, want)
	}
	next := Plugins()
	for i := range plugins {
		if plugins[i] == next[i] {
			t.Fatalf("shared plugin state at %d", i)
		}
	}
}
func TestRegisterOnlyIndependentProviders(t *testing.T) {
	t.Parallel()
	if err := Register(nil); err == nil {
		t.Fatal("nil Registry accepted")
	}
	registry := providerplugin.NewRegistry()
	if err := Register(registry); err != nil {
		t.Fatal(err)
	}
	descriptors := registry.Descriptors()
	want := []providerplugin.ID{"io.mender.provider.mcp", "io.mender.provider.model-tools", "io.mender.provider.openapi"}
	got := make([]providerplugin.ID, 0, len(descriptors))
	for _, d := range descriptors {
		got = append(got, d.ID)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("independent Provider IDs %v != %v", got, want)
	}
	if err := Register(registry); !errors.Is(err, providerplugin.ErrDuplicate) {
		t.Fatalf("duplicate registration: %v", err)
	}
}
