package bootstrap

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/orz-i/Mender/backend/internal/providerplugin/manifest"
)

func TestInstallProviderPluginCompensatesFailedExecutableProbe(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := bootstrapRepositoryRoot(t)
	temp := t.TempDir()
	managedRoot := filepath.Join(temp, "managed-plugins")
	sourceRoot := filepath.Join(temp, "invalid-plugin")
	entrypoint := filepath.Join(sourceRoot, "bin", examplePluginBinaryName())
	if err := os.MkdirAll(filepath.Dir(entrypoint), 0o700); err != nil {
		t.Fatal(err)
	}

	exampleRoot := filepath.Join(root, "examples", "provider-plugin-example")
	// #nosec G204 -- test builds a fixed repository example; only output is under t.TempDir.
	command := exec.CommandContext(
		ctx,
		"go",
		"build",
		"-o",
		entrypoint,
		"./cmd/mender-provider-example",
	)
	command.Dir = exampleRoot
	command.Env = append(os.Environ(), "GOWORK=off")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build external plugin example: %v\n%s", err, output)
	}

	rawManifest, err := os.ReadFile(filepath.Join(exampleRoot, manifest.FileName)) // #nosec G304 -- fixed repository example manifest path.
	if err != nil {
		t.Fatal(err)
	}
	var manifestValue map[string]any
	if err := json.Unmarshal(rawManifest, &manifestValue); err != nil {
		t.Fatal(err)
	}
	manifestValue["version"] = "9.9.9"
	rawManifest, err = json.Marshal(manifestValue)
	if err != nil {
		t.Fatal(err)
	}
	// #nosec G703 -- destination is a fixed filename under t.TempDir.
	if err := os.WriteFile(
		filepath.Join(sourceRoot, manifest.FileName),
		rawManifest,
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	runtime, err := NewRuntime(ctx, RuntimeOptions{
		DatabasePath:       filepath.Join(temp, "mender.db"),
		ProviderPluginRoot: managedRoot,
		DefaultTimeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	if _, err := runtime.InstallProviderPlugin(ctx, sourceRoot); err == nil {
		t.Fatal("InstallProviderPlugin() error = nil")
	}

	installations, err := runtime.ListProviderPluginInstallations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(installations) != 0 {
		t.Fatalf("installations after failed probe = %+v", installations)
	}

	entries, err := os.ReadDir(managedRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("managed plugin root after compensation = %+v", entries)
	}
}
