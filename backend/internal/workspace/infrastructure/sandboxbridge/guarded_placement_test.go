package sandboxbridge

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governance "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocation "github.com/orz-i/Mender/backend/internal/invocation/domain"
	profile "github.com/orz-i/Mender/backend/internal/profile/domain"
	sandboxapp "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandbox "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sandboxrouter "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/router"
	"github.com/orz-i/Mender/backend/internal/toolsuite/infrastructure/curated"
	workspaceapp "github.com/orz-i/Mender/backend/internal/workspace/application"
	workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"
)

const (
	testWorkspace  = "wrk_0123456789abcdef0123456789abcdef"
	testProfile    = "prof_0123456789abcdef0123456789abcdef"
	testSandboxA   = "sbx_11111111111111111111111111111111"
	testSandboxB   = "sbx_22222222222222222222222222222222"
	testInvocation = "inv_1234567890abcdef1234567890abcdef"
)

type guardedTestStore struct {
	mu        sync.Mutex
	sandboxes map[sandbox.ID]sandbox.Sandbox
	runs      map[sandbox.ID]sandbox.Run
	leases    map[sandbox.ID]bool
	grants    map[sandbox.ID]bool
	released  int
	reserved  int
}

func (s *guardedTestStore) Get(ctx context.Context, id sandbox.ID) (sandbox.Sandbox, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.sandboxes[id]
	if !ok {
		return sandbox.Sandbox{}, sandbox.ErrNotFound
	}
	return item, nil
}

func (s *guardedTestStore) GetRun(ctx context.Context, id sandbox.ID) (sandbox.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[id]
	if !ok {
		return sandbox.Run{}, sandbox.ErrNotFound
	}
	return run, nil
}

func (s *guardedTestStore) HasProfileGrant(ctx context.Context, id sandbox.ID, _ string, _ sandbox.Capability) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.grants[id], nil
}

func (s *guardedTestStore) ReserveExecution(ctx context.Context, id sandbox.ID, generation uint64, caller governance.Context, cap sandbox.Capability) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.leases[id] || s.runs[id].Generation != generation || s.runs[id].State != sandbox.StateReady {
		return "", sandbox.ErrConflict
	}
	if caller.Scope().Kind() == governance.ScopeProfile && !s.grants[id] {
		return "", sandbox.ErrNotAuthorized
	}
	s.leases[id] = true
	s.reserved++
	return "lease-token", nil
}

func (s *guardedTestStore) ReleaseExecution(ctx context.Context, id sandbox.ID, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if token != "lease-token" || !s.leases[id] {
		return sandbox.ErrConflict
	}
	s.leases[id] = false
	s.released++
	return nil
}

type guardedTestBackend struct {
	mu     sync.Mutex
	opens  int
	closes int
	caps   sandbox.Capabilities
}

func (*guardedTestBackend) Kind() sandbox.BackendKind            { return sandbox.BackendLocalWorkspace }
func (b *guardedTestBackend) Capabilities() sandbox.Capabilities { return b.caps }
func (b *guardedTestBackend) Open(context.Context, sandbox.Sandbox, sandbox.Run) (sandboxapp.Session, error) {
	b.mu.Lock()
	b.opens++
	b.mu.Unlock()
	return &guardedTestSession{backend: b}, nil
}

type guardedTestSession struct{ backend *guardedTestBackend }

func (s *guardedTestSession) Close() error {
	s.backend.mu.Lock()
	s.backend.closes++
	s.backend.mu.Unlock()
	return nil
}
func (*guardedTestSession) Read(ctx context.Context, req workspace.ReadRequest) (workspace.ReadResult, error) {
	return workspace.ReadResult{Path: req.Path}, nil
}
func (*guardedTestSession) Search(ctx context.Context, req workspace.SearchRequest) (workspace.SearchResult, error) {
	return workspace.SearchResult{}, nil
}
func (*guardedTestSession) Edit(ctx context.Context, req workspace.EditRequest) (workspace.EditResult, error) {
	return workspace.EditResult{Path: req.Path}, nil
}
func (*guardedTestSession) Shell(ctx context.Context, req workspace.ShellRequest) (workspace.ShellResult, error) {
	return workspace.ShellResult{}, nil
}

