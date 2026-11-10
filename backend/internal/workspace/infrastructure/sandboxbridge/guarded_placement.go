package sandboxbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governance "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocation "github.com/orz-i/Mender/backend/internal/invocation/domain"
	profile "github.com/orz-i/Mender/backend/internal/profile/domain"
	sandboxapp "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandbox "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	"github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/typedlease"
	"github.com/orz-i/Mender/backend/internal/toolsuite/infrastructure/curated"
	workspaceapp "github.com/orz-i/Mender/backend/internal/workspace/application"
	workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"
)

// WorkspaceAuthority is the host-owned, read-only Workspace access check.
// A valid WorkspaceRef or existing placement does not grant execution.
type WorkspaceAuthority interface {
	MayUseWorkspace(context.Context, governance.Context, workspace.Ref) (bool, error)
}

// BindingScopeLookup reads the *current* exact Profile ToolBinding target
// restriction. Missing or stale records MUST fail closed; the official
// Profile v3 persistence is the authority for the current binding.
type BindingScopeLookup interface {
	BindingTargetScope(context.Context, string, catalog.ToolID) (profile.TargetScope, bool, error)
}

// RootAttestor verifies that a candidate's backend-specific TargetRoot is
// really bound to the selected Sandbox. It must be host-owned and inert;
// a Plugin-provided description or matching path string is not a proof.
type RootAttestor interface {
	AttestRoot(context.Context, workspaceapp.PlacementCandidate, sandboxapp.Placement) (bool, error)
}

// InvocationAdmissionVerifier checks an already-approved canonical Invocation
// attempt (Policy, Approval, ToolBinding and Runtime capacity). It is supplied
// by Invocation composition, never by the Plugin, caller or Sandbox router.
// Matching target facts alone is not evidence that a Tool was admitted.
type InvocationAdmissionVerifier interface {
	VerifyAdmittedAttempt(context.Context, invocation.InvocationID, governance.Context, catalog.ToolID, invocation.GovernanceFacts) error
}

// GuardedDependencies deliberately require every authority; constructing a
// route must never bypass a missing Profile or Workspace access source.
type GuardedDependencies struct {
	Placements workspaceapp.PlacementQuery
	Router     sandboxapp.ExecutionRouter
	Backends   sandboxapp.BackendResolver
	Grants     sandboxapp.ProfileDelegationLookup
	Workspaces WorkspaceAuthority
	Bindings   BindingScopeLookup
	Roots      RootAttestor
	Admissions InvocationAdmissionVerifier
}

// GuardedRouter is the production v3 native Workspace placement and lease
// adapter. It consumes only an approved canonical Invocation attempt; no
// caller-supplied target facts can authorize a Tool call.
type GuardedRouter struct{ deps GuardedDependencies }

func NewGuardedRouter(deps GuardedDependencies) (GuardedRouter, error) {
	if deps.Router == nil || deps.Backends == nil || deps.Grants == nil ||
		deps.Workspaces == nil || deps.Bindings == nil || deps.Roots == nil || deps.Admissions == nil {
		return GuardedRouter{}, fmt.Errorf("governed Workspace router: all authorities required")
	}
	return GuardedRouter{deps: deps}, nil
}

// ResolveRequest is constructed by trusted Invocation composition, never
// directly from model arguments. A pinned Sandbox and isolation class are
// owner-approved decisions, not loose heuristics or an ordered security tier.
type ResolveRequest struct {
	Caller            governance.Context
	ToolID            catalog.ToolID
	Workspace         workspace.Ref
	Operation         workspace.Operation
	PinnedSandboxID   string
	RequiredIsolation sandbox.IsolationClass
	// Arguments are the original canonical, schema-validated invocation bytes.
	// The host extracts the workspace_path Policy fact from them (not from a
	// model-supplied target fact) and freezes a defensive copy for execution.
	Arguments []byte
}

// FrozenPlacement captures the complete authorized candidate and its observed
// Sandbox specification, backend binding, and run generation before Policy.
// Its fields are private to prevent callers from changing the execution
// target between reviewed facts and lease acquisition.
type FrozenPlacement struct {
	request   ResolveRequest
	candidate workspaceapp.PlacementCandidate
	placement sandboxapp.Placement
	facts     invocation.GovernanceFacts
}

