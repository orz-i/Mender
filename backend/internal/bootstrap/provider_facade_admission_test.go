package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	agentdelegation "github.com/orz-i/Mender/backend/internal/agent/infrastructure/delegationprovider"
	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sshjobsplugin "github.com/orz-i/Mender/backend/internal/sshjobs/infrastructure/plugin"
	workspaceplugin "github.com/orz-i/Mender/backend/internal/workspace/infrastructure/plugin"
)

func TestGenericProviderCreateDeniesEveryResourceOwnedFacade(t *testing.T) {
	ctx := context.Background()
	r := newProviderTestRuntime(t)
	owner := testCatalogGovernanceContext(t, "internal", "deny-private-facade")
	before, err := r.Catalog.ListProviders.Execute(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, pluginID := range []providerplugin.ID{
		agentdelegation.ID, workspaceplugin.ID, sshjobsplugin.ID,
	} {
		t.Run(pluginID.String(), func(t *testing.T) {
			_, err := r.RegisterConfiguredProvider(ctx,
				catalogapplication.RegisterProviderRequest{Namespace: "forbidden", DisplayName: "Forbidden"},
				pluginID, json.RawMessage("{}"), owner)
			if !errors.Is(err, sandboxdomain.ErrNotAuthorized) {
				t.Fatalf("public create %s: %v, want forbidden", pluginID, err)
			}
			installedID, _ := providerplugin.ParseInstallationID("pinst_0123456789abcdef0123456789abcdef")
			_, err = r.RegisterInstalledProvider(ctx,
				catalogapplication.RegisterProviderRequest{Namespace: "forbidden", DisplayName: "Forbidden"},
				pluginID, installedID, json.RawMessage("{}"), owner)
			if !errors.Is(err, sandboxdomain.ErrNotAuthorized) {
				t.Fatalf("native spoof of private %s: %v", pluginID, err)
			}
		})
	}
	after, err := r.Catalog.ListProviders.Execute(ctx)
	if err != nil || len(after) != len(before) {
		t.Fatalf("denied creations mutated providers: %d => %d, err=%v", len(before), len(after), err)
	}
	catalogue, err := r.ListPluginCatalogue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, item := range catalogue {
		if !isManagedProviderFacade(providerplugin.ID(item.PluginID)) {
			continue
		}
		counts[item.PluginID]++
		if item.MarketplaceAction != MarketplaceNone {
			t.Fatalf("private technical entry advertised as market offering: %+v", item)
		}
	}
	for _, id := range []providerplugin.ID{agentdelegation.ID, workspaceplugin.ID, sshjobsplugin.ID} {
		if counts[id.String()] != 1 {
			t.Fatalf("private %s absent from technical catalogue: %d", id, counts[id.String()])
		}
	}
}

func TestManagedAgentProviderCannotBeEditedThroughGenericProviderAPI(t *testing.T) {
	ctx := context.Background()
	r, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "managed.db"), AgentExecutor: fixtureAgentExecutor{}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	owner := agentOwnerContext(t, r)
	agent, err := r.CreateAgentWithTools(ctx, agentapplication.CreateRequest{
		Name: "Owner", Backend: agentdomain.BackendGrok, WorkspaceRoot: t.TempDir(),
	}, owner)
	if err != nil {
		t.Fatal(err)
	}
	tools, err := r.AgentTools(ctx, agent.ID(), owner)
	if err != nil || tools.Status != "ready" || len(tools.Tools) != 4 {
		t.Fatalf("owned tools missing: %+v %v", tools, err)
	}
	providerID, err := catalogdomain.ParseProviderID(tools.ProviderID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.UpdateConfiguredProvider(ctx, providerID,
		catalogapplication.UpdateProviderRequest{DisplayName: "Forged", Description: "Changed"},
		json.RawMessage("{}"), owner)
	if !errors.Is(err, ErrProviderInUse) {
		t.Fatalf("generic managed Provider update accepted: %v", err)
	}
	state, err := r.AgentTools(ctx, agent.ID(), owner)
	if err != nil || state.Status != "ready" || state.ProviderID != tools.ProviderID {
		t.Fatalf("generic update mutated facade: %+v %v", state, err)
	}
}
