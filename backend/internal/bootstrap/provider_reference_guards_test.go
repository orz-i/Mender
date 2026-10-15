package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	govdomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	mcpplugin "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/plugin"
	policyapplication "github.com/orz-i/Mender/backend/internal/policy/application"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	providerconfigstore "github.com/orz-i/Mender/backend/internal/providerplugin/configstore"
)

func providerReferenceFixture(t *testing.T, r *Runtime) (catalogdomain.ProviderID, catalogdomain.ToolID) {
	t.Helper()
	ctx := context.Background()
	raw := mustProviderConfigJSON(t, mcpplugin.Config{
		Transport: mcpplugin.TransportConfig{Kind: mcpplugin.TransportStdio, Executable: "audit-fixture-no-launch"},
	})
	provider, err := registerConfiguredProviderTest(t, r, ctx, catalogapplication.RegisterProviderRequest{
		Namespace: "reference-audit", DisplayName: "Reference Audit",
	}, mcpplugin.ID, raw)
	if err != nil {
		t.Fatal(err)
	}
	id := mustBootstrapToolID(t, "tool_00000000000000000000000000000091")
	schema, err := catalogdomain.NewSchema([]byte(`{"type":"object"}`))
	if err != nil {
		t.Fatal(err)
	}
	tool, err := catalogdomain.NewTool(id, provider.ID(), catalogdomain.ToolSpec{
		SourceKey: "audit", Name: "audit", InputSchema: schema,
	}, systemClock{}.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Catalog.store.ReplaceProviderTools(ctx, provider.ID(), []catalogdomain.Tool{tool}); err != nil {
		t.Fatal(err)
	}
	return provider.ID(), id
}

func profileReferenceFixture(t *testing.T, r *Runtime) profiledomain.Slug {
	t.Helper()
	p, err := r.Profiles.Create.Execute(context.Background(), profileapplication.CreateProfileRequest{
		Slug: "reference-test", DisplayName: "Reference Test", Mode: profiledomain.ExposureDynamic,
	})
	if err != nil {
		t.Fatal(err)
	}
	return p.Slug()
}

func TestProviderDeleteReferenceGuardBlocksLateProfileAndRollsBackConfig(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	r := newProviderTestRuntime(t)
	providerID, toolID := providerReferenceFixture(t, r)
	slug := profileReferenceFixture(t, r)
	// The deletion preflight already passed; a second writer adds a binding.
	if err := r.ensureProviderUnused(ctx, providerID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Profiles.AddTool.Execute(ctx, profileapplication.AddToolRequest{ProfileSlug: slug, ToolID: toolID}); err != nil {
		t.Fatal(err)
	}
	if err := deleteConfiguredProviderRows(ctx, r.providerDatabasePath, providerID); !errors.Is(err, ErrProviderInUse) {
		t.Fatalf("late Profile binding, delete error=%v", err)
	}
	if _, err := r.ProviderConfig(ctx, providerID); err != nil {
		t.Fatalf("config rollback: %v", err)
	}
	if _, err := r.Catalog.GetProvider.Execute(ctx, providerID); err != nil {
		t.Fatalf("catalog rollback: %v", err)
	}
	db, err := openProviderReferenceDB(ctx, r.providerDatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var fts int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tools_fts WHERE provider_id=?", providerID.String()).Scan(&fts); err != nil {
		t.Fatal(err)
	}
	if fts != 1 {
		t.Fatalf("fts rollback entries=%d, want 1", fts)
	}
	if _, err := r.Profiles.RemoveTool.Execute(ctx, profileapplication.RemoveToolRequest{ProfileSlug: slug, ToolID: toolID}); err != nil {
		t.Fatal(err)
	}
	if err := r.DeleteConfiguredProvider(ctx, providerID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ProviderConfig(ctx, providerID); !errors.Is(err, providerconfigstore.ErrNotFound) {
		t.Fatalf("deleted config=%v", err)
	}
	if _, err := r.Catalog.GetProvider.Execute(ctx, providerID); !errors.Is(err, catalogapplication.ErrProviderNotFound) {
		t.Fatalf("deleted catalog=%v", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tools_fts WHERE provider_id=?", providerID.String()).Scan(&fts); err != nil {
		t.Fatal(err)
	}
	if fts != 0 {
		t.Fatalf("fts entries after deletion=%d", fts)
	}
	profile, err := r.Profiles.Get.Execute(ctx, slug)
	if err != nil {
		t.Fatal(err)
	}
	// Bypass application pre-validation to reproduce the delayed SQL commit
	// of a ToolBinding that was checked before Provider deletion.
	_, err = db.ExecContext(ctx, `INSERT INTO profile_tools(profile_id,tool_id,direct) VALUES(?,?,1)`, profile.ID().String(), toolID.String())
	if err == nil || !strings.Contains(err.Error(), "Profile Tool reference unavailable") {
		t.Fatalf("late Profile insert error=%v, want guarded refusal", err)
	}
	var dangling int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM profile_tools WHERE tool_id=?", toolID.String()).Scan(&dangling); err != nil {
		t.Fatal(err)
	}
	if dangling != 0 {
		t.Fatalf("dangling ToolBindings=%d", dangling)
	}
}

func TestProviderDeletePolicyToolIDReferenceAndLateRuleGuard(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	r := newProviderTestRuntime(t)
	providerID, toolID := providerReferenceFixture(t, r)
	if err := r.ensureProviderUnused(ctx, providerID); err != nil {
		t.Fatal(err)
	}
	rule, err := r.Policy.CreateRule.Execute(ctx, policyapplication.RuleInput{
		DisplayName: "ToolID only reference", Enabled: true, Decision: policydomain.DecisionDeny,
		Match: policydomain.MatchSpec{
			Actions: []govdomain.Action{govdomain.ActionToolInvoke}, ToolIDs: []string{toolID.String()},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.DeleteConfiguredProvider(ctx, providerID); !errors.Is(err, ErrProviderInUse) {
		t.Fatalf("ToolID-only Policy rule deletion error=%v", err)
	}
	if err := r.Policy.DeleteRule.Execute(ctx, rule.ID()); err != nil {
		t.Fatal(err)
	}
	if err := r.DeleteConfiguredProvider(ctx, providerID); err != nil {
		t.Fatal(err)
	}
	db, err := openProviderReferenceDB(ctx, r.providerDatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	lateRuleJSON := fmt.Sprintf(`{"actions":["tool.invoke"],"providerIds":[],"toolIds":["%s"],"factKnowledge":"any"}`, toolID.String())
	_, err = db.ExecContext(ctx, `INSERT INTO policy_rules(rule_id,display_name,enabled,decision,match_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`,
		"rule_00000000000000000000000000000091", "late Policy", 1, "deny", lateRuleJSON, "now", "now")
	if err == nil || !strings.Contains(err.Error(), "Policy reference unavailable") {
		t.Fatalf("late Policy insert error=%v, want guarded refusal", err)
	}
}

func TestProviderDeletionAndProfileBindingConcurrentSeparateRuntimes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	r := newProviderTestRuntime(t)
	providerID, toolID := providerReferenceFixture(t, r)
	slug := profileReferenceFixture(t, r)
	second, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: r.providerDatabasePath})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	results := make(chan struct {
		name string
		err  error
	}, 2)
	go func() {
		defer wg.Done()
		<-start
		results <- struct {
			name string
			err  error
		}{"delete", r.DeleteConfiguredProvider(ctx, providerID)}
	}()
	go func() {
		defer wg.Done()
		<-start
		_, err := second.Profiles.AddTool.Execute(ctx, profileapplication.AddToolRequest{ProfileSlug: slug, ToolID: toolID})
		results <- struct {
			name string
			err  error
		}{"bind", err}
	}()
	close(start)
	wg.Wait()
	close(results)
	outcomes := map[string]error{}
	for result := range results {
		outcomes[result.name] = result.err
	}
	if outcomes["delete"] == nil {
		if outcomes["bind"] == nil {
			t.Fatal("delete AND binding succeeded, dangling reference possible")
		}
		if !errors.Is(outcomes["bind"], catalogapplication.ErrToolNotFound) &&
			!errors.Is(outcomes["bind"], profileapplication.ErrToolReferenceConflict) {
			t.Fatalf("binding lost deletion race with unexpected error %v", outcomes["bind"])
		}
	} else {
		if outcomes["bind"] != nil || !errors.Is(outcomes["delete"], ErrProviderInUse) {
			t.Fatalf("concurrent outcomes: delete=%v bind=%v", outcomes["delete"], outcomes["bind"])
		}
	}
	// The end state is never an orphan, regardless of who won the write gate.
	db, err := openProviderReferenceDB(ctx, r.providerDatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var dangling int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM profile_tools b LEFT JOIN tools t ON t.id=b.tool_id WHERE t.id IS NULL`).Scan(&dangling); err != nil {
		t.Fatal(err)
	}
	if dangling != 0 {
		t.Fatalf("dangling Profile bindings=%d", dangling)
	}
}

func TestProviderReferenceGuardRejectsInvalidPolicyUpdate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	r := newProviderTestRuntime(t)
	providerID, _ := providerReferenceFixture(t, r)
	rule, err := r.Policy.CreateRule.Execute(ctx, policyapplication.RuleInput{
		DisplayName: "Unrelated rule", Enabled: true, Decision: policydomain.DecisionDeny,
		Match: policydomain.MatchSpec{Actions: []govdomain.Action{govdomain.ActionToolInvoke}},
	})
	if err != nil {
		t.Fatal(err)
	}
	db, err := openProviderReferenceDB(ctx, r.providerDatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	invalid := fmt.Sprintf(`{"actions":["tool.invoke"],"providerIds":["%s"],"toolIds":[],"factKnowledge":"any"}`, "prov_00000000000000000000000000000098")
	_, err = db.ExecContext(ctx, "UPDATE policy_rules SET match_json=? WHERE rule_id=?", invalid, rule.ID().String())
	if err == nil || !strings.Contains(err.Error(), "Policy reference unavailable") {
		t.Fatalf("invalid Policy UPDATE: %v", err)
	}
	// Ensure unaffected rules with null optional selector arrays remain writable.
	_, err = r.Policy.UpdateRule.Execute(ctx, rule.ID(), policyapplication.RuleInput{
		DisplayName: "Still Unrelated", Enabled: true, Decision: policydomain.DecisionDeny,
		Match: policydomain.MatchSpec{Actions: []govdomain.Action{govdomain.ActionToolInvoke}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.DeleteConfiguredProvider(ctx, providerID); err != nil {
		t.Fatal(err)
	}
}

func TestProviderReferenceGuardsRejectLateConfigAndProviderIDPolicy(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	r := newProviderTestRuntime(t)
	providerID, _ := providerReferenceFixture(t, r)
	if err := r.DeleteConfiguredProvider(ctx, providerID); err != nil {
		t.Fatal(err)
	}
	db, err := openProviderReferenceDB(ctx, r.providerDatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	_, err = db.ExecContext(ctx, `INSERT INTO provider_configs(provider_id,plugin_id,config_version,revision,config_json)
        VALUES (?,?,?,?,?)`, providerID.String(), mcpplugin.ID.String(), 1, "fresh", `{}`)
	if err == nil || !strings.Contains(err.Error(), "Provider config reference unavailable") {
		t.Fatalf("late config creation error: %v", err)
	}
	matchJSON := fmt.Sprintf(`{"actions":["tool.invoke"],"providerIds":["%s"],"toolIds":[],"factKnowledge":"any"}`, providerID.String())
	_, err = db.ExecContext(ctx, `INSERT INTO policy_rules(rule_id,display_name,enabled,decision,match_json,created_at,updated_at)
        VALUES(?,?,?,?,?,?,?)`, "rule_00000000000000000000000000000092", "late provider", 1, "deny", matchJSON, "now", "now")
	if err == nil || !strings.Contains(err.Error(), "Policy reference unavailable") {
		t.Fatalf("late ProviderID policy creation error: %v", err)
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM provider_configs WHERE provider_id=?", providerID.String()).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("orphan configs = %d", count)
	}
}

func TestProviderReferenceSQLDeleteGuardWithoutPreflight(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	r := newProviderTestRuntime(t)
	providerID, toolID := providerReferenceFixture(t, r)
	slug := profileReferenceFixture(t, r)
	_, err := r.Profiles.AddTool.Execute(ctx, profileapplication.AddToolRequest{ProfileSlug: slug, ToolID: toolID})
	if err != nil {
		t.Fatal(err)
	}
	db, err := openProviderReferenceDB(ctx, r.providerDatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	_, err = db.ExecContext(ctx, "DELETE FROM providers WHERE id=?", providerID.String())
	if err == nil || !strings.Contains(err.Error(), "Provider has active references") {
		t.Fatalf("direct Provider deletion error: %v", err)
	}
	if _, err := r.Catalog.GetProvider.Execute(ctx, providerID); err != nil {
		t.Fatal(err)
	}
}