func (p FrozenPlacement) ToolID() catalog.ToolID         { return p.request.ToolID }
func (p FrozenPlacement) Operation() workspace.Operation { return p.request.Operation }
func (p FrozenPlacement) Arguments() []byte              { return bytes.Clone(p.request.Arguments) }
func (p FrozenPlacement) Workspace() workspace.Ref       { return p.request.Workspace }
func (p FrozenPlacement) SandboxID() sandbox.ID          { return p.placement.Sandbox.ID() }
func (p FrozenPlacement) RunGeneration() uint64          { return p.placement.Run.Generation }
func (p FrozenPlacement) Facts() invocation.GovernanceFacts {
	if p.facts.Validate() != nil {
		return invocation.GovernanceFacts{}
	}
	return p.facts // GovernanceFacts exposes only copy-returning accessors.
}

type guardedInspector func(context.Context, workspace.Operation, workspaceapp.PlacementCandidate) (bool, error)

func (f guardedInspector) InspectPlacement(ctx context.Context, op workspace.Operation, candidate workspaceapp.PlacementCandidate) (bool, error) {
	return f(ctx, op, candidate)
}

// Resolve is a side-effect-free target selection. It never calls Acquire,
// starts a process, opens a backend, mounts a Workspace or resolves secrets.
// It returns the *exact* reviewed facts to feed the canonical Policy gate.
func (r GuardedRouter) Resolve(ctx context.Context, req ResolveRequest) (FrozenPlacement, error) {
	if err := ctx.Err(); err != nil {
		return FrozenPlacement{}, err
	}
	if err := r.validateRequest(req); err != nil {
		return FrozenPlacement{}, err
	}
	if ok, err := r.deps.Workspaces.MayUseWorkspace(ctx, req.Caller, req.Workspace); err != nil {
		return FrozenPlacement{}, err
	} else if !ok {
		return FrozenPlacement{}, sandbox.ErrNotAuthorized
	}
	scope, err := r.currentBindingScope(ctx, req)
	if err != nil {
		return FrozenPlacement{}, err
	}
	observed := map[string]sandboxapp.Placement{}
	selected, err := r.deps.Placements.Select(ctx, req.Workspace, req.Operation, req.PinnedSandboxID,
		guardedInspector(func(ctx context.Context, _ workspace.Operation, candidate workspaceapp.PlacementCandidate) (bool, error) {
			placement, eligible, err := r.inspect(ctx, req, candidate, scope)
			if err != nil {
				return false, err
			}
			if eligible {
				observed[candidate.SandboxID] = placement
			}
			return eligible, nil
		}))
	if err != nil {
		return FrozenPlacement{}, err
	}
	placement, ok := observed[selected.SandboxID]
	if !ok || placement.Sandbox.ID().String() != selected.SandboxID {
		return FrozenPlacement{}, sandbox.ErrConflict
	}
	facts, err := selectionFacts(req.Operation, req.Workspace, placement.Sandbox.ID(), req.Arguments)
	if err != nil {
		return FrozenPlacement{}, err
	}
	req.Arguments = bytes.Clone(req.Arguments)
	return FrozenPlacement{request: req, candidate: selected, placement: placement, facts: facts}, nil
}

func (r GuardedRouter) validateRequest(req ResolveRequest) error {
	if err := req.Caller.Validate(); err != nil {
		return fmt.Errorf("governed Workspace caller: %w", err)
	}
	if err := req.Workspace.Validate(); err != nil {
		return fmt.Errorf("governed Workspace identity: %w", err)
	}
	// An omitted class means the caller did not require a stricter class,
	// NOT that the runtime may select any arbitrary target: Profile TargetScope,
	// Sandbox grant, exact Workspace root, verified Backend and unique placement
	// are still enforced. The selected Sandbox isolation is pinned in the lease.
	if req.RequiredIsolation != "" {
		if err := req.RequiredIsolation.Validate(); err != nil {
			return fmt.Errorf("governed Workspace isolation: %w", err)
		}
	}
	if req.PinnedSandboxID != "" {
		if _, err := sandbox.ParseID(req.PinnedSandboxID); err != nil {
			return err
		}
	}
	expected, err := operationToolID(req.Operation)
	if err != nil {
		return err
	}
	if expected != req.ToolID {
		return fmt.Errorf("governed Workspace: ToolID is not the registered native %s capability", req.Operation)
	}
	return nil
}

