package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
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

func TestPolicyV2ToV3UpgradePreservesExistingUserRules(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v2-to-v3.db")
	old, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	stmts := []string{
		`CREATE TABLE policy_schema_migrations(version INTEGER PRIMARY KEY,applied_at TEXT NOT NULL)`,
		`INSERT INTO policy_schema_migrations VALUES(1,'v1'),(2,'v2')`,
		`CREATE TABLE policy_settings(singleton_id INTEGER PRIMARY KEY,default_decision TEXT NOT NULL,updated_at TEXT NOT NULL)`,
		`INSERT INTO policy_settings VALUES(1,'deny','2026-10-01T00:00:00Z')`,
		`CREATE TABLE policy_rules(rule_id TEXT PRIMARY KEY,display_name TEXT NOT NULL,enabled INTEGER NOT NULL,decision TEXT NOT NULL,match_json TEXT NOT NULL,created_at TEXT NOT NULL,updated_at TEXT NOT NULL)`,
		`INSERT INTO policy_rules VALUES('rule_00000000000000000000000000000033','Keep this existing rule',1,'allow','{"actions":["agent.task.submit"],"origins":[],"principalKinds":[],"principalRefs":[],"scopeKinds":[],"profileRefs":[],"providerIds":[],"toolIds":[],"factKnowledge":"any","effectsAny":[],"resourcesAny":[]}','2026-10-01T00:00:00Z','2026-10-01T00:00:00Z')`,
	}
	for _, q := range stmts {
		if _, err := old.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	var version int
	if err := store.db.QueryRowContext(ctx, "SELECT MAX(version) FROM policy_schema_migrations").Scan(&version); err != nil || version != 3 {
		t.Fatalf("v2→v3 failed version=%d %v", version, err)
	}
	var description, decision, rawMatch string
	if err := store.db.QueryRowContext(ctx, "SELECT display_name,decision,match_json FROM policy_rules WHERE rule_id='rule_00000000000000000000000000000033'").Scan(&description, &decision, &rawMatch); err != nil {
		t.Fatal(err)
	}
	if description != "Keep this existing rule" || decision != "allow" || rawMatch == "" {
		t.Fatal("Policy migration corrupted existing rule")
	}
	if n, err := store.CurrentRevision(ctx); err != nil || n != 1 {
		t.Fatalf("Policy v3 migration revision=%d %v", n, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	if n, err := reopened.CurrentRevision(ctx); err != nil || n != 1 {
		t.Fatalf("Policy generation reset on reopen=%d %v", n, err)
	}
	var receipts int
	if err := reopened.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM policy_schema_migrations").Scan(&receipts); err != nil || receipts != 3 {
		t.Fatalf("upgrade wrote duplicate Policy receipts=%d %v", receipts, err)
	}
}
