package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	credentialapplication "github.com/orz-i/Mender/backend/internal/credential/application"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	modeltoolsplugin "github.com/orz-i/Mender/backend/internal/modeltools/infrastructure/plugin"
	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

func TestModelToolsProviderSyncIsLocalAndProfileCredentialScopeBlocksExecution(
	t *testing.T,
) {
	t.Parallel()

	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "mender.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	// Intentionally do not set this environment variable. A Provider sync that
	// incorrectly resolves the Credential would fail.
	credential, err := runtime.Credentials.CreateEnvironment.Execute(
		ctx,
		credentialapplication.CreateEnvironmentRequest{
			Namespace:           "model-tools",
			Name:                "api",
			DisplayName:         "Model API",
			EnvironmentVariable: "MENDER_MODEL_TOOLS_TEST_KEY_UNSET",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	ref := credential.Reference()

	raw, err := json.Marshal(modeltoolsplugin.Config{
		Engine:        modeltoolsplugin.EngineOpenAI,
		Model:         "gpt-test",
		CredentialRef: ref,
		Capabilities: []modeltoolsplugin.Capability{
			modeltoolsplugin.CapabilityWebSearch,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	provider, err := registerConfiguredProviderTest(t, runtime,
		ctx,
		catalogapplication.RegisterProviderRequest{
			Namespace:   "research",
			DisplayName: "Research Search",
		},
		modeltoolsplugin.ID,
		raw,
	)
	if err != nil {
		t.Fatalf("RegisterConfiguredProvider() error = %v", err)
	}

	syncResult, err := syncConfiguredProviderTest(t, runtime, ctx, provider.ID())
	if err != nil {
		t.Fatalf(
			"SyncConfiguredProvider() resolved Credential or contacted provider unexpectedly: %v",
			err,
		)
	}
	if syncResult.Created != 1 || syncResult.Total != 1 {
		t.Fatalf("initial sync = %+v", syncResult)
	}

	search, err := runtime.Catalog.SearchTools.Execute(
		ctx,
		catalogapplication.SearchToolsRequest{
			Query:      "web search",
			Limit:      10,
			ProviderID: providerIDPtr(provider.ID()),
		},
	)
	if err != nil || len(search) != 1 {
		t.Fatalf("search = %+v err=%v", search, err)
	}
	toolID := search[0].ToolID

	if err := runtime.Credentials.Delete.Execute(ctx, ref); !errors.Is(
		err,
		credentialapplication.ErrCredentialInUse,
	) {
		t.Fatalf("delete Provider-bound Credential error = %v", err)
	}

	updatedRaw, err := json.Marshal(modeltoolsplugin.Config{
		Engine:        modeltoolsplugin.EngineOpenAI,
		Model:         "gpt-test-v2",
		CredentialRef: ref,
		Capabilities: []modeltoolsplugin.Capability{
			modeltoolsplugin.CapabilityWebSearch,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := updateConfiguredProviderTest(t, runtime,
		ctx,
		provider.ID(),
		catalogapplication.UpdateProviderRequest{
			DisplayName: "Research Search v2",
		},
		updatedRaw,
	); err != nil {
		t.Fatalf("UpdateConfiguredProvider() error = %v", err)
	}
	syncResult, err = syncConfiguredProviderTest(t, runtime, ctx, provider.ID())
	if err != nil {
		t.Fatalf("SyncConfiguredProvider(after model change) error = %v", err)
	}
	if syncResult.Total != 1 {
		t.Fatalf("model refresh sync = %+v", syncResult)
	}

	search, err = runtime.Catalog.SearchTools.Execute(
		ctx,
		catalogapplication.SearchToolsRequest{
			Query:      "web search",
			Limit:      10,
			ProviderID: providerIDPtr(provider.ID()),
		},
	)
	if err != nil || len(search) != 1 {
		t.Fatalf("refresh search = %+v err=%v", search, err)
	}
	if search[0].ToolID != toolID {
		t.Fatalf(
			"web_search ToolID changed across model refresh: %s -> %s",
			toolID,
			search[0].ToolID,
		)
	}

	profile, err := runtime.Profiles.Create.Execute(
		ctx,
		profileapplication.CreateProfileRequest{
			Slug:        "model-tools-denied",
			DisplayName: "Model Tools Denied",
			Mode:        profiledomain.ExposureDynamic,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	profile, err = runtime.Profiles.AddTool.Execute(
		ctx,
		profileapplication.AddToolRequest{
			ProfileSlug: profile.Slug(),
			ToolID:      toolID,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	scope, err := governancedomain.ProfileScope(profile.ID().String())
	if err != nil {
		t.Fatal(err)
	}
	invocationContext, err := governancedomain.NewContext(
		governancedomain.OriginMCP,
		testPrincipal(t, "model-tools-denied"),
		scope,
	)
	if err != nil {
		t.Fatal(err)
	}
	response, err := runtime.InvokeTool.Execute(
		ctx,
		invocationapplication.InvokeToolRequest{
			ToolID:    toolID,
			Arguments: []byte(`{"query":"latest Mender release"}`),
			Context:   invocationContext,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if response.Outcome.Kind() != invocationdomain.OutcomeKindFailed ||
		response.Outcome.FailureCode() != invocationdomain.FailureCredentialOutsideScope {
		t.Fatalf("denied outcome = %#v", response.Outcome)
	}

	records, err := runtime.ListAudit.Execute(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("audit records = %d, want 1", len(records))
	}
	if records[0].FailureCode() != invocationdomain.FailureCredentialOutsideScope {
		t.Fatalf("audit failure code = %q", records[0].FailureCode())
	}
}

func TestRuntimeRegistersModelToolsPlugin(t *testing.T) {
	t.Parallel()

	runtime := newProviderTestRuntime(t)
	descriptors := runtime.ProviderPluginDescriptors()
	found := false
	for _, descriptor := range descriptors {
		if descriptor.ID != modeltoolsplugin.ID {
			continue
		}
		found = true
		if descriptor.ConfigVersion != modeltoolsplugin.ConfigVersion ||
			descriptor.DisplayName != "Model Tools" {
			t.Fatalf("Model Tools descriptor = %+v", descriptor)
		}
	}
	if !found {
		t.Fatalf("Model Tools descriptor missing: %+v", descriptors)
	}
}
