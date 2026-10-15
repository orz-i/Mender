package plugin

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverOperationBindingsBuildsCanonicalInputSchema(t *testing.T) {
	t.Parallel()

	parsed := loadSchemaFixture(t, `openapi: 3.1.2
info:
  title: Demo
  version: "1"
paths:
  /users/{id}:
    parameters:
      - name: id
        in: path
        required: true
        schema:
          type: string
    post:
      operationId: updateUser
      summary: Update user
      description: Updates one user.
      tags: [users]
      parameters:
        - name: fields
          in: query
          style: form
          explode: false
          schema:
            type: array
            items:
              type: string
        - name: X-Trace
          in: header
          required: true
          schema:
            type: string
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/UserUpdate'
      responses:
        "200":
          description: ok
components:
  schemas:
    UserUpdate:
      type: object
      required: [name]
      properties:
        name:
          type: string
    Unused:
      type: object
`)

	bindings, err := discoverOperationBindings(parsed, Config{
		Source:    SourceConfig{Kind: SourceFile, Path: filepath.Join(t.TempDir(), "openapi.yaml")},
		ServerURL: "https://api.example.test",
	})
	if err != nil {
		t.Fatalf("discoverOperationBindings() error = %v", err)
	}
	if len(bindings) != 1 {
		t.Fatalf("bindings len = %d, want 1", len(bindings))
	}
	binding := bindings[0]
	if binding.sourceKey != "updateUser" || binding.name != "updateUser" {
		t.Fatalf("binding identity = sourceKey %q name %q", binding.sourceKey, binding.name)
	}
	if binding.method != "POST" || binding.path != "/users/{id}" {
		t.Fatalf("binding route = %s %s", binding.method, binding.path)
	}
	if binding.requestBody == nil ||
		binding.requestBody.MediaType != "application/json" ||
		!binding.requestBody.Required {
		t.Fatalf("request body = %+v", binding.requestBody)
	}
	if len(binding.parameters) != 3 {
		t.Fatalf("parameters = %+v", binding.parameters)
	}

	var schema map[string]any
	if err := json.Unmarshal(binding.inputSchema, &schema); err != nil {
		t.Fatalf("input schema JSON error = %v", err)
	}
	if schema["type"] != "object" || schema["additionalProperties"] != false {
		t.Fatalf("input schema root = %#v", schema)
	}
	required := stringSlice(schema["required"])
	if !containsString(required, "path") ||
		!containsString(required, "headers") ||
		!containsString(required, "body") ||
		containsString(required, "query") {
		t.Fatalf("root required = %v", required)
	}
	properties := schema["properties"].(map[string]any)

	pathGroup := properties["path"].(map[string]any)
	if !containsString(stringSlice(pathGroup["required"]), "id") {
		t.Fatalf("path required = %#v", pathGroup["required"])
	}
	queryGroup := properties["query"].(map[string]any)
	queryProps := queryGroup["properties"].(map[string]any)
	if _, ok := queryProps["fields"]; !ok {
		t.Fatalf("query properties = %#v", queryProps)
	}
	headerGroup := properties["headers"].(map[string]any)
	if !containsString(stringSlice(headerGroup["required"]), "X-Trace") {
		t.Fatalf("headers required = %#v", headerGroup["required"])
	}

	body := properties["body"].(map[string]any)
	if body["$ref"] != "#/$defs/UserUpdate" {
		t.Fatalf("body schema = %#v", body)
	}
	defs := schema["$defs"].(map[string]any)
	if len(defs) != 1 || defs["UserUpdate"] == nil {
		t.Fatalf("$defs = %#v", defs)
	}
	if _, exists := defs["Unused"]; exists {
		t.Fatal("unused component leaked into Tool schema")
	}

	if !containsString(binding.tags, "openapi") ||
		!containsString(binding.tags, "http:post") ||
		!containsString(binding.tags, "oas:users") {
		t.Fatalf("tags = %v", binding.tags)
	}
}

func TestDiscoverOperationBindingsFallbackNamesAreStableAndDistinct(t *testing.T) {
	t.Parallel()

	parsed := loadSchemaFixture(t, `openapi: 3.1.2
info:
  title: Demo
  version: "1"
paths:
  /a-b:
    get:
      responses:
        "200":
          description: ok
  /a_b:
    get:
      responses:
        "200":
          description: ok
`)

	config := Config{Source: SourceConfig{Kind: SourceFile, Path: filepath.Join(t.TempDir(), "openapi.yaml")}, ServerURL: "https://api.example.test"}
	first, err := discoverOperationBindings(parsed, config)
	if err != nil {
		t.Fatal(err)
	}
	second, err := discoverOperationBindings(parsed, config)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 2 || len(second) != 2 {
		t.Fatalf("bindings = %d / %d", len(first), len(second))
	}
	if first[0].name == first[1].name {
		t.Fatalf("fallback names collided: %q", first[0].name)
	}
	for index := range first {
		if first[index].sourceKey != second[index].sourceKey ||
			first[index].name != second[index].name {
			t.Fatalf("discovery is not deterministic: %#v != %#v", first[index], second[index])
		}
		if !strings.Contains(first[index].sourceKey, "get /") {
			t.Fatalf("fallback SourceKey = %q", first[index].sourceKey)
		}
	}
}

