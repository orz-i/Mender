package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	app "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	sourcecatalog "github.com/orz-i/Mender/backend/internal/catalog/infrastructure/sourcecatalog"
	credentialapp "github.com/orz-i/Mender/backend/internal/credential/application"
	hardcut "github.com/orz-i/Mender/backend/internal/migration/toolhardcut"
	modeltoolsplugin "github.com/orz-i/Mender/backend/internal/modeltools/infrastructure/plugin"
	"github.com/orz-i/Mender/backend/internal/toolsuite/infrastructure/curated"
)

func migratedTypedCatalogCandidate(t *testing.T) (legacy string, candidate string, providerID catalog.ProviderID, toolID catalog.ToolID) {
	t.Helper()
	ctx := context.Background()
	legacy = filepath.Join(t.TempDir(), "legacy.sqlite")
	runtime, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: legacy})
	if err != nil {
		t.Fatal(err)
	}
	env, err := runtime.Credentials.CreateEnvironment.Execute(ctx, credentialapp.CreateEnvironmentRequest{
		Namespace: "mender-test", Name: "api", DisplayName: "Test model credential", EnvironmentVariable: "MENDER_F4_MISSING_CREDENTIAL"})
	if err != nil {
		t.Fatal(err)
	}
	config, err := json.Marshal(modeltoolsplugin.Config{Engine: modeltoolsplugin.EngineOpenAI, Model: "test-model", CredentialRef: env.Reference(), Capabilities: []modeltoolsplugin.Capability{modeltoolsplugin.CapabilityWebSearch}})
	if err != nil {
		t.Fatal(err)
	}
	provider, err := registerConfiguredProviderTest(t, runtime, ctx, app.RegisterProviderRequest{Namespace: "external-model-tools", DisplayName: "External model tools"}, modeltoolsplugin.ID, config)
	if err != nil {
		t.Fatal(err)
	}
	syncResult, err := syncConfiguredProviderTest(t, runtime, ctx, provider.ID())
	if err != nil || syncResult.Total != 1 {
		t.Fatalf("legacy configured Provider Tool sync: %+v %v", syncResult, err)
	}
	tools, err := runtime.Catalog.store.ListProviderTools(ctx, provider.ID())
	if err != nil || len(tools) != 1 {
		t.Fatalf("legacy external tool not registered: %+v %v", tools, err)
	}
	originalID := tools[0].ID()
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	report, err := hardcut.AssessPath(ctx, legacy)
	if err != nil || report.Status != "ready" {
		t.Fatalf("disposable source migration unexpectedly blocked: %+v %v", report.Conflicts, err)
	}
	artifact, err := hardcut.StageCopy(ctx, legacy, filepath.Join(t.TempDir(), "typed.sqlite"), report.ManifestDigest)
	if err != nil {
		t.Fatal(err)
	}
	if err := hardcut.VerifyStagedCopy(ctx, artifact); err != nil {
		t.Fatal(err)
	}
	return legacy, artifact.CandidatePath, provider.ID(), originalID
}

