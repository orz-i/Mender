package domain

import (
	"strings"
	"testing"
	"time"

	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

func TestNewInstallationValidatesManifestIdentityAndCopiesBytes(t *testing.T) {
	t.Parallel()

	raw := validManifestJSON("dev.example.echo", "1.2.3")
	id, _ := ParseInstallationID("pinst_0123456789abcdef0123456789abcdef")
	pluginID, _ := providerplugin.ParseID("dev.example.echo")
	installed, err := NewInstallation(
		id,
		pluginID,
		"1.2.3",
		SourceLocal,
		"/tmp/example-plugin",
		"sha256:"+strings.Repeat("a", 64),
		raw,
		StatusEnabled,
		time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("NewInstallation() error = %v", err)
	}
	raw[0] = 'x'
	if installed.ManifestJSON()[0] != '{' {
		t.Fatal("NewInstallation() retained mutable manifest bytes")
	}
}

func TestNewInstallationRejectsManifestMismatchAndInvalidDigest(t *testing.T) {
	t.Parallel()

	id, _ := ParseInstallationID("pinst_0123456789abcdef0123456789abcdef")
	pluginID, _ := providerplugin.ParseID("dev.example.echo")
	base := func(raw []byte, digest string) error {
		_, err := NewInstallation(
			id,
			pluginID,
			"1.2.3",
			SourceLocal,
			"/tmp/example-plugin",
			digest,
			raw,
			StatusEnabled,
			time.Now().UTC(),
		)
		return err
	}

	if err := base(validManifestJSON("dev.other.echo", "1.2.3"), "sha256:"+strings.Repeat("a", 64)); err == nil {
		t.Fatal("NewInstallation() accepted mismatched plugin id")
	}
	if err := base(validManifestJSON("dev.example.echo", "2.0.0"), "sha256:"+strings.Repeat("a", 64)); err == nil {
		t.Fatal("NewInstallation() accepted mismatched package version")
	}
	if err := base(validManifestJSON("dev.example.echo", "1.2.3"), "sha256:ABC"); err == nil {
		t.Fatal("NewInstallation() accepted invalid digest")
	}
}

func TestWithStatusPreservesInstallationIdentity(t *testing.T) {
	t.Parallel()

	installed := validInstallation(t)
	disabled, err := installed.WithStatus(StatusDisabled)
	if err != nil {
		t.Fatal(err)
	}
	if disabled.ID() != installed.ID() ||
		disabled.PluginID() != installed.PluginID() ||
		disabled.Digest() != installed.Digest() ||
		disabled.Status() != StatusDisabled {
		t.Fatalf("WithStatus() = %+v", disabled)
	}
}

func TestParseInstallationID(t *testing.T) {
	t.Parallel()

	valid := "pinst_0123456789abcdef0123456789abcdef"
	if _, err := ParseInstallationID(valid); err != nil {
		t.Fatalf("ParseInstallationID() error = %v", err)
	}
	for _, value := range []string{
		"inst_0123456789abcdef0123456789abcdef",
		"pinst_0123",
		"pinst_0123456789ABCDEF0123456789ABCDEF",
		"pinst_zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz",
	} {
		if _, err := ParseInstallationID(value); err == nil {
			t.Errorf("ParseInstallationID(%q) error = nil", value)
		}
	}
}

func validInstallation(t *testing.T) Installation {
	t.Helper()
	id, _ := ParseInstallationID("pinst_0123456789abcdef0123456789abcdef")
	pluginID, _ := providerplugin.ParseID("dev.example.echo")
	item, err := NewInstallation(
		id,
		pluginID,
		"1.2.3",
		SourceLocal,
		"/tmp/example-plugin",
		"sha256:"+strings.Repeat("a", 64),
		validManifestJSON(pluginID.String(), "1.2.3"),
		StatusEnabled,
		time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	return item
}

func validManifestJSON(pluginID, version string) []byte {
	return []byte(`{
		"manifestVersion": 1,
		"pluginId": "` + pluginID + `",
		"version": "` + version + `",
		"protocolVersions": [1],
		"displayName": "Echo",
		"description": "Example",
		"entrypoints": {"linux/amd64": "bin/echo"}
	}`)
}