type guardedTestBackends struct {
	backend *guardedTestBackend
	missing bool
}

func (r *guardedTestBackends) ResolveBackend(kind sandbox.BackendKind, pin sandbox.BackendBinding, cap sandbox.Capability) (sandboxapp.Backend, error) {
	if r.missing || kind != sandbox.BackendLocalWorkspace || pin != sandbox.BuiltinBinding(sandbox.BackendLocalWorkspace) {
		return nil, sandbox.ErrBackendUnavailable
	}
	if !r.backend.Capabilities().Supports(cap) {
		return nil, sandbox.ErrCapabilityUnavailable
	}
	return r.backend, nil
}
func (r *guardedTestBackends) Available(kind sandbox.BackendKind, binding sandbox.BackendBinding) bool {
	return !r.missing && kind == sandbox.BackendLocalWorkspace && binding == sandbox.BuiltinBinding(sandbox.BackendLocalWorkspace)
}

type guardedTestGate struct {
	denied  bool
	hits    int
	onAdmit func()
}

func (g *guardedTestGate) AdmitAcquire(context.Context, governance.Context, sandboxapp.Placement) error {
	g.hits++
	if g.onAdmit != nil {
		g.onAdmit()
	}
	if g.denied {
		return sandbox.ErrNotAuthorized
	}
	return nil
}

type guardedTestWorkspaces struct {
	denied bool
	hits   int
}

func (w *guardedTestWorkspaces) MayUseWorkspace(ctx context.Context, caller governance.Context, ref workspace.Ref) (bool, error) {
	w.hits++
	return !w.denied && ref.String() == testWorkspace, nil
}

type guardedTestBindings struct {
	scope profile.TargetScope
	found bool
	hits  int
}

func (b *guardedTestBindings) BindingTargetScope(ctx context.Context, profileID string, tool catalog.ToolID) (profile.TargetScope, bool, error) {
	b.hits++
	if profileID != testProfile {
		return profile.TargetScope{}, false, nil
	}
	return b.scope, b.found, nil
}

type guardedTestAdmissions struct {
	denied bool
	checks int
}

func (a *guardedTestAdmissions) VerifyAdmittedAttempt(ctx context.Context, id invocation.InvocationID, caller governance.Context, tool catalog.ToolID, facts invocation.GovernanceFacts) error {
	a.checks++
	if a.denied || id != testInvocation || caller.Validate() != nil || tool == "" || facts.Validate() != nil {
		return sandbox.ErrNotAuthorized
	}
	return nil
}

type guardedTestPlacements struct {
	entries []workspaceapp.PlacementCandidate
}

func (p *guardedTestPlacements) ListWorkspacePlacements(ctx context.Context, ref workspace.Ref) ([]workspaceapp.PlacementCandidate, error) {
	return append([]workspaceapp.PlacementCandidate(nil), p.entries...), nil
}

// Test-only Native Router adapter exercises the existing Sandbox lease path.
// Production AcquireNative is implemented on sandboxrouter.Router only when
// it owns the concrete typedlease.Store; this stub is not a runtime binding.
type guardedTestNativeRouter struct{ *sandboxrouter.Router }

func (r guardedTestNativeRouter) AcquireNative(ctx context.Context, caller governance.Context, placement sandboxapp.Placement) (*sandboxapp.Lease, error) {
	return r.Acquire(ctx, caller, placement)
}

type guardedFixture struct {
	route      GuardedRouter
	reader     *guardedTestPlacements
	store      *guardedTestStore
	backend    *guardedTestBackend
	backends   *guardedTestBackends
	gate       *guardedTestGate
	bindings   *guardedTestBindings
	work       *guardedTestWorkspaces
	admissions *guardedTestAdmissions
	request    ResolveRequest
}

