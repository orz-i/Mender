package plugin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

func TestPluginOpenDiscoverAndExecute(t *testing.T) {
	t.Parallel()

	var serverURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/openapi.yaml", func(w http.ResponseWriter, _ *http.Request) {
		spec := `openapi: 3.1.2
info:
  title: Demo
  version: "1"
servers:
  - url: ` + serverURL + `
paths:
  /echo/{id}:
    get:
      operationId: echo
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
        - name: q
          in: query
          schema:
            type: string
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema:
                type: object
                required: [id, q]
                properties:
                  id: { type: string }
                  q: { type: string }
`
		_, _ = w.Write([]byte(spec))
	})
	mux.HandleFunc("/echo/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := r.URL.Path[len("/echo/"):]
		_ = json.NewEncoder(w).Encode(map[string]string{
			"id": id,
			"q":  r.URL.Query().Get("q"),
		})
	})
	server := httptest.NewServer(mux)
	serverURL = server.URL
	t.Cleanup(server.Close)

	configRaw, err := json.Marshal(Config{
		Source: SourceConfig{
			Kind: SourceURL,
			URL:  server.URL + "/openapi.yaml",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	instanceRaw, err := New().Open(t.Context(), providerplugin.OpenRequest{
		ConfigVersion: ConfigVersion,
		Config:        configRaw,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	instance := instanceRaw.(*Instance)
	t.Cleanup(func() { _ = instance.Close() })

	tools, err := instance.DiscoverTools(t.Context(), catalogdomain.Provider{})
	if err != nil {
		t.Fatalf("DiscoverTools() error = %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("DiscoverTools() len = %d, want 1", len(tools))
	}
	if tools[0].SourceKey != "echo" ||
		tools[0].Name != "echo" ||
		len(tools[0].OutputSchema) == 0 {
		t.Fatalf("Discovered Tool = %+v", tools[0])
	}

	tool := testCatalogToolFromDiscovery(t, tools[0])
	arguments, err := invocationdomain.NewJSONValue(
		[]byte(`{"path":{"id":"abc"},"query":{"q":"hello"}}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := instance.Execute(
		t.Context(),
		invocationapplication.ExecutionRequest{
			Tool:      tool,
			Arguments: arguments,
		},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if outcome.Kind() != invocationdomain.OutcomeKindComplete {
		t.Fatalf("outcome kind = %q", outcome.Kind())
	}

	var body map[string]string
	if err := json.Unmarshal(outcome.Content().Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["id"] != "abc" || body["q"] != "hello" {
		t.Fatalf("outcome body = %#v", body)
	}
}

func TestPluginOpenRejectsUnsupportedConfigVersion(t *testing.T) {
	t.Parallel()

	_, err := New().Open(t.Context(), providerplugin.OpenRequest{
		ConfigVersion: ConfigVersion + 1,
		Config:        json.RawMessage(`{"source":{"kind":"file","path":"/tmp/openapi.yaml"}}`),
	})
	if err == nil {
		t.Fatal("Open() error = nil, want unsupported version")
	}
}

func testCatalogToolFromDiscovery(
	t *testing.T,
	discovered catalogapplication.DiscoveredTool,
) catalogdomain.Tool {
	t.Helper()

	toolID, err := catalogdomain.ParseToolID(
		"tool_00000000000000000000000000000042",
	)
	if err != nil {
		t.Fatal(err)
	}
	providerID, err := catalogdomain.ParseProviderID(
		"prov_00000000000000000000000000000042",
	)
	if err != nil {
		t.Fatal(err)
	}
	inputSchema, err := catalogdomain.NewSchema(discovered.InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	var outputSchema *catalogdomain.Schema
	if len(discovered.OutputSchema) != 0 {
		schema, schemaErr := catalogdomain.NewSchema(discovered.OutputSchema)
		if schemaErr != nil {
			t.Fatal(schemaErr)
		}
		outputSchema = &schema
	}
	tool, err := catalogdomain.NewTool(
		toolID,
		providerID,
		catalogdomain.ToolSpec{
			SourceKey:    discovered.SourceKey,
			Name:         discovered.Name,
			Title:        discovered.Title,
			Description:  discovered.Description,
			InputSchema:  inputSchema,
			OutputSchema: outputSchema,
			Tags:         discovered.Tags,
		},
		time.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}
	return tool
}
