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
		if err != nil || state.Status != "ready" || len(state.Tools) != 4 || state.ProviderName != name {
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
		if err != nil || next.ProviderID != state.ProviderID || len(next.Tools) != 4 {
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

func TestAgentToolFacadeAdoptsLegacyWithoutRebinding(t *testing.T) {
	ctx := context.Background()
	r, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "mender.sqlite"), AgentExecutor: fixtureAgentExecutor{}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	owner := agentOwnerContext(t, r)
	agent, err := r.Agents.Create(ctx, agentapp.CreateRequest{Name: "Legacy Grok", Backend: agentdomain.BackendGrok, WorkspaceRoot: t.TempDir()}, owner)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]string{"agentId": string(agent.ID())})
	provider, err := registerConfiguredProviderTest(t, r, ctx, catalogapp.RegisterProviderRequest{
		Namespace: "grok-legacy", DisplayName: "Existing Grok Provider",
	}, delegation.ID, raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.SyncConfiguredProvider(ctx, provider.ID(), owner); err != nil {
		t.Fatal(err)
	}
	before, err := r.AgentTools(ctx, agent.ID(), owner)
	if err != nil || before.Status != "legacy" || len(before.Tools) != 4 {
		t.Fatalf("legacy before=%+v err=%v", before, err)
	}
	claimed, err := r.EnsureAgentTools(ctx, agent.ID(), owner)
	if err != nil || claimed.Status != "ready" || claimed.ProviderID != provider.ID().String() {
		t.Fatalf("adoption=%+v err=%v", claimed, err)
	}
	if claimed.ProviderName != "Existing Grok Provider" {
		t.Fatal("display name was rewritten")
	}
	for i, tool := range before.Tools {
		if claimed.Tools[i].ToolID != tool.ToolID {
			t.Fatal("existing ToolID was changed")
		}
	}
	if err := r.agentStore.ClaimToolFacade(ctx, agent.ID(), provider.ID().String()); err != nil {
		t.Fatal(err)
	}
}

func TestAgentToolFacadeConflictingLegacyProvidersFailClosed(t *testing.T) {
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