func guardedToolID(t *testing.T, op workspace.Operation) catalog.ToolID {
	t.Helper()
	tools, err := curated.PlannedNativeTools()
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools {
		if tool.SourceKey == operationSourceKey(op) {
			return tool.ToolID
		}
	}
	t.Fatalf("missing native tool for %s", op)
	return ""
}

func newGuardedFixture(t *testing.T) *guardedFixture {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ref, _ := workspace.ParseRef(testWorkspace)
	item, err := sandbox.NewSandbox(sandbox.Spec{
		ID: sandbox.ID(testSandboxA), Name: "A",
		Backend: sandbox.BackendLocalWorkspace, Binding: sandbox.BuiltinBinding(sandbox.BackendLocalWorkspace),
		Isolation: sandbox.IsolationRestrictedHost, LocalRoot: root,
		OwnerKind: "local_user", OwnerRef: "owner",
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := sandbox.NewRun(item.ID(), 2, sandbox.StateReady, "running-A")
	if err != nil {
		t.Fatal(err)
	}
	st := &guardedTestStore{
		sandboxes: map[sandbox.ID]sandbox.Sandbox{item.ID(): item},
		runs:      map[sandbox.ID]sandbox.Run{item.ID(): run},
		grants:    map[sandbox.ID]bool{item.ID(): true},
		leases:    make(map[sandbox.ID]bool),
	}
	b := &guardedTestBackend{caps: sandbox.Capabilities{Read: true, Edit: true, Search: true, Shell: true}}
	backends := &guardedTestBackends{backend: b}
	g := &guardedTestGate{}
	router, err := sandboxrouter.New(st, st, g, st, backends)
	if err != nil {
		t.Fatal(err)
	}
	target, _ := profile.NewExecutionTarget(ref.String(), item.ID().String())
	scope, _ := profile.NewTargetScope([]profile.ExecutionTarget{target})
	bindings := &guardedTestBindings{scope: scope, found: true}
	reader := &guardedTestPlacements{entries: []workspaceapp.PlacementCandidate{{
		Workspace: ref, SandboxID: item.ID().String(), TargetRoot: root,
	}}}
	q, err := workspaceapp.NewPlacementQuery(reader)
	if err != nil {
		t.Fatal(err)
	}
	work := &guardedTestWorkspaces{}
	admissions := &guardedTestAdmissions{}
	route, err := NewGuardedRouter(GuardedDependencies{
		Placements: q, Router: guardedTestNativeRouter{router}, Backends: backends, Grants: st, Workspaces: work,
		Bindings: bindings, Roots: LocalRootAttestor{}, Admissions: admissions,
	})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := governance.NewPrincipal(governance.PrincipalService, "profile:"+testProfile)
	ps, _ := governance.ProfileScope(testProfile)
	caller, _ := governance.NewContext(governance.OriginMCP, p, ps)
	return &guardedFixture{
		route: route, reader: reader, store: st, backend: b, backends: backends,
		gate: g, bindings: bindings, work: work, admissions: admissions,
		request: ResolveRequest{Caller: caller, ToolID: guardedToolID(t, workspace.Read),
			Workspace: ref, Operation: workspace.Read, RequiredIsolation: sandbox.IsolationRestrictedHost,
			Arguments: []byte(`{"path":"README.md"}`)},
	}
}

func TestGuardedResolveAndAcquireUseSamePinnedSandboxAndLease(t *testing.T) {
	f := newGuardedFixture(t)
	ctx := context.Background()
	plan, err := f.route.Resolve(ctx, f.request)
	if err != nil {
		t.Fatal(err)
	}
	if plan.SandboxID() != testSandboxA || plan.Workspace() != f.request.Workspace ||
		plan.RunGeneration() != 2 || plan.ToolID() != f.request.ToolID {
		t.Fatalf("incorrect frozen selection: %+v", plan)
	}
	if f.backend.opens != 0 || f.store.reserved != 0 || f.gate.hits != 0 {
		t.Fatal("preflight opened backend, reserved lease, or invoked Sandbox Policy")
	}
	facts := plan.Facts()
	if facts.Knowledge() != invocation.FactKnowledgeKnown || len(facts.Resources()) != 3 {
		t.Fatalf("unreviewed facts: %+v", facts)
	}
	lease, err := f.route.Acquire(ctx, plan, testInvocation, facts)
	if err != nil {
		t.Fatal(err)
	}
	result, err := lease.Read(ctx, workspace.ReadRequest{Path: "README.md"})
	if err != nil || result.Path != "README.md" {
		t.Fatalf("typed Workspace execution failed: %+v %v", result, err)
	}
	if f.backend.opens != 1 || f.backend.closes != 1 || f.gate.hits != 1 ||
		f.store.reserved != 1 || f.store.released != 1 {
		t.Fatalf("not exactly one guarded lease/backend: opens=%d closes=%d gate=%d reservations=%d releases=%d",
			f.backend.opens, f.backend.closes, f.gate.hits, f.store.reserved, f.store.released)
	}
}

func TestGuardedFactsRejectApprovedAExecutedBOrMissingFacts(t *testing.T) {
	f := newGuardedFixture(t)
	plan, err := f.route.Resolve(context.Background(), f.request)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	wrong, _ := invocation.NewResourceFact("sandbox_id", testSandboxB)
	workspaceFact, _ := invocation.NewResourceFact("workspace_id", testWorkspace)
	wrongPath, _ := invocation.NewResourceFact("workspace_path", "other.txt")
	sandboxFact, _ := invocation.NewResourceFact("sandbox_id", testSandboxA)
	for _, tc := range []struct {
		name  string
		facts invocation.GovernanceFacts
	}{
		{"unknown facts", invocation.UnknownGovernanceFacts()},
		{"zero facts", invocation.GovernanceFacts{}},
		{"missing sandbox", mustGuardedFacts(t, []invocation.EffectFact{"filesystem.read"}, []invocation.ResourceFact{workspaceFact})},
		{"switched sandbox", mustGuardedFacts(t, []invocation.EffectFact{"filesystem.read"}, []invocation.ResourceFact{workspaceFact, wrong})},
		{"wrong effect", mustGuardedFacts(t, []invocation.EffectFact{"filesystem.write"}, plan.Facts().Resources())},
		{"switched path", mustGuardedFacts(t, []invocation.EffectFact{"filesystem.read"}, []invocation.ResourceFact{workspaceFact, sandboxFact, wrongPath})},
		{"duplicate sandbox", mustGuardedFacts(t, []invocation.EffectFact{"filesystem.read"}, append(plan.Facts().Resources(), plan.Facts().Resources()[1]))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := f.route.Acquire(ctx, plan, testInvocation, tc.facts); !errors.Is(err, sandbox.ErrNotAuthorized) {
				t.Fatalf("should reject mismatched policy facts: %v", err)
			}
			if f.backend.opens != 0 || f.gate.hits != 0 {
				t.Fatal("bad facts reached Sandbox backend or admission")
			}
		})
	}
}

func mustGuardedFacts(t *testing.T, effects []invocation.EffectFact, resources []invocation.ResourceFact) invocation.GovernanceFacts {
	t.Helper()
	facts, err := invocation.KnownGovernanceFacts(effects, resources)
	if err != nil {
		t.Fatal(err)
	}
	return facts
}

func TestGuardedAcquireRevalidatesAuthorizationAndExactPlacement(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*guardedFixture)
	}{
		{"workspace grant revoked", func(f *guardedFixture) { f.work.denied = true }},
		{"Profile ToolBinding target revoked", func(f *guardedFixture) { f.bindings.found = false }},
		{"Sandbox capability grant revoked", func(f *guardedFixture) { f.store.grants[sandbox.ID(testSandboxA)] = false }},
		{"binding changed root", func(f *guardedFixture) { f.reader.entries[0].TargetRoot = "/other" }},
		{"binding disappeared", func(f *guardedFixture) { f.reader.entries = nil }},
		{"run generation changed", func(f *guardedFixture) {
			r := f.store.runs[sandbox.ID(testSandboxA)]
			r.Generation++
			f.store.runs[sandbox.ID(testSandboxA)] = r
		}},
		{"run became stopped", func(f *guardedFixture) {
			r := f.store.runs[sandbox.ID(testSandboxA)]
			r.State = sandbox.StateStopped
			f.store.runs[sandbox.ID(testSandboxA)] = r
		}},
		{"backend vanished", func(f *guardedFixture) { f.backends.missing = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newGuardedFixture(t)
			plan, err := f.route.Resolve(context.Background(), f.request)
			if err != nil {
				t.Fatal(err)
			}
			tc.change(f)
			if _, err := f.route.Acquire(context.Background(), plan, testInvocation, plan.Facts()); err == nil {
				t.Fatal("stale placement was executed")
			}
			if f.backend.opens != 0 || f.gate.hits != 0 || f.store.reserved != 0 {
				t.Fatal("revocation or changed route reached backend")
			}
		})
	}
}

