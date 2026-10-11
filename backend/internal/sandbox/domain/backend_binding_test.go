package domain

import (
	"strings"
	"testing"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
)

func TestBuiltinSandboxBindingMustBeExplicitAndExact(t *testing.T) {
	t.Parallel()
	for _, kind := range []BackendKind{BackendLocalWorkspace, BackendDocker, BackendDaytona} {
		binding := BuiltinBinding(kind)
		if err := binding.Validate(); err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if err := RequireBuiltinBinding(kind, binding); err != nil {
			t.Fatal(err)
		}
		if err := RequireBuiltinBinding(kind, BackendBinding{}); err == nil {
			t.Fatalf("%s accepted missing extension identity", kind)
		}
		swapped := binding
		swapped.PluginID = "dev.example.other"
		if err := RequireBuiltinBinding(kind, swapped); err == nil {
			t.Fatalf("%s accepted another PluginID", kind)
		}
		swapped = binding
		swapped.ExtensionID = "other"
		if err := RequireBuiltinBinding(kind, swapped); err == nil {
			t.Fatalf("%s accepted another ExtensionID", kind)
		}
		swapped = binding
		swapped.InstallationID = platformplugin.InstallationID("pinst_" + strings.Repeat("1", 32))
		if err := RequireBuiltinBinding(kind, swapped); err == nil {
			t.Fatalf("%s silently followed external installation", kind)
		}
	}
	if b := BuiltinBinding(BackendE2B); b.PluginID != "" {
		t.Fatalf("unimplemented E2B got trusted binding %+v", b)
	}
}

func TestSandboxConstructorRefusesUnboundResource(t *testing.T) {
	t.Parallel()
	basic := Spec{ID: ID("sbx_0123456789abcdef0123456789abcdef"), Name: "example", Backend: BackendLocalWorkspace,
		Isolation: IsolationHost, LocalRoot: t.TempDir(), OwnerKind: "local_user", OwnerRef: "owner"}
	if _, err := NewSandbox(basic); err == nil {
		t.Fatal("missing binding allowed")
	}
	basic.Binding = BuiltinBinding(BackendLocalWorkspace)
	if _, err := NewSandbox(basic); err != nil {
		t.Fatal(err)
	}
	basic.Binding = BackendBinding{PluginID: "io.mender.sandbox.docker", ExtensionID: "main"}
	if _, err := NewSandbox(basic); err == nil {
		t.Fatal("different registered backend accepted")
	}
}
