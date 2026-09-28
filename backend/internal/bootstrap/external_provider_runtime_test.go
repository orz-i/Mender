package bootstrap

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"testing"
	"time"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
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
	entrypoint := filepath.Join(sourceRoot, "bin", externalFixtureBinaryName())
	if err := os.MkdirAll(filepath.Dir(entrypoint), 0o700); err != nil {
		t.Fatal(err)
	}

	// #nosec G204 -- test builds a fixed repository fixture; only the output path is under t.TempDir.
	command := exec.CommandContext(
		ctx,
		"go",
		"build",
		"-o",
		entrypoint,
		"./backend/internal/bootstrap/testdata/externalplugin",
	)
	command.Dir = root
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("build external plugin fixture: %v\n%s", err, output)
	}

	manifestJSON := []byte(fmt.Sprintf(
		"{\n"+
			"  \"manifestVersion\": 1,\n"+
			"  \"pluginId\": \"dev.mender.test.echo\",\n"+
			"  \"version\": \"1.0.0\",\n"+
			"  \"protocolVersions\": [1],\n"+
			"  \"displayName\": \"External Echo\",\n"+
			"  \"description\": \"External runtime integration fixture\",\n"+
			"  \"entrypoints\": {%q: %q}\n"+
			"}",
		goruntime.GOOS+"/"+goruntime.GOARCH,
		"bin/"+externalFixtureBinaryName(),
	))
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

	installationID, err := providerplugin.ParseInstallationID(
		"pinst_0123456789abcdef0123456789abcdef",
	)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := runtime.pluginPackages.InstallLocal(ctx, sourceRoot, installationID)
	if err != nil {
		t.Fatalf("InstallLocal() error = %v", err)
	}
	pluginID, err := providerplugin.ParseID(pkg.Manifest.PluginID)
	if err != nil {
		t.Fatal(err)
	}
	installation, err := installationdomain.NewInstallation(
		installationID,
		pluginID,
		pkg.Manifest.Version,
		installationdomain.SourceLocal,
		pkg.SourceReference,
		pkg.Digest,
		pkg.ManifestJSON,
		installationdomain.StatusEnabled,
		time.Now().UTC(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.pluginInstallations.Create(ctx, installation); err != nil {
		t.Fatalf("persist installation: %v", err)
	}

	provider, err := runtime.RegisterInstalledProvider(
		ctx,
		catalogapplication.RegisterProviderRequest{
			Namespace:   "external-echo",
			DisplayName: "External Echo",
			Description: "Runtime external Provider fixture",
		},
		installationID,
		[]byte("{}"),
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
	if response.Outcome.Kind() != invocationdomain.OutcomeKindComplete ||
		!bytes.Equal(response.Outcome.Content().Bytes(), arguments) {
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
		[]byte("{}"),
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

	if err := runtime.Close(); err != nil {
		t.Fatalf("Runtime.Close() error = %v", err)
	}
	closed = true
	if err := runtime.pluginPackages.Remove(context.Background(), installationID); err != nil {
		t.Fatalf("remove managed external plugin package: %v", err)
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

func externalFixtureBinaryName() string {
	if goruntime.GOOS == "windows" {
		return "external-plugin.exe"
	}
	return "external-plugin"
}