func TestGuardedSelectionRespectsTargetScopeAndPinnedIsolation(t *testing.T) {
	f := newGuardedFixture(t)
	f.bindings.found = false
	if _, err := f.route.Resolve(context.Background(), f.request); !errors.Is(err, sandbox.ErrNotAuthorized) {
		t.Fatalf("missing Profile scope should deny: %v", err)
	}
	f.bindings.found = true
	unauthorized, _ := profile.NewExecutionTarget(testWorkspace, testSandboxB)
	f.bindings.scope, _ = profile.NewTargetScope([]profile.ExecutionTarget{unauthorized})
	if _, err := f.route.Resolve(context.Background(), f.request); !errors.Is(err, workspaceapp.ErrPlacementUnavailable) {
		t.Fatalf("scope cannot widen to other Sandbox: %v", err)
	}
	allowed, _ := profile.NewExecutionTarget(testWorkspace, testSandboxA)
	f.bindings.scope, _ = profile.NewTargetScope([]profile.ExecutionTarget{allowed})
	f.request.RequiredIsolation = sandbox.IsolationHost
	if _, err := f.route.Resolve(context.Background(), f.request); !errors.Is(err, workspaceapp.ErrPlacementUnavailable) {
		t.Fatalf("restricted→host substitution should not be automatic: %v", err)
	}
	f.request.RequiredIsolation = sandbox.IsolationRestrictedHost
	f.request.PinnedSandboxID = testSandboxB
	if _, err := f.route.Resolve(context.Background(), f.request); !errors.Is(err, workspaceapp.ErrPlacementUnavailable) {
		t.Fatalf("missing pinned Sandbox should not fall back: %v", err)
	}
	f.request.PinnedSandboxID = ""
	f.backend.caps = sandbox.Capabilities{Read: false}
	if _, err := f.route.Resolve(context.Background(), f.request); !errors.Is(err, workspaceapp.ErrPlacementUnavailable) {
		t.Fatalf("missing verified capability should deny: %v", err)
	}
	if f.backend.opens != 0 || f.gate.hits != 0 {
		t.Fatal("failed selection reached backend")
	}
}

