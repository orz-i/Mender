package bootstrap

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	jobsplugin "github.com/orz-i/Mender/backend/internal/sshjobs/infrastructure/plugin"
	_ "modernc.org/sqlite"
)

func newGuardTestSSH(t *testing.T, r *Runtime, ctx context.Context, id sandboxdomain.ID) (sandboxdomain.Run, gov.Context) {
	t.Helper()
	owner, err := gov.NewContext(gov.OriginInternal, r.OwnerPrincipal(), gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	item, err := sandboxdomain.NewSandbox(sandboxdomain.Spec{
		ID: id, Name: "Guarded SSH", Backend: sandboxdomain.BackendSSHRemote,
		Binding:   sandboxdomain.BuiltinBinding(sandboxdomain.BackendSSHRemote),
		Isolation: sandboxdomain.IsolationRemote,
		OwnerKind: string(owner.Principal().Kind()), OwnerRef: owner.Principal().Reference(),
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := sandboxdomain.NewRun(id, 1, sandboxdomain.StateReady, "ssh_remote:"+id.String())
	if err != nil {
		t.Fatal(err)
	}
	// Test-only local metadata: no SSH Credential or remote host ever used.
	if err = r.Sandboxes.store.CreateReady(ctx, item, run); err != nil {
		t.Fatal(err)
	}
	return run, owner
}

func makeGuardTestConfig(t *testing.T, r *Runtime, ctx context.Context, id sandboxdomain.ID, namespace string) providerplugin.Configuration {
	t.Helper()
	provider, err := r.Catalog.RegisterProvider.Execute(ctx, catalogapplication.RegisterProviderRequest{
		Namespace: namespace, DisplayName: "Guard test provider",
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(jobsplugin.Config{SandboxID: id.String()})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := providerplugin.NewConfiguration(provider.ID(), jobsplugin.ID, jobsplugin.ConfigVersion, raw)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestJobsProviderReferenceCannotRaceOrFollowSandboxDestroy(t *testing.T) {
	ctx := context.Background()
	database := filepath.Join(t.TempDir(), "job-guard.db")
	r, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: database})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	id := sandboxdomain.ID("sbx_" + strings.Repeat("d", 32))
	run, owner := newGuardTestSSH(t, r, ctx, id)
	config := makeGuardTestConfig(t, r, ctx, id, "jobs-reference-guard")
	if err = r.providerConfigs.Save(ctx, config); err != nil {
		t.Fatal(err)
	}
	if _, err = r.Sandboxes.store.ChangeSSHRemoteLifecycle(ctx, run, gov.ActionSandboxDestroy, owner); err == nil {
		t.Fatal("Destroy bypassed a preexisting unclaimed Jobs Provider")
	}
	if err = r.providerConfigs.Delete(ctx, config.ProviderID); err != nil {
		t.Fatal(err)
	}
	destroyed, err := r.Sandboxes.store.ChangeSSHRemoteLifecycle(ctx, run, gov.ActionSandboxDestroy, owner)
	if err != nil || destroyed.State != sandboxdomain.StateDestroyed {
		t.Fatalf("Destroy after unbind: %+v %v", destroyed, err)
	}
	if err = r.providerConfigs.Save(ctx, config); err == nil {
		t.Fatal("new Jobs Provider was accepted after logical Sandbox Destroy")
	}
	db, err := sql.Open("sqlite", database)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	// A checked resource-owned Suite binding cannot be inserted after the
	// Run was destroyed, even if a future adapter attempts a raw SQL write.
	for _, stmt := range []string{
		`INSERT INTO sandbox_job_suite_bindings(sandbox_id,provider_id,created_at) VALUES(?,?,'now')`,
		`INSERT INTO sandbox_tool_suite_bindings(sandbox_id,suite_id,provider_id,created_at) VALUES(?,'sandbox.workspace',?,'now')`,
	} {
		if _, err = db.ExecContext(ctx, stmt, id.String(), config.ProviderID.String()); err == nil {
			t.Fatal("late Suite binding was accepted after logical Destroy")
		}
	}
}

func TestJobsProviderCreateAndDestroySerializedAcrossSQLiteConnections(t *testing.T) {
	ctx := context.Background()
	r, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "jobs-guard-race.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	for i := 0; i < 18; i++ {
		id := sandboxdomain.ID(fmt.Sprintf("sbx_%032x", i+2000))
		run, owner := newGuardTestSSH(t, r, ctx, id)
		config := makeGuardTestConfig(t, r, ctx, id, fmt.Sprintf("jobs-guard-race-%d", i))
		start := make(chan struct{})
		var submitErr, destroyErr error
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); <-start; submitErr = r.providerConfigs.Save(ctx, config) }()
		go func() {
			defer wg.Done()
			<-start
			_, destroyErr = r.Sandboxes.store.ChangeSSHRemoteLifecycle(ctx, run, gov.ActionSandboxDestroy, owner)
		}()
		close(start)
		wg.Wait()
		if submitErr == nil && destroyErr == nil {
			t.Fatalf("iteration %d: Jobs config inserted AND Sandbox destroyed", i)
		}
		now, err := r.Sandboxes.store.GetRun(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		all, err := r.providerConfigs.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, v := range all {
			if v.ProviderID == config.ProviderID {
				found = true
				break
			}
		}
		if now.State == sandboxdomain.StateDestroyed && found {
			t.Fatalf("iteration %d left a Jobs facade pointing to Destroyed Run", i)
		}
		if now.State != sandboxdomain.StateDestroyed && now.State != sandboxdomain.StateReady {
			t.Fatalf("iteration %d unexpected Run: %+v", i, now)
		}
	}
}
