package daytona

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sandboxsqlite "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sqlite"
)

func testDaytonaSandbox(t *testing.T, id sandboxdomain.ID, owner string) sandboxdomain.Sandbox {
	t.Helper()
	item, err := sandboxdomain.NewSandbox(sandboxdomain.Spec{ID: id, Name: "Daytona allocated", Backend: sandboxdomain.BackendDaytona, Isolation: sandboxdomain.IsolationRemote, OwnerKind: "local_user", OwnerRef: owner})
	if err != nil {
		t.Fatal(err)
	}
	return item
}
func TestDaytonaTemplateTransactionalQuotaAndRestart(t *testing.T) {
	ctx := context.Background()
	db := filepath.Join(t.TempDir(), "daytona.db")
	runs, err := sandboxsqlite.Open(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runs.Close() }()
	first, err := OpenTemplateStore(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	second, err := OpenTemplateStore(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()
	template := validTemplate()
	ids := []sandboxdomain.ID{"sbx_11111111111111111111111111111111", "sbx_22222222222222222222222222222222", "sbx_33333333333333333333333333333333"}
	var group sync.WaitGroup
	outcomes := make(chan error, 3)
	for n, id := range ids {
		group.Add(1)
		go func(i int, sandboxID sandboxdomain.ID) {
			defer group.Done()
			store := first
			if i%2 == 1 {
				store = second
			}
			outcomes <- store.CreateDefinedSandbox(ctx, testDaytonaSandbox(t, sandboxID, "same-owner"), template)
		}(n, id)
	}
	group.Wait()
	close(outcomes)
	admitted, denied := 0, 0
	for err := range outcomes {
		switch {
		case err == nil:
			admitted++
		case errors.Is(err, sandboxdomain.ErrConflict):
			denied++
		default:
			t.Fatalf("unexpected allocation: %v", err)
		}
	}
	if admitted != MaxActivePerPrincipal || denied != 1 {
		t.Fatalf("capacity allowed %d denied %d", admitted, denied)
	}
	all, err := runs.List(ctx)
	if err != nil || len(all) != MaxActivePerPrincipal {
		t.Fatalf("partial resources %+v %v", all, err)
	}
	for _, item := range all {
		seen, err := first.GetDaytonaTemplate(ctx, item.ID())
		if err != nil || seen != template {
			t.Fatalf("saved template %+v %v", seen, err)
		}
		run, err := runs.GetRun(ctx, item.ID())
		if err != nil || run.State != sandboxdomain.StateDefined || run.Generation != 1 {
			t.Fatalf("run %+v %v", run, err)
		}
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenTemplateStore(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	for _, item := range all {
		loaded, err := reopened.GetDaytonaTemplate(ctx, item.ID())
		if err != nil || loaded != template {
			t.Fatalf("reopened %+v %v", loaded, err)
		}
	}
	raw, err := sql.Open("sqlite", db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	if _, err := raw.ExecContext(ctx, `UPDATE sandbox_daytona_templates SET ttl_minutes=999 WHERE sandbox_id=?`, all[0].ID().String()); err == nil {
		t.Fatal("immutable TTL mutated by raw SQL")
	}
	if _, err := raw.ExecContext(ctx, `DELETE FROM sandbox_daytona_templates WHERE sandbox_id=?`, all[0].ID().String()); err == nil {
		t.Fatal("credential-reference template removed by raw SQL")
	}
	if _, err := reopened.GetDaytonaTemplate(ctx, ids[2]); err == nil && denied > 0 {
		// IDs are nondeterministic concurrent contenders; only bound-quantity tested.
		t.Log("third ID won race; capacity invariants still intact")
	}
}
func TestDaytonaTemplateRejectsWrongBackendOrInvalidConfig(t *testing.T) {
	ctx := context.Background()
	db := filepath.Join(t.TempDir(), "invalid.db")
	runs, err := sandboxsqlite.Open(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runs.Close() }()
	templates, err := OpenTemplateStore(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = templates.Close() }()
	id := sandboxdomain.ID("sbx_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	invalid := validTemplate()
	invalid.TTLMinutes = 0
	if err := templates.CreateDefinedSandbox(ctx, testDaytonaSandbox(t, id, "me"), invalid); err == nil {
		t.Fatal("TTL disabled accepted")
	}
	if _, err := runs.Get(ctx, id); !errors.Is(err, sandboxdomain.ErrNotFound) {
		t.Fatalf("invalid Template created resource: %v", err)
	}
	host, err := sandboxdomain.NewSandbox(sandboxdomain.Spec{ID: id, Name: "host", Backend: sandboxdomain.BackendLocalWorkspace, Isolation: sandboxdomain.IsolationHost, LocalRoot: t.TempDir(), OwnerKind: "local_user", OwnerRef: "me"})
	if err != nil {
		t.Fatal(err)
	}
	if err := templates.CreateDefinedSandbox(ctx, host, validTemplate()); err == nil {
		t.Fatal("Daytona template linked to host")
	}
	if err := templates.CreateDefinedSandbox(ctx, testDaytonaSandbox(t, id, "me"), validTemplate()); err != nil {
		t.Fatal(err)
	}
	if err := templates.CreateDefinedSandbox(ctx, testDaytonaSandbox(t, id, "me"), validTemplate()); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("duplicate Create accepted: %v", err)
	}
	if _, err := templates.GetDaytonaTemplate(ctx, sandboxdomain.ID(fmt.Sprintf("sbx_%032x", 999))); !errors.Is(err, sandboxdomain.ErrNotFound) {
		t.Fatalf("missing template: %v", err)
	}
}
