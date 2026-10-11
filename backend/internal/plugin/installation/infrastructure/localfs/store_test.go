package localfs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	installationapplication "github.com/orz-i/Mender/backend/internal/plugin/installation/application"
	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
	"github.com/orz-i/Mender/backend/internal/plugin/manifest"
)

func TestInstallLocalCopiesAndHardensPackage(t *testing.T) {
	t.Parallel()

	source := writeTestPackage(t, t.TempDir(), "dev.example.echo", "1.2.3", 0o755)
	managedRoot := filepath.Join(t.TempDir(), "plugins")
	store, err := newStore(
		managedRoot,
		testGOOS(),
		"amd64",
		DefaultLimits(),
	)
	if err != nil {
		t.Fatal(err)
	}
	id := testInstallationID(t, "01")

	pkg, err := store.InstallLocal(context.Background(), source, id)
	if err != nil {
		t.Fatalf("InstallLocal() error = %v", err)
	}
	if pkg.Manifest.PluginID != "dev.example.echo" ||
		pkg.Manifest.Version != "1.2.3" ||
		!strings.HasPrefix(pkg.Digest, "sha256:") {
		t.Fatalf("InstallLocal() = %+v", pkg)
	}
	absoluteSource, _ := filepath.Abs(source)
	if pkg.SourceReference != absoluteSource {
		t.Fatalf("SourceReference = %q, want %q", pkg.SourceReference, absoluteSource)
	}
	installation := installationFromManagedPackage(t, id, pkg)
	resolved, err := store.Resolve(context.Background(), installation)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.EntrypointPath != filepath.Join(managedRoot, id.String(), "bin", "plugin") ||
		resolved.WorkingDirectory != filepath.Join(managedRoot, id.String()) ||
		len(resolved.ExecutableChecksum) != 32 {
		t.Fatalf("Resolve() = %+v", resolved)
	}

	managedPlugin := filepath.Join(managedRoot, id.String(), "bin", "plugin")
	info, err := os.Stat(managedPlugin)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o500 {
		t.Fatalf("managed executable mode = %#o, want 0500", info.Mode().Perm())
	}
	dataInfo, err := os.Stat(filepath.Join(managedRoot, id.String(), "data.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && dataInfo.Mode().Perm() != 0o400 {
		t.Fatalf("managed data mode = %#o, want 0400", dataInfo.Mode().Perm())
	}

	if err := os.WriteFile(filepath.Join(source, "data.txt"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	managedPackage, err := os.OpenRoot(filepath.Join(managedRoot, id.String()))
	if err != nil {
		t.Fatal(err)
	}
	managedData, err := managedPackage.ReadFile("data.txt")
	_ = managedPackage.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(managedData) != "payload" {
		t.Fatalf("managed data = %q", managedData)
	}

	if err := store.Remove(context.Background(), id); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if _, err := os.Lstat(filepath.Join(managedRoot, id.String())); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("managed package still exists: %v", err)
	}
	if err := store.Remove(context.Background(), id); err != nil {
		t.Fatalf("Remove(idempotent) error = %v", err)
	}
}

func TestResolveRejectsMutatedManagedPackage(t *testing.T) {
	t.Parallel()

	source := writeTestPackage(t, t.TempDir(), "dev.example.echo", "1.0.0", 0o755)
	managedRoot := filepath.Join(t.TempDir(), "plugins")
	store, err := newStore(managedRoot, testGOOS(), "amd64", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	id := testInstallationID(t, "09")
	t.Cleanup(func() { _ = store.Remove(context.Background(), id) })
	pkg, err := store.InstallLocal(context.Background(), source, id)
	if err != nil {
		t.Fatal(err)
	}
	installation := installationFromManagedPackage(t, id, pkg)
	managedData := filepath.Join(managedRoot, id.String(), "data.txt")
	if err := os.Chmod(managedData, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(managedData, []byte("mutated"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(managedData, 0o400); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Resolve(context.Background(), installation); err == nil ||
		!strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("Resolve() error = %v", err)
	}
}

func TestInstallLocalDigestIsDeterministicForEquivalentTrees(t *testing.T) {
	t.Parallel()

	sourceA := writeTestPackage(t, t.TempDir(), "dev.example.echo", "1.0.0", 0o755)
	sourceB := writeTestPackage(t, t.TempDir(), "dev.example.echo", "1.0.0", 0o755)
	store, err := newStore(
		filepath.Join(t.TempDir(), "plugins"),
		testGOOS(),
		"amd64",
		DefaultLimits(),
	)
	if err != nil {
		t.Fatal(err)
	}

	firstID := testInstallationID(t, "01")
	secondID := testInstallationID(t, "02")
	t.Cleanup(func() {
		_ = store.Remove(context.Background(), firstID)
		_ = store.Remove(context.Background(), secondID)
	})
	first, err := store.InstallLocal(context.Background(), sourceA, firstID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.InstallLocal(context.Background(), sourceB, secondID)
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest != second.Digest {
		t.Fatalf("equivalent package digests differ: %s != %s", first.Digest, second.Digest)
	}
}

func TestInstallLocalRejectsSymlinkAndNonExecutableEntrypoint(t *testing.T) {
	t.Parallel()

	t.Run("symlink", func(t *testing.T) {
		source := writeTestPackage(t, t.TempDir(), "dev.example.echo", "1.0.0", 0o755)
		if err := os.Symlink("data.txt", filepath.Join(source, "link")); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		store := testStore(t, DefaultLimits())
		_, err := store.InstallLocal(context.Background(), source, testInstallationID(t, "03"))
		if err == nil || !strings.Contains(err.Error(), "symbolic links") {
			t.Fatalf("InstallLocal() error = %v", err)
		}
	})

	t.Run("non-executable", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Windows does not model executable permission bits")
		}
		source := writeTestPackage(t, t.TempDir(), "dev.example.echo", "1.0.0", 0o644)
		store := testStore(t, DefaultLimits())
		_, err := store.InstallLocal(context.Background(), source, testInstallationID(t, "04"))
		if err == nil || !strings.Contains(err.Error(), "not executable") {
			t.Fatalf("InstallLocal() error = %v", err)
		}
	})
}

func TestInstallLocalEnforcesPackageLimits(t *testing.T) {
	t.Parallel()

	source := writeTestPackage(t, t.TempDir(), "dev.example.echo", "1.0.0", 0o755)

	store := testStore(t, Limits{MaxFiles: 1, MaxBytes: defaultMaxBytes})
	if _, err := store.InstallLocal(
		context.Background(),
		source,
		testInstallationID(t, "05"),
	); err == nil || !strings.Contains(err.Error(), "more than 1 files") {
		t.Fatalf("file limit error = %v", err)
	}

	store = testStore(t, Limits{MaxFiles: defaultMaxFiles, MaxBytes: 32})
	if _, err := store.InstallLocal(
		context.Background(),
		source,
		testInstallationID(t, "06"),
	); err == nil || !strings.Contains(err.Error(), "exceeds 32 bytes") {
		t.Fatalf("byte limit error = %v", err)
	}
}

func TestInstallLocalRejectsSourceInsideManagedRoot(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "plugins")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	source := writeTestPackage(t, root, "dev.example.echo", "1.0.0", 0o755)
	store, err := newStore(root, testGOOS(), "amd64", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.InstallLocal(
		context.Background(),
		source,
		testInstallationID(t, "07"),
	); err == nil || !strings.Contains(err.Error(), "outside managed plugin root") {
		t.Fatalf("InstallLocal() error = %v", err)
	}
}

func TestInstallLocalHonorsCancelledContext(t *testing.T) {
	t.Parallel()

	source := writeTestPackage(t, t.TempDir(), "dev.example.echo", "1.0.0", 0o755)
	store := testStore(t, DefaultLimits())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := store.InstallLocal(ctx, source, testInstallationID(t, "08"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("InstallLocal() error = %v, want context.Canceled", err)
	}
}

func testStore(t *testing.T, limits Limits) *Store {
	t.Helper()
	store, err := newStore(
		filepath.Join(t.TempDir(), "plugins"),
		testGOOS(),
		"amd64",
		limits,
	)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func writeTestPackage(
	t *testing.T,
	parent string,
	pluginID string,
	version string,
	entrypointMode os.FileMode,
) string {
	t.Helper()

	source := filepath.Join(parent, "source-"+strings.ReplaceAll(version, ".", "-"))
	if err := os.MkdirAll(filepath.Join(source, "bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	rawManifest := []byte(`{
		"manifestVersion": 2,
		"pluginId": "` + pluginID + `",
		"version": "` + version + `",
		"extensions": [{"id":"main","kind":"provider.tools","protocolVersion":1}],
		"displayName": "Echo",
		"description": "Example",
		"entrypoints": {
			"linux/amd64": "bin/plugin",
			"windows/amd64": "bin/plugin"
		}
	}`)
	if err := os.WriteFile(
		filepath.Join(source, manifest.FileName),
		rawManifest,
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(source, "bin", "plugin"),
		[]byte("#!/bin/sh\necho ok\n"),
		entrypointMode,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(source, "data.txt"),
		[]byte("payload"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	return source
}

func installationFromManagedPackage(
	t *testing.T,
	id installationdomain.InstallationID,
	pkg installationapplication.ManagedPackage,
) installationdomain.Installation {
	t.Helper()
	pluginID, err := platformplugin.ParseID(pkg.Manifest.PluginID)
	if err != nil {
		t.Fatal(err)
	}
	item, err := installationdomain.NewInstallation(
		id,
		pluginID,
		pkg.Manifest.Version,
		installationdomain.SourceLocal,
		pkg.SourceReference,
		pkg.Digest,
		pkg.ManifestJSON,
		installationdomain.StatusEnabled,
		time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	return item
}

func testGOOS() string {
	if runtime.GOOS == "windows" {
		return "windows"
	}
	return "linux"
}

func testInstallationID(t *testing.T, suffix string) installationdomain.InstallationID {
	t.Helper()
	raw := "pinst_" + strings.Repeat("0", 30) + suffix
	id, err := installationdomain.ParseInstallationID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
