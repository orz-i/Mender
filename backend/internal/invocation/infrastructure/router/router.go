package router

import (
	"context"
	"fmt"
	"sync"

	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governance "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocation "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	"github.com/orz-i/Mender/backend/internal/toolsuite/infrastructure/curated"
)

// Router binds exact verified canonical Tool sources to executors. The current
// Runtime still registers only Provider sources; SourceRef-aware Agent and
// native registrations are reserved for the E/F atomic cutover.
type Router struct {
	mu        sync.RWMutex
	executors map[catalog.SourceRef]invocation.ToolExecutor
}

func New() *Router {
	return &Router{executors: make(map[catalog.SourceRef]invocation.ToolExecutor)}
}

// Register retains the current Provider-only composition API, without
// creating another source, granting a Profile, or activating a backend.
func (r *Router) Register(providerID catalog.ProviderID, executor invocation.ToolExecutor) error {
	source, err := catalog.ProviderSource(providerID)
	if err != nil {
		return fmt.Errorf("register provider executor: %w", err)
	}
	return r.RegisterSource(source, executor)
}

// RegisterSource is a trusted internal composition operation. The caller must
// independently verify the resource owner before registering an Agent/Native
// SourceRef. It is not a public Tool registration or authorization API.
func (r *Router) RegisterSource(source catalog.SourceRef, executor invocation.ToolExecutor) error {
	if err := source.Validate(); err != nil {
		return fmt.Errorf("register source executor: %w", err)
	}
	if executor == nil {
		return fmt.Errorf("register source executor: executor must not be nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.executors[source] = executor
	return nil
}

func (r *Router) Unregister(providerID catalog.ProviderID) error {
	source, err := catalog.ProviderSource(providerID)
	if err != nil {
		return fmt.Errorf("unregister provider executor: %w", err)
	}
	return r.UnregisterSource(source)
}

func (r *Router) UnregisterSource(source catalog.SourceRef) error {
	if err := source.Validate(); err != nil {
		return fmt.Errorf("unregister source executor: %w", err)
	}
	r.mu.Lock()
	delete(r.executors, source)
	r.mu.Unlock()
	return nil
}

func (r *Router) HasProvider(providerID catalog.ProviderID) bool {
	source, err := catalog.ProviderSource(providerID)
	return err == nil && r.HasSource(source)
}

func (r *Router) HasSource(source catalog.SourceRef) bool {
	if source.Validate() != nil {
		return false
	}
	r.mu.RLock()
	_, ok := r.executors[source]
	r.mu.RUnlock()
	return ok
}

// ExecutorFor refuses an unverified Tool/source combination even when the
// source itself was registered. It cannot route by SandboxID, Tool title,
// backend family or unspecified fallback Provider.
func (r *Router) ExecutorFor(_ context.Context, tool catalog.Tool) (invocation.ToolExecutor, error) {
	source := tool.SourceRef()
	if source.Validate() != nil || !verifiedOperation(source, tool) {
		return nil, invocation.ErrExecutorNotFound
	}
	r.mu.RLock()
	executor, ok := r.executors[source]
	r.mu.RUnlock()
	if !ok {
		return nil, invocation.ErrExecutorNotFound
	}
	return executor, nil
}

// Extract delegates only to the *same* SourceRef-registered executor used by
// Invocation. Future Agent/native executors must implement the trusted facts
// port; a merely executable Provider without this port cannot fabricate
// Workspace/Sandbox admission. Current v2 Provider facts remain in bootstrap
// until F4 removes their Provider-based composition atomically.
func (r *Router) Extract(ctx context.Context, tool catalog.Tool, args invocationdomain.JSONValue,
	caller governance.Context,
) (invocation.GovernanceFactExtraction, error) {
	// External Provider plugins cannot supply authoritative policy facts by
	// implementing a Go interface. Their existing host-reviewed extractor is
	// the sole authority, even after the F4 cutover.
	if tool.SourceRef().Kind() == catalog.SourceProvider {
		return invocation.GovernanceFactExtraction{}, fmt.Errorf("provider facts require host-reviewed configuration")
	}
	executor, err := r.ExecutorFor(ctx, tool)
	if err != nil {
		return invocation.GovernanceFactExtraction{}, err
	}
	facts, ok := executor.(invocation.GovernanceFactExtractor)
	if !ok {
		return invocation.GovernanceFactExtraction{}, fmt.Errorf("tool source has no reviewed governance fact extractor")
	}
	return facts.Extract(ctx, tool, args, caller)
}

var _ invocation.GovernanceFactExtractor = (*Router)(nil)

func verifiedOperation(source catalog.SourceRef, tool catalog.Tool) bool {
	switch source.Kind() {
	case catalog.SourceProvider:
		return true // Provider sync and Plugin discovery own SourceKey validation.
	case catalog.SourceAgent:
		switch tool.SourceKey() {
		case "agent.delegate", "agent.status", "agent.result", "agent.cancel", "agent.resume":
			return true
		default:
			return false
		}
	case catalog.SourceNative:
		entries, err := curated.PlannedNativeTools()
		if err != nil {
			return false
		}
		for _, entry := range entries {
			if entry.Source == source && entry.ToolID == tool.ID() && entry.SourceKey == tool.SourceKey() {
				return true
			}
		}
	}
	return false
}
