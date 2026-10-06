package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/orz-i/Mender/backend/internal/bootstrap"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
)

func TestRuntimeGovernanceManagementAPI(t *testing.T) {
	t.Parallel()

	runtime, err := bootstrap.NewRuntime(
		context.Background(),
		bootstrap.RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "mender.db")},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	server := httptest.NewServer(New(runtime))
	t.Cleanup(server.Close)

	current := requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/runtime-governance", nil, http.StatusOK)
	budget, ok := current["budget"].(map[string]any)
	if !ok || budget["maxInvocations"] != float64(32) {
		t.Fatalf("runtime budget = %#v", current)
	}
	backend, ok := current["backend"].(map[string]any)
	if !ok || backend["name"] == "" {
		t.Fatalf("runtime backend = %#v", current)
	}

	updated := requestJSON(t, server.Client(), http.MethodPut, server.URL+"/api/v1/runtime-governance/budget", map[string]any{
		"maxInvocations":          12,
		"maxPrincipalInvocations": 8,
		"maxProviderInvocations":  8,
		"maxToolInvocations":      4,
		"maxProviderActivations":  4,
		"maxExecutionTimeoutMs":   120000,
		"maxArgumentBytes":        1048576,
		"maxResultBytes":          1048576,
	}, http.StatusOK)
	updatedBudget := updated["budget"].(map[string]any)
	if updatedBudget["maxInvocations"] != float64(12) {
		t.Fatalf("updated runtime budget = %#v", updated)
	}

	requestJSON(t, server.Client(), http.MethodPut, server.URL+"/api/v1/runtime-governance/budget", map[string]any{
		"maxInvocations":          0,
		"maxPrincipalInvocations": 8,
		"maxProviderInvocations":  8,
		"maxToolInvocations":      4,
		"maxProviderActivations":  4,
		"maxExecutionTimeoutMs":   120000,
		"maxArgumentBytes":        1048576,
		"maxResultBytes":          1048576,
	}, http.StatusBadRequest)

	requestJSON(t, server.Client(), http.MethodPut, server.URL+"/api/v1/runtime-governance/isolation", map[string]any{
		"workspaceShell":   "host",
		"externalProvider": "host",
		"tunnelClient":     "host",
	}, http.StatusOK)
}

