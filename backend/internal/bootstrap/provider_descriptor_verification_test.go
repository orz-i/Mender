package bootstrap

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	installationapplication "github.com/orz-i/Mender/backend/internal/plugin/installation/application"
	installationidentity "github.com/orz-i/Mender/backend/internal/plugin/installation/infrastructure/identity"
	"github.com/orz-i/Mender/backend/internal/plugin/manifest"
)

func TestDescriptorVerificationCannotCloseRuntimeOwnedPluginProcess(t *testing.T) {
	ctx := context.Background()
	temp := t.TempDir()
	source := filepath.Join(temp, "source")
	entrypoint := filepath.Join(source, "bin", examplePluginBinaryName())
	if err := os.MkdirAll(filepath.Dir(entrypoint), 0o700); err != nil {
		t.Fatal(err)
	}
	example := filepath.Join(bootstrapRepositoryRoot(t), "examples", "provider-plugin-example")
	// #nosec G204 -- fixed repository fixture, with output under t.TempDir.
	build := exec.CommandContext(ctx, "go", "build", "-o", entrypoint, "./cmd/mender-provider-example")
	build.Dir = example
	build.Env = append(os.Environ(), "GOWORK=off")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build example: %v\n%s", err, output)
	}
	raw, err := os.ReadFile(filepath.Join(example, manifest.FileName)) // #nosec G304 -- fixed repository fixture.
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, manifest.FileName), raw, 0o600); err != nil { // #nosec G703 -- test-owned fixture directory.
		t.Fatal(err)
	}
	runtime, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(temp, "runtime.db"), ProviderPluginRoot: filepath.Join(temp, "plugins")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	// Model the runtime-owned process that can be admitted immediately after
	// another verifier publishes the first descriptor. The verification owner
	// must never share or tear down this process registry.
	item, err := installationapplication.NewInstallLocal(runtime.pluginInstallations, runtime.pluginPackages,
		installationidentity.NewGenerator(), systemClock{}).Execute(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	process, err := runtime.externalProcesses.Ensure(ctx, item)
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := runtime.VerifyProviderPluginInstallation(ctx, item.ID())
			results <- err
		}()
	}
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if process.Exited() || !runtime.externalProcesses.Running(item.ID()) {
		t.Fatal("descriptor verification closed the runtime-owned plugin process")
	}
	if _, err := runtime.VerifyProviderPluginInstallation(ctx, item.ID()); err != nil {
		t.Fatal(err)
	}
	if process.Exited() {
		t.Fatal("already verified snapshot verification restarted or closed runtime process")
	}
}
