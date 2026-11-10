package bootstrap

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"sync"
	"time"

	agentapp "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationapp "github.com/orz-i/Mender/backend/internal/invocation/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"
)

// agentTaskToolCapability is an ephemeral capability owned by the original
// Runtime, not a reusable principal or a profile-facing Tool argument.
// The exact ToolID allowlist comes from a trusted host configuration and
// the canonical, pinned Sandbox Workspace suite. It is never advertised to
// other MCP/Agent profiles.
type agentTaskToolCapability struct {
	taskID              agentdomain.TaskID
	agentID             agentdomain.ID
	generation          uint64
	profileID           string
	workspaceRoot       string
	workspaceID         workspace.Ref
	sandboxID           sandboxdomain.ID
	caller              gov.Context
	allowed             map[catalogdomain.ToolID]string // ToolID -> immutable Workspace SourceKey
	configuredPlacement *agentdomain.ToolPlacement      // nil for internal Phase 45 synthetic callers
	expiresAt           time.Time
}

type agentTaskToolGateway struct {
	mu      sync.Mutex
	tickets map[string]agentTaskToolCapability
	servers map[string]*taskScopedMCPServer
	closed  bool
}

const maxAgentToolCapabilities = 16

func (r *Runtime) closeAgentTaskToolGateway() {
	r.agentToolGateway.mu.Lock()
	r.agentToolGateway.closed = true
	clear(r.agentToolGateway.tickets)
	r.agentToolGateway.mu.Unlock()
}

