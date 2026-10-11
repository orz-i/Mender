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

	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	credentialapplication "github.com/orz-i/Mender/backend/internal/credential/application"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	installationapplication "github.com/orz-i/Mender/backend/internal/plugin/installation/application"
	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
	"github.com/orz-i/Mender/backend/internal/plugin/manifest"
	runtimegovernancedomain "github.com/orz-i/Mender/backend/internal/runtimegovernance/domain"
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

	t.Setenv("MENDER_EXTERNAL_ECHO_TOKEN", "external-echo-secret")
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
	verifiedDescriptor, ok := installation.VerifiedDescriptor()
	if !ok || verifiedDescriptor.PluginID != pluginID || verifiedDescriptor.Extension.ProtocolVersion != 1 {
		t.Fatalf("installed verified descriptor = %+v, present=%v", verifiedDescriptor, ok)
	}
	if runtime.externalProcesses.Running(installationID) {
		t.Fatal("installation verification left plugin process running")
	}

	if runtime.RuntimeGovernance.processIsolation.Info().RestrictedHostAvailable {
		currentIsolation, err := runtime.RuntimeGovernance.GetIsolation.Execute(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := runtime.RuntimeGovernance.UpdateIsolation(
			ctx,
			runtimegovernancedomain.IsolationSpec{
				WorkspaceShell:   currentIsolation.WorkspaceShell(),
				ExternalProvider: runtimegovernancedomain.IsolationRestrictedHost,
				TunnelClient:     currentIsolation.TunnelClient(),
			},
		); err != nil {
			t.Fatal(err)
		}
	}

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
	if runtime.externalProcesses.Running(installationID) {
		t.Fatal("listing verified plugin catalog started external process")
	}
	if external.SourceKind != ProviderPluginSourceExternal ||
		external.Descriptor.ID != pluginID ||
		external.Descriptor.ProtocolVersion != 1 ||
		len(external.Descriptor.ConfigSchema) == 0 {
		t.Fatalf("external available plugin = %+v", *external)
	}

	credential, err := runtime.Credentials.CreateEnvironment.Execute(
		ctx,
		credentialapplication.CreateEnvironmentRequest{
			Namespace:           "external-echo",
			Name:                "token",
			DisplayName:         "External Echo Token",
			EnvironmentVariable: "MENDER_EXTERNAL_ECHO_TOKEN",
		},
	)
	if err != nil {
		t.Fatalf("create external Provider Credential: %v", err)
	}
	credentialRef := credential.Reference()
	providerConfig := []byte(
		"{\"prefix\":\"test: \",\"credentialRef\":\"" + credentialRef.String() + "\"}",
	)
	provider, err := registerInstalledProviderTest(t, runtime,
		ctx,
		catalogapplication.RegisterProviderRequest{
			Namespace:   "external-echo",
			DisplayName: "External Echo",
			Description: "Runtime external Provider fixture",
		},
		pluginID,
		installationID,
		providerConfig,
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
	configMetadata, ok := config.ConfigMetadata()
	if !ok || len(configMetadata.CredentialRefs) != 1 ||
		configMetadata.CredentialRefs[0] != credentialRef ||
		configMetadata.ConnectionSummary != "local example echo plugin" {
		t.Fatalf("ProviderConfig metadata = %+v, present=%v", configMetadata, ok)
	}

	syncResult, err := syncConfiguredProviderTest(t, runtime, ctx, provider.ID())
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
	originalToolID := search[0].ToolID

	rebindSourceRoot := filepath.Join(temp, "source-plugin-rebind")
	rebindEntrypoint := filepath.Join(
		rebindSourceRoot,
		"bin",
		examplePluginBinaryName(),
	)
	if err := os.MkdirAll(filepath.Dir(rebindEntrypoint), 0o700); err != nil {
		t.Fatal(err)
	}
	binary, err := os.ReadFile(entrypoint) // #nosec G304 -- test-owned build output path.
	if err != nil {
		t.Fatal(err)
	}
	rebindRoot, err := os.OpenRoot(rebindSourceRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rebindRoot.Close() }()
	if err := rebindRoot.WriteFile(
		filepath.ToSlash(filepath.Join("bin", examplePluginBinaryName())),
		binary,
		0o755,
	); err != nil { // #nosec G306 -- test-owned executable fixture under t.TempDir.
		t.Fatal(err)
	}
	if err := rebindRoot.WriteFile(manifest.FileName, manifestJSON, 0o600); err != nil { // #nosec G306 -- test fixture manifest.
		t.Fatal(err)
	}
	if err := rebindRoot.WriteFile(
		"installation-marker.txt",
		[]byte("second installation"),
		0o600,
	); err != nil { // #nosec G306 -- test fixture data.
		t.Fatal(err)
	}
	targetInstallation, err := runtime.InstallProviderPlugin(ctx, rebindSourceRoot)
	if err != nil {
		t.Fatalf("install rebind target: %v", err)
	}
	if targetInstallation.ID() == installationID {
		t.Fatal("rebind target reused source InstallationID")
	}
	if _, err := rebindProviderInstallationTest(t, runtime,
		ctx,
		provider.ID(),
		targetInstallation.ID(),
		[]byte(`{"credentialRef":"not-a-credential-reference"}`),
	); err == nil {
		t.Fatal("RebindProviderInstallation(invalid config) error = nil")
	}
	failedRebindConfig, err := runtime.ProviderConfig(ctx, provider.ID())
	if err != nil {
		t.Fatal(err)
	}
	if failedRebindConfig.InstallationID == nil ||
		*failedRebindConfig.InstallationID != installationID {
		t.Fatalf("failed rebind changed installation pin: %+v", failedRebindConfig)
	}
	failedRebindSearch, err := runtime.Catalog.SearchTools.Execute(
		ctx,
		catalogapplication.SearchToolsRequest{Query: "echo", Limit: 10},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(failedRebindSearch) != 1 || failedRebindSearch[0].ToolID != originalToolID {
		t.Fatalf(
			"failed rebind changed Tool identity: before=%s after=%+v",
			originalToolID,
			failedRebindSearch,
		)
	}
	if !runtime.executors.HasProvider(provider.ID()) {
		t.Fatal("failed rebind removed current execution route")
	}

	rebindResult, err := rebindProviderInstallationTest(t, runtime,
		ctx,
		provider.ID(),
		targetInstallation.ID(),
		providerConfig,
	)
	if err != nil {
		t.Fatalf("RebindProviderInstallation() error = %v", err)
	}
	if rebindResult.Total != 1 || rebindResult.Updated != 1 {
		t.Fatalf("rebind sync result = %+v", rebindResult)
	}
	reboundConfig, err := runtime.ProviderConfig(ctx, provider.ID())
	if err != nil {
		t.Fatal(err)
	}
	if reboundConfig.InstallationID == nil ||
		*reboundConfig.InstallationID != targetInstallation.ID() ||
		reboundConfig.PluginID != pluginID {
		t.Fatalf("rebound ProviderConfig() = %+v", reboundConfig)
	}
	reboundSearch, err := runtime.Catalog.SearchTools.Execute(
		ctx,
		catalogapplication.SearchToolsRequest{Query: "echo", Limit: 10},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(reboundSearch) != 1 || reboundSearch[0].ToolID != originalToolID {
		t.Fatalf(
			"rebind changed stable Tool identity: before=%s after=%+v",
			originalToolID,
			reboundSearch,
		)
	}
	if err := runtime.RemoveProviderPluginInstallation(ctx, installationID); err != nil {
		t.Fatalf("remove unreferenced old installation after rebind: %v", err)
	}
	installation = targetInstallation
	installationID = targetInstallation.ID()
	search = reboundSearch

	invocationContext, err := governancedomain.NewContext(
		governancedomain.OriginInternal,
		testPrincipal(t, "external-runtime-test"),
		governancedomain.CatalogScope(),
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

	if _, err := updateConfiguredProviderTest(t, runtime,
		ctx,
		provider.ID(),
		catalogapplication.UpdateProviderRequest{
			DisplayName: "External Echo Updated",
			Description: "Still pinned",
		},
		providerConfig,
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
	if err := runtime.Credentials.Delete.Execute(ctx, credentialRef); !errors.Is(
		err,
		credentialapplication.ErrCredentialInUse,
	) {
		t.Fatalf("delete Credential while plugin disabled error = %v", err)
	}
	if runtime.externalProcesses.Running(installationID) {
		t.Fatal("offline Credential usage check restarted disabled plugin process")
	}
	if err := activateConfiguredProviderTest(t, runtime, ctx, provider.ID()); err == nil ||
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
	if _, err := syncConfiguredProviderTest(t, runtime, ctx, provider.ID()); err != nil {
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
