package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	desktopbinding "github.com/orz-i/Mender/backend/internal/desktop/binding"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
)

// Both transports keep their established wire shape while sharing source
// attribution, availability and safe configuration from the same inert query.
func TestManagementProviderViewsPreserveTransportContracts(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "provider-views.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	caller, err := governancedomain.NewContext(governancedomain.OriginDesktop, runtime.OwnerPrincipal(), governancedomain.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	var connections atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		connections.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer upstream.Close()
	rawConfig, err := json.Marshal(map[string]any{"transport": map[string]any{
		"kind": "streamable-http", "endpoint": upstream.URL,
	}})
	if err != nil {
		t.Fatal(err)
	}
	standalone, err := runtime.RegisterConfiguredProvider(ctx, catalogapplication.RegisterProviderRequest{
		Namespace: "independent", DisplayName: "Independent",
	}, "io.mender.provider.mcp", rawConfig, caller)
	if err != nil {
		t.Fatal(err)
	}
	unconfigured, err := runtime.Catalog.RegisterProvider.Execute(ctx, catalogapplication.RegisterProviderRequest{
		Namespace: "unconfigured", DisplayName: "Unconfigured",
	})
	if err != nil {
		t.Fatal(err)
	}
	sandbox, err := runtime.Sandboxes.CreateLocalSandbox(ctx, bootstrap.CreateLocalSandboxRequest{
		Name: "Workspace Owner", Root: t.TempDir(), Isolation: "host",
	}, caller)
	if err != nil {
		t.Fatal(err)
	}
	suite, err := runtime.EnsureSandboxWorkspaceTools(ctx, sandbox.ID(), caller)
	if err != nil || suite.Status != "ready" {
		t.Fatalf("workspace tools = %+v, %v", suite, err)
	}
	service := desktopbinding.NewService(ctx, runtime)
	readInventory := func() (map[string]map[string]any, map[string]map[string]any) {
		t.Helper()
		response := httptest.NewRecorder()
		New(runtime).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/providers", nil))
		if response.Code != http.StatusOK {
			t.Fatalf("HTTP inventory status %d: %s", response.Code, response.Body.String())
		}
		var httpResult struct {
			Providers []map[string]any `json:"providers"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &httpResult); err != nil {
			t.Fatal(err)
		}
		desktopResult, err := service.ListProviders()
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(desktopResult)
		if err != nil {
			t.Fatal(err)
		}
		var desktopJSON struct {
			Providers []map[string]any `json:"providers"`
		}
		if err := json.Unmarshal(encoded, &desktopJSON); err != nil {
			t.Fatal(err)
		}
		index := func(items []map[string]any) map[string]map[string]any {
			result := make(map[string]map[string]any, len(items))
			for _, item := range items {
				id, ok := item["providerId"].(string)
				if !ok {
					t.Fatalf("missing Provider identity: %#v", item)
				}
				result[id] = item
			}
			return result
		}
		return index(httpResult.Providers), index(desktopJSON.Providers)
	}
	httpItems, desktopItems := readInventory()
	for _, items := range []map[string]map[string]any{httpItems, desktopItems} {
		if len(items) != 3 {
			t.Fatalf("inventory size = %d, want 3", len(items))
		}
		if item := items[standalone.ID().String()]; item["toolSourceKind"] != "provider" || item["toolSourceSelectable"] != true || item["toolSourceId"] != standalone.ID().String() {
			t.Fatalf("standalone source = %#v", item)
		}
		if item := items[unconfigured.ID().String()]; item["toolSourceKind"] != "unclaimed" || item["toolSourceSelectable"] != false {
			t.Fatalf("unconfigured source = %#v", item)
		}
		if item := items[suite.ProviderID]; item["toolSourceKind"] != "sandbox" || item["toolSourceId"] != sandbox.ID().String() || item["toolSourceName"] != "Workspace Owner" || item["toolSourceSelectable"] != true {
			t.Fatalf("workspace source = %#v", item)
		}
	}
	for id, httpItem := range httpItems {
		for _, field := range []string{"providerId", "namespace", "displayName", "description", "createdAt", "updatedAt", "transport", "toolCount", "config", "pluginVersion", "toolSourceKind", "toolSourceId", "toolSourceName", "toolSourceSelectable"} {
			if !reflect.DeepEqual(httpItem[field], desktopItems[id][field]) {
				t.Fatalf("%s field %s differs: HTTP %#v, desktop %#v", id, field, httpItem[field], desktopItems[id][field])
			}
		}
	}
	// HTTP includes empty MCP collections; the Wails DTO continues to omit
	// them. Do not accidentally equate the two transport representations.
	if headers, ok := httpItems[standalone.ID().String()]["headers"].(map[string]any); !ok || len(headers) != 0 {
		t.Fatalf("HTTP empty headers changed: %#v", httpItems[standalone.ID().String()])
	}
	if headers, ok := httpItems[standalone.ID().String()]["credentialHeaders"].([]any); !ok || len(headers) != 0 {
		t.Fatalf("HTTP empty credentialHeaders changed: %#v", httpItems[standalone.ID().String()])
	}
	if _, present := desktopItems[standalone.ID().String()]["headers"]; present {
		t.Fatal("desktop empty headers must retain omitempty")
	}
	if _, err := runtime.Sandboxes.ChangeLocalLifecycle(ctx, sandbox.ID(), governancedomain.ActionSandboxStop, caller); err != nil {
		t.Fatal(err)
	}
	httpItems, desktopItems = readInventory()
	if httpItems[suite.ProviderID]["toolSourceSelectable"] != false || desktopItems[suite.ProviderID]["toolSourceSelectable"] != false {
		t.Fatal("stopped Workspace source remained selectable")
	}
	if connections.Load() != 0 {
		t.Fatalf("inert Provider inventory contacted the upstream %d times", connections.Load())
	}
}
