package pluginregistry

import (
	"errors"
	"testing"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

func TestVerifiedWorkspaceContractsRequireExactPinnedRealBackend(t *testing.T) {
	t.Parallel()
	backend := &fakeBackend{kind: sandboxdomain.BackendSSHRemote,
		caps: sandboxdomain.Capabilities{Read: true, Search: true, Shell: true}}
	descriptor, err := BuiltinDescriptor(backend)
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry()
	if err := registry.Register(backend, descriptor); err != nil {
		t.Fatal(err)
	}
	binding := sandboxdomain.BuiltinBinding(sandboxdomain.BackendSSHRemote)
	offers, err := registry.VerifiedWorkspaceContracts(sandboxdomain.BackendSSHRemote, binding)
	if err != nil || len(offers) != 3 {
		t.Fatalf("not real backend subset: %+v %v", offers, err)
	}
	for _, offer := range offers {
		if offer.Version != (platformplugin.CapabilityVersion{Major: 1}) ||
			offer.Mode != platformplugin.ExecutionInline || len(offer.Guarantees) != 0 {
			t.Fatalf("invented non-v1/durable semantics: %+v", offer)
		}
	}
	for _, bad := range []sandboxdomain.BackendBinding{
		{},
		{PluginID: "dev.example.wrong", ExtensionID: "main"},
		{PluginID: binding.PluginID, ExtensionID: binding.ExtensionID,
			InstallationID: "pinst_00000000000000000000000000000001"},
	} {
		if _, err := registry.VerifiedWorkspaceContracts(sandboxdomain.BackendSSHRemote, bad); !errors.Is(err, sandboxdomain.ErrBackendUnavailable) {
			t.Fatalf("wrong binding was eligible: %+v %v", bad, err)
		}
	}
	// Even if a mutable backend later gains a new operation, the pinned
	// descriptor cannot gain corresponding authority by accident.
	backend.caps.Edit = true
	offers, err = registry.VerifiedWorkspaceContracts(sandboxdomain.BackendSSHRemote, binding)
	if err != nil || len(offers) != 3 {
		t.Fatalf("runtime capability growth escaped pin: %+v %v", offers, err)
	}
	// Losing a real operation immediately removes its offer and makes a
	// complete Workspace Suite fail negotiation without executing anything.
	backend.caps.Read = false
	offers, err = registry.VerifiedWorkspaceContracts(sandboxdomain.BackendSSHRemote, binding)
	if err != nil || len(offers) != 2 || offers[0].ID != "file.search" {
		t.Fatalf("unavailable operation was still advertised: %+v %v", offers, err)
	}
	// A catalogue-only descriptor cannot be used to acquire a backend.
	if _, err := NewRegistry().VerifiedWorkspaceContracts(sandboxdomain.BackendDocker,
		sandboxdomain.BuiltinBinding(sandboxdomain.BackendDocker)); !errors.Is(err, sandboxdomain.ErrBackendUnavailable) {
		t.Fatalf("unregistered Docker catalogue enabled execution: %v", err)
	}
}
