package delegationprovider

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
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

func (inertRunner) ExecuteConfigured(context.Context, agentdomain.Agent, string, agentapplication.RunConfig) (string, error) {
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
	a, err := svc.Create(ctx, agentapplication.CreateRequest{Name: "Codex Test", Backend: agentdomain.BackendCodex, WorkspaceRoot: t.TempDir(), Isolation: "host"}, c)
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
	if err != nil || len(items) != 5 {
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
	var delegate struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(submitInput, &delegate); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(delegate.Properties["timeoutMinutes"]), "[10,20,30]") {
		t.Fatal("missing explicit finite Agent execution budgets")
	}
	if !strings.Contains(string(taskOutput), "lastNativeProgressAt") ||
		!strings.Contains(string(taskOutput), "timeoutSeconds") {
		t.Fatal("Task result schema lost native progress/budget")
	}
	antigravity, err := svc.Create(ctx, agentapplication.CreateRequest{
		Name: "Agy Host Test", Backend: agentdomain.BackendAntigravity,
		WorkspaceRoot: t.TempDir(), Isolation: "host",
	}, c)
	if err != nil {
		t.Fatal(err)
	}
	agyConfig := []byte(`{"agentId":"` + string(antigravity.ID()) + `"}`)
	agyInstance, err := p.Open(ctx, providerplugin.OpenRequest{ConfigVersion: ConfigVersion, Config: agyConfig})
	if err != nil {
		t.Fatal(err)
	}
	agyTools, err := agyInstance.DiscoverTools(ctx, catalogdomain.Provider{})
	if err != nil || len(agyTools) != 5 || agyTools[4].SourceKey != sourceResume ||
		!strings.Contains(agyTools[4].Description, "exact conversation ID") ||
		slices.Contains(agyTools[4].Tags, "unavailable") {
		t.Fatalf("Agy host conversation continuation capability misrepresented: %+v %v", agyTools, err)
	}
	if items[4].SourceKey != agyTools[4].SourceKey || items[4].Name != agyTools[4].Name ||
		strings.Contains(items[4].Description, "Unavailable") {
		t.Fatal("canonical ToolID or supported ACP metadata drifted")
	}
	restricted, err := svc.Create(ctx, agentapplication.CreateRequest{
		Name: "Restricted Agy", Backend: agentdomain.BackendAntigravity,
		WorkspaceRoot: t.TempDir(), Isolation: "restricted_host",
	}, c)
	if err != nil {
		t.Fatal(err)
	}
	lockedConfig := []byte(`{"agentId":"` + string(restricted.ID()) + `"}`)
	lockedInstance, err := p.Open(ctx, providerplugin.OpenRequest{
		ConfigVersion: ConfigVersion, Config: lockedConfig,
	})
	if err != nil {
		t.Fatal(err)
	}
	lockedTools, err := lockedInstance.DiscoverTools(ctx, catalogdomain.Provider{})
	if err != nil || len(lockedTools) != 5 ||
		!slices.Contains(lockedTools[4].Tags, "unavailable") {
		t.Fatalf("Restricted Agy incorrectly advertised as restorable: %+v %v", lockedTools, err)
	}
}
