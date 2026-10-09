package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	governance "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

func leasePrincipal(t *testing.T) governance.Context {
	t.Helper()
	principal, err := governance.NewPrincipal(governance.PrincipalLocalUser, "local-owner")
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := governance.NewContext(governance.OriginCLI, principal, governance.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}
func TestSandboxLeasePersistsAcrossProcessesAndRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "lease.db")
	first, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	sandbox := testSandbox(t)
	if err := first.Create(ctx, sandbox); err != nil {
		t.Fatal(err)
	}
	run, err := sandboxdomain.NewRun(sandboxID, 1, sandboxdomain.StateReady, "local-workspace")
	if err != nil {
		t.Fatal(err)
	}
	if err := first.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	second, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()
	token, err := first.ReserveExecution(ctx, sandboxID, 1, leasePrincipal(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	// A Runtime restart must NOT silently free uncertain local execution.
	if _, err := second.ReserveExecution(ctx, sandboxID, 1, leasePrincipal(t)); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("post-crash double acquisition: %v", err)
	}
	count, err := second.ActiveLeaseCount(ctx, sandboxID)
	if err != nil || count != 1 {
		t.Fatalf("durable count %d, %v", count, err)
	}
	if err := second.ReleaseExecution(ctx, sandboxID, "00000000000000000000000000000000"); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("wrong token released another owner: %v", err)
	}
	if err := second.ReleaseExecution(ctx, sandboxID, token); err != nil {
		t.Fatal(err)
	}
	if _, err := second.ReserveExecution(ctx, sandboxID, 1, leasePrincipal(t)); err != nil {
		t.Fatalf("after release: %v", err)
	}
}
func TestLeaseReservationAtomicAcrossConcurrentSQLiteConnections(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "race.db")
	first, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	if err := first.Create(ctx, testSandbox(t)); err != nil {
		t.Fatal(err)
	}
	run, err := sandboxdomain.NewRun(sandboxID, 1, sandboxdomain.StateReady, "local")
	if err != nil {
		t.Fatal(err)
	}
	if err := first.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	second, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()
	var wg sync.WaitGroup
	results := make(chan error, 2)
	caller := leasePrincipal(t)
	for _, store := range []*Store{first, second} {
		wg.Add(1)
		go func(s *Store) {
			defer wg.Done()
			_, err := s.ReserveExecution(ctx, sandboxID, 1, caller)
			results <- err
		}(store)
	}
	wg.Wait()
	close(results)
	success, conflicts := 0, 0
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, sandboxdomain.ErrConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent reservation: %v", err)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatalf("expected one winner, got success=%d conflict=%d", success, conflicts)
	}
}
func TestLeaseReservationValidatesRunGenerationAndState(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	if err := store.Create(ctx, testSandbox(t)); err != nil {
		t.Fatal(err)
	}
	run, err := sandboxdomain.NewRun(sandboxID, 4, sandboxdomain.StateStopped, "local")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReserveExecution(ctx, sandboxID, 4, leasePrincipal(t)); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("stopped run accessible: %v", err)
	}
	ready, err := store.TransitionRun(ctx, run, sandboxdomain.StateProvisioning, "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.TransitionRun(ctx, ready, sandboxdomain.StateReady, "local")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReserveExecution(ctx, sandboxID, 4, leasePrincipal(t)); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("old generation accessible: %v", err)
	}
	token, err := store.ReserveExecution(ctx, sandboxID, 5, leasePrincipal(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReleaseExecution(ctx, sandboxID, token); err != nil {
		t.Fatal(err)
	}
}