func TestGuardedSandboxAcquireDenialCannotOpenBackend(t *testing.T) {
	f := newGuardedFixture(t)
	plan, err := f.route.Resolve(context.Background(), f.request)
	if err != nil {
		t.Fatal(err)
	}
	f.gate.denied = true
	if _, err := f.route.Acquire(context.Background(), plan, testInvocation, plan.Facts()); !errors.Is(err, sandbox.ErrNotAuthorized) {
		t.Fatalf("Sandbox acquire policy denial should propagate: %v", err)
	}
	if f.backend.opens != 0 || f.store.reserved != 0 {
		t.Fatal("denied Sandbox acquisition opened or reserved backend")
	}
}

func TestGuardedExecutorRejectsWrongToolAndUntrustedCaller(t *testing.T) {
	f := newGuardedFixture(t)
	f.request.ToolID = catalog.ToolID("tool_00000000000000000000000000000001")
	if _, err := f.route.Resolve(context.Background(), f.request); err == nil {
		t.Fatal("random ToolID may not use host Workspace route")
	}
	if f.work.hits != 0 {
		t.Fatal("invalid ToolID accessed Workspace inventory")
	}
	f.request.ToolID = guardedToolID(t, workspace.Read)
	caller, err := governance.NewContext(governance.OriginCLI, f.request.Caller.Principal(), f.request.Caller.Scope())
	if err != nil {
		t.Fatal(err)
	}
	f.request.Caller = caller
	if _, err := f.route.Resolve(context.Background(), f.request); !errors.Is(err, sandbox.ErrNotAuthorized) {
		t.Fatalf("untrusted Profile principal should deny: %v", err)
	}
	if f.backend.opens != 0 {
		t.Fatal("untrusted caller opened backend")
	}
}

