package bootstrap

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"
	"time"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	installationapplication "github.com/orz-i/Mender/backend/internal/providerplugin/installation/application"
	installationdomain "github.com/orz-i/Mender/backend/internal/providerplugin/installation/domain"
	"github.com/orz-i/Mender/backend/internal/providerplugin/manifest"
)

func TestRuntimeInstalledExternalProviderEndToEnd(t *testing.T) {
	ctx := context.Background()
	root := bootstrapRepositoryRoot(t)
	temp := t.TempDir()
	databasePath := filepath.Join(temp, "mender.db")
	managedRoot := filepath.Join(temp, "managed-plugins")
	sourceRoot := filepath.Join(temp, "source-plugin")
	entrypoint := filepath.Join(sourceRoot, "bin", examplePluginBinaryName())
	if err := os.MkdirAll(filepath.Dir(entrypoint), 0o700); err != nil {
		t.Fatal(err)
	}

	exampleRoot := filepath.Join(root, "examples", "provider-plugin-example")
	// #nosec G204 -- test builds a fixed repository example; only the output path is under t.TempDir.
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
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("build external plugin example: %v\n%s", err, output)
	}

	manifestJSON, err := os.ReadFile(filepath.Join(exampleRoot, manifest.FileName)) // #nosec G304 -- fixed repository example manifest path.
	if err != nil {
		t.Fatalf("read example manifest: %v", err)
	}
	// #nosec G703 -- destination is a fixed filename under t.TempDir.
	if err := os.WriteFile(
		filepath.Join(sourceRoot, manifest.FileName),
		manifestJSON,
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	runtime, err := NewRuntime(ctx, RuntimeOptions{
		DatabasePath:       databasePath,
		ProviderPluginRoot: managedRoot,
		DefaultTimeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	closed := false
	t.Cleanup(func() {
		if !closed {
			if err := runtime.Close(); err != nil {
				t.Errorf("Runtime.Close() error = %v", err)
			}
		}
	})

	installation, err := runtime.InstallProviderPlugin(ctx, sourceRoot)
	if err != nil {
		t.Fatalf("InstallProviderPlugin() error = %v", err)
	}
	installationID := installation.ID()
	pluginID := installation.PluginID()

	available, err := runtime.ListAvailableProviderPlugins(ctx)
	if err != nil {
		t.Fatalf("ListAvailableProviderPlugins() error = %v", err)
	}
	var external *AvailableProviderPlugin
	for index := range available {
		item := &available[index]
		if item.InstallationID != nil && *item.InstallationID == installationID {
			external = item
			break
		}
	}
	if external == nil {
		t.Fatalf("external installation %s missing from plugin catalog", installationID)
	}
	if external.SourceKind != ProviderPluginSourceExternal ||
		external.Descriptor.ID != pluginID ||
		external.Descriptor.ProtocolVersion != 1 ||
		len(external.Descriptor.ConfigSchema) == 0 {
		t.Fatalf("external available plugin = %+v", *external)
	}

	provider, err := runtime.RegisterInstalledProvider(
		ctx,
		catalogapplication.RegisterProviderRequest{
			Namespace:   "external-echo",
			DisplayName: "External Echo",
			Description: "Runtime external Provider fixture",
		},
		pluginID,
		installationID,
		[]byte("{\"prefix\":\"test: \"}"),
	)
	if err != nil {
		t.Fatalf("RegisterInstalledProvider() error = %v", err)
	}
	config, err := runtime.ProviderConfig(ctx, provider.ID())
	if err != nil {
		t.Fatal(err)
	}
	if config.InstallationID == nil ||
		*config.InstallationID != installationID ||
		config.PluginID != pluginID {
		t.Fatalf("ProviderConfig() = %+v", config)
	}

	syncResult, err := runtime.SyncConfiguredProvider(ctx, provider.ID())
	if err != nil {
		t.Fatalf("SyncConfiguredProvider() error = %v", err)
	}
	if syncResult.Total != 1 || syncResult.Created != 1 {
		t.Fatalf("sync result = %+v", syncResult)
	}

	search, err := runtime.Catalog.SearchTools.Execute(
		ctx,
		catalogapplication.SearchToolsRequest{
			Query: "echo",
			Limit: 10,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(search) != 1 {
		t.Fatalf("SearchTools = %+v", search)
	}

	invocationContext, err := invocationdomain.NewInvocationContext(
		invocationdomain.OriginInternal,
		"external-runtime-test",
		invocationdomain.CatalogExecutionScope(),
	)
	if err != nil {
		t.Fatal(err)
	}
	arguments := []byte("{\"value\":\"from-external-process\"}")
	response, err := runtime.InvokeTool.Execute(
		ctx,
		invocationapplication.InvokeToolRequest{
			ToolID:    search[0].ToolID,
			Arguments: arguments,
			Context:   invocationContext,
		},
	)
	if err != nil {
		t.Fatalf("InvokeTool.Execute() error = %v", err)
	}
	expected := []byte("{\"echo\":\"test: from-external-process\"}")
	if response.Outcome.Kind() != invocationdomain.OutcomeKindComplete ||
		!bytes.Equal(response.Outcome.Content().Bytes(), expected) {
		t.Fatalf(
			"external outcome kind=%q content=%s",
			response.Outcome.Kind(),
			response.Outcome.Content().Bytes(),
		)
	}

	if _, err := runtime.UpdateConfiguredProvider(
		ctx,
		provider.ID(),
		catalogapplication.UpdateProviderRequest{
			DisplayName: "External Echo Updated",
			Description: "Still pinned",
		},
		[]byte("{\"prefix\":\"test: \"}"),
	); err != nil {
		t.Fatalf("UpdateConfiguredProvider() error = %v", err)
	}
	updatedConfig, err := runtime.ProviderConfig(ctx, provider.ID())
	if err != nil {
		t.Fatal(err)
	}
	if updatedConfig.InstallationID == nil ||
		*updatedConfig.InstallationID != installationID {
		t.Fatalf(
			"UpdateConfiguredProvider() lost installation pin: %+v",
			updatedConfig,
		)
	}

	disabled, err := runtime.SetProviderPluginInstallationStatus(
		ctx,
		installationID,
		installationdomain.StatusDisabled,
	)
	if err != nil {
		t.Fatalf("disable Provider plugin installation: %v", err)
	}
	if disabled.Status() != installationdomain.StatusDisabled {
		t.Fatalf("disabled status = %q", disabled.Status())
	}
	if runtime.executors.HasProvider(provider.ID()) {
		t.Fatal("disabled installation left active executor route")
	}
	if err := runtime.EnsureConfiguredProvider(ctx, provider.ID()); err == nil ||
		!strings.Contains(err.Error(), "disabled") {
		t.Fatalf("EnsureConfiguredProvider(disabled) error = %v", err)
	}

	enabled, err := runtime.SetProviderPluginInstallationStatus(
		ctx,
		installationID,
		installationdomain.StatusEnabled,
	)
	if err != nil {
		t.Fatalf("enable Provider plugin installation: %v", err)
	}
	if enabled.Status() != installationdomain.StatusEnabled {
		t.Fatalf("enabled status = %q", enabled.Status())
	}
	if runtime.executors.HasProvider(provider.ID()) {
		t.Fatal("enable unexpectedly activated Provider before sync")
	}
	if _, err := runtime.SyncConfiguredProvider(ctx, provider.ID()); err != nil {
		t.Fatalf("SyncConfiguredProvider(after enable) error = %v", err)
	}
	if !runtime.executors.HasProvider(provider.ID()) {
		t.Fatal("sync after enable did not restore executor route")
	}

	if err := runtime.RemoveProviderPluginInstallation(ctx, installationID); !errors.Is(
		err,
		installationapplication.ErrInstallationInUse,
	) {
		t.Fatalf("RemoveProviderPluginInstallation(in use) error = %v", err)
	}
	process, err := runtime.externalProcesses.Ensure(ctx, installation)
	if err != nil {
		t.Fatalf("ensure external process before Runtime.Close(): %v", err)
	}
	if err := runtime.DeleteConfiguredProvider(ctx, provider.ID()); err != nil {
		t.Fatalf("DeleteConfiguredProvider() error = %v", err)
	}
	if process.Exited() {
		t.Fatal("deleting Provider unexpectedly terminated shared package process")
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("Runtime.Close() error = %v", err)
	}
	closed = true
	if !process.Exited() {
		t.Fatal("Runtime.Close() left external plugin process running")
	}
	if err := runtime.pluginPackages.Remove(context.Background(), installationID); err != nil {
		t.Fatalf("remove managed package after Runtime.Close(): %v", err)
	}
}

func bootstrapRepositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func examplePluginBinaryName() string {
	if goruntime.GOOS == "windows" {
		return "mender-provider-example.exe"
	}
	return "mender-provider-example"
}
