package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	agentapp "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	delegation "github.com/orz-i/Mender/backend/internal/agent/infrastructure/delegationprovider"
	catalogapp "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
)

func TestAgentManagedToolsCreateDistinctAndIdempotent(t *testing.T) {
	ctx := context.Background()
	r, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "runtime.sqlite"), AgentExecutor: fixtureAgentExecutor{}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	owner := agentOwnerContext(t, r)
	var identities = map[string]bool{}
	for _, name := range []string{"Grok Frontend", "Grok Review"} {
		agent, err := r.CreateAgentWithTools(ctx, agentapp.CreateRequest{
			Name: name, Backend: agentdomain.BackendGrok, WorkspaceRoot: t.TempDir(),
		}, owner)
		if err != nil {
			t.Fatal(err)
		}
		state, err := r.AgentTools(ctx, agent.ID(), owner)
		if err != nil || state.Status != "ready" || len(state.Tools) != 5 || state.ProviderName != name {
			t.Fatalf("managed Agent facade=%+v err=%v", state, err)
		}
		if identities[state.ProviderID] {
			t.Fatalf("shared Provider identity across Agents: %s", state.ProviderID)
		}
		identities[state.ProviderID] = true
		if state.Tools[0].ToolID == "" {
			t.Fatal("missing stable ToolID")
		}
		pid, err := catalogdomain.ParseProviderID(state.ProviderID)
		if err != nil {
			t.Fatal(err)
		}
		if err := r.DeleteConfiguredProvider(ctx, pid); !errors.Is(err, ErrProviderInUse) {
			t.Fatalf("managed suite deleted through Provider CRUD: %v", err)
		}
		next, err := r.EnsureAgentTools(ctx, agent.ID(), owner)
		if err != nil || next.ProviderID != state.ProviderID || len(next.Tools) != 5 {
			t.Fatalf("ensure not idempotent: %+v %v", next, err)
		}
		for i, tool := range state.Tools {
			if next.Tools[i].ToolID != tool.ToolID {
				t.Fatal("ToolID changed on ensure")
			}
		}
		grants, err := r.Agents.Grants(ctx, agent.ID(), owner)
		if err != nil || len(grants) != 0 {
			t.Fatalf("tool provisioning granted Profiles: %v %v", grants, err)
		}
		if err = r.DeleteAgentWithTools(ctx, agent.ID(), owner); err != nil {
			t.Fatalf("unexposed Agent should be deletable: %v", err)
		}
	}
}

func TestAgentToolFacadeUnpinnedProviderFailsClosedWithoutRebinding(t *testing.T) {
	ctx := context.Background()
	r, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "mender.sqlite"), AgentExecutor: fixtureAgentExecutor{}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	owner := agentOwnerContext(t, r)
	agent, err := r.Agents.Create(ctx, agentapp.CreateRequest{Name: "Unpinned Grok", Backend: agentdomain.BackendGrok, WorkspaceRoot: t.TempDir()}, owner)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]string{"agentId": string(agent.ID())})
	provider, err := registerConfiguredProviderTest(t, r, ctx, catalogapp.RegisterProviderRequest{
		Namespace: "grok-unpinned", DisplayName: "Existing Unpinned Provider",
	}, delegation.ID, raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.syncConfiguredProviderForOwner(ctx, provider.ID(), owner, true); err != nil {
		t.Fatal(err)
	}
	original, err := r.Catalog.store.ListProviderTools(ctx, provider.ID())
	if err != nil || len(original) != 5 {
		t.Fatalf("expected four existing tools: %d %v", len(original), err)
	}
	before, err := r.AgentTools(ctx, agent.ID(), owner)
	if err != nil || before.Status != "conflict" || before.ProviderID != "" {
		t.Fatalf("unpinned Provider was treated as adoptable: %+v err=%v", before, err)
	}
	if _, err = r.EnsureAgentTools(ctx, agent.ID(), owner); !errors.Is(err, agentdomain.ErrConflict) {
		t.Fatalf("unpinned Provider was silently claimed: %v", err)
	}
	if err = r.DeleteAgentWithTools(ctx, agent.ID(), owner); !errors.Is(err, agentdomain.ErrConflict) {
		t.Fatalf("Agent referencing unpinned Provider was deleted: %v", err)
	}
	pinned, err := r.agentStore.GetToolFacade(ctx, agent.ID())
	if err != nil || pinned != "" {
		t.Fatalf("Provider was unexpectedly pinned: %q %v", pinned, err)
	}
	current, err := r.Catalog.store.ListProviderTools(ctx, provider.ID())
	if err != nil || len(current) != len(original) {
		t.Fatalf("Tool inventory altered: %d %v", len(current), err)
	}
	for i, tool := range original {
		if current[i].ID() != tool.ID() {
			t.Fatal("preexisting ToolID rotated")
		}
	}
	configs, err := r.providerConfigs.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	matches := 0
	for _, config := range configs {
		if config.ProviderID == provider.ID() {
			matches++
		}
	}
	if matches != 1 {
		t.Fatalf("original Provider removed or duplicated: %d", matches)
	}
}

func TestAgentToolFacadeMultipleUnpinnedProvidersFailClosed(t *testing.T) {
	ctx := context.Background()
	r, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "mender.sqlite"), AgentExecutor: fixtureAgentExecutor{}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	owner := agentOwnerContext(t, r)
	agent, err := r.Agents.Create(ctx, agentapp.CreateRequest{Name: "Conflict", Backend: agentdomain.BackendGrok, WorkspaceRoot: t.TempDir()}, owner)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]string{"agentId": string(agent.ID())})
	for _, ns := range []string{"agent-first", "agent-second"} {
		if _, err = registerConfiguredProviderTest(t, r, ctx, catalogapp.RegisterProviderRequest{Namespace: ns, DisplayName: ns}, delegation.ID, raw); err != nil {
			t.Fatal(err)
		}
	}
	state, err := r.AgentTools(ctx, agent.ID(), owner)
	if err != nil || state.Status != "conflict" {
		t.Fatalf("expected conflict: %+v %v", state, err)
	}
	if _, err := r.EnsureAgentTools(ctx, agent.ID(), owner); !errors.Is(err, agentdomain.ErrConflict) {
		t.Fatalf("multiple Providers silently adopted: %v", err)
	}
	if err := r.DeleteAgentWithTools(ctx, agent.ID(), owner); !errors.Is(err, agentdomain.ErrConflict) {
		t.Fatalf("conflicting resource silently deleted: %v", err)
	}
}
