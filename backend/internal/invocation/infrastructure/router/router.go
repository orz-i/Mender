package router

import (
	"context"
	"fmt"
	"sync"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
)

// Router maps canonical Provider identities to Tool executors.
type Router struct {
	mu        sync.RWMutex
	executors map[catalogdomain.ProviderID]invocationapplication.ToolExecutor
}

func New() *Router {
	return &Router{
		executors: make(map[catalogdomain.ProviderID]invocationapplication.ToolExecutor),
	}
}

// Register associates one Provider with an executor.
//
// Registering the same Provider again replaces the previous executor so
// composition can explicitly refresh provider runtime wiring.
func (r *Router) Register(
	providerID catalogdomain.ProviderID,
	executor invocationapplication.ToolExecutor,
) error {
	if _, err := catalogdomain.ParseProviderID(providerID.String()); err != nil {
		return fmt.Errorf("register executor: %w", err)
	}
	if executor == nil {
		return fmt.Errorf("register executor: executor must not be nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.executors[providerID] = executor
	return nil
}

// ExecutorFor returns the executor registered for the Tool's Provider.
func (r *Router) ExecutorFor(
	_ context.Context,
	tool catalogdomain.Tool,
) (invocationapplication.ToolExecutor, error) {
	r.mu.RLock()
	executor, ok := r.executors[tool.ProviderID()]
	r.mu.RUnlock()
	if !ok {
		return nil, invocationapplication.ErrExecutorNotFound
	}
	return executor, nil
}
