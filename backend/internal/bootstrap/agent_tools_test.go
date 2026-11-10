package bootstrap

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	agentapp "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	sourcecatalog "github.com/orz-i/Mender/backend/internal/catalog/infrastructure/sourcecatalog"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	policyapplication "github.com/orz-i/Mender/backend/internal/policy/application"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
)

// Every Agent as Tool is a directly owned Catalog source. No resource Provider
// is synthesized, even when two Agents use the same Backend and tool names.
func TestAgentOwnedToolsAreDistinctIdempotentAndRetireAtomically(t *testing.T) {
	ctx := context.Background()
	r, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "v3.db"), AgentExecutor: fixtureAgentExecutor{}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	owner := agentOwnerContext(t, r)
	identities := map[string]bool{}
	for _, name := range []string{"Grok Frontend", "Grok Review"} {
		agent, err := r.CreateAgentWithTools(ctx, agentapp.CreateRequest{Name: name, Backend: agentdomain.BackendGrok, WorkspaceRoot: t.TempDir()}, owner)
		if err != nil {
			t.Fatal(err)
		}
		state, err := r.AgentTools(ctx, agent.ID(), owner)
		if err != nil || state.Status != "ready" || len(state.Tools) != 5 {
			t.Fatalf("Agent-owned source: %+v %v", state, err)
		}
		for _, tool := range state.Tools {
			if identities[tool.ToolID] {
				t.Fatalf("Tool ID shared between Agents: %s", tool.ToolID)
			}
			identities[tool.ToolID] = true
		}
		again, err := r.EnsureAgentTools(ctx, agent.ID(), owner)
		if err != nil || len(again.Tools) != 5 {
			t.Fatalf("idempotent source refresh: %+v %v", again, err)
		}
		for i, tool := range state.Tools {
			if tool.ToolID != again.Tools[i].ToolID {
				t.Fatal("ToolID rotated on refresh")
			}
		}
		grants, err := r.Agents.Grants(ctx, agent.ID(), owner)
		if err != nil || len(grants) != 0 {
			t.Fatalf("source creation granted authority: %v %v", grants, err)
		}
		if err = r.DeleteAgentWithTools(ctx, agent.ID(), owner); err != nil {
			t.Fatalf("unreferenced Agent not retired: %v", err)
		}
		if _, err = r.AgentTools(ctx, agent.ID(), owner); !errors.Is(err, agentdomain.ErrNotFound) {
			t.Fatalf("retired Agent remained visible: %v", err)
		}
	}
}

// Exact/prefix agent_id Policy matchers persist under resourcesAny. Retiring an
// Agent must not orphan those rules, nor rebind identity when Tools disappear.
func TestAgentRetirementHonorsReviewedPolicyReferences(t *testing.T) {
	ctx := context.Background()
	r, e := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "v3.sqlite")})
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = r.Close() }()
	owner := agentOwnerContext(t, r)
	agent, e := r.CreateAgentWithTools(ctx, agentapp.CreateRequest{Name: "Policy protected", Backend: agentdomain.BackendGrok, WorkspaceRoot: t.TempDir()}, owner)
	if e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		name  string
		op    policydomain.ResourceOperator
		value string
	}{
		{"exact", policydomain.ResourceExact, string(agent.ID())},
		{"prefix", policydomain.ResourcePrefix, string(agent.ID())[:12]},
	} {
		matcher, e := policydomain.NewResourceMatcher("agent_id", tc.op, tc.value)
		if e != nil {
			t.Fatal(e)
		}
		rule, e := r.Policy.CreateRule.Execute(ctx, policyapplication.RuleInput{DisplayName: tc.name + " Agent deny", Enabled: true, Decision: policydomain.DecisionDeny, Match: policydomain.MatchSpec{Actions: []gov.Action{gov.ActionAgentTaskSubmit}, ResourcesAny: []policydomain.ResourceMatcher{matcher}}})
		if e != nil {
			t.Fatal(e)
		}
		if e = r.DeleteAgentWithTools(ctx, agent.ID(), owner); !errors.Is(e, sourcecatalog.ErrRetirementRequiresReview) {
			t.Fatalf("retirement ignored %s Policy reference: %v", tc.name, e)
		}
		if _, e = r.Agents.Get(ctx, agent.ID(), owner); e != nil {
			t.Fatalf("failed deletion removed Agent: %v", e)
		}
		if e = r.Policy.DeleteRule.Execute(ctx, rule.ID()); e != nil {
			t.Fatal(e)
		}
	}
	if e = r.DeleteAgentWithTools(ctx, agent.ID(), owner); e != nil {
		t.Fatal(e)
	}
}