func TestOperationFilterCanExcludeUnsupportedMethod(t *testing.T) {
	t.Parallel()

	parsed := loadSchemaFixture(t, `openapi: 3.1.2
info:
  title: Demo
  version: "1"
paths:
  /ok:
    get:
      operationId: getOK
      responses:
        "200":
          description: ok
  /health:
    head:
      operationId: headHealth
      responses:
        "200":
          description: ok
`)
	base := Config{Source: SourceConfig{Kind: SourceFile, Path: filepath.Join(t.TempDir(), "openapi.yaml")}, ServerURL: "https://api.example.test"}

	if _, err := discoverOperationBindings(parsed, base); err == nil ||
		!strings.Contains(err.Error(), "HEAD") {
		t.Fatalf("discover all error = %v, want unsupported HEAD", err)
	}

	filtered := base
	filtered.Operations.Exclude = []string{"headHealth"}
	bindings, err := discoverOperationBindings(parsed, filtered)
	if err != nil {
		t.Fatalf("filtered discovery error = %v", err)
	}
	if len(bindings) != 1 || bindings[0].sourceKey != "getOK" {
		t.Fatalf("filtered bindings = %+v", bindings)
	}

	included := base
	included.Operations.Include = []string{"getOK"}
	bindings, err = discoverOperationBindings(parsed, included)
	if err != nil {
		t.Fatalf("included discovery error = %v", err)
	}
	if len(bindings) != 1 || bindings[0].sourceKey != "getOK" {
		t.Fatalf("included bindings = %+v", bindings)
	}

	unknown := base
	unknown.Operations.Include = []string{"missing"}
	if _, err := discoverOperationBindings(parsed, unknown); err == nil {
		t.Fatal("unknown include error = nil")
	}
}

func TestDiscoverOperationRejectsUnsupportedRequestMediaType(t *testing.T) {
	t.Parallel()

	parsed := loadSchemaFixture(t, `openapi: 3.1.2
info:
  title: Demo
  version: "1"
paths:
  /upload:
    post:
      operationId: upload
      requestBody:
        content:
          multipart/form-data:
            schema:
              type: object
      responses:
        "200":
          description: ok
`)

	_, err := discoverOperationBindings(parsed, Config{
		Source:    SourceConfig{Kind: SourceFile, Path: filepath.Join(t.TempDir(), "openapi.yaml")},
		ServerURL: "https://api.example.test",
	})
	if err == nil || !strings.Contains(err.Error(), "application/json") {
		t.Fatalf("discover error = %v, want JSON media error", err)
	}
}

func TestDiscoverOperationRejectsSensitiveOrdinaryHeader(t *testing.T) {
	t.Parallel()

	parsed := loadSchemaFixture(t, `openapi: 3.1.2
info:
  title: Demo
  version: "1"
paths:
  /secret:
    get:
      operationId: secret
      parameters:
        - name: Authorization
          in: header
          required: true
          schema:
            type: string
      responses:
        "200":
          description: ok
`)

	_, err := discoverOperationBindings(parsed, Config{
		Source:    SourceConfig{Kind: SourceFile, Path: filepath.Join(t.TempDir(), "openapi.yaml")},
		ServerURL: "https://api.example.test",
	})
	if err == nil || !strings.Contains(err.Error(), "Security Scheme") {
		t.Fatalf("discover error = %v, want Security Scheme guidance", err)
	}
}

func TestSelectJSONMediaTypePrefersApplicationJSON(t *testing.T) {
	t.Parallel()

	parsed := loadSchemaFixture(t, `openapi: 3.1.2
info:
  title: Demo
  version: "1"
paths:
  /body:
    post:
      operationId: body
      requestBody:
        content:
          application/vnd.demo+json:
            schema:
              type: object
          application/json:
            schema:
              type: string
      responses:
        "200":
          description: ok
`)

	bindings, err := discoverOperationBindings(parsed, Config{
		Source:    SourceConfig{Kind: SourceFile, Path: filepath.Join(t.TempDir(), "openapi.yaml")},
		ServerURL: "https://api.example.test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if bindings[0].requestBody == nil ||
		bindings[0].requestBody.MediaType != "application/json" {
		t.Fatalf("request body = %+v", bindings[0].requestBody)
	}
}
