package plugin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
)

func TestOpenAPILargeCatalogFilteringIsDeterministic(t *testing.T) {
	t.Parallel()

	const operationCount = 1000
	paths := make(map[string]any, operationCount)
	for index := 0; index < operationCount; index++ {
		operationID := fmt.Sprintf("operation%04d", index)
		paths[fmt.Sprintf("/items/%04d", index)] = map[string]any{
			"get": map[string]any{
				"operationId": operationID,
				"responses": map[string]any{
					"200": map[string]any{
						"description": "ok",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type": "object",
								},
							},
						},
					},
				},
			},
		}
	}
	spec := map[string]any{
		"openapi": "3.1.2",
		"info": map[string]any{
			"title":   "Large",
			"version": "1",
		},
		"paths": paths,
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "openapi.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	parsed, err := loadOpenAPIDocument(
		t.Context(),
		newSourceLoader(),
		SourceConfig{Kind: SourceFile, Path: path},
	)
	if err != nil {
		t.Fatalf("loadOpenAPIDocument() error = %v", err)
	}

	config := Config{
		Source:    SourceConfig{Kind: SourceFile, Path: path},
		ServerURL: "https://api.example.test",
		Operations: OperationFilter{
			Include: []string{"operation0001", "operation0500", "operation0999"},
		},
	}
	first, err := discoverOperationBindings(parsed, config)
	if err != nil {
		t.Fatalf("discoverOperationBindings(first) error = %v", err)
	}
	second, err := discoverOperationBindings(parsed, config)
	if err != nil {
		t.Fatalf("discoverOperationBindings(second) error = %v", err)
	}

	if len(first) != 3 || len(second) != 3 {
		t.Fatalf("filtered discovery lengths = %d / %d, want 3", len(first), len(second))
	}
	want := []string{"operation0001", "operation0500", "operation0999"}
	for index, sourceKey := range want {
		if first[index].sourceKey != sourceKey ||
			second[index].sourceKey != sourceKey ||
			first[index].name != second[index].name {
			t.Fatalf(
				"discovery[%d] first=%q/%q second=%q/%q want SourceKey %q",
				index,
				first[index].sourceKey,
				first[index].name,
				second[index].sourceKey,
				second[index].name,
				sourceKey,
			)
		}
	}
}

func TestOpenAPIJSONMethodsExecute(t *testing.T) {
	t.Parallel()

	methods := []string{
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"method": r.Method})
	}))
	t.Cleanup(server.Close)

	for _, method := range methods {
		method := method
		t.Run(method, func(t *testing.T) {
			t.Parallel()

			request, err := buildHTTPRequest(
				t.Context(),
				operationBinding{
					method:    method,
					path:      "/method",
					serverURL: server.URL,
				},
				[]byte(`{}`),
			)
			if err != nil {
				t.Fatalf("buildHTTPRequest() error = %v", err)
			}
			outcome, err := executeHTTPRequest(t.Context(), newExecutionHTTPClient(), request)
			if err != nil {
				t.Fatalf("executeHTTPRequest() error = %v", err)
			}
			if outcome.Kind() != invocationdomain.OutcomeKindComplete {
				t.Fatalf("outcome kind = %q", outcome.Kind())
			}
			var body map[string]string
			if err := json.Unmarshal(outcome.Content().Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body["method"] != method {
				t.Fatalf("response method = %q, want %q", body["method"], method)
			}
		})
	}
}

func TestOpenAPIConcreteJSONSuffixRequestAndResponse(t *testing.T) {
	t.Parallel()

	const mediaType = "application/vnd.example.resource+json"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Content-Type"); got != mediaType {
			http.Error(w, "wrong content type", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", mediaType)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(server.Close)

	request, err := buildHTTPRequest(
		t.Context(),
		operationBinding{
			method:    http.MethodPost,
			path:      "/resource",
			serverURL: server.URL,
			requestBody: &requestBodyBinding{
				MediaType: mediaType,
				Required:  true,
			},
		},
		[]byte(`{"body":{"name":"demo"}}`),
	)
	if err != nil {
		t.Fatalf("buildHTTPRequest() error = %v", err)
	}
	outcome, err := executeHTTPRequest(t.Context(), newExecutionHTTPClient(), request)
	if err != nil {
		t.Fatalf("executeHTTPRequest() error = %v", err)
	}
	if outcome.Kind() != invocationdomain.OutcomeKindComplete ||
		string(outcome.Content().Bytes()) != `{"ok":true}` {
		t.Fatalf("outcome = kind=%q content=%s", outcome.Kind(), outcome.Content().Bytes())
	}
}

func TestOpenAPIResponseSizeIsBounded(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`"`))
		_, _ = w.Write([]byte(strings.Repeat("x", maxResponseBytes)))
		_, _ = w.Write([]byte(`"`))
	}))
	t.Cleanup(server.Close)

	request, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodGet,
		server.URL,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = executeHTTPRequest(t.Context(), newExecutionHTTPClient(), request)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("executeHTTPRequest() error = %v, want size overflow", err)
	}
}

func TestOpenAPISecuritySchemesDoNotAppearInToolInputSchema(t *testing.T) {
	t.Parallel()

	ref := mustCredentialRef(t, "demo", "token")
	parsed := loadSchemaFixture(t, `openapi: 3.1.2
info:
  title: Secure
  version: "1"
components:
  securitySchemes:
    BearerAuth:
      type: http
      scheme: bearer
paths:
  /secure:
    get:
      operationId: secure
      security:
        - BearerAuth: []
      parameters:
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
`)

	bindings, err := discoverOperationBindings(parsed, Config{
		Source:    SourceConfig{Kind: SourceFile, Path: filepath.Join(t.TempDir(), "openapi.yaml")},
		ServerURL: "https://api.example.test",
		Credentials: map[string]credentialdomain.Reference{
			"BearerAuth": ref,
		},
	})
	if err != nil {
		t.Fatalf("discoverOperationBindings() error = %v", err)
	}
	if len(bindings) != 1 {
		t.Fatalf("bindings len = %d", len(bindings))
	}
	input := string(bindings[0].inputSchema)
	for _, forbidden := range []string{
		"BearerAuth",
		"Authorization",
		ref.String(),
	} {
		if strings.Contains(input, forbidden) {
			t.Fatalf("Tool input schema contains authentication material %q: %s", forbidden, input)
		}
	}
	if !strings.Contains(input, `"q"`) {
		t.Fatalf("Tool input schema lost ordinary query parameter: %s", input)
	}
}

func TestOpenAPIHTTPClientsDoNotUseAmbientProxy(t *testing.T) {
	t.Parallel()

	sourceClient := newSourceLoader().httpClient
	sourceTransport, ok := sourceClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("source transport = %T", sourceClient.Transport)
	}
	if sourceTransport.Proxy != nil {
		t.Fatal("source HTTP client has ambient proxy function")
	}

	executionClient := newExecutionHTTPClient()
	executionTransport, ok := executionClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("execution transport = %T", executionClient.Transport)
	}
	if executionTransport.Proxy != nil {
		t.Fatal("execution HTTP client has ambient proxy function")
	}
}