func operationToolID(op workspace.Operation) (catalog.ToolID, error) {
	tools, err := curated.PlannedNativeTools()
	if err != nil {
		return "", err
	}
	for _, tool := range tools {
		if tool.Source.ID() == "workspace" && string(tool.Capability) == string(op) &&
			tool.SourceKey == operationSourceKey(op) {
			return tool.ToolID, nil
		}
	}
	return "", sandbox.ErrCapabilityUnavailable
}

func operationSourceKey(op workspace.Operation) string {
	switch op {
	case workspace.Read:
		return "workspace.read"
	case workspace.Search:
		return "workspace.search"
	case workspace.Edit:
		return "workspace.edit"
	case workspace.Shell:
		return "workspace.shell"
	}
	return ""
}

// currentBindingScope uses the exact authenticated Profile MCP principal.
// Catalog owner calls have no Profile ToolBinding, but must be the actual
// Workspace/Sandbox owner and explicitly select a trusted Workspace.
func (r GuardedRouter) currentBindingScope(ctx context.Context, req ResolveRequest) (profile.TargetScope, error) {
	if req.Caller.Scope().Kind() != governance.ScopeProfile {
		if req.Caller.Scope().Kind() != governance.ScopeCatalog ||
			req.Caller.Principal().Kind() != governance.PrincipalLocalUser {
			return profile.TargetScope{}, sandbox.ErrNotAuthorized
		}
		switch req.Caller.Origin() {
		case governance.OriginCLI, governance.OriginDesktop, governance.OriginManagementHTTP:
			return profile.TargetScope{}, nil
		default:
			return profile.TargetScope{}, sandbox.ErrNotAuthorized
		}
	}
	id, ok := sandboxapp.ProfileInvocationID(req.Caller)
	if !ok {
		return profile.TargetScope{}, sandbox.ErrNotAuthorized
	}
	scope, found, err := r.deps.Bindings.BindingTargetScope(ctx, id, req.ToolID)
	if err != nil {
		return profile.TargetScope{}, err
	}
	if !found || len(scope.Targets()) == 0 {
		return profile.TargetScope{}, sandbox.ErrNotAuthorized
	}
	return scope, nil
}

func (r GuardedRouter) inspect(
	ctx context.Context, req ResolveRequest, candidate workspaceapp.PlacementCandidate, scope profile.TargetScope,
) (sandboxapp.Placement, bool, error) {
	target, err := profile.NewExecutionTarget(req.Workspace.String(), candidate.SandboxID)
	if err != nil {
		return sandboxapp.Placement{}, false, err
	}
	if req.Caller.Scope().Kind() == governance.ScopeProfile && !scope.Allows(target) {
		return sandboxapp.Placement{}, false, nil
	}
	capability := sandbox.Capability(req.Operation)
	placement, err := r.deps.Router.Resolve(ctx, sandbox.ID(candidate.SandboxID), capability)
	if err != nil {
		if errors.Is(err, sandbox.ErrNotFound) || errors.Is(err, sandbox.ErrBackendUnavailable) ||
			errors.Is(err, sandbox.ErrCapabilityUnavailable) || errors.Is(err, sandbox.ErrConflict) {
			return sandboxapp.Placement{}, false, nil
		}
		return sandboxapp.Placement{}, false, err
	}
	if placement.Sandbox.Validate() != nil || placement.Run.SandboxID != placement.Sandbox.ID() ||
		placement.Sandbox.ID().String() != candidate.SandboxID || placement.Required != capability ||
		placement.Run.Generation == 0 || placement.Run.State != sandbox.StateReady ||
		(req.RequiredIsolation != "" && placement.Sandbox.Isolation() != req.RequiredIsolation) {
		return sandboxapp.Placement{}, false, nil
	}
	// The exact registered backend must prove the operation, not a Plugin
	// descriptor's untrusted advertising claims or a host fallback.
	backend, err := r.deps.Backends.ResolveBackend(placement.Sandbox.Backend(), placement.Sandbox.Spec().Binding, capability)
	if err != nil {
		if errors.Is(err, sandbox.ErrBackendUnavailable) || errors.Is(err, sandbox.ErrCapabilityUnavailable) {
			return sandboxapp.Placement{}, false, nil
		}
		return sandboxapp.Placement{}, false, err
	}
	if backend == nil || backend.Kind() != placement.Sandbox.Backend() ||
		!backend.Capabilities().Supports(capability) {
		return sandboxapp.Placement{}, false, nil
	}
	rootMatches, err := r.deps.Roots.AttestRoot(ctx, candidate, placement)
	if err != nil {
		return sandboxapp.Placement{}, false, err
	}
	if !rootMatches {
		return sandboxapp.Placement{}, false, nil
	}
	if sandboxapp.LocalCatalogOwner(req.Caller) && sandboxapp.IsOwner(req.Caller, placement.Sandbox) {
		return placement, true, nil
	}
	id, ok := sandboxapp.ProfileInvocationID(req.Caller)
	if !ok {
		return sandboxapp.Placement{}, false, nil
	}
	granted, err := r.deps.Grants.HasProfileGrant(ctx, placement.Sandbox.ID(), id, capability)
	if err != nil {
		return sandboxapp.Placement{}, false, err
	}
	return placement, granted, nil
}

