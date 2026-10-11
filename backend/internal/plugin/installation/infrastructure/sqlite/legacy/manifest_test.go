package manifest

import (
	"strings"
	"testing"
)

func TestParseAcceptsValidManifest(t *testing.T) {
	t.Parallel()

	parsed, err := Parse([]byte(`{
		"manifestVersion": 1,
		"pluginId": "dev.example.echo",
		"version": "1.2.3-beta.1+build.7",
		"protocolVersions": [1, 2],
		"displayName": "Example Echo",
		"description": "Example Provider plugin",
		"entrypoints": {
			"darwin/arm64": "bin/mender-provider-echo",
			"linux/amd64": "bin/mender-provider-echo"
		}
	}`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if parsed.PluginID != "dev.example.echo" || parsed.Version != "1.2.3-beta.1+build.7" {
		t.Fatalf("Parse() = %+v", parsed)
	}
	entrypoint, err := parsed.Entrypoint("darwin", "arm64")
	if err != nil {
		t.Fatalf("Entrypoint() error = %v", err)
	}
	if entrypoint != "bin/mender-provider-echo" {
		t.Fatalf("Entrypoint() = %q", entrypoint)
	}
}

func TestParseRejectsUnknownFieldAndTrailingDocument(t *testing.T) {
	t.Parallel()

	base := `{
		"manifestVersion": 1,
		"pluginId": "dev.example.echo",
		"version": "1.0.0",
		"protocolVersions": [1],
		"displayName": "Echo",
		"description": "",
		"entrypoints": {"linux/amd64": "bin/echo"}
	}`
	if _, err := Parse([]byte(strings.Replace(
		base,
		`"displayName": "Echo"`,
		`"displayName": "Echo", "unknown": true`,
		1,
	))); err == nil {
		t.Fatal("Parse() accepted unknown field")
	}
	if _, err := Parse([]byte(base + " {}")); err == nil {
		t.Fatal("Parse() accepted trailing JSON document")
	}
}

func TestValidateRejectsInvalidVersionAndProtocolList(t *testing.T) {
	t.Parallel()

	manifest := validManifest()
	manifest.Version = "v1.0.0"
	if err := manifest.Validate(); err == nil {
		t.Fatal("Validate() accepted package version with v prefix")
	}

	manifest = validManifest()
	manifest.ProtocolVersions = []uint32{1, 1}
	if err := manifest.Validate(); err == nil {
		t.Fatal("Validate() accepted duplicate protocol version")
	}

	manifest = validManifest()
	manifest.ProtocolVersions = []uint32{0}
	if err := manifest.Validate(); err == nil {
		t.Fatal("Validate() accepted protocol version zero")
	}
}

func TestValidateRejectsUnsafeEntrypoints(t *testing.T) {
	t.Parallel()

	tests := []string{
		"/usr/local/bin/plugin",
		"../plugin",
		"bin/../plugin",
		`bin\plugin.exe`,
		"./plugin",
	}
	for _, entrypoint := range tests {
		entrypoint := entrypoint
		t.Run(entrypoint, func(t *testing.T) {
			t.Parallel()
			manifest := validManifest()
			manifest.Entrypoints = map[string]string{
				"linux/amd64": entrypoint,
			}
			if err := manifest.Validate(); err == nil {
				t.Fatalf("Validate() accepted unsafe entrypoint %q", entrypoint)
			}
		})
	}
}

func TestEntrypointRejectsUnsupportedPlatform(t *testing.T) {
	t.Parallel()

	if _, err := validManifest().Entrypoint("windows", "amd64"); err == nil {
		t.Fatal("Entrypoint() error = nil")
	}
}

func validManifest() Manifest {
	return Manifest{
		ManifestVersion:  CurrentVersion,
		PluginID:         "dev.example.echo",
		Version:          "1.0.0",
		ProtocolVersions: []uint32{1},
		DisplayName:      "Echo",
		Description:      "Example",
		Entrypoints: map[string]string{
			"linux/amd64": "bin/mender-provider-echo",
		},
	}
}
