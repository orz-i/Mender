package plugin

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestPluginIdentityAndKindValidation(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"", "provider", "Provider.MCP", " bad.example", "a..b", "a.b/", "a.b\n"} {
		if _, err := ParseID(value); err == nil {
			t.Errorf("ParseID(%q) accepted invalid identity", value)
		}
	}
	for _, value := range []string{"", "pinst_", "pinst_Z0000000000000000000000000000000", "pinst_0000000000000000000000000000000G"} {
		if _, err := ParseInstallationID(value); err == nil {
			t.Errorf("ParseInstallationID(%q) accepted invalid identity", value)
		}
	}
	if id, err := ParseID("io.mender.sandbox.docker"); err != nil || id.String() != "io.mender.sandbox.docker" {
		t.Fatalf("ParseID valid = %s, %v", id, err)
	}
	if id, err := ParseInstallationID("pinst_0123456789abcdef0123456789abcdef"); err != nil || id.String() != "pinst_0123456789abcdef0123456789abcdef" {
		t.Fatalf("ParseInstallationID valid = %s, %v", id, err)
	}
	for _, kind := range []Kind{ProviderTools, SandboxBackend, TunnelTransport} {
		if err := kind.Validate(); err != nil {
			t.Errorf("kind %q: %v", kind, err)
		}
	}
	if err := Kind("arbitrary").Validate(); err == nil {
		t.Fatal("accepted unknown extension kind")
	}
}

func validDescriptor(kind Kind) Descriptor {
	return Descriptor{
		PluginID:       "io.mender.example",
		PackageVersion: "1.0.0",
		Extension:      Extension{ID: "main", Kind: kind, ProtocolVersion: 1},
		DisplayName:    "Example",
		ConfigVersion:  1,
		ConfigSchema:   json.RawMessage(`{"type":"object"}`),
		Capabilities:   []string{"execute"},
	}
}

func TestRegistryKeepsTypedContractsAndImmutableSnapshots(t *testing.T) {
	t.Parallel()
	for _, kind := range []Kind{ProviderTools, SandboxBackend, TunnelTransport} {
		t.Run(string(kind), func(t *testing.T) {
			r := NewRegistry()
			descriptor := validDescriptor(kind)
			if err := r.Register(descriptor); err != nil {
				t.Fatal(err)
			}
			if err := r.Register(descriptor); err == nil {
				t.Fatal("duplicate accepted")
			}
			got, err := r.Lookup(descriptor.PluginID, descriptor.Extension.ID, kind)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, descriptor) {
				t.Fatalf("lookup = %+v", got)
			}
			got.Capabilities[0] = "mutated"
			got.ConfigSchema[0] = 'x'
			list := r.Descriptors()
			if len(list) != 1 || list[0].Capabilities[0] != "execute" || string(list[0].ConfigSchema) != `{"type":"object"}` {
				t.Fatalf("mutable catalog snapshot: %+v", list)
			}
			if _, err := r.Lookup(descriptor.PluginID, descriptor.Extension.ID, Kind("nonexistent")); err == nil {
				t.Fatal("lookup accepted unknown kind")
			}
			for _, wrong := range []Kind{ProviderTools, SandboxBackend, TunnelTransport} {
				if wrong == kind {
					continue
				}
				if _, err := r.Lookup(descriptor.PluginID, descriptor.Extension.ID, wrong); err == nil {
					t.Fatalf("wrong kind %s resolved", wrong)
				}
			}
		})
	}
}

func TestRegistryRejectsInvalidAndMixedVersions(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	d := validDescriptor(SandboxBackend)
	d.Capabilities = []string{"file.read", "file.read"}
	if err := r.Register(d); err == nil {
		t.Fatal("duplicate capabilities accepted")
	}
	d = validDescriptor(SandboxBackend)
	d.Extension.ProtocolVersion = 0
	if err := r.Register(d); err == nil {
		t.Fatal("invalid protocol version accepted")
	}
	d = validDescriptor(SandboxBackend)
	if err := r.Register(d); err != nil {
		t.Fatal(err)
	}
	d.Extension.ID = "aux"
	d.PackageVersion = "2.0.0"
	if err := r.Register(d); err == nil {
		t.Fatal("mixed package versions accepted")
	}
}
