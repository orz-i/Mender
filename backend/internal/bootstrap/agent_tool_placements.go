package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"time"

	agentapp "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// ListAgentToolPlacements reads only the owner's per-workspace execution
// choices. They are not Profile-facing tool descriptions or permissions.
func (r *Runtime) ListAgentToolPlacements(ctx context.Context, agentID agentdomain.ID, owner gov.Context) ([]agentdomain.ToolPlacement, error) {
	if _, err := r.Agents.Get(ctx, agentID, owner); err != nil {
		return nil, err
	}
	return r.agentStore.ListToolPlacements(ctx, agentID)
}
func (r *Runtime) ConfigureAgentToolPlacement(ctx context.Context, agentID agentdomain.ID,
	key, sandboxID string, toolIDs []string, owner gov.Context) (agentdomain.ToolPlacement, error) {
	agent, err := r.Agents.Get(ctx, agentID, owner)
	if err != nil {
		return agentdomain.ToolPlacement{}, err
	}
	if agent.Spec().Isolation != "restricted_host" {
		return agentdomain.ToolPlacement{}, fmt.Errorf("%w: governed tools require an Agent with restricted_host isolation", agentdomain.ErrUnavailable)
	}
	root := agent.Spec().WorkspaceRoot
	if key != "" {
		root, err = r.agentStore.Workspace(ctx, agentID, key)
		if err != nil {
			return agentdomain.ToolPlacement{}, err
		}
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil || canonical != root {
		return agentdomain.ToolPlacement{}, agentdomain.ErrUnavailable
	}
	sandboxKey, err := sandboxdomain.ParseID(sandboxID)
	if err != nil {
		return agentdomain.ToolPlacement{}, agentdomain.ErrInvalid
	}
	sandbox, err := r.Sandboxes.Get(ctx, sandboxKey)
	if err != nil {
		return agentdomain.ToolPlacement{}, err
	}
	spec := sandbox.Spec()
	if spec.Backend != sandboxdomain.BackendLocalWorkspace ||
		spec.Isolation != sandboxdomain.IsolationRestrictedHost ||
		spec.LocalRoot != root || spec.OwnerKind != agent.Spec().OwnerKind ||
		spec.OwnerRef != agent.Spec().OwnerRef {
		return agentdomain.ToolPlacement{}, fmt.Errorf("%w: Sandbox does not match this Agent workspace", agentdomain.ErrForbidden)
	}
	run, err := r.Sandboxes.GetRun(ctx, sandboxKey)
	if err != nil {
		return agentdomain.ToolPlacement{}, err
	}
	if run.State != sandboxdomain.StateReady {
		return agentdomain.ToolPlacement{}, agentdomain.ErrUnavailable
	}
	state, err := r.SandboxWorkspaceTools(ctx, sandboxKey, owner)
	if err != nil {
		return agentdomain.ToolPlacement{}, err
	}
	if state.Status != "ready" {
		return agentdomain.ToolPlacement{}, agentdomain.ErrUnavailable
	}
	toolSources := make(map[string]string, len(state.Tools))
	for _, item := range state.Tools {
		toolSources[item.ToolID] = item.SourceKey
	}
	if len(toolIDs) == 0 || len(toolIDs) > 3 {
		return agentdomain.ToolPlacement{}, agentdomain.ErrInvalid
	}
	normalized := append([]string(nil), toolIDs...)
	sort.Strings(normalized)
	for i, tid := range normalized {
		if _, err := catalogdomain.ParseToolID(tid); err != nil {
			return agentdomain.ToolPlacement{}, agentdomain.ErrInvalid
		}
		if i > 0 && normalized[i-1] == tid {
			return agentdomain.ToolPlacement{}, agentdomain.ErrInvalid
		}
		switch toolSources[tid] {
		case "workspace.read", "workspace.search", "workspace.edit":
		default:
			return agentdomain.ToolPlacement{}, agentdomain.ErrForbidden
		}
	}
	placement := agentdomain.ToolPlacement{
		AgentID: agentID, WorkspaceKey: key, WorkspaceRoot: root, SandboxID: sandboxID,
		ToolIDs: normalized, UpdatedAt: time.Now().UTC(),
	}
	if err := r.agentStore.PutToolPlacement(ctx, placement); err != nil {
		return agentdomain.ToolPlacement{}, err
	}
	r.closeAgentPlacementServers(agentID, key)
	return placement, nil
}
func (r *Runtime) DisableAgentToolPlacement(ctx context.Context, id agentdomain.ID, key string, owner gov.Context) error {
	if _, err := r.Agents.Get(ctx, id, owner); err != nil {
		return err
	}
	if err := r.agentStore.DeleteToolPlacement(ctx, id, key); err != nil {
		return err
	}
	r.closeAgentPlacementServers(id, key)
	return nil
}

func (r *Runtime) resolveAgentToolSupply(ctx context.Context, task agentdomain.Task) (agentapp.ToolSupply, func(), error) {
	empty := agentapp.ToolSupply{}
	noop := func() {}
	placement, err := r.agentStore.GetToolPlacement(ctx, task.AgentID, task.WorkspaceKey)
	if errors.Is(err, agentdomain.ErrNotFound) {
		return empty, noop, nil
	}
	if err != nil {
		return empty, noop, err
	}
	if placement.WorkspaceRoot != task.WorkspaceRoot {
		return empty, noop, fmt.Errorf("%w: governed tool workspace changed after admission", agentdomain.ErrConflict)
	}
	agent, err := r.agentStore.GetAgent(ctx, task.AgentID)
	if err != nil {
		return empty, noop, err
	}
	if agent.Spec().Isolation != "restricted_host" {
		return empty, noop, fmt.Errorf("%w: governed tool placement requires restricted_host", agentdomain.ErrUnavailable)
	}
	sandboxID, err := sandboxdomain.ParseID(placement.SandboxID)
	if err != nil {
		return empty, noop, err
	}
	selected := make([]catalogdomain.ToolID, 0, len(placement.ToolIDs))
	for _, raw := range placement.ToolIDs {
		id, parseErr := catalogdomain.ParseToolID(raw)
		if parseErr != nil {
			return empty, noop, agentdomain.ErrInvalid
		}
		selected = append(selected, id)
	}
	scoped, err := r.openAgentTaskToolMCPServer(ctx, task.ID, sandboxID, selected)
	if err != nil {
		return empty, noop, err
	}
	// Pin the exact owner placement to this active capability. Every subsequent
	// Tool invocation compares it against current owner state and denies
	// immediately if an owner disables/reconfigures the placement.
	r.agentToolGateway.mu.Lock()
	existing, exists := r.agentToolGateway.tickets[scoped.token]
	if exists {
		pinned := placement
		pinned.ToolIDs = slices.Clone(placement.ToolIDs)
		existing.configuredPlacement = &pinned
		r.agentToolGateway.tickets[scoped.token] = existing
	}
	r.agentToolGateway.mu.Unlock()
	current, err := r.agentStore.GetToolPlacement(ctx, task.AgentID, task.WorkspaceKey)
	if err != nil || !exists || !agentdomain.SameToolPlacement(current, placement) {
		_ = scoped.close()
		return empty, noop, fmt.Errorf("%w: governed tool configuration was changed", agentdomain.ErrConflict)
	}
	supply := agentapp.ToolSupply{Delivery: agentapp.ToolDeliveryMCPHTTP, Endpoint: scoped.endpoint, BearerToken: scoped.token}
	return supply, func() { _ = scoped.close() }, nil
}
func (r *Runtime) closeAgentPlacementServers(id agentdomain.ID, key string) {
	r.agentToolGateway.mu.Lock()
	stale := []*taskScopedMCPServer{}
	for token, capability := range r.agentToolGateway.tickets {
		if capability.configuredPlacement != nil &&
			capability.agentID == id && capability.configuredPlacement.WorkspaceKey == key {
			if server := r.agentToolGateway.servers[token]; server != nil {
				stale = append(stale, server)
			}
		}
	}
	r.agentToolGateway.mu.Unlock()
	for _, item := range stale {
		_ = item.close()
	}
}
