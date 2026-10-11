package localfs

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
)

func TestResolvePreviouslyInstalledProviderWithoutLegacyParsing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "plugins")
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := installationdomain.ParseInstallationID("pinst_0123456789abcdef0123456789abcdef")
	directory := filepath.Join(root, id.String())
	if err := os.MkdirAll(filepath.Join(directory, "bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	// This is intentionally not valid generic plugin v2 JSON.
	original := []byte(`{"manifestVersion":1,"pluginId":"dev.example.echo","version":"1.2.3","protocolVersions":[1],"displayName":"Old","entrypoints":{"` + runtime.GOOS + `/` + runtime.GOARCH + `":"bin/echo"}}`)
	if err := os.WriteFile(filepath.Join(directory, "mender-provider.json"), original, 0o400); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(directory, "bin", "echo")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\necho ok\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(executable, 0o500); err != nil {
		t.Fatal(err)
	} // #nosec G302 -- executable fixture requires executable permission.
	// Existing records are migrated into the generic descriptor while their
	// exact source tree and digest remain pinned to the original bytes.
	generic := []byte(`{"manifestVersion":2,"pluginId":"dev.example.echo","version":"1.2.3","displayName":"Old","entrypoints":{"` + runtime.GOOS + `/` + runtime.GOARCH + `":"bin/echo"},"extensions":[{"id":"main","kind":"provider.tools","protocolVersion":1}]}`)
	create := func(digest string) installationdomain.Installation {
		t.Helper()
		item, err := installationdomain.NewInstallation(
			id, platformplugin.ID("dev.example.echo"), "1.2.3", installationdomain.SourceLocal,
			"/tmp/old-source", digest, generic, installationdomain.StatusEnabled, time.Now().UTC(),
		)
		if err != nil {
			t.Fatal(err)
		}
		item, err = item.WithArtifactManifestName("mender-provider.json")
		if err != nil {
			t.Fatal(err)
		}
		return item
	}
	staging := create("sha256:" + strings.Repeat("a", 64))
	snapshot, err := store.inspect(ctx, directory, &staging)
	if err != nil {
		t.Fatal(err)
	}
	item := create(snapshot.digest)
	resolved, err := store.Resolve(ctx, item)
	if err != nil {
		t.Fatalf("old Provider package no longer resolves: %v", err)
	}
	if resolved.EntrypointPath != executable || len(resolved.ExecutableChecksum) != 32 {
		t.Fatalf("old verified package = %+v", resolved)
	}
	if err := os.Chmod(filepath.Join(directory, "mender-provider.json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "mender-provider.json"), []byte("{}"), 0o400); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Resolve(ctx, item); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("modified old manifest bypassed integrity verification: %v", err)
	}
}
