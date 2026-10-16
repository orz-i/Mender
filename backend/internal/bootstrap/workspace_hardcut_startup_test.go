package bootstrap

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	workspaceplugin "github.com/orz-i/Mender/backend/internal/workspace/infrastructure/plugin"
	_ "modernc.org/sqlite"
)

// A v1 Workspace Provider is intentionally NOT upgraded by the Runtime.
// Operators must run the older one-way upgrade before crossing this hard cut.
// A failed upgrade preserves the exact Tool and Profile authorization rows.
func TestRuntimeRejectsWorkspaceV1WithoutRewritingToolOrProfile(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "old-workspace.db")
	runtime, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: path})
	if err != nil {
		t.Fatal(err)
	}
	caller, err := governancedomain.NewContext(governancedomain.OriginInternal, runtime.OwnerPrincipal(), governancedomain.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	sb, err := runtime.Sandboxes.CreateLocalSandbox(ctx, CreateLocalSandboxRequest{
		Name: "preupgrade", Root: t.TempDir(), Isolation: "host",
	}, caller)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(workspaceplugin.Config{SandboxID: sb.ID().String()})
	if err != nil {
		t.Fatal(err)
	}
	provider, err := runtime.registerConfiguredProvider(ctx, catalogapplication.RegisterProviderRequest{
		Namespace: "workspace-v1-hardcut", DisplayName: "Workspace"}, workspaceplugin.ID, raw, caller, true)
	if err != nil {
		t.Fatal(err)
	}
	result, err := runtime.SyncConfiguredProvider(ctx, provider.ID(), caller)
	if err != nil || result.Total != 4 {
		t.Fatalf("sync %+v, %v", result, err)
	}
	tools, err := runtime.Catalog.store.ListProviderTools(ctx, provider.ID())
	if err != nil || len(tools) != 4 {
		t.Fatalf("tools %+v %v", tools, err)
	}
	profile, err := runtime.Profiles.Create.Execute(ctx, profileapplication.CreateProfileRequest{
		Slug: "v1-bound", DisplayName: "Bound", Mode: profiledomain.ExposureDynamic,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = runtime.Profiles.AddTool.Execute(ctx, profileapplication.AddToolRequest{
		ProfileSlug: profile.Slug(), ToolID: tools[0].ID(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}

	// Construct an exact pre-hard-cut (v3) schema in a disposable database,
	// bypassing the current version gate only for this negative fixture.
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	for _, stmt := range []string{
		"DROP TRIGGER workspace_v2_only_provider_config_insert",
		"DROP TRIGGER workspace_v2_only_provider_config_update",
		"DELETE FROM provider_config_schema_migrations WHERE version=4",
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx,
		"UPDATE provider_configs SET config_version=1,config_json=? WHERE provider_id=?",
		`{"root":"/tmp"}`, provider.ID().String()); err != nil {
		t.Fatal(err)
	}
	upgraded, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: path})
	if err == nil {
		_ = upgraded.Close()
		t.Fatal("Workspace v1 silently upgraded")
	}
	if !strings.Contains(err.Error(), "not v2-only") {
		t.Fatalf("unexpected refusal: %v", err)
	}
	var version int
	if err := db.QueryRowContext(ctx, "SELECT MAX(version) FROM provider_config_schema_migrations").Scan(&version); err != nil || version != 3 {
		t.Fatalf("upgrade version=%d err=%v", version, err)
	}
	var retained int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM profile_tools WHERE profile_id=? AND tool_id=?", profile.ID().String(), tools[0].ID().String()).Scan(&retained); err != nil || retained != 1 {
		t.Fatalf("Profile binding lost: count=%d err=%v", retained, err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tools WHERE provider_id=?", provider.ID().String()).Scan(&retained); err != nil || retained != 4 {
		t.Fatalf("canonical ToolIDs changed: count=%d err=%v", retained, err)
	}
}
