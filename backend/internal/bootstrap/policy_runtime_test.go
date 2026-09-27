package bootstrap

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	policyapplication "github.com/orz-i/Mender/backend/internal/policy/application"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

func TestRuntimePersistentPolicyPreservesAllowThenSupportsDefaultDenyAllowlist(
	t *testing.T,
) {
	t.Parallel()

	ctx := context.Background()
	runtime, toolID, executor := governedRuntimeFixture(t, RuntimeOptions{
		DatabasePath:   filepath.Join(t.TempDir(), "mender.db"),
		DefaultTimeout: time.Second,
	})
	defer func() { _ = runtime.Close() }()

	policy, err := runtime.Policy.Get.Execute(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if policy.DefaultDecision() != policydomain.DecisionAllow ||
		len(policy.Rules()) != 0 {
		t.Fatalf(
			"fresh Policy = default %q rules %#v",
			policy.DefaultDecision(),
			policy.Rules(),
		)
	}
	if runtime.Status().PolicyMode != "configured" {
		t.Fatalf("PolicyMode = %q", runtime.Status().PolicyMode)
	}

	executionContext := mustGovernanceInvocationContext(
		t,
		invocationdomain.OriginInternal,
		"policy-runtime-test",
		invocationdomain.CatalogExecutionScope(),
	)
	request := invocationapplication.InvokeToolRequest{
		ToolID:    toolID,
		Arguments: []byte("{\"count\":1}"),
		Context:   executionContext,
	}

	allowed, err := runtime.InvokeTool.Execute(ctx, request)
	if err != nil {
		t.Fatalf("initial Execute() error = %v", err)
	}
	if allowed.Outcome.Kind() != invocationdomain.OutcomeKindComplete ||
		executor.Calls() != 1 {
		t.Fatalf("initial outcome/calls = %#v/%d", allowed.Outcome, executor.Calls())
	}

	if _, err := runtime.Policy.SetDefault.Execute(
		ctx,
		policydomain.DecisionDeny,
	); err != nil {
		t.Fatal(err)
	}
	denied, err := runtime.InvokeTool.Execute(ctx, request)
	if err != nil {
		t.Fatalf("default-deny Execute() error = %v", err)
	}
	if denied.Outcome.Kind() != invocationdomain.OutcomeKindRejected ||
		denied.Outcome.RejectionCode() != invocationdomain.RejectionPolicyDenied ||
		executor.Calls() != 1 {
		t.Fatalf("default-deny outcome/calls = %#v/%d", denied.Outcome, executor.Calls())
	}

	rule, err := runtime.Policy.CreateRule.Execute(ctx, policyapplication.RuleInput{
		DisplayName: "Allow internal Catalog calls",
		Enabled:     true,
		Decision:    policydomain.DecisionAllow,
		Match: policydomain.MatchSpec{
			Origins:    []policydomain.Origin{policydomain.OriginInternal},
			ScopeKinds: []policydomain.ScopeKind{policydomain.ScopeCatalog},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	reallowed, err := runtime.InvokeTool.Execute(ctx, request)
	if err != nil {
		t.Fatalf("allowlist Execute() error = %v", err)
	}
	if reallowed.Outcome.Kind() != invocationdomain.OutcomeKindComplete ||
		executor.Calls() != 2 {
		t.Fatalf("allowlist outcome/calls = %#v/%d", reallowed.Outcome, executor.Calls())
	}

	record, err := runtime.audit.Get(ctx, reallowed.InvocationID)
	if err != nil {
		t.Fatal(err)
	}
	if record.PolicyKind() != policydomain.DecisionAllow ||
		!strings.HasSuffix(
			record.PolicyReason(),
			strings.TrimPrefix(rule.ID().String(), "rule_"),
		) {
		t.Fatalf(
			"Audit policy = %q/%q, rule=%q",
			record.PolicyKind(),
			record.PolicyReason(),
			rule.ID(),
		)
	}
}

func TestProviderDeletionIsBlockedByPolicyReference(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runtime, toolID, _ := governedRuntimeFixture(t, RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "mender.db"),
	})
	defer func() { _ = runtime.Close() }()

	tool, err := runtime.Catalog.DescribeTool.Execute(ctx, toolID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = runtime.Policy.CreateRule.Execute(ctx, policyapplication.RuleInput{
		DisplayName: "Provider reference",
		Enabled:     true,
		Decision:    policydomain.DecisionDeny,
		Match: policydomain.MatchSpec{
			ProviderIDs: []string{tool.ProviderID().String()},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	err = runtime.DeleteMCPProvider(ctx, tool.ProviderID())
	if !errors.Is(err, ErrProviderInUse) {
		t.Fatalf("DeleteMCPProvider() error = %v, want ErrProviderInUse", err)
	}
}

func TestInvocationPolicyAdapterMapsOnlyBoundedGovernanceContext(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runtime, toolID, _ := governedRuntimeFixture(t, RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "mender.db"),
	})
	defer func() { _ = runtime.Close() }()

	tool, err := runtime.Catalog.DescribeTool.Execute(ctx, toolID)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := runtime.Profiles.Create.Execute(
		ctx,
		profileapplication.CreateProfileRequest{
			Slug:        "policy-map",
			DisplayName: "Policy Map",
			Mode:        profiledomain.ExposureDynamic,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	resourceMatcher, err := policydomain.NewResourceMatcher(
		"path",
		policydomain.ResourcePrefix,
		"/workspace/release",
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = runtime.Policy.CreateRule.Execute(ctx, policyapplication.RuleInput{
		DisplayName: "Mapped bounded context",
		Enabled:     true,
		Decision:    policydomain.DecisionDeny,
		Match: policydomain.MatchSpec{
			Origins:       []policydomain.Origin{policydomain.OriginMCP},
			ScopeKinds:    []policydomain.ScopeKind{policydomain.ScopeProfile},
			ProfileRefs:   []string{profile.ID().String()},
			ProviderIDs:   []string{tool.ProviderID().String()},
			ToolIDs:       []string{tool.ID().String()},
			FactKnowledge: policydomain.FactKnown,
			EffectsAny:    []string{"filesystem.write"},
			ResourcesAny:  []policydomain.ResourceMatcher{resourceMatcher},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	resourceFact, err := invocationdomain.NewResourceFact(
		"path",
		"/workspace/release/v1",
	)
	if err != nil {
		t.Fatal(err)
	}
	facts, err := invocationdomain.KnownGovernanceFacts(
		[]invocationdomain.EffectFact{"filesystem.write"},
		[]invocationdomain.ResourceFact{resourceFact},
	)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := invocationdomain.ProfileExecutionScope(profile.ID().String())
	if err != nil {
		t.Fatal(err)
	}
	executionContext := mustGovernanceInvocationContext(
		t,
		invocationdomain.OriginMCP,
		"untrusted-descriptive-caller",
		scope,
	)

	decision, err := newInvocationPolicyEvaluator(runtime.Policy.Evaluate).Evaluate(
		ctx,
		invocationapplication.PolicyRequest{
			Tool:                  tool,
			Context:               executionContext,
			Facts:                 facts,
			Caller:                "caller-must-not-be-identity",
			HasCredentialResolver: true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Kind() != policydomain.DecisionDeny {
		t.Fatalf("mapped decision = %#v", decision)
	}
}
