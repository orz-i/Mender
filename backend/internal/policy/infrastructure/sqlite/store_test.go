package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	policyapplication "github.com/orz-i/Mender/backend/internal/policy/application"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
	_ "modernc.org/sqlite"
)

func TestFreshStoreDefaultsToAllowWithNoRules(t *testing.T) {
	t.Parallel()

	store := openTestStore(t)
	policy, err := store.GetPolicySet(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if policy.DefaultDecision() != policydomain.DecisionAllow {
		t.Fatalf("DefaultDecision() = %q, want allow", policy.DefaultDecision())
	}
	if len(policy.Rules()) != 0 {
		t.Fatalf("Rules() = %#v, want empty", policy.Rules())
	}

	decision, err := policy.Evaluate(policydomain.EvaluationInput{
		Origin:        policydomain.OriginCLI,
		FactKnowledge: policydomain.FactUnknown,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Kind() != policydomain.DecisionAllow ||
		decision.ReasonCode() != "policy.default.allow" {
		t.Fatalf("default decision = %#v", decision)
	}
}

func TestPolicyRuleRoundTrip(t *testing.T) {
	t.Parallel()

	store := openTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)

	resource, err := policydomain.NewResourceMatcher(
		"path",
		policydomain.ResourcePrefix,
		"/workspace/release",
	)
	if err != nil {
		t.Fatal(err)
	}
	match, err := policydomain.NewMatch(policydomain.MatchSpec{
		Origins:       []policydomain.Origin{policydomain.OriginMCP},
		ScopeKinds:    []policydomain.ScopeKind{policydomain.ScopeProfile},
		ProfileRefs:   []string{"prof_00000000000000000000000000000001"},
		ProviderIDs:   []string{"prov_00000000000000000000000000000001"},
		ToolIDs:       []string{"tool_00000000000000000000000000000001"},
		FactKnowledge: policydomain.FactKnown,
		EffectsAny:    []string{"filesystem.write"},
		ResourcesAny:  []policydomain.ResourceMatcher{resource},
	})
	if err != nil {
		t.Fatal(err)
	}
	id := mustRuleID(t, "rule_00000000000000000000000000000001")
	rule, err := policydomain.NewRule(
		id,
		"Release writes require approval",
		true,
		policydomain.DecisionRequireApproval,
		match,
		now,
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.CreateRule(ctx, rule); err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}
	if err := store.SetDefaultDecision(
		ctx,
		policydomain.DecisionDeny,
		now.Add(time.Minute),
	); err != nil {
		t.Fatalf("SetDefaultDecision() error = %v", err)
	}

	loaded, err := store.GetRule(ctx, id)
	if err != nil {
		t.Fatalf("GetRule() error = %v", err)
	}
	if loaded.DisplayName() != "Release writes require approval" ||
		!loaded.Enabled() ||
		loaded.Decision() != policydomain.DecisionRequireApproval {
		t.Fatalf("loaded Rule = %#v", loaded)
	}
	loadedMatch := loaded.Match()
	if len(loadedMatch.Origins()) != 1 ||
		loadedMatch.Origins()[0] != policydomain.OriginMCP ||
		len(loadedMatch.ProfileRefs()) != 1 ||
		loadedMatch.FactKnowledge() != policydomain.FactKnown ||
		len(loadedMatch.ResourcesAny()) != 1 ||
		loadedMatch.ResourcesAny()[0].Operator() != policydomain.ResourcePrefix {
		t.Fatalf("loaded Match = %#v", loadedMatch)
	}

	policy, err := store.GetPolicySet(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if policy.DefaultDecision() != policydomain.DecisionDeny ||
		len(policy.Rules()) != 1 {
		t.Fatalf(
			"PolicySet = default %q rules %#v",
			policy.DefaultDecision(),
			policy.Rules(),
		)
	}

	updated, err := loaded.Update(
		"Deny release writes",
		policydomain.DecisionDeny,
		loaded.Match(),
		now.Add(2*time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	updated, err = updated.SetEnabled(false, now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateRule(ctx, updated); err != nil {
		t.Fatalf("UpdateRule() error = %v", err)
	}
	reloaded, err := store.GetRule(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Enabled() || reloaded.Decision() != policydomain.DecisionDeny ||
		reloaded.DisplayName() != "Deny release writes" {
		t.Fatalf("updated Rule = %#v", reloaded)
	}

	if err := store.DeleteRule(ctx, id); err != nil {
		t.Fatalf("DeleteRule() error = %v", err)
	}
	if _, err := store.GetRule(ctx, id); !errors.Is(err, policyapplication.ErrRuleNotFound) {
		t.Fatalf("GetRule(deleted) error = %v", err)
	}
}

func TestCreateRuleMapsDuplicateToConflict(t *testing.T) {
	t.Parallel()

	store := openTestStore(t)
	match, err := policydomain.NewMatch(policydomain.MatchSpec{})
	if err != nil {
		t.Fatal(err)
	}
	id := mustRuleID(t, "rule_00000000000000000000000000000001")
	rule, err := policydomain.NewRule(
		id,
		"Global allow",
		true,
		policydomain.DecisionAllow,
		match,
		time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRule(context.Background(), rule); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRule(context.Background(), rule); !errors.Is(err, policyapplication.ErrRuleConflict) {
		t.Fatalf("CreateRule(duplicate) error = %v", err)
	}
}

func TestOpenRejectsFuturePolicySchemaVersion(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "mender.db")
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
	if _, err := db.Exec(
		"INSERT INTO policy_schema_migrations(version, applied_at) VALUES (99, 'future')",
	); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(context.Background(), path)
	if store != nil {
		_ = store.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "newer than supported version") {
		t.Fatalf("Open() error = %v", err)
	}
}

func TestPolicyRestoreFailsClosedOnMalformedDefaultDecision(t *testing.T) {
	t.Parallel()

	store := openTestStore(t)
	if _, err := store.db.ExecContext(
		context.Background(),
		"UPDATE policy_settings SET default_decision = 'maybe' WHERE singleton_id = 1",
	); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetPolicySet(context.Background()); err == nil ||
		!strings.Contains(err.Error(), "unknown value") {
		t.Fatalf("GetPolicySet() error = %v", err)
	}
}

func TestPolicyRestoreFailsClosedOnMalformedMatch(t *testing.T) {
	t.Parallel()

	store := openTestStore(t)
	match, err := policydomain.NewMatch(policydomain.MatchSpec{})
	if err != nil {
		t.Fatal(err)
	}
	id := mustRuleID(t, "rule_00000000000000000000000000000001")
	rule, err := policydomain.NewRule(
		id,
		"Rule",
		true,
		policydomain.DecisionAllow,
		match,
		time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRule(context.Background(), rule); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(
		context.Background(),
		`UPDATE policy_rules
		SET match_json = '{"origins":["not_an_origin"],"factKnowledge":"any"}'
		WHERE rule_id = ?`,
		id.String(),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetPolicySet(context.Background()); err == nil ||
		!strings.Contains(err.Error(), "unknown value") {
		t.Fatalf("GetPolicySet() error = %v", err)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(
		context.Background(),
		filepath.Join(t.TempDir(), "mender.db"),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	return store
}

func mustRuleID(t *testing.T, raw string) policydomain.RuleID {
	t.Helper()
	id, err := policydomain.ParseRuleID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
