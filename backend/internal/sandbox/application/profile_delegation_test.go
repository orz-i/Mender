package application

import (
	"context"
	"errors"
	"testing"

	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
	sand "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

const grantProfileID = "prof_0123456789abcdef0123456789abcdef"

type dummyGrants struct {
	allowed bool
	calls   int
}

func (g *dummyGrants) HasProfileGrant(_ context.Context, _ sand.ID, id string, capability sand.Capability) (bool, error) {
	g.calls++
	return g.allowed && id == grantProfileID && capability == sand.CapabilityRead, nil
}
func mkProfileCaller(t *testing.T, profileID string, origin gov.Origin, ref string) gov.Context {
	t.Helper()
	p, e := gov.NewPrincipal(gov.PrincipalService, ref)
	if e != nil {
		t.Fatal(e)
	}
	scope, e := gov.ProfileScope(profileID)
	if e != nil {
		t.Fatal(e)
	}
	c, e := gov.NewContext(origin, p, scope)
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func TestProfileDelegationRequiresGrantPlusPolicyAndValidIdentity(t *testing.T) {
	owner, place, policy, approval, audit := setUpGate(t)
	_ = owner
	policy.decision, _ = policydomain.Allow("allow", "allow")
	grants := &dummyGrants{}
	gate := NewAdmit(policy, approval, audit, gateID{}, gateClock{}, grants)
	caller := mkProfileCaller(t, grantProfileID, gov.OriginMCP, "profile:"+grantProfileID)
	if err := gate.AdmitAcquire(context.Background(), caller, place); !errors.Is(err, sand.ErrNotAuthorized) {
		t.Fatalf("no grant: %v", err)
	}
	if len(audit.records) != 0 {
		t.Fatal("denied before admission must not record success")
	}
	grants.allowed = true
	if err := gate.AdmitAcquire(context.Background(), caller, place); err != nil {
		t.Fatalf("authorized grant %v", err)
	}
	if len(audit.records) != 1 || audit.records[0].PrincipalKind != gov.PrincipalService || audit.records[0].Origin != gov.OriginMCP {
		t.Fatalf("wrong audit %v", audit.records)
	}
	if err := gate.AdmitAction(context.Background(), caller, place.Sandbox, gov.ActionSandboxDestroy); !errors.Is(err, sand.ErrNotAuthorized) {
		t.Fatalf("grant enabled destroy: %v", err)
	}
	if err := gate.AdmitAction(context.Background(), caller, place.Sandbox, gov.ActionSandboxStart); !errors.Is(err, sand.ErrNotAuthorized) {
		t.Fatalf("grant enabled start: %v", err)
	}
	for _, bad := range []gov.Context{
		mkProfileCaller(t, grantProfileID, gov.OriginCLI, "profile:"+grantProfileID),
		mkProfileCaller(t, grantProfileID, gov.OriginMCP, "profile:prof_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
	} {
		if err := gate.AdmitAcquire(context.Background(), bad, place); !errors.Is(err, sand.ErrNotAuthorized) {
			t.Fatalf("forged identity accepted %v", err)
		}
	}
	policy.decision = mustDeny(t)
	if err := gate.AdmitAcquire(context.Background(), caller, place); !errors.Is(err, sand.ErrNotAuthorized) {
		t.Fatalf("granted caller bypassed policy %v", err)
	}
	if len(audit.records) != 2 || audit.records[1].Status != "denied" {
		t.Fatalf("denied grant audit %v", audit.records)
	}
}
