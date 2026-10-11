package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
	installationsqlite "github.com/orz-i/Mender/backend/internal/plugin/installation/infrastructure/sqlite"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

func externalSQLTestContext(t *testing.T) (*Store, sandboxdomain.Sandbox, gov.Context) {
	t.Helper()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "sandbox.db")
	store, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	installations, err := installationsqlite.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = installations.Close() })
	ext := platformplugin.Extension{ID: "main", Kind: platformplugin.SandboxBackend, ProtocolVersion: 1}
	manifest := []byte(`{"manifestVersion":2,"pluginId":"dev.example.sandbox","version":"1.0.0","displayName":"Example","entrypoints":{"linux/amd64":"bin/driver"},"extensions":[{"id":"main","kind":"sandbox.backend","protocolVersion":1}]}`)
	install, err := installationdomain.NewInstallation(
		platformplugin.InstallationID("pinst_"+strings.Repeat("a", 32)), platformplugin.ID("dev.example.sandbox"),
		"1.0.0", installationdomain.SourceLocal, "/tmp/fixture", "sha256:"+strings.Repeat("a", 64),
		manifest, installationdomain.StatusEnabled, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	install, err = install.WithVerifiedDescriptor(platformplugin.Descriptor{
		PluginID: install.PluginID(), PackageVersion: "1.0.0", Extension: ext,
		DisplayName: "Example", ConfigVersion: 1, Capabilities: []string{"file.read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := installations.Create(ctx, install); err != nil {
		t.Fatal(err)
	}
	owner, err := gov.NewPrincipal(gov.PrincipalLocalUser, "owner")
	if err != nil {
		t.Fatal(err)
	}
	caller, err := gov.NewContext(gov.OriginCLI, owner, gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	item, err := sandboxdomain.NewSandbox(sandboxdomain.Spec{
		ID: "sbx_0123456789abcdef0123456789abcdef", Name: "Fake",
		Backend: sandboxdomain.BackendExternal,
		Binding: sandboxdomain.BackendBinding{PluginID: "dev.example.sandbox", ExtensionID: "main",
			InstallationID: platformplugin.InstallationID("pinst_" + strings.Repeat("a", 32))},
		Isolation: sandboxdomain.IsolationRemote, OwnerKind: "local_user", OwnerRef: "owner",
	})
	if err != nil {
		t.Fatal(err)
	}
	config := ExternalConfig{ConfigVersion: 1, JSON: []byte(`{"message":"safe"}`)}
	if err := store.CreateDefinedExternal(ctx, item, config); err != nil {
		t.Fatal(err)
	}
	return store, item, caller
}

func TestExternalOperationJournalFencesExactRunAndLease(t *testing.T) {
	t.Parallel()
	store, item, caller := externalSQLTestContext(t)
	ctx := context.Background()
	current, err := store.GetRun(ctx, item.ID())
	if err != nil {
		t.Fatal(err)
	}
	staged, err := store.TransitionExternalRun(ctx, current, sandboxdomain.StateProvisioning, "",
		gov.ActionSandboxProvision, caller, false)
	if err != nil {
		t.Fatal(err)
	}
	id := "sbop_" + strings.Repeat("c", 32)
	if err := store.BeginExternalOperation(ctx, staged, gov.ActionSandboxProvision, caller, id); err != nil {
		t.Fatal(err)
	}
	if err := store.BeginExternalOperation(ctx, staged, gov.ActionSandboxProvision, caller, id); err == nil {
		t.Fatal("duplicate remote idempotency key allowed")
	}
	if err := store.BeginExternalOperation(ctx, current, gov.ActionSandboxProvision, caller,
		"sbop_"+strings.Repeat("d", 32)); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("stale generation/state accepted: %v", err)
	}
	if err := store.FinishExternalOperation(ctx, id, false); err != nil {
		t.Fatal(err)
	}
	if err := store.FinishExternalOperation(ctx, id, false); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("completed operation was allowed to finish again: %v", err)
	}
	var state string
	if err := store.db.QueryRowContext(ctx, "SELECT state FROM sandbox_external_operations WHERE operation_id=?", id).Scan(&state); err != nil || state != "complete" {
		t.Fatalf("durable completion lost: %s %v", state, err)
	}
	_, err = store.db.ExecContext(ctx, `INSERT INTO sandbox_execution_leases(sandbox_id,lease_token,generation,principal_kind,principal_ref,scope_kind,scope_ref,acquired_at)
        VALUES(?, ?, ?, 'local_user','owner','catalog','',CURRENT_TIMESTAMP)`, item.ID().String(), strings.Repeat("d", 32), staged.Generation)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.BeginExternalOperation(ctx, staged, gov.ActionSandboxProvision, caller, "sbop_"+strings.Repeat("e", 32)); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("active lease did not fence remote operation: %v", err)
	}
	if _, err := store.TransitionExternalRun(ctx, staged, sandboxdomain.StateUnknown, "",
		gov.ActionSandboxProvision, caller, true); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("active lease did not fence lifecycle CAS: %v", err)
	}
}

func TestExternalConfigRejectsSecretMaterialAndMutation(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		`{"credentialRef":"credential://demo/key"}`,
		`{"api_key":"abcdef"}`, `{"token":"secret"}`, `{"private_key":"abc"}`,
		`{"password":"plaintext"}`, `{"ok":"secret material"}`, `[]`,
		`{"\u0073ecret":"escaped key"}`,
		`{"nested":{"\u0070assword":"escaped nested key"}}`,
		`{"items":[{"key":"credential:\/\/test"}]}`,
	} {
		if err := (ExternalConfig{ConfigVersion: 1, JSON: []byte(raw)}).Validate(); err == nil {
			t.Fatalf("unsafe config accepted: %s", raw)
		}
	}
	store, item, _ := externalSQLTestContext(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, "UPDATE sandbox_external_configs SET config_json='{}' WHERE sandbox_id=?", item.ID()); err == nil {
		t.Fatal("immutable config updated with direct SQL")
	}
	if _, err := store.db.ExecContext(ctx, "UPDATE sandboxes SET backend_installation_id=? WHERE id=?", "pinst_"+strings.Repeat("b", 32), item.ID()); err == nil {
		t.Fatal("exact external installation binding mutated")
	}
	config, err := store.GetExternalConfig(ctx, item.ID())
	if err != nil || string(config.JSON) != `{"message":"safe"}` {
		t.Fatalf("config mutated: %+v %v", config, err)
	}
	count, err := store.ReferencedInstallation(ctx, item.Spec().Binding.InstallationID.String())
	if err != nil || count != 1 {
		t.Fatalf("installation usage count=%d err=%v", count, err)
	}
}
