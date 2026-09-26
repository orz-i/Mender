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
)

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
	requestJSON(
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

	replacedProfile := requestJSON(
		t,
		server.Client(),
		http.MethodPut,
		server.URL+"/api/v1/profiles/default/tools",
		map[string]any{
			"tools": []map[string]any{
				{"toolId": toolID, "direct": true},
			},
		},
		http.StatusOK,
	)
	replacedTools, ok := replacedProfile["tools"].([]any)
	if !ok || len(replacedTools) != 1 {
		t.Fatalf("replace profile tools = %#v", replacedProfile)
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
	assertHTTPMCPServerTool(t, serverEndpoint, "upstream__echo")

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
	assertHTTPMCPServerTool(t, restartedEndpoint, "upstream__echo")

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

	audit := requestJSON(
		t,
		server.Client(),
		http.MethodGet,
		server.URL+"/api/v1/audit?limit=10",
		nil,
		http.StatusOK,
	)
	records, ok := audit["records"].([]any)
	if !ok || len(records) != 1 {
		t.Fatalf("audit response = %#v", audit)
	}
	record, _ := records[0].(map[string]any)
	for _, forbidden := range []string{"arguments", "result", "content", "secret", "credential"} {
		if _, exists := record[forbidden]; exists {
			t.Fatalf("audit record contains forbidden field %q: %#v", forbidden, record)
		}
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
	if policy["editable"] != false {
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
	if credentials["editable"] != false || credentials["resolver"] != "unavailable" {
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
