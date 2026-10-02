package budget

import (
	"context"
	"fmt"
	"sync"

	runtimegovernanceapplication "github.com/orz-i/Mender/backend/internal/runtimegovernance/application"
	runtimegovernancedomain "github.com/orz-i/Mender/backend/internal/runtimegovernance/domain"
)

type Usage struct {
	Invocations int
	Activations int
}

type Controller struct {
	repository runtimegovernanceapplication.Repository

	mu          sync.Mutex
	invocations int
	activations int
	principals  map[string]int
	providers   map[string]int
	tools       map[string]int
}

func NewController(repository runtimegovernanceapplication.Repository) *Controller {
	return &Controller{
		repository: repository,
		principals: make(map[string]int),
		providers:  make(map[string]int),
		tools:      make(map[string]int),
	}
}

func (c *Controller) Snapshot(ctx context.Context) (runtimegovernancedomain.Budget, error) {
	if c == nil || c.repository == nil {
		return runtimegovernancedomain.Budget{}, fmt.Errorf("runtime budget controller is unavailable")
	}
	return c.repository.GetBudget(ctx)
}

func (c *Controller) AcquireInvocation(
	ctx context.Context,
	snapshot runtimegovernancedomain.Budget,
	principalRef string,
	providerID string,
	toolID string,
) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := snapshot.Validate(); err != nil {
		return nil, fmt.Errorf("runtime budget snapshot: %w", err)
	}
	if principalRef == "" || providerID == "" || toolID == "" {
		return nil, fmt.Errorf("runtime budget identity is incomplete")
	}

	c.mu.Lock()
	if c.invocations >= snapshot.MaxInvocations() {
		c.mu.Unlock()
		return nil, runtimegovernancedomain.BudgetExceededError{Dimension: "global_invocations"}
	}
	if c.principals[principalRef] >= snapshot.MaxPrincipalInvocations() {
		c.mu.Unlock()
		return nil, runtimegovernancedomain.BudgetExceededError{Dimension: "principal_invocations"}
	}
	if c.providers[providerID] >= snapshot.MaxProviderInvocations() {
		c.mu.Unlock()
		return nil, runtimegovernancedomain.BudgetExceededError{Dimension: "provider_invocations"}
	}
	if c.tools[toolID] >= snapshot.MaxToolInvocations() {
		c.mu.Unlock()
		return nil, runtimegovernancedomain.BudgetExceededError{Dimension: "tool_invocations"}
	}
	c.invocations++
	c.principals[principalRef]++
	c.providers[providerID]++
	c.tools[toolID]++
	c.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			c.mu.Lock()
			c.invocations--
			decrement(c.principals, principalRef)
			decrement(c.providers, providerID)
			decrement(c.tools, toolID)
			c.mu.Unlock()
		})
	}, nil
}

func (c *Controller) AcquireActivation(
	ctx context.Context,
	snapshot runtimegovernancedomain.Budget,
) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := snapshot.Validate(); err != nil {
		return nil, fmt.Errorf("runtime budget snapshot: %w", err)
	}
	c.mu.Lock()
	if c.activations >= snapshot.MaxProviderActivations() {
		c.mu.Unlock()
		return nil, runtimegovernancedomain.BudgetExceededError{Dimension: "provider_activations"}
	}
	c.activations++
	c.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			c.mu.Lock()
			c.activations--
			c.mu.Unlock()
		})
	}, nil
}

func (c *Controller) Usage() Usage {
	if c == nil {
		return Usage{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return Usage{Invocations: c.invocations, Activations: c.activations}
}

func decrement(values map[string]int, key string) {
	next := values[key] - 1
	if next <= 0 {
		delete(values, key)
		return
	}
	values[key] = next
}