// openAgentTaskToolCapability is only for the trusted Runtime/ACP transport.
// It is NOT exposed as a Wails, HTTP, MCP, CLI or model-callable action.
// It does not grant, install, create, or activate any Sandbox or Tool source.
func (r *Runtime) openAgentTaskToolCapability(ctx context.Context,
	taskID agentdomain.TaskID, sandboxID sandboxdomain.ID, requested []catalogdomain.ToolID) (string, error) {
	if r == nil || len(requested) == 0 || len(requested) > 3 {
		return "", agentdomain.ErrInvalid
	}
	if _, err := agentdomain.ParseTaskID(string(taskID)); err != nil {
		return "", err
	}
	if _, err := sandboxdomain.ParseID(sandboxID.String()); err != nil {
		return "", err
	}
	task, err := r.agentStore.GetTask(ctx, taskID)
	if err != nil {
		return "", err
	}
	if !r.Agents.TaskLocallyActive(taskID) {
		return "", agentdomain.ErrForbidden
	}
	if task.Status != agentdomain.TaskRunning || task.CancelRequested || task.Generation == 0 ||
		task.ScopeKind != string(gov.ScopeProfile) || task.PrincipalKind != string(gov.PrincipalService) ||
		task.PrincipalRef != "profile:"+task.ScopeRef || task.ScopeRef == "" ||
		time.Now().After(task.CreatedAt.Add(agentapp.TaskTimeout)) {
		return "", agentdomain.ErrForbidden
	}
	agent, err := r.agentStore.GetAgent(ctx, task.AgentID)
	if err != nil {
		return "", err
	}
	if !agent.OwnedBy(string(r.ownerPrincipal.Kind()), r.ownerPrincipal.Reference()) {
		return "", agentdomain.ErrForbidden
	}
	if err := r.verifyBoundTaskWorkspace(ctx, agent, task); err != nil {
		return "", err
	}
	profileID, err := profiledomain.ParseProfileID(task.ScopeRef)
	if err != nil {
		return "", agentdomain.ErrForbidden
	}
	allowed, err := r.agentStore.HasProfileGrant(ctx, task.AgentID, profileID.String())
	if err != nil {
		return "", err
	}
	if !allowed {
		return "", agentdomain.ErrForbidden
	}
	profile, err := r.Profiles.GetByID.Execute(ctx, profileID)
	if err != nil {
		return "", err
	}
	sandbox, err := r.Sandboxes.Get(ctx, sandboxID)
	if err != nil {
		return "", err
	}
	spec := sandbox.Spec()
	if spec.Backend != sandboxdomain.BackendLocalWorkspace ||
		spec.Isolation != sandboxdomain.IsolationRestrictedHost ||
		spec.LocalRoot != task.WorkspaceRoot || spec.OwnerKind != agent.Spec().OwnerKind ||
		spec.OwnerRef != agent.Spec().OwnerRef {
		return "", agentdomain.ErrForbidden
	}
	run, err := r.Sandboxes.GetRun(ctx, sandboxID)
	if err != nil {
		return "", err
	}
	if run.State != sandboxdomain.StateReady {
		return "", agentdomain.ErrUnavailable
	}
	owner, err := gov.NewContext(gov.OriginInternal, r.ownerPrincipal, gov.CatalogScope())
	if err != nil {
		return "", err
	}
	suite, err := r.SandboxWorkspaceTools(ctx, sandboxID, owner)
	if err != nil {
		return "", err
	}
	if suite.Status != "ready" {
		return "", agentdomain.ErrUnavailable
	}
	sources := make(map[catalogdomain.ToolID]string, len(suite.Tools))
	for _, tool := range suite.Tools {
		parsed, e := catalogdomain.ParseToolID(tool.ToolID)
		if e != nil {
			return "", e
		}
		sources[parsed] = tool.SourceKey
	}
	permitted := make(map[catalogdomain.ToolID]string, len(requested))
	for _, id := range requested {
		if _, exists := permitted[id]; exists {
			return "", agentdomain.ErrInvalid
		}
		source := sources[id]
		switch source {
		case "workspace.read", "workspace.search", "workspace.edit":
		default:
			return "", agentdomain.ErrForbidden // no shell, recursion, credential or owner actions
		}
		if !profile.Contains(id) {
			return "", agentdomain.ErrForbidden
		}
		permitted[id] = source
	}
	principal, err := gov.NewPrincipal(gov.PrincipalService, task.PrincipalRef)
	if err != nil {
		return "", err
	}
	scope, err := gov.ProfileScope(task.ScopeRef)
	if err != nil {
		return "", err
	}
	// Evaluate every Tool under the ORIGINAL, admitted Task origin rather
	// than substituting MCP origin (which could change Policy results).
	caller, err := gov.NewContext(gov.Origin(task.Origin), principal, scope)
	if err != nil {
		return "", err
	}
	// The workspace is NOT inferred from an Agent alias or a path string.
	// Resolve one existing, owner-attested and Profile-authorized Workspace
	// placement for this exact Sandbox and every selected native Tool.
	workspaceRef, err := r.resolveTaskNativeWorkspace(ctx, caller, sandboxID, task.WorkspaceRoot, requested)
	if err != nil {
		return "", err
	}
	secret := [32]byte{}
	if _, err := rand.Read(secret[:]); err != nil {
		return "", err
	}
	token := hex.EncodeToString(secret[:])
	expires := task.CreatedAt.Add(agentapp.TaskTimeout)
	r.agentToolGateway.mu.Lock()
	defer r.agentToolGateway.mu.Unlock()
	if r.agentToolGateway.closed {
		return "", agentdomain.ErrUnavailable
	}
	if r.agentToolGateway.tickets == nil {
		r.agentToolGateway.tickets = map[string]agentTaskToolCapability{}
	}
	for key, value := range r.agentToolGateway.tickets {
		if !time.Now().Before(value.expiresAt) {
			delete(r.agentToolGateway.tickets, key)
		}
	}
	if len(r.agentToolGateway.tickets) >= maxAgentToolCapabilities {
		return "", agentdomain.ErrConflict
	}
	r.agentToolGateway.tickets[token] = agentTaskToolCapability{
		taskID: taskID, agentID: task.AgentID, generation: task.Generation,
		profileID: profileID.String(), workspaceRoot: task.WorkspaceRoot, workspaceID: workspaceRef, sandboxID: sandboxID,
		caller: caller, allowed: permitted, expiresAt: expires,
	}
	return token, nil
}

