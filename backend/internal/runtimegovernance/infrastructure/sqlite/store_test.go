package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	runtimegovernancedomain "github.com/orz-i/Mender/backend/internal/runtimegovernance/domain"
)

func TestStoreDefaultsAndUpdatesBudget(t *testing.T) {
	t.Parallel()
	store, err := Open(context.Background(), filepath.Join(t.TempDir(), "runtime.db"))
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
}
