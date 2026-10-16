package bootstrap

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	agentapp "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	delegation "github.com/orz-i/Mender/backend/internal/agent/infrastructure/delegationprovider"
	agentsqlite "github.com/orz-i/Mender/backend/internal/agent/infrastructure/sqlite"
	catalogapp "github.com/orz-i/Mender/backend/internal/catalog/application"
)

func TestAgentFacadeConfigVersionIsImmutableAcrossSQLiteConnections(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "mender.db")
	r, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: path, AgentExecutor: fixtureAgentExecutor{}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	agent, err := r.CreateAgentWithTools(ctx, agentapp.CreateRequest{
		Name: "Pinned facade", Backend: agentdomain.BackendGrok, WorkspaceRoot: t.TempDir(),
	}, agentOwnerContext(t, r))
	if err != nil {
		t.Fatal(err)
	}
	state, err := r.AgentTools(ctx, agent.ID(), agentOwnerContext(t, r))
	if err != nil || state.Status != "ready" {
		t.Fatalf("facade=%+v err=%v", state, err)
	}

	independent, err := openProviderReferenceDB(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = independent.Close() }()
	for _, test := range []struct {
		name, query string
		value       any
	}{
		{"config_version", "UPDATE provider_configs SET config_version=? WHERE provider_id=?", 2},
		{"config_json", "UPDATE provider_configs SET config_json=? WHERE provider_id=?", `{"agentId":"agent_attacker"}`},
		{"plugin_id", "UPDATE provider_configs SET plugin_id=? WHERE provider_id=?", "io.mender.provider.mcp"},
		{"plugin_installation_id", "UPDATE provider_configs SET plugin_installation_id=? WHERE provider_id=?", "pinst_fake"},
	} {
		_, err := independent.ExecContext(ctx, test.query, test.value, state.ProviderID)
		if err == nil || !strings.Contains(err.Error(), "managed Agent facade config is immutable") {
			t.Fatalf("update %s error=%v, want facade immutable", test.name, err)
		}
	}
	if _, err := independent.ExecContext(ctx, "UPDATE provider_configs SET revision='metadata-refresh' WHERE provider_id=?", state.ProviderID); err != nil {
		t.Fatalf("non-security revision metadata must remain updateable: %v", err)
	}
	after, err := r.AgentTools(ctx, agent.ID(), agentOwnerContext(t, r))
	if err != nil || after.ProviderID != state.ProviderID || after.Status != "ready" {
		t.Fatalf("after=%+v err=%v", after, err)
	}
	var version int
	if err := independent.QueryRowContext(ctx, "SELECT config_version FROM provider_configs WHERE provider_id=?", state.ProviderID).Scan(&version); err != nil || version != 1 {
		t.Fatalf("config_version=%d err=%v", version, err)
	}
}

func TestAgentFacadeClaimRejectsWrongPluginConfigVersion(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	r, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "mender.db"), AgentExecutor: fixtureAgentExecutor{}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	owner := agentOwnerContext(t, r)
	agent, err := r.Agents.Create(ctx, agentapp.CreateRequest{
		Name: "Preclaim", Backend: agentdomain.BackendGrok, WorkspaceRoot: t.TempDir(),
	}, owner)
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"agentId":"` + string(agent.ID()) + `"}`)
	provider, err := registerConfiguredProviderTest(t, r, ctx, catalogapp.RegisterProviderRequest{
		Namespace: "preclaim-version", DisplayName: "Preclaim Version",
	}, delegation.ID, raw)
	if err != nil {
		t.Fatal(err)
	}
	independent, err := openProviderReferenceDB(ctx, r.providerDatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = independent.Close() }()
	if _, err := independent.ExecContext(ctx, "UPDATE provider_configs SET config_version=2 WHERE provider_id=?", provider.ID().String()); err != nil {
		t.Fatal(err)
	}
	if err := r.agentStore.ClaimToolFacade(ctx, agent.ID(), provider.ID().String()); !errors.Is(err, agentdomain.ErrConflict) {
		t.Fatalf("claim mismatched config version=%v, want conflict", err)
	}
	if _, err := independent.ExecContext(ctx, "UPDATE provider_configs SET config_version=1 WHERE provider_id=?", provider.ID().String()); err != nil {
		t.Fatal(err)
	}
	if err := r.agentStore.ClaimToolFacade(ctx, agent.ID(), provider.ID().String()); err != nil {
		t.Fatal(err)
	}
}

func TestAgentFacadeV8PreservesStrictConfigGuard(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "mender.db")
	r, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: path, AgentExecutor: fixtureAgentExecutor{}})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := r.CreateAgentWithTools(ctx, agentapp.CreateRequest{
		Name: "Upgrade Guard", Backend: agentdomain.BackendGrok, WorkspaceRoot: t.TempDir(),
	}, agentOwnerContext(t, r))
	if err != nil {
		t.Fatal(err)
	}
	state, err := r.AgentTools(ctx, agent.ID(), agentOwnerContext(t, r))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := openProviderReferenceDB(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	// V6 retains the V5 hard-cut guard without requiring an artificial
	// downgrade of schema history or dual-read migration logic.
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := agentsqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	verify, err := openProviderReferenceDB(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = verify.Close() }()
	var version int
	if err := verify.QueryRowContext(ctx, "SELECT MAX(version) FROM agent_schema_migrations").Scan(&version); err != nil || version != 8 {
		t.Fatalf("migrated version=%d err=%v", version, err)
	}
	_, err = verify.ExecContext(ctx, "UPDATE provider_configs SET config_version=2 WHERE provider_id=?", state.ProviderID)
	if err == nil || !strings.Contains(err.Error(), "managed Agent facade config is immutable") {
		t.Fatalf("upgraded guard permits version change: %v", err)
	}
	second, err := agentsqlite.Open(ctx, path)
	if err != nil {
		t.Fatalf("idempotent migration: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
}