func TestGuardedRouterRequiresAllAuthorities(t *testing.T) {
	if _, err := NewGuardedRouter(GuardedDependencies{}); err == nil {
		t.Fatal("empty guard accepted")
	}
}

func TestGuardedInvocationReceiptRequiredEvenWithCorrectFacts(t *testing.T) {
	f := newGuardedFixture(t)
	plan, err := f.route.Resolve(context.Background(), f.request)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []invocation.InvocationID{"", "inv_00000000000000000000000000000000"} {
		if _, err := f.route.Acquire(context.Background(), plan, id, plan.Facts()); !errors.Is(err, sandbox.ErrNotAuthorized) {
			t.Fatalf("unrecognized Invocation receipt %q: %v", id, err)
		}
	}
	f.admissions.denied = true
	if _, err := f.route.Acquire(context.Background(), plan, testInvocation, plan.Facts()); !errors.Is(err, sandbox.ErrNotAuthorized) {
		t.Fatalf("admission refused but Sandbox executed: %v", err)
	}
	if f.backend.opens != 0 || f.gate.hits != 0 || f.store.reserved != 0 {
		t.Fatal("unadmitted Tool reached Sandbox")
	}
}

func TestGuardedProfileScopeGrantRevokedAtAtomicReservation(t *testing.T) {
	f := newGuardedFixture(t)
	plan, err := f.route.Resolve(context.Background(), f.request)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a concurrent revoke *after* the adapter's final grant query,
	// but before SandboxRouter reserves its durable lease. The real store's
	// atomic Profile-grant fence must remain the last word.
	f.gate.onAdmit = func() {
		f.store.mu.Lock()
		f.store.grants[sandbox.ID(testSandboxA)] = false
		f.store.mu.Unlock()
	}
	if _, err := f.route.Acquire(context.Background(), plan, testInvocation, plan.Facts()); !errors.Is(err, sandbox.ErrNotAuthorized) {
		t.Fatalf("revoked Sandbox grant should fail during reservation: %v", err)
	}
	if f.backend.opens != 0 || f.store.reserved != 0 || f.store.released != 0 || f.gate.hits != 1 {
		t.Fatal("concurrent grant revoke was bypassed or leaked lease")
	}
}

