package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestSchemaNormalizerBuildsReachableDefsAndPreservesCycles(t *testing.T) {
	t.Parallel()

	parsed := loadSchemaFixture(t, `openapi: 3.1.2
info:
  title: Demo
  version: "1"
paths: {}
components:
  schemas:
    User:
      type: object
      required: [name, manager]
      properties:
        name:
          type: string
        manager:
          $ref: '#/components/schemas/User'
    Unused:
      type: object
      properties:
        ignored:
          type: string
`)

	ref := &openapi3.SchemaRef{
		Ref:   "#/components/schemas/User",
		Value: parsed.document.Components.Schemas["User"].Value,
	}
	normalizer := newSchemaNormalizer(
		parsed.document,
		parsed.sourceVersion,
		schemaRequest,
	)
	body, err := normalizer.normalizeRef(ref)
	if err != nil {
		t.Fatalf("normalizeRef() error = %v", err)
	}

	root := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"body": body,
		},
	}
	normalizer.attachDefs(root)

	properties := root["properties"].(map[string]any)
	bodySchema := properties["body"].(map[string]any)
	if got := bodySchema["$ref"]; got != "#/$defs/User" {
		t.Fatalf("body $ref = %#v, want #/$defs/User", got)
	}

	defs, ok := root["$defs"].(map[string]any)
	if !ok {
		t.Fatalf("$defs = %#v", root["$defs"])
	}
	if len(defs) != 1 {
		t.Fatalf("$defs len = %d, want 1; defs=%#v", len(defs), defs)
	}
	if _, exists := defs["Unused"]; exists {
		t.Fatal("unused component was copied into Tool schema")
	}
	user, ok := defs["User"].(map[string]any)
	if !ok {
		t.Fatalf("User def = %#v", defs["User"])
	}
	userProps := user["properties"].(map[string]any)
	manager := userProps["manager"].(map[string]any)
	if got := manager["$ref"]; got != "#/$defs/User" {
		t.Fatalf("recursive manager $ref = %#v", got)
	}

	raw, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	var roundTrip map[string]any
	if err := json.Unmarshal(raw, &roundTrip); err != nil {
		t.Fatalf("normalized schema is not JSON: %v", err)
	}
}

func TestSchemaNormalizerConvertsOpenAPI30Keywords(t *testing.T) {
	t.Parallel()

	parsed := loadSchemaFixture(t, `openapi: 3.0.4
info:
  title: Demo
  version: "1"
paths: {}
components:
  schemas:
    Request:
      type: object
      required: [name, score, generated]
      properties:
        name:
          type: string
          nullable: true
        score:
          type: number
          minimum: 1
          exclusiveMinimum: true
        generated:
          type: string
          readOnly: true
`)

	normalizer := newSchemaNormalizer(
		parsed.document,
		parsed.sourceVersion,
		schemaRequest,
	)
	value, err := normalizer.normalizeRef(
		parsed.document.Components.Schemas["Request"],
	)
	if err != nil {
		t.Fatalf("normalizeRef() error = %v", err)
	}
	schema := value.(map[string]any)
	properties := schema["properties"].(map[string]any)

	name := properties["name"].(map[string]any)
	types, ok := name["type"].([]any)
	if !ok || len(types) != 2 || types[0] != "string" || types[1] != "null" {
		t.Fatalf("nullable type = %#v, want [string null]", name["type"])
	}
	if _, exists := name["nullable"]; exists {
		t.Fatal("nullable keyword remained after OAS 3.0 normalization")
	}

	score := properties["score"].(map[string]any)
	if got := score["exclusiveMinimum"]; got != float64(1) {
		t.Fatalf("exclusiveMinimum = %#v, want 1", got)
	}
	if _, exists := score["minimum"]; exists {
		t.Fatal("minimum remained alongside converted exclusiveMinimum")
	}

	required := stringSlice(schema["required"])
	if containsString(required, "generated") {
		t.Fatalf("request required = %v; readOnly property must not be required", required)
	}
	if !containsString(required, "name") || !containsString(required, "score") {
		t.Fatalf("request required = %v", required)
	}
}

func TestSchemaNormalizerDropsWriteOnlyFromResponseRequired(t *testing.T) {
	t.Parallel()

	parsed := loadSchemaFixture(t, `openapi: 3.1.2
info:
  title: Demo
  version: "1"
paths: {}
components:
  schemas:
    Response:
      type: object
      required: [id, secret]
      properties:
        id:
          type: string
        secret:
          type: string
          writeOnly: true
`)

	normalizer := newSchemaNormalizer(
		parsed.document,
		parsed.sourceVersion,
		schemaResponse,
	)
	value, err := normalizer.normalizeRef(
		parsed.document.Components.Schemas["Response"],
	)
	if err != nil {
		t.Fatalf("normalizeRef() error = %v", err)
	}
	required := stringSlice(value.(map[string]any)["required"])
	if containsString(required, "secret") || !containsString(required, "id") {
		t.Fatalf("response required = %v", required)
	}
}

func TestSchemaNormalizerRejectsUnsupportedLocalRef(t *testing.T) {
	t.Parallel()

	parsed := loadSchemaFixture(t, `openapi: 3.1.2
info:
  title: Demo
  version: "1"
paths: {}
components: {}
`)
	normalizer := newSchemaNormalizer(
		parsed.document,
		parsed.sourceVersion,
		schemaRequest,
	)
	_, err := normalizer.normalizeNode(map[string]any{
		"$ref": "#/components/parameters/Limit",
	})
	if err == nil {
		t.Fatal("normalizeNode() error = nil, want unsupported local ref")
	}
}

func loadSchemaFixture(t *testing.T, spec string) parsedDocument {
	t.Helper()

	path := filepath.Join(t.TempDir(), "openapi.yaml")
	if err := os.WriteFile(path, []byte(spec), 0o600); err != nil {
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
	return parsed
}

func stringSlice(value any) []string {
	items, _ := value.([]any)
	result := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
