package manifest

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/orz-i/Mender/backend/internal/plugin"
)

func good() Manifest {
	return Manifest{
		ManifestVersion: CurrentVersion,
		PluginID:        "dev.example.sandbox",
		Version:         "1.2.3",
		DisplayName:     "Example Sandbox",
		Entrypoints:     map[string]string{"darwin/arm64": "bin/sandbox", "linux/amd64": "bin/sandbox"},
		Extensions:      []plugin.Extension{{ID: "main", Kind: plugin.SandboxBackend, ProtocolVersion: 1}},
	}
}

func TestManifestKindAndPlatform(t *testing.T) {
	t.Parallel()
	for _, kind := range []plugin.Kind{plugin.ProviderTools, plugin.SandboxBackend, plugin.TunnelTransport} {
		m := good()
		m.Extensions[0].Kind = kind
		raw, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if parsed.Extensions[0].Kind != kind {
			t.Fatalf("parsed kind %q", parsed.Extensions[0].Kind)
		}
		if entry, err := parsed.Entrypoint("darwin", "arm64"); err != nil || entry != "bin/sandbox" {
			t.Fatalf("Entrypoint = %q, %v", entry, err)
		}
		if _, err := parsed.Entrypoint("windows", "amd64"); err == nil {
			t.Fatal("accepted unsupported platform")
		}
	}
}

func TestManifestRejectsUnsafeOrUnrecognizedPackages(t *testing.T) {
	t.Parallel()
	cases := map[string]func(*Manifest){
		"missing extension":       func(m *Manifest) { m.Extensions = nil },
		"multiple extensions":     func(m *Manifest) { m.Extensions = append(m.Extensions, m.Extensions[0]) },
		"unknown kind":            func(m *Manifest) { m.Extensions[0].Kind = "plugin.elevate" },
		"invalid protocol":        func(m *Manifest) { m.Extensions[0].ProtocolVersion = 0 },
		"bad plugin ID":           func(m *Manifest) { m.PluginID = "../bad" },
		"old version":             func(m *Manifest) { m.ManifestVersion = 1 },
		"invalid SemVer":          func(m *Manifest) { m.Version = "v1.2.3" },
		"absolute entrypoint":     func(m *Manifest) { m.Entrypoints["darwin/arm64"] = "/bin/sh" },
		"escaping entrypoint":     func(m *Manifest) { m.Entrypoints["darwin/arm64"] = "../bin/sh" },
		"noncanonical entrypoint": func(m *Manifest) { m.Entrypoints["darwin/arm64"] = "bin/../sh" },
		"backslash entrypoint":    func(m *Manifest) { m.Entrypoints["darwin/arm64"] = `bin\sh` },
		"nul entrypoint":          func(m *Manifest) { m.Entrypoints["darwin/arm64"] = "bin/\x00sh" },
	}
	for name, modify := range cases {
		t.Run(name, func(t *testing.T) {
			m := good()
			modify(&m)
			if err := m.Validate(); err == nil {
				t.Fatalf("invalid manifest accepted: %+v", m)
			}
		})
	}
	raw, _ := json.Marshal(good())
	if _, err := Parse(append(raw, []byte(" {}")...)); err == nil {
		t.Fatal("accepted trailing document")
	}
	if _, err := Parse([]byte(strings.Replace(string(raw), `"manifestVersion":2`, `"manifestVersion":2,"unexpected":true`, 1))); err == nil {
		t.Fatal("accepted unknown field")
	}
	if _, err := Parse(make([]byte, MaxManifestBytes+1)); err == nil {
		t.Fatal("accepted oversized package")
	}
}
