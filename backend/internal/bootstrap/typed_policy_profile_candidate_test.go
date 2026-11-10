package bootstrap

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	app "github.com/orz-i/Mender/backend/internal/catalog/application"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	modeltoolsplugin "github.com/orz-i/Mender/backend/internal/modeltools/infrastructure/plugin"
	policyapp "github.com/orz-i/Mender/backend/internal/policy/application"
	policy "github.com/orz-i/Mender/backend/internal/policy/domain"
	profileapp "github.com/orz-i/Mender/backend/internal/profile/application"
	profile "github.com/orz-i/Mender/backend/internal/profile/domain"
	providerconfig "github.com/orz-i/Mender/backend/internal/providerplugin/configstore"
	"github.com/orz-i/Mender/backend/internal/toolsuite/infrastructure/curated"
	_ "modernc.org/sqlite"
)

func TestTypedCatalogProfilePolicyAndProviderConfigReuseSameBusinessContracts(t *testing.T) {
	ctx := context.Background()
	dbPath, providerID, providerToolID := v3CatalogFixture(t)
	catalogRoot, err := NewTypedCatalog(ctx, CatalogOptions{DatabasePath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = catalogRoot.Close() }()
	profileRoot, err := NewTypedProfiles(ctx, dbPath, catalogRoot.store, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = profileRoot.Close() }()
	policyRoot, err := NewTypedPolicies(ctx, dbPath, catalogRoot, profileRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = policyRoot.Close() }()
	configStore, err := providerconfig.OpenTyped(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = configStore.Close() }()
	configured, err := configStore.GetProviderConfig(ctx, providerID)
	if err != nil || configured.ProviderID != providerID || configured.PluginID != modeltoolsplugin.ID {
		t.Fatalf("v3 Provider config/Plugin identity changed: %+v %v", configured.ProviderID, err)
	}
	p, err := profileRoot.Create.Execute(ctx, profileapp.CreateProfileRequest{Slug: "runtime-review", DisplayName: "Runtime review"})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := profileRoot.AddTool.Execute(ctx, profileapp.AddToolRequest{
		ProfileSlug: p.Slug(), ToolID: providerToolID, Alias: "external_search",
		PolicyConstraint: profile.PolicyRequireApproval, ConcurrencyConstraint: profile.ConcurrencySerial,
	})
	if err != nil {
		t.Fatalf("existing Provider ToolBinding failed on typed Catalog: %v", err)
	}
	binding, found := updated.Binding(providerToolID)
	if !found || binding.PolicyConstraint() != profile.PolicyRequireApproval || binding.ConcurrencyConstraint() != profile.ConcurrencySerial {
		t.Fatalf("typed Profile SourceRef ToolBinding constraints lost: %+v", binding)
	}
	selected, err := catalogRoot.DescribeTool.Execute(ctx, providerToolID)
	if err != nil || selected.SourceRef().String() != "provider:"+providerID.String() {
		t.Fatalf("typed Profile selected noncanonical Tool: %+v %v", selected, err)
	}
	native, err := curated.PlannedNativeTools()
	if err != nil {
		t.Fatal(err)
	}
	var nativeID string
	for _, item := range native {
		if item.SourceKey == "workspace.read" {
			nativeID = item.ToolID.String()
		}
	}
	if nativeID == "" {
		t.Fatal("canonical native ToolID missing")
	}
	rule, err := policyRoot.CreateRule.Execute(ctx, policyapp.RuleInput{
		DisplayName: "Deny this native operation", Enabled: true, Decision: policy.DecisionDeny,
		Match: policy.MatchSpec{Actions: []gov.Action{gov.ActionToolInvoke}, ToolIDs: []string{nativeID}, FactKnowledge: policy.FactKnown},
	})
	if err != nil {
		t.Fatalf("Policy reference to typed native ToolID was rejected: %v", err)
	}
	policySet, err := policyRoot.Get.Execute(ctx)
	if err != nil || len(policySet.Rules()) != 1 || policySet.Rules()[0].ID() != rule.ID() {
		t.Fatalf("typed Policy rule not persisted: %+v %v", policySet, err)
	}
	input := policyapp.EvaluateRequest{Action: gov.ActionToolInvoke, Origin: gov.OriginMCP,
		PrincipalKind: gov.PrincipalService, PrincipalReference: "profile:" + p.ID().String(),
		ScopeKind: gov.ScopeProfile, ScopeReference: p.ID().String(), SourceRef: "native:workspace",
		ToolID: nativeID, FactKnowledge: policy.FactKnown, Effects: []string{"filesystem.read"},
		Resources: []policyapp.ResourceFact{{Kind: "workspace_path", Value: "README.md"}},
	}
	denied, err := policyRoot.Evaluate.Execute(ctx, input)
	if err != nil || denied.Kind() != policy.DecisionDeny {
		t.Fatalf("native ToolID-specific Policy denied no longer: %+v %v", denied, err)
	}
	input.SourceRef = "provider:" + providerID.String()
	input.ToolID = providerToolID.String()
	allowed, err := policyRoot.Evaluate.Execute(ctx, input)
	if err != nil || allowed.Kind() != policy.DecisionAllow {
		t.Fatalf("native rule leaked to Provider Tool: %+v %v", allowed, err)
	}
	if _, err := policyRoot.CreateRule.Execute(ctx, policyapp.RuleInput{DisplayName: "Nonexistent Tool", Enabled: true,
		Decision: policy.DecisionDeny, Match: policy.MatchSpec{Actions: []gov.Action{gov.ActionToolInvoke},
			ToolIDs: []string{"tool_99999999999999999999999999999999"}}}); err == nil {
		t.Fatal("Policy accepted an unregistered ToolID")
	}
	if _, err := catalogRoot.PageTools.Execute(ctx, app.PageToolsRequest{Mode: "browse", Limit: 100}); err != nil {
		t.Fatal(err)
	}
}

func TestTypedPolicyAndProviderConfigStoresRejectIncompatibleDatabase(t *testing.T) {
	ctx := context.Background()
	dbPath, _, _ := v3CatalogFixture(t)
	bad := filepath.Join(t.TempDir(), "bad.sqlite")
	raw, err := sql.Open("sqlite", bad)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`CREATE TABLE tool_hardcut_schema_migrations(version INTEGER PRIMARY KEY); INSERT INTO tool_hardcut_schema_migrations(version) VALUES(0)`); err != nil {
		_ = raw.Close()
		t.Fatal(err)
	}
	_ = raw.Close()
	for _, open := range []struct {
		name  string
		check func(string) error
	}{
		{"Policy", func(path string) error {
			store, err := NewTypedPolicies(ctx, path, nil, nil)
			if err != nil {
				return err
			}
			return store.Close()
		}},
		{"Provider", func(path string) error {
			store, err := providerconfig.OpenTyped(ctx, path)
			if err != nil {
				return err
			}
			return store.Close()
		}},
	} {
		t.Run(open.name, func(t *testing.T) {
			if err := open.check(bad); err == nil {
				t.Fatal("accepted incompatible schema database")
			}
			if err := open.check(filepath.Join(t.TempDir(), "missing.db")); err == nil {
				t.Fatal("created missing database")
			}
		})
	}
	cat, err := NewTypedCatalog(ctx, CatalogOptions{DatabasePath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cat.Close() }()
	profiles, err := NewTypedProfiles(ctx, dbPath, cat.store, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = profiles.Close() }()
	rules, err := NewTypedPolicies(ctx, dbPath, cat, profiles)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rules.Close() }()
	if _, err := rules.Get.Execute(ctx); err != nil {
		t.Fatal(err)
	}
}
