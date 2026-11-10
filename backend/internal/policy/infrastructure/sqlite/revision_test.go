package sqlite

import (
	"context"
	"testing"
	"time"

	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
)

func TestPolicyRevisionV3TracksSameTransactionMutations(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	revision := func() int64 {
		t.Helper()
		r, err := store.CurrentRevision(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	if v := revision(); v != 1 {
		t.Fatalf("fresh v3 Policy revision=%d want 1", v)
	}
	if _, err := store.GetPolicySet(ctx); err != nil {
		t.Fatal(err)
	}
	if v := revision(); v != 1 {
		t.Fatalf("read-only Policy evaluation bumped revision: %d", v)
	}
	if err := store.SetDefaultDecision(ctx, policydomain.DecisionDeny, time.Now()); err != nil {
		t.Fatal(err)
	}
	if v := revision(); v != 2 {
		t.Fatalf("default decision update revision=%d", v)
	}
	// Same decision is not a Policy mutation.
	if err := store.SetDefaultDecision(ctx, policydomain.DecisionDeny, time.Now()); err != nil {
		t.Fatal(err)
	}
	if v := revision(); v != 2 {
		t.Fatalf("no-op default change bumped revision=%d", v)
	}
	match, err := policydomain.NewMatch(policydomain.MatchSpec{Actions: []gov.Action{gov.ActionAgentTaskSubmit}})
	if err != nil {
		t.Fatal(err)
	}
	id := mustRuleID(t, "rule_00000000000000000000000000000031")
	rule, err := policydomain.NewRule(id, "approval safety", true, policydomain.DecisionDeny, match, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRule(ctx, rule); err != nil {
		t.Fatal(err)
	}
	if v := revision(); v != 3 {
		t.Fatalf("Policy rule create revision=%d", v)
	}
	if _, err := store.db.ExecContext(ctx, "UPDATE policy_rules SET enabled=0 WHERE rule_id=?", id.String()); err != nil {
		t.Fatal(err)
	}
	if v := revision(); v != 4 {
		t.Fatalf("direct SQL rule UPDATE did not increment revision=%d", v)
	}
	if err := store.DeleteRule(ctx, id); err != nil {
		t.Fatal(err)
	}
	if v := revision(); v != 5 {
		t.Fatalf("rule delete did not increment revision=%d", v)
	}
	if _, err := store.db.ExecContext(ctx, "UPDATE policy_settings SET default_decision='allow' WHERE singleton_id=1"); err != nil {
		t.Fatal(err)
	}
	if v := revision(); v != 6 {
		t.Fatalf("direct default SQL update missed revision=%d", v)
	}
	p, err := store.GetPolicySet(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if p.DefaultDecision() != policydomain.DecisionAllow || len(p.Rules()) != 0 {
		t.Fatal("revision triggers corrupted Policy state")
	}
}
