package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	catalogapp "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	mcpdelivery "github.com/orz-i/Mender/backend/internal/mcp/application/delivery"
	profile "github.com/orz-i/Mender/backend/internal/profile/domain"
	tunnelapp "github.com/orz-i/Mender/backend/internal/tunnel/application"
	tunnel "github.com/orz-i/Mender/backend/internal/tunnel/domain"
)

func TestHTTPMCPDeliverySeparatesSelectionAndIndependentConfiguration(t *testing.T) {
	t.Parallel()
	runtime, server, runner, toolIDs := httpMCPDeliveryFixture(t)
	api := server.URL + "/api/v1"
	client := server.Client()
	created := requestJSON(t, client, http.MethodPost, api+"/profiles", map[string]any{"slug": "bundle", "displayName": "Bundle"}, http.StatusCreated)
	assertPureProfileProjection(t, created)
	url := api + "/profiles/bundle/mcp-delivery"
	for range 2 {
		missing := requestJSON(t, client, http.MethodGet, url, nil, http.StatusNotFound)
		if missing["code"] != "mcp_delivery_not_configured" {
			t.Fatalf("missing configuration error=%#v", missing)
		}
	}
	id, err := profile.ParseProfileID(created["profileId"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.MCPDelivery.Get.Execute(context.Background(), id); !errors.Is(err, mcpdelivery.ErrNotFound) {
		t.Fatalf("GET repaired absent configuration: %v", err)
	}
	selected := requestJSON(t, client, http.MethodPost, api+"/profiles/bundle/tools", map[string]any{
		"toolId": toolIDs[0], "alias": "selected", "policyConstraint": "require_approval", "concurrencyConstraint": "serial",
	}, http.StatusOK)
	assertPureProfileProjection(t, selected)
	selectionRevision := selected["revision"]
	outside := requestJSON(t, client, http.MethodPut, url, map[string]any{
		"mode": "hybrid", "directToolIds": []string{toolIDs[1]}, "expectedProfileRevision": selectionRevision,
	}, http.StatusBadRequest)
	if outside["code"] != "validation_error" {
		t.Fatalf("outside member classification=%#v", outside)
	}
	config := requestJSON(t, client, http.MethodPut, url, map[string]any{
		"mode": "hybrid", "directToolIds": []string{toolIDs[0]}, "expectedProfileRevision": selectionRevision,
	}, http.StatusOK)
	if config["profileId"] != created["profileId"] || config["profileRevision"] != selectionRevision || config["revision"] != float64(0) || config["mode"] != "hybrid" || len(config["directToolIds"].([]any)) != 1 {
		t.Fatalf("configuration=%#v", config)
	}
	if _, exists := config["tunnelId"]; exists {
		t.Fatal("absent Tunnel appeared in projection")
	}
	requestJSON(t, client, http.MethodPut, url, map[string]any{
		"mode": "dynamic", "directToolIds": []string{}, "expectedProfileRevision": selectionRevision,
	}, http.StatusConflict) // omitted expectedRevision remains create-only
	config = requestJSON(t, client, http.MethodPut, url, map[string]any{
		"mode": "dynamic", "directToolIds": []string{}, "expectedProfileRevision": selectionRevision, "expectedRevision": 0,
	}, http.StatusOK)
	if config["revision"] != float64(1) || config["profileRevision"] != selectionRevision {
		t.Fatalf("delivery update changed selection revision: %#v", config)
	}
	for _, body := range []map[string]any{
		{"mode": "dynamic", "directToolIds": []string{}, "expectedProfileRevision": selectionRevision, "expectedRevision": 0},
		{"mode": "dynamic", "directToolIds": []string{}, "expectedProfileRevision": 0, "expectedRevision": 1},
	} {
		response := requestJSON(t, client, http.MethodPut, url, body, http.StatusConflict)
		if response["code"] != "revision_conflict" {
			t.Fatalf("revision error=%#v", response)
		}
	}
	current := requestJSON(t, client, http.MethodGet, api+"/profiles/bundle", nil, http.StatusOK)
	if current["revision"] != selectionRevision {
		t.Fatal("MCP-only writes changed Profile selection/Skill revision")
	}
	assertPureProfileProjection(t, current)
	selected = requestJSON(t, client, http.MethodPost, api+"/profiles/bundle/tools", map[string]any{"toolId": toolIDs[1]}, http.StatusOK)
	requestJSON(t, client, http.MethodPut, url, map[string]any{
		"mode": "hybrid", "directToolIds": toolIDs, "expectedProfileRevision": selectionRevision, "expectedRevision": 1,
	}, http.StatusConflict)
	config = requestJSON(t, client, http.MethodPut, url, map[string]any{
		"mode": "hybrid", "directToolIds": toolIDs, "expectedProfileRevision": selected["revision"], "expectedRevision": 1,
	}, http.StatusOK)
	requestJSON(t, client, http.MethodDelete, api+"/profiles/bundle/tools/"+toolIDs[0], nil, http.StatusOK)
	pruned := requestJSON(t, client, http.MethodGet, url, nil, http.StatusOK)
	if pruned["revision"] != config["revision"].(float64)+1 || len(pruned["directToolIds"].([]any)) != 1 || pruned["directToolIds"].([]any)[0] != toolIDs[1] {
		t.Fatalf("HTTP lost membership-revocation fence: %#v", pruned)
	}
	assertMCPConfigurationDidNotStartIO(t, runtime, runner)
}

func TestHTTPMCPDeliveryRejectsRetiredAuthorityAndMissingRequiredFields(t *testing.T) {
	t.Parallel()
	runtime, server, runner, tools := httpMCPDeliveryFixture(t)
	api := server.URL + "/api/v1"
	client := server.Client()
	requestJSON(t, client, http.MethodPost, api+"/profiles", map[string]any{"slug": "legacy", "displayName": "Legacy", "mode": "direct"}, http.StatusBadRequest)
	requestJSON(t, client, http.MethodGet, api+"/profiles/legacy", nil, http.StatusNotFound)
	requestJSON(t, client, http.MethodPost, api+"/profiles", map[string]any{"slug": "bundle", "displayName": "Bundle"}, http.StatusCreated)
	requestJSON(t, client, http.MethodPost, api+"/profiles/bundle/tools", map[string]any{"toolId": tools[0], "direct": true}, http.StatusBadRequest)
	requestJSON(t, client, http.MethodPut, api+"/profiles/bundle/tools", map[string]any{"tools": []map[string]any{{"toolId": tools[0], "direct": false}}}, http.StatusBadRequest)
	url := api + "/profiles/bundle/mcp-delivery"
	for _, body := range []map[string]any{
		{"mode": "hybrid", "directToolIds": []string{}},
		{"mode": "hybrid", "expectedProfileRevision": 0},
		{"mode": "hybrid", "directToolIds": nil, "expectedProfileRevision": 0},
		{"mode": "unknown", "directToolIds": []string{}, "expectedProfileRevision": 0},
		{"mode": "hybrid", "directToolIds": []string{}, "expectedProfileRevision": 0, "expectedRevision": -1},
		{"mode": "hybrid", "directToolIds": []string{}, "expectedProfileRevision": 0, "profileId": "prof_ffffffffffffffffffffffffffffffff"},
	} {
		requestJSON(t, client, http.MethodPut, url, body, http.StatusBadRequest)
	}
	requestJSON(t, client, http.MethodGet, url, nil, http.StatusNotFound)
	requestStatus(t, client, http.MethodPut, api+"/profiles/bundle/tunnel", []byte(`{"tunnelId":""}`), http.StatusNotFound)
	assertMCPConfigurationDidNotStartIO(t, runtime, runner)
}

func TestHTTPMCPDeliveryTunnelBindingDoesNotStartAndUnconfiguredProfilesDoNotBlockDeletion(t *testing.T) {
	t.Parallel()
	runtime, server, runner, _ := httpMCPDeliveryFixture(t)
	api := server.URL + "/api/v1"
	client := server.Client()
	for _, slug := range []string{"bound", "not-configured"} {
		requestJSON(t, client, http.MethodPost, api+"/profiles", map[string]any{"slug": slug, "displayName": slug}, http.StatusCreated)
	}
	credential := requestJSON(t, client, http.MethodPost, api+"/credentials/environment", map[string]any{
		"namespace": "openai", "name": "delivery", "displayName": "Delivery", "environmentVariable": "MENDER_UNUSED_MCP_DELIVERY_KEY",
	}, http.StatusCreated)
	created := requestJSON(t, client, http.MethodPost, api+"/tunnels/openai", map[string]any{
		"displayName": "Configured only", "externalTunnelId": "delivery-only", "runtimeCredentialRef": credential["reference"],
	}, http.StatusCreated)
	url := api + "/profiles/bound/mcp-delivery"
	configured := requestJSON(t, client, http.MethodPut, url, map[string]any{
		"mode": "dynamic", "directToolIds": []string{}, "tunnelId": created["tunnelId"], "expectedProfileRevision": 0,
	}, http.StatusOK)
	if configured["tunnelId"] != created["tunnelId"] || configured["profileRevision"] != float64(0) {
		t.Fatalf("Tunnel projection=%#v", configured)
	}
	assertMCPConfigurationDidNotStartIO(t, runtime, runner)
	requestJSON(t, client, http.MethodDelete, api+"/tunnels/"+created["tunnelId"].(string), nil, http.StatusConflict)
	cleared := requestJSON(t, client, http.MethodPut, url, map[string]any{
		"mode": "dynamic", "directToolIds": []string{}, "tunnelId": "", "expectedProfileRevision": 0, "expectedRevision": configured["revision"],
	}, http.StatusOK)
	if _, exists := cleared["tunnelId"]; exists {
		t.Fatalf("cleared binding persisted: %#v", cleared)
	}
	requestNoContent(t, client, http.MethodDelete, api+"/tunnels/"+created["tunnelId"].(string), http.StatusNoContent)
	requestJSON(t, client, http.MethodGet, api+"/profiles/not-configured/mcp-delivery", nil, http.StatusNotFound)
	assertMCPConfigurationDidNotStartIO(t, runtime, runner)
}

func httpMCPDeliveryFixture(t *testing.T) (*bootstrap.Runtime, *httptest.Server, *inertDeliveryTunnelRunner, []string) {
	t.Helper()
	ctx := context.Background()
	runner := &inertDeliveryTunnelRunner{}
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "delivery.db"), TunnelProcessRunner: runner})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	provider, err := runtime.Catalog.RegisterProvider.Execute(ctx, catalogapp.RegisterProviderRequest{Namespace: "delivery-tools", DisplayName: "Delivery metadata"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Catalog.SyncProviderTools(deliveryHTTPToolSource{}).Execute(ctx, provider.ID()); err != nil {
		t.Fatal(err)
	}
	selected, err := runtime.Catalog.BrowseTools.Execute(ctx, catalogapp.BrowseToolsRequest{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(selected))
	for _, tool := range selected {
		ids = append(ids, tool.ToolID.String())
	}
	if len(ids) != 2 {
		t.Fatalf("Tool metadata fixture=%d", len(ids))
	}
	server := httptest.NewServer(New(runtime))
	t.Cleanup(server.Close)
	return runtime, server, runner, ids
}

func assertPureProfileProjection(t *testing.T, p map[string]any) {
	t.Helper()
	for _, field := range []string{"mode", "tunnelId", "directToolIds"} {
		if _, exists := p[field]; exists {
			t.Fatalf("Profile retained MCP authority %q: %#v", field, p)
		}
	}
	for _, value := range p["tools"].([]any) {
		if _, exists := value.(map[string]any)["direct"]; exists {
			t.Fatalf("Profile binding retained direct authority: %#v", value)
		}
	}
}

func assertMCPConfigurationDidNotStartIO(t *testing.T, runtime *bootstrap.Runtime, runner *inertDeliveryTunnelRunner) {
	t.Helper()
	if len(runtime.ListMCPServers()) != 0 || runner.preflights.Load() != 0 || runner.starts.Load() != 0 {
		t.Fatalf("configuration activated delivery: servers=%d preflight=%d starts=%d", len(runtime.ListMCPServers()), runner.preflights.Load(), runner.starts.Load())
	}
}

type deliveryHTTPToolSource struct{}

func (deliveryHTTPToolSource) DiscoverTools(context.Context, catalog.Provider) ([]catalogapp.DiscoveredTool, error) {
	return []catalogapp.DiscoveredTool{{SourceKey: "one", Name: "one", InputSchema: []byte(`{"type":"object"}`)}, {SourceKey: "two", Name: "two", InputSchema: []byte(`{"type":"object"}`)}}, nil
}

type inertDeliveryTunnelRunner struct{ preflights, starts atomic.Int32 }

func (r *inertDeliveryTunnelRunner) Preflight(context.Context, tunnel.Tunnel) error {
	r.preflights.Add(1)
	return errors.New("test must not preflight Tunnel")
}
func (r *inertDeliveryTunnelRunner) Start(context.Context, tunnel.Tunnel, string, []byte) (tunnelapp.Process, error) {
	r.starts.Add(1)
	return nil, errors.New("test must not start Tunnel")
}
