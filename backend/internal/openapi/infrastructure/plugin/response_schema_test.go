package plugin

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildOperationOutputSchemaSupportsJSONAndNoContent(t *testing.T) {
	t.Parallel()

	parsed := loadSchemaFixture(t, `openapi: 3.1.2
info:
  title: Demo
  version: "1"
paths:
  /users:
    get:
      operationId: listUsers
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/User'
        "204":
          description: empty
components:
  schemas:
    User:
      type: object
      required: [id, secret]
      properties:
        id:
          type: string
        secret:
          type: string
          writeOnly: true
`)

	operation := parsed.document.Paths.Value("/users").Get
	raw, err := buildOperationOutputSchema(parsed, operation.Responses)
	if err != nil {
		t.Fatalf("buildOperationOutputSchema() error = %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("output schema is empty")
	}

	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	anyOf, ok := schema["anyOf"].([]any)
	if !ok || len(anyOf) != 2 {
		t.Fatalf("output schema = %#v", schema)
	}
	defs, ok := schema["$defs"].(map[string]any)
	if !ok || defs["User"] == nil {
		t.Fatalf("$defs = %#v", schema["$defs"])
	}
	user := defs["User"].(map[string]any)
	required := stringSlice(user["required"])
	if containsString(required, "secret") {
		t.Fatalf("response User required = %v; writeOnly must not remain required", required)
	}
}

func TestBuildOperationOutputSchemaRejectsNonJSONSuccess(t *testing.T) {
	t.Parallel()

	parsed := loadSchemaFixture(t, `openapi: 3.1.2
info:
  title: Demo
  version: "1"
paths:
  /report:
    get:
      operationId: report
      responses:
        "200":
          description: report
          content:
            text/csv:
              schema:
                type: string
`)

	operation := parsed.document.Paths.Value("/report").Get
	_, err := buildOperationOutputSchema(parsed, operation.Responses)
	if err == nil || !strings.Contains(err.Error(), "only JSON") {
		t.Fatalf("buildOperationOutputSchema() error = %v", err)
	}
}

func TestBuildOperationOutputSchemaOmitsUnknownJSONShape(t *testing.T) {
	t.Parallel()

	parsed := loadSchemaFixture(t, `openapi: 3.1.2
info:
  title: Demo
  version: "1"
paths:
  /unknown:
    get:
      operationId: unknown
      responses:
        "200":
          description: ok
          content:
            application/json: {}
`)

	operation := parsed.document.Paths.Value("/unknown").Get
	raw, err := buildOperationOutputSchema(parsed, operation.Responses)
	if err != nil {
		t.Fatalf("buildOperationOutputSchema() error = %v", err)
	}
	if raw != nil {
		t.Fatalf("output schema = %s, want nil", raw)
	}
}

func TestIsSuccessResponseKey(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"200", "201", "299", "2XX", "2xx"} {
		if !isSuccessResponseKey(value) {
			t.Fatalf("isSuccessResponseKey(%q) = false", value)
		}
	}
	for _, value := range []string{"199", "300", "default", "20"} {
		if isSuccessResponseKey(value) {
			t.Fatalf("isSuccessResponseKey(%q) = true", value)
		}
	}
}
