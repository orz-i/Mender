package budget

import (
	"context"
	"errors"
	"sync"
	"testing"

	runtimegovernancedomain "github.com/orz-i/Mender/backend/internal/runtimegovernance/domain"
)

type memoryRepository struct {
	mu     sync.Mutex
	budget runtimegovernancedomain.Budget
}

func (r *memoryRepository) GetBudget(context.Context) (runtimegovernancedomain.Budget, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.budget, nil
}

func (r *memoryRepository) SetBudget(_ context.Context, value runtimegovernancedomain.Budget) error {
	r.mu.Lock()
	r.budget = value
	r.mu.Unlock()
	return nil
}

func testBudget(t *testing.T, max int) runtimegovernancedomain.Budget {
	t.Helper()
	spec := runtimegovernancedomain.DefaultBudget().Spec()
	spec.MaxInvocations = max
	spec.MaxPrincipalInvocations = max
	spec.MaxProviderInvocations = max
	spec.MaxToolInvocations = max
	spec.MaxProviderActivations = max
	value, err := runtimegovernancedomain.NewBudget(spec)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestControllerFailFastAndRelease(t *testing.T) {
	t.Parallel()
	repo := &memoryRepository{budget: testBudget(t, 1)}
	controller := NewController(repo)
	snapshot, err := controller.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	release, err := controller.AcquireInvocation(
		context.Background(), snapshot, "principal", "provider", "tool",
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.AcquireInvocation(
		context.Background(), snapshot, "principal-2", "provider-2", "tool-2",
	); !errors.Is(err, runtimegovernancedomain.ErrBudgetExceeded) {
		t.Fatalf("second AcquireInvocation error = %v", err)
	}
	release()
	release()
	if usage := controller.Usage(); usage.Invocations != 0 {
		t.Fatalf("usage after release = %#v", usage)
	}
	if _, err := controller.AcquireInvocation(
		context.Background(), snapshot, "principal", "provider", "tool",
	); err != nil {
		t.Fatalf("reacquire after release: %v", err)
	}
}

func TestControllerDimensionsAreIndependent(t *testing.T) {
	t.Parallel()
	spec := runtimegovernancedomain.DefaultBudget().Spec()
	spec.MaxInvocations = 10
	spec.MaxPrincipalInvocations = 1
	spec.MaxProviderInvocations = 10
	spec.MaxToolInvocations = 10
	value, err := runtimegovernancedomain.NewBudget(spec)
	if err != nil {
		t.Fatal(err)
	}
	controller := NewController(&memoryRepository{budget: value})
	release, err := controller.AcquireInvocation(
		context.Background(), value, "same", "provider-a", "tool-a",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	_, err = controller.AcquireInvocation(
		context.Background(), value, "same", "provider-b", "tool-b",
	)
	var exceeded runtimegovernancedomain.BudgetExceededError
	if !errors.As(err, &exceeded) || exceeded.Dimension != "principal_invocations" {
		t.Fatalf("error = %v", err)
	}
}

func TestControllerLoweredLimitAffectsNewAcquisitionsOnly(t *testing.T) {
	t.Parallel()
	controller := NewController(&memoryRepository{budget: testBudget(t, 2)})
	old := testBudget(t, 2)
	first, err := controller.AcquireInvocation(
		context.Background(), old, "p1", "provider-1", "tool-1",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer first()
	second, err := controller.AcquireInvocation(
		context.Background(), old, "p2", "provider-2", "tool-2",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer second()
	lowered := testBudget(t, 1)
	if _, err := controller.AcquireInvocation(
		context.Background(), lowered, "p3", "provider-3", "tool-3",
	); !errors.Is(err, runtimegovernancedomain.ErrBudgetExceeded) {
		t.Fatalf("lowered acquire error = %v", err)
	}
}

func TestControllerActivationBudget(t *testing.T) {
	t.Parallel()
	value := testBudget(t, 1)
	controller := NewController(&memoryRepository{budget: value})
	release, err := controller.AcquireActivation(context.Background(), value)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := controller.AcquireActivation(context.Background(), value); !errors.Is(err, runtimegovernancedomain.ErrBudgetExceeded) {
		t.Fatalf("second activation error = %v", err)
	}
}

func TestControllerConcurrentReleaseIsRaceSafe(t *testing.T) {
	t.Parallel()
	value := testBudget(t, 64)
	controller := NewController(&memoryRepository{budget: value})
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, err := controller.AcquireInvocation(
				context.Background(), value, "principal", "provider", "tool",
			)
			if err == nil {
				release()
				release()
			}
		}()
	}
	wg.Wait()
	if usage := controller.Usage(); usage.Invocations != 0 {
		t.Fatalf("usage = %#v", usage)
	}
}