func TestGuardedRouterRejectsMultipleEligibleSandboxesWithoutPinnedChoice(t *testing.T) {
	f := newGuardedFixture(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	second, err := sandbox.NewSandbox(sandbox.Spec{
		ID: sandbox.ID(testSandboxB), Name: "B", Backend: sandbox.BackendLocalWorkspace,
		Binding:   sandbox.BuiltinBinding(sandbox.BackendLocalWorkspace),
		Isolation: sandbox.IsolationRestrictedHost, LocalRoot: root,
		OwnerKind: "local_user", OwnerRef: "owner",
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := sandbox.NewRun(second.ID(), 1, sandbox.StateReady, "running-B")
	if err != nil {
		t.Fatal(err)
	}
	f.store.sandboxes[second.ID()] = second
	f.store.runs[second.ID()] = run
	f.store.grants[second.ID()] = true
	f.reader.entries = append(f.reader.entries, workspaceapp.PlacementCandidate{
		Workspace: f.request.Workspace, SandboxID: second.ID().String(), TargetRoot: root,
	})
	a, _ := profile.NewExecutionTarget(testWorkspace, testSandboxA)
	b, _ := profile.NewExecutionTarget(testWorkspace, testSandboxB)
	f.bindings.scope, _ = profile.NewTargetScope([]profile.ExecutionTarget{a, b})
	if _, err := f.route.Resolve(context.Background(), f.request); !errors.Is(err, workspaceapp.ErrPlacementAmbiguous) {
		t.Fatalf("two compatible Sandboxes without explicit binding: %v", err)
	}
	f.request.PinnedSandboxID = testSandboxB
	plan, err := f.route.Resolve(context.Background(), f.request)
	if err != nil || plan.SandboxID().String() != testSandboxB {
		t.Fatalf("explicitly pinned Sandbox B was not selected: %+v %v", plan, err)
	}
	f.store.grants[second.ID()] = false
	if _, err := f.route.Acquire(context.Background(), plan, testInvocation, plan.Facts()); err == nil {
		t.Fatal("pinned B revoked; must not fallback to A")
	}
	if f.backend.opens != 0 || f.store.reserved != 0 {
		t.Fatal("revoked B unexpectedly executed in A")
	}
}

func TestGuardedWorkspacePathFactIsHostNormalizedAndFrozen(t *testing.T) {
	f := newGuardedFixture(t)
	req := f.request
	req.Arguments = []byte(`{"path":"src/../README.md","workspaceId":"` + testWorkspace + `"}`)
	plan, err := f.route.Resolve(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	pathFacts := 0
	for _, fact := range plan.Facts().Resources() {
		if fact.Kind() == "workspace_path" {
			pathFacts++
			if fact.Value() != "README.md" {
				t.Fatalf("path Policy fact was not normalized: %+v", fact)
			}
		}
	}
	if pathFacts != 1 {
		t.Fatal("Policy did not review one exact workspace_path")
	}
	original := plan.Arguments()
	req.Arguments[0] = 'X'
	if !bytes.Equal(plan.Arguments(), original) {
		t.Fatal("model argument buffer mutation changed frozen approved execution")
	}
	original[0] = 'X'
	if !bytes.Equal(plan.Arguments(), []byte(`{"path":"src/../README.md","workspaceId":"`+testWorkspace+`"}`)) {
		t.Fatal("FrozenPlacement leaked writable argument memory")
	}
}

func TestGuardedWorkspaceInvalidPathNeverReachesApprovalOrBackend(t *testing.T) {
	for _, raw := range []string{
		`{"path":"../private.txt"}`, `{"path":"/etc/passwd"}`,
		`{"path":"README.md","workspaceId":"wrk_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`,
		`{"path":123}`, `{"path":""}`, `{"path":" README.md"}`,
		`{"path":"README.md"}{"path":"another"}`,
	} {
		t.Run(raw, func(t *testing.T) {
			f := newGuardedFixture(t)
			f.request.Arguments = []byte(raw)
			if _, err := f.route.Resolve(context.Background(), f.request); err == nil {
				t.Fatalf("untrusted native path %s was accepted", raw)
			}
			if f.backend.opens != 0 || f.gate.hits != 0 || f.store.reserved != 0 {
				t.Fatal("invalid path reached admission or Sandbox backend")
			}
		})
	}
}