func TestTypedCatalogCompositionRetainsProviderAndCanonicalNativeToolIDs(t *testing.T) {
	ctx := context.Background()
	legacy, candidate, providerID, toolID := migratedTypedCatalogCandidate(t)
	catalogRoot, err := NewTypedCatalog(ctx, CatalogOptions{DatabasePath: candidate})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = catalogRoot.Close() }()
	provider, err := catalogRoot.GetProvider.Execute(ctx, providerID)
	if err != nil || provider.Namespace().String() != "external-model-tools" {
		t.Fatalf("provider projection lost: %+v %v", provider, err)
	}
	restored, err := catalogRoot.DescribeTool.Execute(ctx, toolID)
	if err != nil || restored.ID() != toolID || restored.SourceRef().Kind() != catalog.SourceProvider || restored.ProviderID() != providerID {
		t.Fatalf("existing external ToolID changed: %+v %v", restored, err)
	}
	native, err := curated.PlannedNativeTools()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range native {
		item, e := catalogRoot.DescribeTool.Execute(ctx, entry.ToolID)
		if e != nil || item.SourceRef() != entry.Source || item.SourceKey() != entry.SourceKey {
			t.Fatalf("native Tool differs from pinned identity: %s %+v %v", entry.SourceKey, item, e)
		}
	}
	page, err := catalogRoot.PageTools.Execute(ctx, app.PageToolsRequest{Mode: "browse", Limit: 100})
	if err != nil || len(page.Tools) != len(native)+1 {
		t.Fatalf("typed Catalog PageTools mixed source listing: %+v %v", page, err)
	}
	providerCounts, err := catalogRoot.ListProviderToolCounts.Execute(ctx)
	if err != nil || len(providerCounts) != 1 || providerCounts[providerID] != 1 {
		t.Fatalf("native Sources fabricated Provider counts: %+v %v", providerCounts, err)
	}
	snapshot, err := catalogRoot.ProviderToolSnapshots.Snapshot(ctx, providerID)
	if err != nil || len(snapshot) != 1 || snapshot[0].ID() != toolID {
		t.Fatalf("provider snapshot lost identity: %+v %v", snapshot, err)
	}
	result, err := catalogRoot.SyncProviderTools(staticSource{tools: []app.DiscoveredTool{{
		SourceKey: restored.SourceKey(), Name: restored.Name(), Title: restored.Title(), Description: restored.Description(),
		InputSchema: restored.InputSchema().Bytes(), Tags: restored.Tags(),
	}}}).Execute(ctx, providerID)
	if err != nil || result.Updated != 1 || result.Created != 0 {
		t.Fatalf("existing provider application sync failed: %+v %v", result, err)
	}
	same, err := catalogRoot.DescribeTool.Execute(ctx, toolID)
	if err != nil || same.ID() != toolID || same.SourceRef() != restored.SourceRef() {
		t.Fatalf("sync replaced existing Provider ToolID: %+v %v", same, err)
	}
	if err := catalogRoot.ProviderToolSnapshots.Restore(ctx, providerID, nil); !errors.Is(err, sourcecatalog.ErrRetirementRequiresReview) {
		t.Fatalf("unreviewed Provider Tool retirement accepted: %v", err)
	}
	// The independent legacy source DB is unchanged by the disposable v3 Catalog.
	report, err := hardcut.AssessPath(ctx, legacy)
	if err != nil || report.Status != "ready" {
		t.Fatalf("typed Candidate modified legacy DB: %v %+v", err, report.Conflicts)
	}
}

func TestTypedCatalogRefusesUnconfiguredProviderSyncAndLegacySchema(t *testing.T) {
	ctx := context.Background()
	legacy, candidate, _, _ := migratedTypedCatalogCandidate(t)
	if _, err := NewTypedCatalog(ctx, CatalogOptions{DatabasePath: legacy}); err == nil {
		t.Fatal("typed Catalog opened old Provider-v2 database")
	}
	if _, err := NewTypedCatalog(ctx, CatalogOptions{DatabasePath: filepath.Join(t.TempDir(), "missing.sqlite")}); err == nil {
		t.Fatal("typed Catalog created missing DB")
	}
	root, err := NewTypedCatalog(ctx, CatalogOptions{DatabasePath: candidate})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	provider, err := root.RegisterProvider.Execute(ctx, app.RegisterProviderRequest{Namespace: "unconfigured", DisplayName: "Unconfigured Source"})
	if err != nil {
		t.Fatal(err)
	}
	schema, err := catalog.NewSchema([]byte(`{"type":"object"}`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = root.SyncProviderTools(staticSource{tools: []app.DiscoveredTool{{SourceKey: "api.read", Name: "read", InputSchema: schema.Bytes()}}}).Execute(ctx, provider.ID())
	if !errors.Is(err, sourcecatalog.ErrUnverifiedSource) {
		t.Fatalf("unconfigured Provider minted ToolID: %v", err)
	}
	tools, err := root.store.ListProviderTools(ctx, provider.ID())
	if err != nil || len(tools) != 0 {
		t.Fatalf("rejected unconfigured Provider already has Tools: %+v %v", tools, err)
	}
}
