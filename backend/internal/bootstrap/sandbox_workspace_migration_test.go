package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	providerconfigstore "github.com/orz-i/Mender/backend/internal/providerplugin/configstore"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sandboxsqlite "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sqlite"
	workspaceplugin "github.com/orz-i/Mender/backend/internal/workspace/infrastructure/plugin"
)

func seedWorkspaceConfig(t *testing.T, ctx context.Context, store *providerconfigstore.Store, id catalogdomain.ProviderID, root string, version int) {
	t.Helper()
	raw, err := json.Marshal(map[string]string{"root": root})
	if err != nil {
		t.Fatal(err)
	}
	config, err := providerplugin.NewConfiguration(id, workspaceplugin.ID, version, raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(ctx, config); err != nil {
		t.Fatal(err)
	}
}
func migrationDatabases(t *testing.T) (context.Context, string, *providerconfigstore.Store, *sandboxsqlite.Store) {
	t.Helper()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "sandbox-migrate.db")
	providers, err := providerconfigstore.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = providers.Close() })
	sandboxes, err := sandboxsqlite.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sandboxes.Close() })
	return ctx, dbPath, providers, sandboxes
}
func testMigrationOwner(t *testing.T) governancedomain.Principal {
	t.Helper()
	p, err := governancedomain.NewPrincipal(governancedomain.PrincipalLocalUser, "workspace-owner")
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func TestWorkspaceMigrationCreatesSandboxAndIsIdempotent(t *testing.T) {
	ctx, path, providers, sandboxes := migrationDatabases(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	providerID := catalogdomain.ProviderID("prov_0123456789abcdef0123456789abcdef")
	seedWorkspaceConfig(t, ctx, providers, providerID, root, 1)
	migrate := func() error {
		return migrateWorkspaceProviderV1ToSandbox(ctx, path, testMigrationOwner(t), sandboxdomain.IsolationRestrictedHost)
	}
	if err := migrate(); err != nil {
		t.Fatal(err)
	}
	configs, err := providers.GetProviderConfig(ctx, providerID)
	if err != nil {
		t.Fatal(err)
	}
	if configs.ConfigVersion != workspaceSandboxConfigVersion {
		t.Fatalf("version %d", configs.ConfigVersion)
	}
	var next struct {
		SandboxID string `json:"sandboxId"`
	}
	if err := json.Unmarshal(configs.Config, &next); err != nil {
		t.Fatal(err)
	}
	sandboxID, err := sandboxdomain.ParseID(next.SandboxID)
	if err != nil {
		t.Fatal(err)
	}
	item, err := sandboxes.Get(ctx, sandboxID)
	if err != nil {
		t.Fatal(err)
	}
	if item.Spec().LocalRoot != root || item.Isolation() != sandboxdomain.IsolationRestrictedHost || item.OwnerRef() != "workspace-owner" {
		t.Fatalf("sandbox spec %+v", item.Spec())
	}
	run, err := sandboxes.GetRun(ctx, sandboxID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Generation != 1 || run.State != sandboxdomain.StateReady {
		t.Fatalf("run %+v", run)
	}
	if err := migrate(); err != nil {
		t.Fatalf("second migration: %v", err)
	}
	list, err := sandboxes.List(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("duplicated sandbox records %+v, %v", list, err)
	}
	after, err := providers.GetProviderConfig(ctx, providerID)
	if err != nil || after.Revision != configs.Revision {
		t.Fatalf("idempotency failed: %v", err)
	}
}
func TestWorkspaceMigrationInvalidRowRollsBackAll(t *testing.T) {
	ctx, path, providers, sandboxes := migrationDatabases(t)
	valid, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	goodID := catalogdomain.ProviderID("prov_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	badID := catalogdomain.ProviderID("prov_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	seedWorkspaceConfig(t, ctx, providers, goodID, valid, 1)
	seedWorkspaceConfig(t, ctx, providers, badID, "relative-dir", 1)
	err = migrateWorkspaceProviderV1ToSandbox(ctx, path, testMigrationOwner(t), sandboxdomain.IsolationHost)
	if err == nil {
		t.Fatal("migration accepted invalid root")
	}
	list, err := sandboxes.List(ctx)
	if err != nil || len(list) != 0 {
		t.Fatalf("partial sandbox rows %+v, %v", list, err)
	}
	original, err := providers.GetProviderConfig(ctx, goodID)
	if err != nil {
		t.Fatal(err)
	}
	if original.ConfigVersion != 1 {
		t.Fatal("nontransactional workspace migration updated first row")
	}
	seedWorkspaceConfig(t, ctx, providers, badID, valid, 1)
	if err := migrateWorkspaceProviderV1ToSandbox(ctx, path, testMigrationOwner(t), sandboxdomain.IsolationHost); err != nil {
		t.Fatalf("retry after fix: %v", err)
	}
	list, err = sandboxes.List(ctx)
	if err != nil || len(list) != 2 {
		t.Fatalf("recovery: %+v %v", list, err)
	}
}
func TestWorkspaceMigrationRefusesUnknownConfigVersion(t *testing.T) {
	ctx, path, providers, _ := migrationDatabases(t)
	seedWorkspaceConfig(t, ctx, providers, catalogdomain.ProviderID("prov_00000000000000000000000000000009"), t.TempDir(), 3)
	err := migrateWorkspaceProviderV1ToSandbox(ctx, path, testMigrationOwner(t), sandboxdomain.IsolationHost)
	if err == nil || errors.Is(err, context.Canceled) {
		t.Fatalf("unknown config version not rejected: %v", err)
	}
}
