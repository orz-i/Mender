package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sand "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

const delegatedProfileID = "prof_0123456789abcdef0123456789abcdef"
const otherProfileID = "prof_abcdef0123456789abcdef0123456789"

func sandboxProfileContext(t *testing.T, profileID string) gov.Context {
	t.Helper()
	p, err := gov.NewPrincipal(gov.PrincipalService, "profile:"+profileID)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := gov.ProfileScope(profileID)
	if err != nil {
		t.Fatal(err)
	}
	c, err := gov.NewContext(gov.OriginMCP, p, scope)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func readyGrantStore(t *testing.T) *Store {
	t.Helper()
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "grants.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Create(ctx, testSandbox(t)); err != nil {
		t.Fatal(err)
	}
	run, err := sand.NewRun(sandboxID, 1, sand.StateReady, "local")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	return store
}
func TestProfileGrantRequiresOwnerAndCapability(t *testing.T) {
	ctx := context.Background()
	store := readyGrantStore(t)
	service := sandboxProfileContext(t, delegatedProfileID)
	if _, err := store.ReserveExecution(ctx, sandboxID, 1, service, sand.CapabilityRead); err == nil {
		t.Fatal("ungranted service acquired Sandbox")
	}
	if err := store.ReplaceProfileGrants(ctx, sandboxID, delegatedProfileID, []sand.Capability{sand.CapabilityRead}, service); !errors.Is(err, sand.ErrNotAuthorized) {
		t.Fatalf("service grant %v", err)
	}
	owner := leasePrincipal(t)
	for _, cap := range []sand.Capability{sand.CapabilityJobSubmit, sand.CapabilityJobObserve, sand.CapabilityJobCancel} {
		if err := store.ReplaceProfileGrants(ctx, sandboxID, delegatedProfileID, []sand.Capability{cap}, owner); !errors.Is(err, sand.ErrCapabilityUnavailable) {
			t.Fatalf("local Workspace accepted SSH-only grant %s: %v", cap, err)
		}
	}
	if err := store.ReplaceProfileGrants(ctx, sandboxID, delegatedProfileID, []sand.Capability{sand.CapabilityRead}, owner); err != nil {
		t.Fatal(err)
	}
	grants, err := store.ListProfileGrants(ctx, sandboxID, owner)
	if err != nil || len(grants) != 1 || len(grants[0].Capabilities) != 1 || grants[0].ProfileID != delegatedProfileID {
		t.Fatalf("grants %+v %v", grants, err)
	}
	if _, err := store.ListProfileGrants(ctx, sandboxID, service); !errors.Is(err, sand.ErrNotAuthorized) {
		t.Fatalf("service read grants %v", err)
	}
	if ok, err := store.HasProfileGrant(ctx, sandboxID, delegatedProfileID, sand.CapabilityRead); err != nil || !ok {
		t.Fatalf("grant lookup %v %v", ok, err)
	}
	if _, err := store.ReserveExecution(ctx, sandboxID, 1, service, sand.CapabilitySearch); err == nil {
		t.Fatal("search beyond read grant")
	}
	if _, err := store.ReserveExecution(ctx, sandboxID, 1, sandboxProfileContext(t, otherProfileID), sand.CapabilityRead); err == nil {
		t.Fatal("other Profile acquired Sandbox")
	}
	token, err := store.ReserveExecution(ctx, sandboxID, 1, service, sand.CapabilityRead)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReleaseExecution(ctx, sandboxID, token); err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceProfileGrants(ctx, sandboxID, delegatedProfileID, nil, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReserveExecution(ctx, sandboxID, 1, service, sand.CapabilityRead); err == nil {
		t.Fatal("revoked Profile acquired Sandbox")
	}
	events, err := store.ListGrantEvents(ctx, sandboxID, owner, 5)
	if err != nil || len(events) != 2 || events[0].Action != "revoke" || events[1].Action != "grant" {
		t.Fatalf("audit %+v %v", events, err)
	}
	if err := store.ReplaceProfileGrants(ctx, sandboxID, delegatedProfileID, nil, owner); err != nil {
		t.Fatal(err)
	}
	events, _ = store.ListGrantEvents(ctx, sandboxID, owner, 5)
	if len(events) != 2 {
		t.Fatalf("idempotent duplicate audit %+v", events)
	}
}
func TestProfileGrantStrictPrincipalAndAtomicRevoke(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "grants-race.db")
	first, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	if err := first.Create(ctx, testSandbox(t)); err != nil {
		t.Fatal(err)
	}
	run, err := sand.NewRun(sandboxID, 1, sand.StateReady, "local")
	if err != nil {
		t.Fatal(err)
	}
	if err := first.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	second, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()
	owner := leasePrincipal(t)
	profile := sandboxProfileContext(t, delegatedProfileID)
	if err := first.ReplaceProfileGrants(ctx, sandboxID, delegatedProfileID, []sand.Capability{sand.CapabilityRead}, owner); err != nil {
		t.Fatal(err)
	}
	contexts := []gov.Context{}
	principal, _ := gov.NewPrincipal(gov.PrincipalService, "profile:"+delegatedProfileID)
	wrongScope, _ := gov.ProfileScope(otherProfileID)
	badScope, _ := gov.NewContext(gov.OriginMCP, principal, wrongScope)
	wrongOrigin, _ := gov.NewContext(gov.OriginCLI, principal, profile.Scope())
	contexts = append(contexts, badScope, wrongOrigin)
	for _, caller := range contexts {
		if _, err := second.ReserveExecution(ctx, sandboxID, 1, caller, sand.CapabilityRead); !errors.Is(err, sand.ErrNotAuthorized) {
			t.Fatalf("bad caller permitted %v", err)
		}
	}
	var wg sync.WaitGroup
	wg.Add(2)
	out := make(chan error, 2)
	go func() {
		defer wg.Done()
		out <- first.ReplaceProfileGrants(ctx, sandboxID, delegatedProfileID, nil, owner)
	}()
	go func() {
		defer wg.Done()
		token, err := second.ReserveExecution(ctx, sandboxID, 1, profile, sand.CapabilityRead)
		if err == nil {
			err = second.ReleaseExecution(ctx, sandboxID, token)
		}
		out <- err
	}()
	wg.Wait()
	close(out)
	for e := range out {
		if e != nil && !errors.Is(e, sand.ErrConflict) {
			t.Fatalf("race result %v", e)
		}
	}
	if _, err := second.ReserveExecution(ctx, sandboxID, 1, profile, sand.CapabilityRead); err == nil {
		t.Fatal("post-revoke acquisition permitted")
	}
}
func TestDestroyedSandboxCannotReceiveProfileGrant(t *testing.T) {
	store := readyGrantStore(t)
	ctx := context.Background()
	owner := leasePrincipal(t)
	run, err := store.GetRun(ctx, sandboxID)
	if err != nil {
		t.Fatal(err)
	}
	destroying, err := store.TransitionRun(ctx, run, sand.StateDestroying, run.ExternalID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.TransitionRun(ctx, destroying, sand.StateDestroyed, run.ExternalID); err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceProfileGrants(ctx, sandboxID, delegatedProfileID, []sand.Capability{sand.CapabilityRead}, owner); !errors.Is(err, sand.ErrConflict) {
		t.Fatalf("destroyed grant %v", err)
	}
}