func selectionFacts(op workspace.Operation, ref workspace.Ref, id sandbox.ID, arguments []byte) (invocation.GovernanceFacts, error) {
	pathFact, err := reviewedWorkspacePath(op, ref, arguments)
	if err != nil {
		return invocation.GovernanceFacts{}, err
	}
	var effect invocation.EffectFact
	switch op {
	case workspace.Read:
		effect = "filesystem.read"
	case workspace.Search:
		effect = "filesystem.search"
	case workspace.Edit:
		effect = "filesystem.write"
	case workspace.Shell:
		effect = "process.execute"
	default:
		return invocation.GovernanceFacts{}, sandbox.ErrCapabilityUnavailable
	}
	w, err := invocation.NewResourceFact("workspace_id", ref.String())
	if err != nil {
		return invocation.GovernanceFacts{}, err
	}
	s, err := invocation.NewResourceFact("sandbox_id", id.String())
	if err != nil {
		return invocation.GovernanceFacts{}, err
	}
	p, err := invocation.NewResourceFact("workspace_path", pathFact)
	if err != nil {
		return invocation.GovernanceFacts{}, err
	}
	return invocation.KnownGovernanceFacts([]invocation.EffectFact{effect}, []invocation.ResourceFact{w, s, p})
}

// reviewedWorkspacePath derives the same normalized path fact as the current
// Workspace Tool plugin. It refuses requests for another WorkspaceRef and
// never accepts a caller-provided Policy fact in place of argument parsing.
func reviewedWorkspacePath(op workspace.Operation, ref workspace.Ref, arguments []byte) (string, error) {
	var fields map[string]json.RawMessage
	if len(arguments) == 0 || json.Unmarshal(arguments, &fields) != nil || fields == nil {
		return "", fmt.Errorf("governed Workspace: canonical tool arguments required")
	}
	if raw, ok := fields["workspaceId"]; ok {
		var requested string
		if json.Unmarshal(raw, &requested) != nil || requested != ref.String() {
			return "", sandbox.ErrNotAuthorized
		}
	}
	field := "path"
	allowRoot := false
	switch op {
	case workspace.Read, workspace.Edit:
	case workspace.Search:
		allowRoot = true
	case workspace.Shell:
		field = "cwd"
		allowRoot = true
	default:
		return "", sandbox.ErrCapabilityUnavailable
	}
	var value string
	if raw, ok := fields[field]; ok {
		if json.Unmarshal(raw, &value) != nil {
			return "", fmt.Errorf("governed Workspace: invalid path field")
		}
	}
	return workspace.NormalizeRelativePath(value, allowRoot)
}

