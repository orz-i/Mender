package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	runtimegovernancedomain "github.com/orz-i/Mender/backend/internal/runtimegovernance/domain"
)

func TestStorePersistsRestrictedIsolationWithoutDefaultFallback(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "runtime.db")
	store, err := Open(
		context.Background(),
		path,
		runtimegovernancedomain.DefaultIsolation(false),
	)
	if err != nil {
		t.Fatal(err)
	}
	restricted, err := runtimegovernancedomain.NewIsolation(
		runtimegovernancedomain.IsolationSpec{
			WorkspaceShell:   runtimegovernancedomain.IsolationRestrictedHost,
			ExternalProvider: runtimegovernancedomain.IsolationRestrictedHost,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetIsolation(context.Background(), restricted); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopening with a host-only default must not rewrite a persisted
	// restricted_host choice. Backend availability is enforced at execution.
	reopened, err := Open(
		context.Background(),
		path,
		runtimegovernancedomain.DefaultIsolation(false),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	loaded, err := reopened.GetIsolation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Spec() != restricted.Spec() {
		t.Fatalf("reopened isolation = %#v, want %#v", loaded.Spec(), restricted.Spec())
	}
}

func TestStoreDefaultsAndUpdatesBudget(t *testing.T) {
	t.Parallel()
	defaultIsolation := runtimegovernancedomain.DefaultIsolation(false)
	store, err := Open(
		context.Background(),
		filepath.Join(t.TempDir(), "runtime.db"),
		defaultIsolation,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	value, err := store.GetBudget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if value.Spec() != runtimegovernancedomain.DefaultBudget().Spec() {
		t.Fatalf("default = %#v", value.Spec())
	}

	spec := value.Spec()
	spec.MaxInvocations = 7
	updated, err := runtimegovernancedomain.NewBudget(spec)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetBudget(context.Background(), updated); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.GetBudget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.MaxInvocations() != 7 {
		t.Fatalf("MaxInvocations = %d", loaded.MaxInvocations())
	}

	isolation, err := store.GetIsolation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if isolation.Spec() != defaultIsolation.Spec() {
		t.Fatalf("default isolation = %#v", isolation.Spec())
	}
	updatedIsolation, err := runtimegovernancedomain.NewIsolation(
		runtimegovernancedomain.IsolationSpec{
			WorkspaceShell:   runtimegovernancedomain.IsolationRestrictedHost,
			ExternalProvider: runtimegovernancedomain.IsolationRestrictedHost,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetIsolation(context.Background(), updatedIsolation); err != nil {
		t.Fatal(err)
	}
	loadedIsolation, err := store.GetIsolation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if loadedIsolation.Spec() != updatedIsolation.Spec() {
		t.Fatalf("loaded isolation = %#v", loadedIsolation.Spec())
	}
}
