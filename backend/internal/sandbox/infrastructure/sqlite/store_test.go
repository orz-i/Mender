package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

const sandboxID sandboxdomain.ID = "sbx_0123456789abcdef0123456789abcdef"

func testSandbox(t *testing.T) sandboxdomain.Sandbox {
	t.Helper()
	item, err := sandboxdomain.NewSandbox(sandboxdomain.Spec{ID: sandboxID, Name: "workspace", Backend: sandboxdomain.BackendLocalWorkspace, Binding: sandboxdomain.BuiltinBinding(sandboxdomain.BackendLocalWorkspace), Isolation: sandboxdomain.IsolationHost, LocalRoot: t.TempDir(), OwnerKind: "local_user", OwnerRef: "local-owner"})
	if err != nil {
		t.Fatal(err)
	}
	return item
}
func TestStoreRoundtripCASAndRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "sandbox.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	item := testSandbox(t)
	if err := store.Create(ctx, item); err != nil {
		t.Fatal(err)
	}
	if err := store.Create(ctx, item); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("duplicate: %v", err)
	}
	got, err := store.Get(ctx, sandboxID)
	if err != nil || got.Spec() != item.Spec() {
		t.Fatalf("round trip %+v, %v", got, err)
	}
	list, err := store.List(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %+v, %v", list, err)
	}
	run, err := sandboxdomain.NewRun(sandboxID, 1, sandboxdomain.StateDefined, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	provisioning, err := store.TransitionRun(ctx, run, sandboxdomain.StateProvisioning, "")
	if err != nil {
		t.Fatal(err)
	}
	ready, err := store.TransitionRun(ctx, provisioning, sandboxdomain.StateReady, "backend-123")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.TransitionRun(ctx, provisioning, sandboxdomain.StateFailed, ""); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("stale generation: %v", err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	observed, err := reopened.GetRun(ctx, sandboxID)
	if err != nil || observed != ready {
		t.Fatalf("restart: %+v, %v", observed, err)
	}
}
func TestTransitionsCASConcurrent(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	if err := store.Create(ctx, testSandbox(t)); err != nil {
		t.Fatal(err)
	}
	run, err := sandboxdomain.NewRun(sandboxID, 1, sandboxdomain.StateDefined, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.TransitionRun(ctx, run, sandboxdomain.StateProvisioning, "")
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	success, conflicts := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, sandboxdomain.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatalf("CAS success %d conflicts %d", success, conflicts)
	}
}
func TestMigrationFutureSchemaFailsClosed(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	_ = store.Close()
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = raw.Exec(`INSERT INTO sandbox_schema_migrations(version,applied_at) VALUES(999,'future')`)
	_ = raw.Close()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(ctx, path); err == nil {
		t.Fatal("future schema accepted")
	}
}