// Acquire takes facts ALREADY admitted by the canonical Invocation policy and
// binding gate. It proves the exact facts/target still match and then delegates
// Sandbox policy, grant, budget and durable lease checks to the existing Router.
// Only the already-admitted Invocation may call Acquire after it has frozen
// TargetScope and governance facts; stale grants or generation fail closed.
func (r GuardedRouter) Acquire(ctx context.Context, frozen FrozenPlacement, invocationID invocation.InvocationID, admitted invocation.GovernanceFacts) (*workspaceapp.Lease, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	req := frozen.request
	if err := r.validateRequest(req); err != nil {
		return nil, err
	}
	if !matchAdmittedFacts(frozen.facts, admitted) {
		return nil, sandbox.ErrNotAuthorized
	}
	if _, err := invocation.ParseInvocationID(invocationID.String()); err != nil {
		return nil, sandbox.ErrNotAuthorized
	}
	if err := r.deps.Admissions.VerifyAdmittedAttempt(ctx, invocationID, req.Caller, req.ToolID, admitted); err != nil {
		return nil, fmt.Errorf("governed Workspace invocation admission: %w", err)
	}
	if ok, err := r.deps.Workspaces.MayUseWorkspace(ctx, req.Caller, req.Workspace); err != nil {
		return nil, err
	} else if !ok {
		return nil, sandbox.ErrNotAuthorized
	}
	scope, err := r.currentBindingScope(ctx, req)
	if err != nil {
		return nil, err
	}
	candidates, err := r.deps.Placements.List(ctx, req.Workspace)
	if err != nil {
		return nil, err
	}
	stillBound := false
	for _, candidate := range candidates {
		if candidate.SandboxID == frozen.candidate.SandboxID {
			if candidate != frozen.candidate {
				return nil, sandbox.ErrConflict
			}
			stillBound = true
			break
		}
	}
	if !stillBound {
		return nil, sandbox.ErrConflict
	}
	current, eligible, err := r.inspect(ctx, req, frozen.candidate, scope)
	if err != nil {
		return nil, err
	}
	if !eligible || current.Sandbox.Spec() != frozen.placement.Sandbox.Spec() ||
		current.Run != frozen.placement.Run || current.Required != frozen.placement.Required {
		return nil, sandbox.ErrConflict
	}
	// Use the existing Router backend/session path, but require its Native-only
	// entrypoint and atomic ToolBinding/TargetScope/Workspace-grant lease store.
	// The legacy Sandbox Acquire path does not have that commit-point fence.
	nativeRouter, ok := r.deps.Router.(interface {
		AcquireNative(context.Context, governance.Context, sandboxapp.Placement) (*sandboxapp.Lease, error)
	})
	if !ok {
		return nil, sandbox.ErrNotAuthorized
	}
	// A known owner-required class stays fixed. Otherwise pin the *actual*
	// verified backend class from the original immutable placement; the
	// typed lease compares it with current SQLite in the writer transaction.
	isolation := req.RequiredIsolation
	if isolation == "" {
		isolation = frozen.placement.Sandbox.Isolation()
	}
	bound, err := typedlease.WithRequest(ctx, typedlease.Request{
		InvocationID: invocationID, Caller: req.Caller, ToolID: req.ToolID,
		Operation: req.Operation, Workspace: req.Workspace,
		Sandbox: frozen.placement.Sandbox.ID(), Generation: frozen.placement.Run.Generation,
		Isolation: isolation, ApprovedFacts: admitted,
	})
	if err != nil {
		return nil, err
	}
	lease, err := nativeRouter.AcquireNative(bound, req.Caller, frozen.placement)
	if err != nil {
		return nil, err
	}
	if lease == nil {
		return nil, fmt.Errorf("governed Workspace acquire: missing lease")
	}
	if lease.SandboxID != frozen.placement.Sandbox.ID() || lease.Generation != frozen.placement.Run.Generation {
		return nil, errors.Join(sandbox.ErrConflict, lease.Close())
	}
	return wrapWorkspaceLease(lease, frozen.placement)
}

func matchAdmittedFacts(expected, admitted invocation.GovernanceFacts) bool {
	if expected.Knowledge() != invocation.FactKnowledgeKnown ||
		admitted.Validate() != nil || admitted.Knowledge() != invocation.FactKnowledgeKnown {
		return false
	}
	for _, effect := range expected.Effects() {
		found := 0
		for _, actual := range admitted.Effects() {
			if actual == effect {
				found++
			}
		}
		if found != 1 {
			return false
		}
	}
	for _, required := range expected.Resources() {
		found := 0
		for _, actual := range admitted.Resources() {
			if actual.Kind() == required.Kind() {
				if actual.Value() != required.Value() {
					return false
				}
				found++
			}
		}
		if found != 1 {
			return false
		}
	}
	return true
}