// invokeAgentTaskTool rejects identity drift BEFORE entering InvokeTool.
// InvokeTool then independently rechecks live Profile ToolBinding, policy,
// runtime budget, Sandbox grant and generation lease for this operation.
func (r *Runtime) invokeAgentTaskTool(ctx context.Context, token string, id catalogdomain.ToolID,
	arguments []byte) (invocationapp.InvokeToolResponse, error) {
	if r == nil || len(token) != 64 || len(arguments) == 0 {
		return invocationapp.InvokeToolResponse{}, agentdomain.ErrForbidden
	}
	if _, err := catalogdomain.ParseToolID(id.String()); err != nil {
		return invocationapp.InvokeToolResponse{}, err
	}
	r.agentToolGateway.mu.Lock()
	capability, found := r.agentToolGateway.tickets[token]
	closed := r.agentToolGateway.closed
	r.agentToolGateway.mu.Unlock()
	if closed || !found || !time.Now().Before(capability.expiresAt) {
		return invocationapp.InvokeToolResponse{}, agentdomain.ErrForbidden
	}
	if _, ok := capability.allowed[id]; !ok {
		return invocationapp.InvokeToolResponse{}, agentdomain.ErrForbidden
	}
	if capability.configuredPlacement != nil {
		pinned := capability.configuredPlacement
		current, readErr := r.agentStore.GetToolPlacement(ctx, capability.agentID, pinned.WorkspaceKey)
		if readErr != nil || !agentdomain.SameToolPlacement(current, *pinned) {
			return invocationapp.InvokeToolResponse{}, agentdomain.ErrForbidden
		}
	}
	task, err := r.agentStore.GetTask(ctx, capability.taskID)
	if err != nil {
		return invocationapp.InvokeToolResponse{}, err
	}
	if !r.Agents.TaskLocallyActive(capability.taskID) {
		return invocationapp.InvokeToolResponse{}, agentdomain.ErrForbidden
	}
	if task.Status != agentdomain.TaskRunning || task.CancelRequested || task.ID != capability.taskID ||
		task.AgentID != capability.agentID || task.Generation != capability.generation ||
		task.ScopeKind != string(gov.ScopeProfile) || task.ScopeRef != capability.profileID ||
		task.PrincipalKind != string(gov.PrincipalService) || task.PrincipalRef != "profile:"+capability.profileID ||
		task.WorkspaceRoot != capability.workspaceRoot {
		return invocationapp.InvokeToolResponse{}, agentdomain.ErrForbidden
	}
	agent, err := r.agentStore.GetAgent(ctx, task.AgentID)
	if err != nil {
		return invocationapp.InvokeToolResponse{}, err
	}
	if err := r.verifyBoundTaskWorkspace(ctx, agent, task); err != nil {
		return invocationapp.InvokeToolResponse{}, err
	}
	granted, err := r.agentStore.HasProfileGrant(ctx, task.AgentID, capability.profileID)
	if err != nil {
		return invocationapp.InvokeToolResponse{}, err
	}
	if !granted {
		return invocationapp.InvokeToolResponse{}, agentdomain.ErrForbidden
	}
	profileID, err := profiledomain.ParseProfileID(capability.profileID)
	if err != nil {
		return invocationapp.InvokeToolResponse{}, err
	}
	profile, err := r.Profiles.GetByID.Execute(ctx, profileID)
	if err != nil {
		return invocationapp.InvokeToolResponse{}, err
	}
	if !profile.Contains(id) {
		return invocationapp.InvokeToolResponse{}, agentdomain.ErrForbidden
	}
	sandbox, err := r.Sandboxes.Get(ctx, capability.sandboxID)
	if err != nil {
		return invocationapp.InvokeToolResponse{}, err
	}
	spec := sandbox.Spec()
	if spec.Backend != sandboxdomain.BackendLocalWorkspace ||
		spec.Isolation != sandboxdomain.IsolationRestrictedHost ||
		spec.LocalRoot != task.WorkspaceRoot || spec.OwnerKind != agent.Spec().OwnerKind ||
		spec.OwnerRef != agent.Spec().OwnerRef {
		return invocationapp.InvokeToolResponse{}, agentdomain.ErrForbidden
	}
	run, err := r.Sandboxes.GetRun(ctx, capability.sandboxID)
	if err != nil {
		return invocationapp.InvokeToolResponse{}, err
	}
	if run.State != sandboxdomain.StateReady {
		return invocationapp.InvokeToolResponse{}, agentdomain.ErrUnavailable
	}
	// Revalidate the original pinned Tool identity. A Provider source replacement
	// cannot turn an allowlisted read into a different Tool class.
	tool, err := r.Catalog.store.GetTool(ctx, id)
	if err != nil {
		return invocationapp.InvokeToolResponse{}, err
	}
	if tool.SourceKey() != capability.allowed[id] ||
		tool.SourceRef().Kind() != catalogdomain.SourceNative ||
		tool.SourceRef().ID() != "workspace" {
		return invocationapp.InvokeToolResponse{}, agentdomain.ErrForbidden
	}
	// Recheck exact Profile TargetScope, Workspace grant and Sandbox grant
	// before entering the canonical Invocation. Revocation invalidates an
	// already-issued task bearer; the atomic Native lease checks again.
	ref, err := r.resolveTaskNativeWorkspace(ctx, capability.caller,
		capability.sandboxID, capability.workspaceRoot, []catalogdomain.ToolID{id})
	if err != nil || ref != capability.workspaceID {
		return invocationapp.InvokeToolResponse{}, agentdomain.ErrForbidden
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(arguments, &fields) != nil || fields == nil {
		return invocationapp.InvokeToolResponse{}, agentdomain.ErrInvalid
	}
	if raw, exists := fields["workspaceId"]; exists {
		var callerRef string
		if json.Unmarshal(raw, &callerRef) != nil || callerRef != ref.String() {
			return invocationapp.InvokeToolResponse{}, agentdomain.ErrForbidden
		}
	}
	trusted, err := json.Marshal(ref.String())
	if err != nil {
		return invocationapp.InvokeToolResponse{}, err
	}
	fields["workspaceId"] = trusted // never allow the model to select a different workspace
	verified, err := json.Marshal(fields)
	if err != nil {
		return invocationapp.InvokeToolResponse{}, err
	}
	return r.CallConfiguredTool(ctx, id, verified, capability.caller)
}

// resolveTaskNativeWorkspace requires ONE pre-existing, fully authorized
// WorkspaceRef whose verified local Sandbox root matches the admitted Task.
// Ambiguous roots, stale grants and missing target scopes all fail closed.
func (r *Runtime) resolveTaskNativeWorkspace(ctx context.Context, caller gov.Context,
	sandboxID sandboxdomain.ID, root string, ids []catalogdomain.ToolID) (workspace.Ref, error) {
	if r == nil || r.workspaceInventory == nil || len(ids) == 0 ||
		caller.Scope().Kind() != gov.ScopeProfile {
		return "", agentdomain.ErrForbidden
	}
	profileID := caller.Scope().Reference()
	refs, err := r.workspaceInventory.ListAccessibleWorkspaces(ctx, caller)
	if err != nil {
		return "", agentdomain.ErrForbidden
	}
	var selected workspace.Ref
	for _, ref := range refs {
		placements, err := r.workspaceInventory.ListWorkspacePlacements(ctx, ref)
		if err != nil {
			return "", agentdomain.ErrForbidden
		}
		for _, placement := range placements {
			if placement.SandboxID != sandboxID.String() || placement.TargetRoot != root {
				continue
			}
			target, err := profiledomain.NewExecutionTarget(ref.String(), sandboxID.String())
			if err != nil {
				return "", agentdomain.ErrForbidden
			}
			admitted := true
			for _, id := range ids {
				tool, err := r.Catalog.store.GetTool(ctx, id)
				if err != nil || tool.SourceRef().Kind() != catalogdomain.SourceNative ||
					tool.SourceRef().ID() != "workspace" {
					return "", agentdomain.ErrForbidden
				}
				var required sandboxdomain.Capability
				switch tool.SourceKey() {
				case "workspace.read":
					required = sandboxdomain.CapabilityRead
				case "workspace.search":
					required = sandboxdomain.CapabilitySearch
				case "workspace.edit":
					required = sandboxdomain.CapabilityEdit
				default:
					return "", agentdomain.ErrForbidden
				}
				scope, found, err := r.workspaceInventory.BindingTargetScope(ctx, profileID, id)
				if err != nil || !found || !scope.Allows(target) {
					admitted = false
					break
				}
				grant, err := r.Sandboxes.store.HasProfileGrant(ctx, sandboxID, profileID, required)
				if err != nil || !grant {
					admitted = false
					break
				}
			}
			if admitted {
				if selected != "" {
					return "", agentdomain.ErrConflict
				}
				selected = ref
			}
		}
	}
	if selected == "" {
		return "", agentdomain.ErrForbidden
	}
	return selected, nil
}

// closeAgentTaskToolCapability expires the bearer token regardless of whether
// Task or Agent state remains in history.
func (r *Runtime) closeAgentTaskToolCapability(token string) {
	r.agentToolGateway.mu.Lock()
	delete(r.agentToolGateway.tickets, token)
	r.agentToolGateway.mu.Unlock()
}

// verifyBoundTaskWorkspace checks the SAME originally admitted root/key. The
// task cannot use an alias re-bound to a different local path or grant.
func (r *Runtime) verifyBoundTaskWorkspace(ctx context.Context,
	agent agentdomain.Agent, task agentdomain.Task) error {
	root := task.WorkspaceRoot
	if root == "" || !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return agentdomain.ErrForbidden
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil || canonical != root {
		return agentdomain.ErrForbidden
	}
	if task.WorkspaceKey == "" {
		if agent.Spec().WorkspaceRoot != root {
			return agentdomain.ErrForbidden
		}
	} else {
		current, err := r.agentStore.Workspace(ctx, task.AgentID, task.WorkspaceKey)
		if err != nil || current != root {
			return agentdomain.ErrForbidden
		}
		authorized, err := r.agentStore.HasWorkspaceGrant(ctx, task.AgentID, task.WorkspaceKey, task.ScopeRef)
		if err != nil || !authorized {
			return agentdomain.ErrForbidden
		}
	}
	return nil
}
