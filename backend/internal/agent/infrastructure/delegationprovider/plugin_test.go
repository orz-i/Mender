package delegationprovider

import (
	"context"
	"path/filepath"
	"testing"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	agentsqlite "github.com/orz-i/Mender/backend/internal/agent/infrastructure/sqlite"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

type inertRunner struct{}

func (inertRunner) Execute(context.Context, agentdomain.Agent, string) (string, error) {
	return "ok", nil
}
func TestPluginConfigAndBoundToolDiscovery(t *testing.T) {
	ctx := context.Background()
	s, err := agentsqlite.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	owner, err := gov.NewPrincipal(gov.PrincipalLocalUser, "os:test")
	if err != nil {
		t.Fatal(err)
	}
	c, err := gov.NewContext(gov.OriginInternal, owner, gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	svc, err := agentapplication.NewService(s, inertRunner{}, func(context.Context, gov.Context, agentdomain.Agent, gov.Action) error { return nil }, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = svc.Close() }()
	a, err := svc.Create(ctx, agentapplication.CreateRequest{Name: "Codex Test", Backend: agentdomain.BackendCodex, WorkspaceRoot: t.TempDir()}, c)
	if err != nil {
		t.Fatal(err)
	}
	p := New(svc, owner)
	config := []byte(`{"agentId":"` + string(a.ID()) + `"}`)
	if err := p.ValidateConfig(ctx, config); err != nil {
		t.Fatal(err)
	}
	if err := p.ValidateConfig(ctx, []byte(`{"agentId":"x"}`)); err == nil {
		t.Fatal("untrusted AgentID accepted")
	}
	instance, err := p.Open(ctx, providerplugin.OpenRequest{ConfigVersion: ConfigVersion, Config: config})
	if err != nil {
		t.Fatal(err)
	}
	items, err := instance.DiscoverTools(ctx, catalogdomain.Provider{})
	if err != nil || len(items) != 4 {
		t.Fatalf("tools=%d err=%v", len(items), err)
	}
	for _, item := range items {
		if len(item.InputSchema) == 0 || len(item.OutputSchema) == 0 {
			t.Fatal("schema missing", item.SourceKey)
		}
	}
	value, err := invocationdomain.NewJSONValue([]byte(`{"prompt":"review"}`))
	if err != nil {
		t.Fatal(err)
	}
	_ = invocationapplication.ExecutionRequest{Arguments: value}
}
