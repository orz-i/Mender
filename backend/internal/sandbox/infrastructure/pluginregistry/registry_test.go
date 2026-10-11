package pluginregistry

import (
	"context"
	"errors"
	"slices"
	"testing"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	sandboxapp "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

type fakeBackend struct {
	kind sandboxdomain.BackendKind
	caps sandboxdomain.Capabilities
}

func (f *fakeBackend) Kind() sandboxdomain.BackendKind          { return f.kind }
func (f *fakeBackend) Capabilities() sandboxdomain.Capabilities { return f.caps }
func (f *fakeBackend) Open(context.Context, sandboxdomain.Sandbox, sandboxdomain.Run) (sandboxapp.Session, error) {
	return nil, errors.New("unexpected backend open")
}

func TestTrustedBuiltinBackendDescriptors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		kind sandboxdomain.BackendKind
		id   platformplugin.ID
	}{
		{sandboxdomain.BackendLocalWorkspace, "io.mender.sandbox.local-workspace"},
		{sandboxdomain.BackendDocker, "io.mender.sandbox.docker"},
		{sandboxdomain.BackendDaytona, "io.mender.sandbox.daytona"},
	} {
		t.Run(string(tc.kind), func(t *testing.T) {
			backend := &fakeBackend{kind: tc.kind, caps: sandboxdomain.Capabilities{Read: true, Search: true, Shell: true}}
			descriptor, err := BuiltinDescriptor(backend)
			if err != nil {
				t.Fatal(err)
			}
			if descriptor.PluginID != tc.id || descriptor.Extension.Kind != platformplugin.SandboxBackend || descriptor.Extension.ID != "main" {
				t.Fatalf("invalid descriptor %+v", descriptor)
			}
			if !slices.Equal(descriptor.Capabilities, []string{"file.read", "file.search", "process.shell"}) {
				t.Fatalf("capabilities do not match real backend: %+v", descriptor.Capabilities)
			}
			registry := NewRegistry()
			if err := registry.Register(backend, descriptor); err != nil {
				t.Fatal(err)
			}
			if _, err := registry.ResolveBackend(tc.kind, sandboxdomain.BuiltinBinding(tc.kind), sandboxdomain.CapabilityEdit); !errors.Is(err, sandboxdomain.ErrCapabilityUnavailable) {
				t.Fatalf("unadvertised edit resolved %v", err)
			}
			// A backend may gain a dynamic capability, but the original
			// host-verified descriptor must continue to bound authority.
			backend.caps.Edit = true
			if _, err := registry.ResolveBackend(tc.kind, sandboxdomain.BuiltinBinding(tc.kind), sandboxdomain.CapabilityEdit); !errors.Is(err, sandboxdomain.ErrCapabilityUnavailable) {
				t.Fatalf("capability grew beyond pinned descriptor: %v", err)
			}
			got, err := registry.ResolveBackend(tc.kind, sandboxdomain.BuiltinBinding(tc.kind), sandboxdomain.CapabilityRead)
			if err != nil || got != backend {
				t.Fatalf("read backend = %T %v", got, err)
			}
			entries := registry.Entries()
			if len(entries) != 1 || !registry.Available(tc.kind, sandboxdomain.BuiltinBinding(tc.kind)) {
				t.Fatalf("missing registry entry %+v", entries)
			}
			entries[0].Descriptor.Capabilities[0] = "mutated"
			if registry.Entries()[0].Descriptor.Capabilities[0] != "file.read" {
				t.Fatal("registry exposes mutable descriptor")
			}
		})
	}
}

func TestSandboxRegistryRejectsWrongKindClaimsAndDuplicates(t *testing.T) {
	t.Parallel()
	backend := &fakeBackend{kind: sandboxdomain.BackendDocker, caps: sandboxdomain.Capabilities{Read: true}}
	original, err := BuiltinDescriptor(backend)
	if err != nil {
		t.Fatal(err)
	}
	for name, alter := range map[string]func(*platformplugin.Descriptor){
		"provider extension":      func(d *platformplugin.Descriptor) { d.Extension.Kind = platformplugin.ProviderTools },
		"tunnel extension":        func(d *platformplugin.Descriptor) { d.Extension.Kind = platformplugin.TunnelTransport },
		"wrong built-in identity": func(d *platformplugin.Descriptor) { d.PluginID = "dev.example.unsafe" },
		"overclaimed shell":       func(d *platformplugin.Descriptor) { d.Capabilities = append(d.Capabilities, "process.shell") },
		"missing read":            func(d *platformplugin.Descriptor) { d.Capabilities = nil },
		"wrong extension":         func(d *platformplugin.Descriptor) { d.Extension.ID = "other" },
	} {
		t.Run(name, func(t *testing.T) {
			descriptor := original.Clone()
			alter(&descriptor)
			if err := NewRegistry().Register(backend, descriptor); err == nil {
				t.Fatal("invalid registration accepted")
			}
		})
	}
	r := NewRegistry()
	if err := r.Register(backend, original); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(backend, original); err == nil {
		t.Fatal("duplicate backend registration accepted")
	}
	if r.Available(sandboxdomain.BackendDaytona, sandboxdomain.BuiltinBinding(sandboxdomain.BackendDaytona)) {
		t.Fatal("unconfigured Daytona advertised as available")
	}
	if _, err := r.ResolveBackend(sandboxdomain.BackendDaytona, sandboxdomain.BuiltinBinding(sandboxdomain.BackendDaytona), sandboxdomain.CapabilityRead); !errors.Is(err, sandboxdomain.ErrBackendUnavailable) {
		t.Fatalf("unavailable driver routed: %v", err)
	}
	if _, err := r.ResolveBackend(sandboxdomain.BackendDocker, sandboxdomain.BuiltinBinding(sandboxdomain.BackendDocker), sandboxdomain.Capability("made.up")); err == nil {
		t.Fatal("invalid capability accepted")
	}
	builtin := sandboxdomain.BuiltinBinding(sandboxdomain.BackendDocker)
	for _, modified := range []sandboxdomain.BackendBinding{
		{PluginID: "dev.example.other", ExtensionID: "main"},
		{PluginID: builtin.PluginID, ExtensionID: "other"},
		{PluginID: builtin.PluginID, ExtensionID: "main", InstallationID: "pinst_00000000000000000000000000000001"},
	} {
		if r.Available(sandboxdomain.BackendDocker, modified) {
			t.Fatalf("wrong binding shown available: %+v", modified)
		}
		if _, err := r.ResolveBackend(sandboxdomain.BackendDocker, modified, sandboxdomain.CapabilityRead); !errors.Is(err, sandboxdomain.ErrBackendUnavailable) {
			t.Fatalf("wrong binding routed to backend: %+v %v", modified, err)
		}
	}
	if _, err := BuiltinDescriptor(&fakeBackend{kind: sandboxdomain.BackendE2B}); err == nil {
		t.Fatal("unimplemented E2B registered")
	}
}
