package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
	_ "modernc.org/sqlite"
)

func TestOpenMigratesV1RulesToToolInvokeAction(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "policy.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE policy_schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE policy_settings (
		singleton_id INTEGER PRIMARY KEY CHECK (singleton_id = 1),
		default_decision TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE policy_rules (
		rule_id TEXT PRIMARY KEY,
		display_name TEXT NOT NULL,
		enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
		decision TEXT NOT NULL,
		match_json TEXT NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		"INSERT INTO policy_schema_migrations(version, applied_at) VALUES (1, 'v1')",
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		"INSERT INTO policy_settings(singleton_id, default_decision, updated_at) VALUES (1, 'allow', '2026-09-27T00:00:00Z')",
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO policy_rules(
		rule_id, display_name, enabled, decision, match_json, created_at, updated_at
	) VALUES (?, ?, 1, 'deny', ?, ?, ?)`,
		"rule_00000000000000000000000000000001",
		"Legacy tool rule",
		`{"origins":["mcp"],"scopeKinds":[],"profileRefs":[],"providerIds":["prov_00000000000000000000000000000001"],"toolIds":[],"factKnowledge":"any","effectsAny":[],"resourcesAny":[]}`,
		"2026-09-27T00:00:00Z",
		"2026-09-27T00:00:00Z",
	); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	var version int
	if err := store.db.QueryRow(
		"SELECT MAX(version) FROM policy_schema_migrations",
	).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 3 {
		t.Fatalf("schema version = %d, want current v3", version)
	}

	policy, err := store.GetPolicySet(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(policy.Rules()) != 1 {
		t.Fatalf("rules = %d, want 1", len(policy.Rules()))
	}
	match := policy.Rules()[0].Match()
	actions := match.Actions()
	if len(actions) != 1 || actions[0] != governancedomain.ActionToolInvoke {
		t.Fatalf("migrated actions = %v", actions)
	}

	toolDecision, err := policy.Evaluate(policydomain.EvaluationInput{
		Action:             governancedomain.ActionToolInvoke,
		Origin:             governancedomain.OriginMCP,
		PrincipalKind:      governancedomain.PrincipalService,
		PrincipalReference: "profile:test",
		ScopeKind:          governancedomain.ScopeCatalog,
		ProviderID:         "prov_00000000000000000000000000000001",
		ToolID:             "tool_00000000000000000000000000000001",
		FactKnowledge:      policydomain.FactUnknown,
	})
	if err != nil {
		t.Fatal(err)
	}
	if toolDecision.Kind() != policydomain.DecisionDeny {
		t.Fatalf("tool decision = %s, want deny", toolDecision.Kind())
	}

	activationDecision, err := policy.Evaluate(policydomain.EvaluationInput{
		Action:             governancedomain.ActionProviderActivate,
		Origin:             governancedomain.OriginMCP,
		PrincipalKind:      governancedomain.PrincipalService,
		PrincipalReference: "profile:test",
		ScopeKind:          governancedomain.ScopeCatalog,
		ProviderID:         "prov_00000000000000000000000000000001",
	})
	if err != nil {
		t.Fatal(err)
	}
	if activationDecision.Kind() != policydomain.DecisionAllow {
		t.Fatalf("activation decision = %s, want default allow", activationDecision.Kind())
	}
}