func TestProviderPluginInstallationManagementRejectsServerPathInstall(t *testing.T) {
	t.Parallel()

	runtime, err := bootstrap.NewRuntime(
		context.Background(),
		bootstrap.RuntimeOptions{
			DatabasePath: filepath.Join(t.TempDir(), "mender.db"),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	server := httptest.NewServer(New(runtime))
	t.Cleanup(server.Close)

	listed := requestJSON(
		t,
		server.Client(),
		http.MethodGet,
		server.URL+"/api/v1/provider-plugin-installations",
		nil,
		http.StatusOK,
	)
	installations, ok := listed["installations"].([]any)
	if !ok || len(installations) != 0 {
		t.Fatalf("installations = %#v", listed)
	}

	request, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		server.URL+"/api/v1/provider-plugin-installations",
		strings.NewReader(`{"sourcePath":"/tmp/untrusted"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusMethodNotAllowed {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf(
			"POST /provider-plugin-installations status = %d body=%q",
			response.StatusCode,
			body,
		)
	}

	requestJSON(
		t,
		server.Client(),
		http.MethodPost,
		server.URL+"/api/v1/provider-plugin-installations/not-an-id/disable",
		nil,
		http.StatusBadRequest,
	)
}

func TestTunnelManagementAPI(t *testing.T) {
	t.Parallel()

	runtime, err := bootstrap.NewRuntime(
		context.Background(),
		bootstrap.RuntimeOptions{
			DatabasePath: filepath.Join(t.TempDir(), "mender.db"),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	server := httptest.NewServer(New(runtime))
	t.Cleanup(server.Close)

	createdCredential := requestJSON(
		t,
		server.Client(),
		http.MethodPost,
		server.URL+"/api/v1/credentials/environment",
		map[string]any{
			"namespace":           "openai",
			"name":                "tunnel-runtime",
			"displayName":         "OpenAI Tunnel Runtime",
			"environmentVariable": "OPENAI_TUNNEL_RUNTIME_KEY",
		},
		http.StatusCreated,
	)
	runtimeCredentialRef, _ := createdCredential["reference"].(string)
	if runtimeCredentialRef == "" {
		t.Fatalf("created Credential = %#v", createdCredential)
	}

	created := requestJSON(
		t,
		server.Client(),
		http.MethodPost,
		server.URL+"/api/v1/tunnels/openai",
		map[string]any{
			"displayName":          "OpenAI Work",
			"externalTunnelId":     "tunnel_external_work",
			"runtimeCredentialRef": runtimeCredentialRef,
		},
		http.StatusCreated,
	)
	tunnelID, _ := created["tunnelId"].(string)
	if tunnelID == "" ||
		created["provider"] != "openai" {
		t.Fatalf("created Tunnel = %#v", created)
	}
	if _, ok := created["secret"]; ok {
		t.Fatalf("Tunnel response leaked secret = %#v", created)
	}

	listed := requestJSON(
		t,
		server.Client(),
		http.MethodGet,
		server.URL+"/api/v1/tunnels",
		nil,
		http.StatusOK,
	)
	items, ok := listed["tunnels"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("Tunnel list = %#v", listed)
	}

	requestJSON(
		t,
		server.Client(),
		http.MethodPost,
		server.URL+"/api/v1/profiles",
		map[string]any{
			"slug":        "external",
			"displayName": "External",
			"mode":        "dynamic",
		},
		http.StatusCreated,
	)
	profile := requestJSON(
		t,
		server.Client(),
		http.MethodPut,
		server.URL+"/api/v1/profiles/external/tunnel",
		map[string]any{"tunnelId": tunnelID},
		http.StatusOK,
	)
	if profile["tunnelId"] != tunnelID {
		t.Fatalf("Profile Tunnel binding = %#v", profile)
	}

	conflict := requestJSON(
		t,
		server.Client(),
		http.MethodDelete,
		server.URL+"/api/v1/tunnels/"+tunnelID,
		nil,
		http.StatusConflict,
	)
	if conflict["code"] != "conflict" {
		t.Fatalf("delete in-use Tunnel response = %#v", conflict)
	}

	cleared := requestJSON(
		t,
		server.Client(),
		http.MethodPut,
		server.URL+"/api/v1/profiles/external/tunnel",
		map[string]any{"tunnelId": ""},
		http.StatusOK,
	)
	if _, ok := cleared["tunnelId"]; ok {
		t.Fatalf("cleared Profile still has tunnelId = %#v", cleared)
	}

	requestNoContent(
		t,
		server.Client(),
		http.MethodDelete,
		server.URL+"/api/v1/tunnels/"+tunnelID,
		http.StatusNoContent,
	)
	requestNoContent(
		t,
		server.Client(),
		http.MethodDelete,
		server.URL+"/api/v1/credentials/openai/tunnel-runtime",
		http.StatusNoContent,
	)
}

func TestManagementAPIEndToEnd(t *testing.T) {
	t.Parallel()

	var upstreamCalls atomic.Int32
	upstream := mcp.NewServer(
		&mcp.Implementation{Name: "management-api-upstream", Version: "1.0.0"},
		nil,
	)
	upstream.AddTool(
		&mcp.Tool{
			Name:         "echo",
			Title:        "Echo",
			Description:  "Echo JSON input",
			InputSchema:  json.RawMessage("{\"type\":\"object\"}"),
			OutputSchema: json.RawMessage("{\"type\":\"object\"}"),
		},
		func(_ context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			upstreamCalls.Add(1)
			var input map[string]any
			if err := json.Unmarshal(request.Params.Arguments, &input); err != nil {
				return nil, err
			}
			return &mcp.CallToolResult{StructuredContent: input}, nil
		},
	)
	upstreamHTTP := httptest.NewServer(
		mcp.NewStreamableHTTPHandler(
			func(*http.Request) *mcp.Server { return upstream },
			&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
		),
	)
	t.Cleanup(upstreamHTTP.Close)

	runtime, err := bootstrap.NewRuntime(context.Background(), bootstrap.RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "mender.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	server := httptest.NewServer(New(runtime))
	t.Cleanup(server.Close)

	status := requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/system/status", nil, http.StatusOK)
	if status["ready"] != true {
		t.Fatalf("system status = %#v", status)
	}

	created := requestJSON(
		t,
		server.Client(),
		http.MethodPost,
		server.URL+"/api/v1/providers/http",
		map[string]any{
			"namespace":   "upstream",
			"displayName": "Upstream",
			"endpoint":    upstreamHTTP.URL,
		},
		http.StatusCreated,
	)
	provider, ok := created["provider"].(map[string]any)
	if !ok {
		t.Fatalf("provider response = %#v", created)
	}
	providerID, _ := provider["providerId"].(string)
	if providerID == "" || provider["transport"] != "http" {
		t.Fatalf("provider = %#v", provider)
	}
	if provider["toolCount"] != float64(1) {
		t.Fatalf("provider toolCount = %#v, want 1", provider["toolCount"])
	}

	browse := requestJSON(
		t,
		server.Client(),
		http.MethodGet,
		server.URL+"/api/v1/tools/browse?limit=100",
		nil,
		http.StatusOK,
	)
	browsedTools, ok := browse["tools"].([]any)
	if !ok || len(browsedTools) != 1 {
		t.Fatalf("browse response = %#v", browse)
	}

	updated := requestJSON(
		t,
		server.Client(),
		http.MethodPut,
		server.URL+"/api/v1/providers/"+providerID,
		map[string]any{
			"transport":   "http",
			"displayName": "Upstream edited",
			"description": "Edited provider",
			"endpoint":    upstreamHTTP.URL,
		},
		http.StatusOK,
	)
	updatedProvider, ok := updated["provider"].(map[string]any)
	if !ok {
		t.Fatalf("updated provider response = %#v", updated)
	}
	if updatedProvider["namespace"] != "upstream" ||
		updatedProvider["displayName"] != "Upstream edited" ||
		updatedProvider["description"] != "Edited provider" {
		t.Fatalf("updated provider = %#v", updatedProvider)
	}

	transportChange := requestJSON(
		t,
		server.Client(),
		http.MethodPut,
		server.URL+"/api/v1/providers/"+providerID,
		map[string]any{
			"transport":   "stdio",
			"displayName": "Upstream edited",
			"executable":  "/bin/echo",
		},
		http.StatusBadRequest,
	)
	if transportChange["code"] != "validation_error" {
		t.Fatalf("transport change error = %#v", transportChange)
	}

	search := requestJSON(
		t,
		server.Client(),
		http.MethodGet,
		server.URL+"/api/v1/tools?query=echo&limit=10",
		nil,
		http.StatusOK,
	)
	tools, ok := search["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("search response = %#v", search)
	}
	toolSummary, _ := tools[0].(map[string]any)
	toolID, _ := toolSummary["toolId"].(string)
	if toolID == "" {
		t.Fatalf("tool summary = %#v", toolSummary)
	}

	detail := requestJSON(
		t,
		server.Client(),
		http.MethodGet,
		server.URL+"/api/v1/tools/"+toolID,
		nil,
		http.StatusOK,
	)
	if detail["name"] != "echo" {
		t.Fatalf("tool detail = %#v", detail)
	}
	if _, ok := detail["inputSchema"].(map[string]any); !ok {
		t.Fatalf("tool detail inputSchema = %#v", detail["inputSchema"])
	}

	credential := requestJSON(
		t,
		server.Client(),
		http.MethodPost,
		server.URL+"/api/v1/credentials/environment",
		map[string]any{
			"namespace":           "demo",
			"name":                "work",
			"displayName":         "Demo work token",
			"environmentVariable": "MENDER_DEMO_WORK_TOKEN",
		},
		http.StatusCreated,
	)
	if credential["reference"] != "credential://demo/work" ||
		credential["source"] != "environment" {
		t.Fatalf("credential response = %#v", credential)
	}
	if _, exists := credential["secret"]; exists {
		t.Fatalf("credential response leaked secret field: %#v", credential)
	}
	credentialList := requestJSON(
		t,
		server.Client(),
		http.MethodGet,
		server.URL+"/api/v1/credentials",
		nil,
		http.StatusOK,
	)
	credentialItems, ok := credentialList["credentials"].([]any)
	if !ok || len(credentialItems) != 1 {
		t.Fatalf("credential list = %#v", credentialList)
	}

	requestJSON(
		t,
		server.Client(),
		http.MethodPost,
		server.URL+"/api/v1/profiles",
		map[string]any{
			"slug":        "default",
			"displayName": "Default",
			"mode":        "hybrid",
		},
		http.StatusCreated,
	)
	legacyProfile := requestJSON(
		t,
		server.Client(),
		http.MethodPost,
		server.URL+"/api/v1/profiles/default/tools",
		map[string]any{
			"toolId": toolID,
			"direct": true,
		},
		http.StatusOK,
	)
	legacyTools, ok := legacyProfile["tools"].([]any)
	if !ok || len(legacyTools) != 1 {
		t.Fatalf("legacy profile tools = %#v", legacyProfile)
	}
	legacyBinding, _ := legacyTools[0].(map[string]any)
	if legacyBinding["alias"] != "" ||
		legacyBinding["policyConstraint"] != "inherit" ||
		legacyBinding["concurrencyConstraint"] != "inherit" {
		t.Fatalf("legacy binding defaults = %#v", legacyBinding)
	}

	invalidBinding := requestJSON(
		t,
		server.Client(),
		http.MethodPost,
		server.URL+"/api/v1/profiles/default/tools",
		map[string]any{
			"toolId": toolID,
			"alias":  "_invalid",
		},
		http.StatusBadRequest,
	)
	if invalidBinding["code"] != "validation_error" {
		t.Fatalf("invalid binding response = %#v", invalidBinding)
	}

	replacedProfile := requestJSON(
		t,
		server.Client(),
		http.MethodPut,
		server.URL+"/api/v1/profiles/default/tools",
		map[string]any{
			"tools": []map[string]any{
				{
					"toolId":                toolID,
					"direct":                true,
					"alias":                 "echo_profile",
					"titleOverride":         "Profile Echo",
					"descriptionOverride":   "Echo through the default Profile",
					"policyConstraint":      "inherit",
					"concurrencyConstraint": "serial",
					"credentialRefs":        []string{"credential://demo/work"},
				},
			},
		},
		http.StatusOK,
	)
	replacedTools, ok := replacedProfile["tools"].([]any)
	if !ok || len(replacedTools) != 1 {
		t.Fatalf("replace profile tools = %#v", replacedProfile)
	}
	replacedBinding, _ := replacedTools[0].(map[string]any)
	if replacedBinding["alias"] != "echo_profile" ||
		replacedBinding["titleOverride"] != "Profile Echo" ||
		replacedBinding["descriptionOverride"] != "Echo through the default Profile" ||
		replacedBinding["policyConstraint"] != "inherit" ||
		replacedBinding["concurrencyConstraint"] != "serial" ||
		len(replacedBinding["credentialRefs"].([]any)) != 1 {
		t.Fatalf("replace profile binding = %#v", replacedBinding)
	}

	providerScoped := requestJSON(
		t,
		server.Client(),
		http.MethodGet,
		server.URL+"/api/v1/tools?query=echo&providerId="+providerID,
		nil,
		http.StatusOK,
	)
	if providerTools, ok := providerScoped["tools"].([]any); !ok || len(providerTools) != 1 {
		t.Fatalf("provider search = %#v", providerScoped)
	}

	scoped := requestJSON(
		t,
		server.Client(),
		http.MethodGet,
		server.URL+"/api/v1/tools?query=echo&profile=default",
		nil,
		http.StatusOK,
	)
	if scopedTools, ok := scoped["tools"].([]any); !ok || len(scopedTools) != 1 {
		t.Fatalf("profile search = %#v", scoped)
	}

	startedServer := requestJSON(
		t,
		server.Client(),
		http.MethodPost,
		server.URL+"/api/v1/mcp/servers",
		map[string]any{
			"profile": "default",
			"address": "127.0.0.1:0",
		},
		http.StatusCreated,
	)
	serverID, _ := startedServer["serverId"].(string)
	serverEndpoint, _ := startedServer["endpoint"].(string)
	if serverID == "" || serverEndpoint == "" {
		t.Fatalf("started MCP server = %#v", startedServer)
	}
	assertHTTPMCPServerTool(t, serverEndpoint, "upstream__echo_profile")

	serverList := requestJSON(
		t,
		server.Client(),
		http.MethodGet,
		server.URL+"/api/v1/mcp/servers",
		nil,
		http.StatusOK,
	)
	runningServers, ok := serverList["servers"].([]any)
	if !ok || len(runningServers) != 1 {
		t.Fatalf("MCP server list = %#v", serverList)
	}

	restartedServer := requestJSON(
		t,
		server.Client(),
		http.MethodPost,
		server.URL+"/api/v1/mcp/servers/"+serverID+"/restart",
		nil,
		http.StatusOK,
	)
	restartedID, _ := restartedServer["serverId"].(string)
	restartedEndpoint, _ := restartedServer["endpoint"].(string)
	if restartedID == "" || restartedID == serverID {
		t.Fatalf("restarted MCP server = %#v", restartedServer)
	}
	assertHTTPMCPServerTool(t, restartedEndpoint, "upstream__echo_profile")

	requestNoContent(
		t,
		server.Client(),
		http.MethodDelete,
		server.URL+"/api/v1/mcp/servers/"+restartedID,
		http.StatusNoContent,
	)

	called := requestJSON(
		t,
		server.Client(),
		http.MethodPost,
		server.URL+"/api/v1/tools/"+toolID+"/call",
		map[string]any{
			"arguments": map[string]any{"value": "web"},
		},
		http.StatusOK,
	)
	if called["kind"] != "complete" {
		t.Fatalf("tool call = %#v", called)
	}
	content, _ := called["content"].(map[string]any)
	if content["value"] != "web" {
		t.Fatalf("tool call content = %#v", content)
	}
	if upstreamCalls.Load() != 1 {
		t.Fatalf("upstream call count = %d", upstreamCalls.Load())
	}

	missingCall := requestJSON(
		t,
		server.Client(),
		http.MethodPost,
		server.URL+"/api/v1/tools/tool_ffffffffffffffffffffffffffffffff/call",
		map[string]any{"arguments": map[string]any{}},
		http.StatusOK,
	)
	if missingCall["kind"] != "rejected" || missingCall["code"] != "tool_not_found" {
		t.Fatalf("missing tool call = %#v", missingCall)
	}
	if upstreamCalls.Load() != 1 {
		t.Fatalf("missing Tool unexpectedly reached upstream; calls = %d", upstreamCalls.Load())
	}

	audit := requestJSON(
		t,
		server.Client(),
		http.MethodGet,
		server.URL+"/api/v1/audit?limit=10",
		nil,
		http.StatusOK,
	)
	records, ok := audit["records"].([]any)
	if !ok || len(records) != 2 {
		t.Fatalf("audit response = %#v", audit)
	}
	record, _ := records[0].(map[string]any)
	if record["origin"] != "management_http" ||
		record["scopeKind"] != "catalog" ||
		record["outcomeKind"] != "rejected" {
		t.Fatalf("audit governance metadata = %#v", record)
	}
	for _, forbidden := range []string{"arguments", "result", "content", "secret", "credential", "progress"} {
		if _, exists := record[forbidden]; exists {
			t.Fatalf("audit record contains forbidden field %q: %#v", forbidden, record)
		}
	}

	credentialConflict := requestJSON(
		t,
		server.Client(),
		http.MethodDelete,
		server.URL+"/api/v1/credentials/demo/work",
		nil,
		http.StatusConflict,
	)
	if credentialConflict["code"] != "conflict" {
		t.Fatalf("credential delete conflict = %#v", credentialConflict)
	}

	conflict := requestJSON(
		t,
		server.Client(),
		http.MethodDelete,
		server.URL+"/api/v1/providers/"+providerID,
		nil,
		http.StatusConflict,
	)
	if conflict["code"] != "conflict" {
		t.Fatalf("provider delete conflict = %#v", conflict)
	}

	requestJSON(
		t,
		server.Client(),
		http.MethodDelete,
		server.URL+"/api/v1/profiles/default/tools/"+toolID,
		nil,
		http.StatusOK,
	)
	requestNoContent(
		t,
		server.Client(),
		http.MethodDelete,
		server.URL+"/api/v1/credentials/demo/work",
		http.StatusNoContent,
	)
	requestNoContent(
		t,
		server.Client(),
		http.MethodDelete,
		server.URL+"/api/v1/providers/"+providerID,
		http.StatusNoContent,
	)

	policy := requestJSON(
		t,
		server.Client(),
		http.MethodGet,
		server.URL+"/api/v1/policy/status",
		nil,
		http.StatusOK,
	)
	if policy["editable"] != true ||
		policy["mode"] != "configured" ||
		policy["defaultDecision"] != "allow" ||
		policy["ruleCount"] != float64(0) ||
		policy["enabledRuleCount"] != float64(0) {
		t.Fatalf("policy status = %#v", policy)
	}
	credentials := requestJSON(
		t,
		server.Client(),
		http.MethodGet,
		server.URL+"/api/v1/credentials/status",
		nil,
		http.StatusOK,
	)
	if credentials["editable"] != true ||
		credentials["resolver"] != "inventory" ||
		credentials["persistentStore"] != true {
		t.Fatalf("credential status = %#v", credentials)
	}
}

func TestManagementAPIResolvesHTTPHeaderTemplatesAtConnectionTime(t *testing.T) {
	const envName = "MENDER_TEST_MCP_API_KEY"
	t.Setenv(envName, "runtime-secret")

	upstream := mcp.NewServer(
		&mcp.Implementation{Name: "management-header-upstream", Version: "1.0.0"},
		nil,
	)
	upstream.AddTool(
		&mcp.Tool{
			Name:        "secured_search",
			Description: "Secured search",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		},
		func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{StructuredContent: map[string]any{"ok": true}}, nil
		},
	)

	mcpHandler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return upstream },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
	)
	upstreamHTTP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer runtime-secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		mcpHandler.ServeHTTP(w, r)
	}))
	t.Cleanup(upstreamHTTP.Close)

	runtime, err := bootstrap.NewRuntime(context.Background(), bootstrap.RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "mender.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	server := httptest.NewServer(New(runtime))
	t.Cleanup(server.Close)

	created := requestJSON(
		t,
		server.Client(),
		http.MethodPost,
		server.URL+"/api/v1/providers/http",
		map[string]any{
			"namespace":   "secured",
			"displayName": "Secured",
			"endpoint":    upstreamHTTP.URL,
			"headers": map[string]any{
				"Authorization": "Bearer ${" + envName + "}",
			},
		},
		http.StatusCreated,
	)

	provider, ok := created["provider"].(map[string]any)
	if !ok {
		t.Fatalf("provider response = %#v", created)
	}
	if provider["toolCount"] != float64(1) {
		t.Fatalf("provider toolCount = %#v, want 1", provider["toolCount"])
	}
	headers, ok := provider["headers"].(map[string]any)
	if !ok {
		t.Fatalf("provider headers = %#v", provider["headers"])
	}
	if headers["Authorization"] != "Bearer ${"+envName+"}" {
		t.Fatalf("persisted header template = %#v", headers["Authorization"])
	}
	if strings.Contains(recorderJSON(created), "runtime-secret") {
		t.Fatal("management response leaked resolved header secret")
	}
}

func recorderJSON(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func TestManagementAPICredentialBackedProviderAuth(t *testing.T) {
	const envName = "MENDER_TEST_INVENTORY_API_KEY"
	t.Setenv(envName, "inventory-secret")

	upstream := mcp.NewServer(
		&mcp.Implementation{Name: "management-credential-upstream", Version: "1.0.0"},
		nil,
	)
	upstream.AddTool(
		&mcp.Tool{
			Name:        "secured_search",
			Description: "Secured search",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		},
		func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{StructuredContent: map[string]any{"ok": true}}, nil
		},
	)

	mcpHandler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return upstream },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
	)
	upstreamHTTP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer inventory-secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		mcpHandler.ServeHTTP(w, r)
	}))
	t.Cleanup(upstreamHTTP.Close)

	runtime, err := bootstrap.NewRuntime(context.Background(), bootstrap.RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "mender.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	server := httptest.NewServer(New(runtime))
	t.Cleanup(server.Close)

	credential := requestJSON(
		t,
		server.Client(),
		http.MethodPost,
		server.URL+"/api/v1/credentials/environment",
		map[string]any{
			"namespace":           "secured",
			"name":                "work",
			"displayName":         "Secured work",
			"environmentVariable": envName,
		},
		http.StatusCreated,
	)
	if credential["reference"] != "credential://secured/work" {
		t.Fatalf("Credential response = %#v", credential)
	}

	// #nosec G101 -- these are non-secret test binding metadata values.
	credentialHeaders := []map[string]any{
		{
			"name":          "Authorization",
			"scheme":        "Bearer",
			"credentialRef": "credential://secured/work",
		},
	}
	created := requestJSON(
		t,
		server.Client(),
		http.MethodPost,
		server.URL+"/api/v1/providers/http",
		map[string]any{
			"namespace":         "secured",
			"displayName":       "Secured",
			"endpoint":          upstreamHTTP.URL,
			"credentialHeaders": credentialHeaders,
		},
		http.StatusCreated,
	)
	provider, ok := created["provider"].(map[string]any)
	if !ok {
		t.Fatalf("provider response = %#v", created)
	}
	authBindings, ok := provider["credentialHeaders"].([]any)
	if !ok || len(authBindings) != 1 {
		t.Fatalf("provider credentialHeaders = %#v", provider["credentialHeaders"])
	}
	auth, _ := authBindings[0].(map[string]any)
	if auth["credentialRef"] != "credential://secured/work" ||
		auth["name"] != "Authorization" ||
		auth["scheme"] != "Bearer" {
		t.Fatalf("provider auth binding = %#v", auth)
	}
	if _, exists := provider["secret"]; exists {
		t.Fatalf("provider response leaked secret field: %#v", provider)
	}
	if provider["toolCount"] != float64(1) {
		t.Fatalf("provider toolCount = %#v, want 1", provider["toolCount"])
	}
}

func TestManagementAPIPolicyGovernanceContract(t *testing.T) {
	t.Parallel()

	runtime, err := bootstrap.NewRuntime(context.Background(), bootstrap.RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "mender.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	server := httptest.NewServer(New(runtime))
	t.Cleanup(server.Close)

	policy := requestJSON(
		t,
		server.Client(),
		http.MethodGet,
		server.URL+"/api/v1/policy",
		nil,
		http.StatusOK,
	)
	if policy["defaultDecision"] != "allow" {
		t.Fatalf("fresh Policy = %#v", policy)
	}

	updated := requestJSON(
		t,
		server.Client(),
		http.MethodPut,
		server.URL+"/api/v1/policy/default",
		map[string]any{"decision": "deny"},
		http.StatusOK,
	)
	if updated["defaultDecision"] != "deny" {
		t.Fatalf("updated Policy = %#v", updated)
	}

	rule := requestJSON(
		t,
		server.Client(),
		http.MethodPost,
		server.URL+"/api/v1/policy/rules",
		map[string]any{
			"displayName": "Allow internal Catalog",
			"enabled":     true,
			"decision":    "allow",
			"match": map[string]any{
				"actions":        []string{"tool.invoke"},
				"origins":        []string{"internal"},
				"principalKinds": []string{},
				"principalRefs":  []string{},
				"scopeKinds":     []string{"catalog"},
				"profileRefs":    []string{},
				"providerIds":    []string{},
				"toolIds":        []string{},
				"factKnowledge":  "any",
				"effectsAny":     []string{},
				"resourcesAny":   []map[string]any{},
			},
		},
		http.StatusCreated,
	)
	ruleID, ok := rule["ruleId"].(string)
	if !ok || !strings.HasPrefix(ruleID, "rule_") {
		t.Fatalf("created Rule = %#v", rule)
	}
	if rule["decision"] != "allow" || rule["displayName"] != "Allow internal Catalog" {
		t.Fatalf("created Rule = %#v", rule)
	}
	if _, exists := rule["priority"]; exists {
		t.Fatalf("Rule response unexpectedly exposes priority: %#v", rule)
	}

	status := requestJSON(
		t,
		server.Client(),
		http.MethodGet,
		server.URL+"/api/v1/policy/status",
		nil,
		http.StatusOK,
	)
	if status["defaultDecision"] != "deny" ||
		status["ruleCount"] != float64(1) ||
		status["enabledRuleCount"] != float64(1) {
		t.Fatalf("Policy status = %#v", status)
	}

	disabled := requestJSON(
		t,
		server.Client(),
		http.MethodPost,
		server.URL+"/api/v1/policy/rules/"+ruleID+"/disable",
		nil,
		http.StatusOK,
	)
	if disabled["enabled"] != false {
		t.Fatalf("disabled Rule = %#v", disabled)
	}

	status = requestJSON(
		t,
		server.Client(),
		http.MethodGet,
		server.URL+"/api/v1/policy/status",
		nil,
		http.StatusOK,
	)
	if status["enabledRuleCount"] != float64(0) {
		t.Fatalf("disabled Policy status = %#v", status)
	}

	invalid := requestJSON(
		t,
		server.Client(),
		http.MethodPost,
		server.URL+"/api/v1/policy/rules",
		map[string]any{
			"ruleId":      "rule_00000000000000000000000000000001",
			"displayName": "Client identity must be rejected",
			"enabled":     true,
			"decision":    "deny",
			"match": map[string]any{
				"actions":        []string{"tool.invoke"},
				"origins":        []string{},
				"principalKinds": []string{},
				"principalRefs":  []string{},
				"scopeKinds":     []string{},
				"profileRefs":    []string{},
				"providerIds":    []string{},
				"toolIds":        []string{},
				"factKnowledge":  "any",
				"effectsAny":     []string{},
				"resourcesAny":   []map[string]any{},
			},
		},
		http.StatusBadRequest,
	)
	if invalid["code"] != "validation_error" {
		t.Fatalf("client-supplied RuleID response = %#v", invalid)
	}

	requestNoContent(
		t,
		server.Client(),
		http.MethodDelete,
		server.URL+"/api/v1/policy/rules/"+ruleID,
		http.StatusNoContent,
	)
}

func TestManagementAPIRejectsNonLocalRequestProvenance(t *testing.T) {
	t.Parallel()

	handler := New(nil)
	tests := []struct {
		name      string
		host      string
		origin    string
		fetchSite string
	}{
		{
			name: "non-loopback Host",
			host: "attacker.example",
		},
		{
			name:   "non-loopback Origin",
			host:   "127.0.0.1:8080",
			origin: "https://attacker.example",
		},
		{
			name:   "opaque Origin",
			host:   "localhost:8080",
			origin: "null",
		},
		{
			name:      "cross-site fetch metadata",
			host:      "[::1]:8080",
			origin:    "http://localhost:5173",
			fetchSite: "cross-site",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(
				http.MethodGet,
				"http://127.0.0.1/api/v1/system/status",
				nil,
			)
			request.Host = tc.host
			if tc.origin != "" {
				request.Header.Set("Origin", tc.origin)
			}
			if tc.fetchSite != "" {
				request.Header.Set("Sec-Fetch-Site", tc.fetchSite)
			}

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
			}
			var response apiError
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if response.Code != "request_forbidden" {
				t.Fatalf("error code = %q, want request_forbidden", response.Code)
			}
			if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
				t.Fatalf("Cache-Control = %q, want no-store", got)
			}
			if got := recorder.Header().Get("X-Content-Type-Options"); got != "nosniff" {
				t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
			}
		})
	}
}

func TestManagementAPIAcceptsLocalBrowserOrigin(t *testing.T) {
	t.Parallel()

	runtime, err := bootstrap.NewRuntime(context.Background(), bootstrap.RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "mender.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	request := httptest.NewRequest(
		http.MethodGet,
		"http://127.0.0.1/api/v1/system/status",
		nil,
	)
	request.Host = "127.0.0.1:8080"
	request.Header.Set("Origin", "http://localhost:5173")
	request.Header.Set("Sec-Fetch-Site", "same-site")

	recorder := httptest.NewRecorder()
	New(runtime).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.Bytes())
	}
}

func TestManagementAPIErrorShape(t *testing.T) {
	t.Parallel()

	runtime, err := bootstrap.NewRuntime(context.Background(), bootstrap.RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "mender.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	server := httptest.NewServer(New(runtime))
	t.Cleanup(server.Close)

	missingID := "tool_00000000000000000000000000000099"
	response := requestJSON(
		t,
		server.Client(),
		http.MethodGet,
		server.URL+"/api/v1/tools/"+missingID,
		nil,
		http.StatusNotFound,
	)
	if response["code"] != "not_found" {
		t.Fatalf("error response = %#v", response)
	}
	if message, _ := response["message"].(string); strings.TrimSpace(message) == "" {
		t.Fatalf("error message = %#v", response["message"])
	}

	invalidBind := requestJSON(
		t,
		server.Client(),
		http.MethodPost,
		server.URL+"/api/v1/mcp/servers",
		map[string]any{
			"profile": "default",
			"address": "0.0.0.0:8765",
		},
		http.StatusBadRequest,
	)
	if invalidBind["code"] != "validation_error" {
		t.Fatalf("invalid bind response = %#v", invalidBind)
	}

	missingServer := requestJSON(
		t,
		server.Client(),
		http.MethodDelete,
		server.URL+"/api/v1/mcp/servers/srv_missing",
		nil,
		http.StatusNotFound,
	)
	if missingServer["code"] != "not_found" {
		t.Fatalf("missing server response = %#v", missingServer)
	}
}

func assertHTTPMCPServerTool(
	t *testing.T,
	endpoint string,
	expectedName string,
) {
	t.Helper()

	client := mcp.NewClient(
		&mcp.Implementation{Name: "management-api-mcp-client", Version: "1.0.0"},
		nil,
	)
	session, err := client.Connect(
		context.Background(),
		&mcp.StreamableClientTransport{
			Endpoint:             endpoint,
			DisableStandaloneSSE: true,
		},
		nil,
	)
	if err != nil {
		t.Fatalf("MCP client.Connect(%q) error = %v", endpoint, err)
	}
	defer func() { _ = session.Close() }()

	listed, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("MCP ListTools() error = %v", err)
	}
	found := false
	for _, tool := range listed.Tools {
		if tool.Name == expectedName {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("MCP tools = %#v, missing %q", listed.Tools, expectedName)
	}
}

func requestJSON(
	t *testing.T,
	client *http.Client,
	method string,
	url string,
	body any,
	wantStatus int,
) map[string]any {
	t.Helper()

	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	request, err := http.NewRequestWithContext(context.Background(), method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()

	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != wantStatus {
		t.Fatalf(
			"%s %s status = %d, want %d; body=%s",
			method,
			url,
			response.StatusCode,
			wantStatus,
			raw,
		)
	}
	if len(raw) == 0 {
		return map[string]any{}
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("decode response %s: %v", raw, err)
	}
	return value
}

func requestNoContent(
	t *testing.T,
	client *http.Client,
	method string,
	url string,
	wantStatus int,
) {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(), method, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != wantStatus {
		raw, _ := io.ReadAll(response.Body)
		t.Fatalf("status = %d, want %d body=%s", response.StatusCode, wantStatus, raw)
	}
}

func TestManagementAPICreatesModelToolsProviderThroughGenericEndpoint(t *testing.T) {
	t.Parallel()

	credentialRef, err := credentialdomain.NewReference("model", "api")
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := bootstrap.NewRuntime(context.Background(), bootstrap.RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "mender.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	server := httptest.NewServer(New(runtime))
	t.Cleanup(server.Close)

	created := requestJSON(
		t,
		server.Client(),
		http.MethodPost,
		server.URL+"/api/v1/providers",
		map[string]any{
			"namespace":   "model-search",
			"displayName": "Model Search",
			"pluginId":    "io.mender.provider.model-tools",
			"config": map[string]any{
				"engine":        "openai",
				"model":         "gpt-test",
				"credentialRef": credentialRef.String(),
				"capabilities":  []any{"web_search"},
			},
		},
		http.StatusCreated,
	)
	provider, ok := created["provider"].(map[string]any)
	if !ok ||
		provider["pluginId"] != "io.mender.provider.model-tools" ||
		provider["toolCount"] != float64(1) {
		t.Fatalf("created Model Tools provider = %#v", created)
	}
	config, ok := provider["config"].(map[string]any)
	if !ok ||
		config["engine"] != "openai" ||
		config["model"] != "gpt-test" {
		t.Fatalf("provider config = %#v", provider["config"])
	}
}

func TestManagementAPICreatesOpenAPIProviderThroughGenericEndpoint(t *testing.T) {
	t.Parallel()

	var serverURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/openapi.yaml", func(w http.ResponseWriter, _ *http.Request) {
		spec := `openapi: 3.1.2
info:
  title: Management OpenAPI
  version: "1"
servers:
  - url: ` + serverURL + `
paths:
  /ping:
    get:
      operationId: ping
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema:
                type: object
                required: [ok]
                properties:
                  ok: { type: boolean }
`
		_, _ = w.Write([]byte(spec))
	})
	mux.HandleFunc("/ping", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	upstream := httptest.NewServer(mux)
	serverURL = upstream.URL
	t.Cleanup(upstream.Close)

	runtime, err := bootstrap.NewRuntime(context.Background(), bootstrap.RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "mender.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	server := httptest.NewServer(New(runtime))
	t.Cleanup(server.Close)

	created := requestJSON(
		t,
		server.Client(),
		http.MethodPost,
		server.URL+"/api/v1/providers",
		map[string]any{
			"namespace":   "openapi-management",
			"displayName": "OpenAPI Management",
			"pluginId":    "io.mender.provider.openapi",
			"config": map[string]any{
				"source": map[string]any{
					"kind": "url",
					"url":  upstream.URL + "/openapi.yaml",
				},
			},
		},
		http.StatusCreated,
	)
	provider, ok := created["provider"].(map[string]any)
	if !ok ||
		provider["pluginId"] != "io.mender.provider.openapi" ||
		provider["toolCount"] != float64(1) {
		t.Fatalf("created OpenAPI provider = %#v", created)
	}

	browse := requestJSON(
		t,
		server.Client(),
		http.MethodGet,
		server.URL+"/api/v1/tools/browse?limit=100",
		nil,
		http.StatusOK,
	)
	tools, ok := browse["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("browse = %#v", browse)
	}
	tool, _ := tools[0].(map[string]any)
	toolID, _ := tool["toolId"].(string)
	if toolID == "" {
		t.Fatalf("tool = %#v", tool)
	}

	result := requestJSON(
		t,
		server.Client(),
		http.MethodPost,
		server.URL+"/api/v1/tools/"+toolID+"/call",
		map[string]any{"arguments": map[string]any{}},
		http.StatusOK,
	)
	if result["kind"] != "complete" {
		t.Fatalf("call result = %#v", result)
	}
	content, ok := result["content"].(map[string]any)
	if !ok || content["ok"] != true {
		t.Fatalf("call content = %#v", result["content"])
	}
}

func TestManagementAPIGenericProviderPluginFlow(t *testing.T) {
	t.Parallel()

	upstream := mcp.NewServer(
		&mcp.Implementation{Name: "generic-provider-upstream", Version: "1.0.0"},
		nil,
	)
	upstream.AddTool(
		&mcp.Tool{
			Name:        "ping",
			Description: "Ping through generic Provider plugin API",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		},
		func(
			context.Context,
			*mcp.CallToolRequest,
		) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{
				StructuredContent: map[string]any{"ok": true},
			}, nil
		},
	)
	upstreamHTTP := httptest.NewServer(
		mcp.NewStreamableHTTPHandler(
			func(*http.Request) *mcp.Server { return upstream },
			&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
		),
	)
	t.Cleanup(upstreamHTTP.Close)

	runtime, err := bootstrap.NewRuntime(context.Background(), bootstrap.RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "mender.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	server := httptest.NewServer(New(runtime))
	t.Cleanup(server.Close)

	plugins := requestJSON(
		t,
		server.Client(),
		http.MethodGet,
		server.URL+"/api/v1/provider-plugins",
		nil,
		http.StatusOK,
	)
	items, ok := plugins["plugins"].([]any)
	if !ok || len(items) != 4 {
		t.Fatalf("provider plugins = %#v", plugins)
	}
	issues, ok := plugins["issues"].([]any)
	if !ok || len(issues) != 0 {
		t.Fatalf("provider plugin issues = %#v", plugins["issues"])
	}
	plugin, _ := items[0].(map[string]any)
	if plugin["pluginId"] != "io.mender.provider.mcp" ||
		plugin["sourceKind"] != "builtin" ||
		plugin["packageVersion"] == "" ||
		plugin["enabled"] != true {
		t.Fatalf("provider plugin = %#v", plugin)
	}
	capabilities, ok := plugin["capabilities"].([]any)
	if !ok || len(capabilities) != 0 {
		t.Fatalf("provider plugin capabilities = %#v", plugin["capabilities"])
	}
	modelToolsPlugin, _ := items[1].(map[string]any)
	if modelToolsPlugin["pluginId"] != "io.mender.provider.model-tools" {
		t.Fatalf("Model Tools provider plugin = %#v", modelToolsPlugin)
	}
	openAPIPlugin, _ := items[2].(map[string]any)
	if openAPIPlugin["pluginId"] != "io.mender.provider.openapi" {
		t.Fatalf("OpenAPI provider plugin = %#v", openAPIPlugin)
	}
	workspacePlugin, _ := items[3].(map[string]any)
	if workspacePlugin["pluginId"] != "io.mender.provider.workspace" {
		t.Fatalf("Workspace provider plugin = %#v", workspacePlugin)
	}
	if _, ok := workspacePlugin["configSchema"].(map[string]any); !ok {
		t.Fatalf("Workspace config schema = %#v", workspacePlugin["configSchema"])
	}
	workspaceCapabilities, ok := workspacePlugin["capabilities"].([]any)
	if !ok || len(workspaceCapabilities) != 4 {
		t.Fatalf("Workspace capabilities = %#v", workspacePlugin["capabilities"])
	}

	created := requestJSON(
		t,
		server.Client(),
		http.MethodPost,
		server.URL+"/api/v1/providers",
		map[string]any{
			"namespace":   "generic-upstream",
			"displayName": "Generic Upstream",
			"pluginId":    "io.mender.provider.mcp",
			"config": map[string]any{
				"transport": map[string]any{
					"kind":     "streamable-http",
					"endpoint": upstreamHTTP.URL,
				},
			},
		},
		http.StatusCreated,
	)
	provider, ok := created["provider"].(map[string]any)
	if !ok {
		t.Fatalf("provider response = %#v", created)
	}
	providerID, _ := provider["providerId"].(string)
	if providerID == "" {
		t.Fatalf("provider id missing: %#v", provider)
	}
	if provider["pluginId"] != "io.mender.provider.mcp" {
		t.Fatalf("provider pluginId = %#v", provider["pluginId"])
	}
	if provider["transport"] != "http" {
		t.Fatalf("compatibility transport = %#v, want http", provider["transport"])
	}
	if provider["toolCount"] != float64(1) {
		t.Fatalf("provider toolCount = %#v, want 1", provider["toolCount"])
	}
	config, ok := provider["config"].(map[string]any)
	if !ok {
		t.Fatalf("provider config = %#v", provider["config"])
	}
	transport, _ := config["transport"].(map[string]any)
	if transport["kind"] != "streamable-http" {
		t.Fatalf("provider config transport = %#v", transport)
	}

	updated := requestJSON(
		t,
		server.Client(),
		http.MethodPut,
		server.URL+"/api/v1/providers/"+providerID,
		map[string]any{
			"displayName": "Generic Upstream Edited",
			"description": "generic plugin update",
			"config": map[string]any{
				"transport": map[string]any{
					"kind":     "streamable-http",
					"endpoint": upstreamHTTP.URL,
				},
			},
		},
		http.StatusOK,
	)
	updatedProvider, _ := updated["provider"].(map[string]any)
	if updatedProvider["displayName"] != "Generic Upstream Edited" ||
		updatedProvider["pluginId"] != "io.mender.provider.mcp" {
		t.Fatalf("updated provider = %#v", updatedProvider)
	}

	listed := requestJSON(
		t,
		server.Client(),
		http.MethodGet,
		server.URL+"/api/v1/providers",
		nil,
		http.StatusOK,
	)
	providers, ok := listed["providers"].([]any)
	if !ok || len(providers) != 1 {
		t.Fatalf("providers response = %#v", listed)
	}
	listedProvider, _ := providers[0].(map[string]any)
	if listedProvider["pluginId"] != "io.mender.provider.mcp" {
		t.Fatalf("listed provider = %#v", listedProvider)
	}

	requestNoContent(
		t,
		server.Client(),
		http.MethodDelete,
		server.URL+"/api/v1/providers/"+providerID,
		http.StatusNoContent,
	)
}
