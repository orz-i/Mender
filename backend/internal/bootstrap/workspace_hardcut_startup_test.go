package bootstrap

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	workspaceplugin "github.com/orz-i/Mender/backend/internal/workspace/infrastructure/plugin"
)

func TestRuntimeStartupMigratesLegacyWorkspaceWithoutChangingToolOrProfile(t *testing.T) {
	ctx := context.Background()
	db := filepath.Join(t.TempDir(), "legacy-workspace.db")
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "original.txt"), []byte("persisted workspace"), 0o600); err != nil {
		t.Fatal(err)
	}
	old, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: db})
	if err != nil {
		t.Fatal(err)
	}
	principal := old.OwnerPrincipal()
	caller, err := governancedomain.NewContext(governancedomain.OriginInternal, principal, governancedomain.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	sandbox, err := old.Sandboxes.CreateLocalSandbox(ctx, CreateLocalSandboxRequest{Name: "preupgrade", Root: root, Isolation: "host"}, caller)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := json.Marshal(workspaceplugin.Config{SandboxID: sandbox.ID().String()})
	provider, err := old.RegisterConfiguredProvider(ctx, catalogapplication.RegisterProviderRequest{Namespace: "migration-stable", DisplayName: "Migrating Workspace"}, workspaceplugin.ID, cfg, caller)
	if err != nil {
		t.Fatal(err)
	}
	syncResult, err := old.SyncConfiguredProvider(ctx, provider.ID(), caller)
	if err != nil || syncResult.Total != 4 {
		t.Fatalf("sync %+v %v", syncResult, err)
	}
	tools, err := old.Catalog.store.ListProviderTools(ctx, provider.ID())
	if err != nil {
		t.Fatal(err)
	}
	var readID catalogdomain.ToolID
	for _, tool := range tools {
		if tool.SourceKey() == "workspace.read" {
			readID = tool.ID()
		}
	}
	if readID == "" {
		t.Fatal("no stable read tool")
	}
	profile, err := old.Profiles.Create.Execute(ctx, profileapplication.CreateProfileRequest{Slug: "migration-editor", DisplayName: "Editor", Mode: profiledomain.ExposureDynamic})
	if err != nil {
		t.Fatal(err)
	}
	profile, err = old.Profiles.AddTool.Execute(ctx, profileapplication.AddToolRequest{ProfileSlug: profile.Slug(), ToolID: readID})
	if err != nil {
		t.Fatal(err)
	}
	legacyJSON, _ := json.Marshal(map[string]string{"root": root})
	legacy, err := providerplugin.NewConfiguration(provider.ID(), workspaceplugin.ID, 1, legacyJSON)
	if err != nil {
		t.Fatal(err)
	}
	if err := old.providerConfigs.Save(ctx, legacy); err != nil {
		t.Fatal(err)
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: db})
	if err != nil {
		t.Fatalf("startup migration: %v", err)
	}
	defer func() { _ = upgraded.Close() }()
	configs, err := upgraded.providerConfigs.GetProviderConfig(ctx, provider.ID())
	if err != nil || configs.ConfigVersion != 2 {
		t.Fatalf("migrated config %+v %v", configs, err)
	}
	var binding workspaceplugin.Config
	if err := json.Unmarshal(configs.Config, &binding); err != nil || binding.SandboxID == "" {
		t.Fatalf("migrated binding %+v %v", binding, err)
	}
	if binding.SandboxID == sandbox.ID().String() {
		t.Fatal("legacy migration should derive stable Provider-owned Sandbox, not reuse old config")
	}
	afterTools, err := upgraded.Catalog.store.ListProviderTools(ctx, provider.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(afterTools) != 4 {
		t.Fatalf("tools missing: %d", len(afterTools))
	}
	var sameID bool
	for _, tool := range afterTools {
		if tool.SourceKey() == "workspace.read" && tool.ID() == readID {
			sameID = true
		}
	}
	if !sameID {
		t.Fatal("ToolID changed after startup migration")
	}
	loadedProfile, err := upgraded.Profiles.GetByID.Execute(ctx, profile.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(loadedProfile.Bindings()) != 1 || loadedProfile.Bindings()[0].ToolID() != readID {
		t.Fatalf("Profile binding lost: %+v", loadedProfile.Bindings())
	}
	scope, err := governancedomain.ProfileScope(profile.ID().String())
	if err != nil {
		t.Fatal(err)
	}
	scopedCaller, err := governancedomain.NewContext(governancedomain.OriginInternal, principal, scope)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := upgraded.CallConfiguredTool(ctx, readID, []byte(`{"path":"original.txt"}`), scopedCaller)
	if err != nil || outcome.Outcome.Kind() != invocationdomain.OutcomeKindComplete {
		t.Fatalf("post-migration tool: %v %s", err, outcome.Outcome.Kind())
	}
	var output struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(outcome.Outcome.Content().Bytes(), &output); err != nil || output.Content != "persisted workspace" {
		t.Fatalf("post-migration file read: %+v %v", output, err)
	}
	if err := upgraded.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: db})
	if err != nil {
		t.Fatalf("repeat startup migration: %v", err)
	}
	defer func() { _ = reopened.Close() }()
	unchanged, err := reopened.providerConfigs.GetProviderConfig(ctx, provider.ID())
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Revision != configs.Revision {
		t.Fatal("idempotent migration changed config revision")
	}
}
