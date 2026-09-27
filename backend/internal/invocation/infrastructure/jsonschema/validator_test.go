package jsonschema

import (
	"context"
	"testing"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
)

func TestValidatorDraft202012ObjectConstraints(t *testing.T) {
	t.Parallel()

	validator := Validator{}
	schema := mustSchema(t, []byte(`{
		"$schema":"https://json-schema.org/draft/2020-12/schema",
		"type":"object",
		"properties":{"count":{"type":"integer"}},
		"required":["count"],
		"additionalProperties":false
	}`))

	for _, tc := range []struct {
		name string
		raw  string
		want bool
	}{
		{"valid", `{"count":2}`, true},
		{"missing required", `{}`, false},
		{"wrong type", `{"count":"2"}`, false},
		{"additional property", `{"count":2,"extra":true}`, false},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			value := mustValue(t, []byte(tc.raw))
			valid, err := validator.Validate(context.Background(), schema, value)
			if err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if valid != tc.want {
				t.Fatalf("Validate() = %v, want %v", valid, tc.want)
			}
		})
	}
}

func TestValidatorBooleanSchemas(t *testing.T) {
	t.Parallel()

	validator := Validator{}
	value := mustValue(t, []byte(`{"any":"value"}`))

	valid, err := validator.Validate(context.Background(), mustSchema(t, []byte(`true`)), value)
	if err != nil || !valid {
		t.Fatalf("true schema = valid %v err %v", valid, err)
	}
	valid, err = validator.Validate(context.Background(), mustSchema(t, []byte(`false`)), value)
	if err != nil {
		t.Fatalf("false schema error = %v", err)
	}
	if valid {
		t.Fatal("false schema accepted instance")
	}
}

func TestValidatorDoesNotApplySchemaDefaults(t *testing.T) {
	t.Parallel()

	validator := Validator{}
	schema := mustSchema(t, []byte(`{
		"type":"object",
		"properties":{"mode":{"type":"string","default":"safe"}}
	}`))
	value := mustValue(t, []byte(`{}`))

	valid, err := validator.Validate(context.Background(), schema, value)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if !valid {
		t.Fatal("Validate() rejected optional property with default")
	}
	if got := string(value.Bytes()); got != "{}" {
		t.Fatalf("validation mutated arguments: %s", got)
	}
}

func TestValidatorRejectsUnsupportedDialectAndRemoteReference(t *testing.T) {
	t.Parallel()

	validator := Validator{}
	value := mustValue(t, []byte(`{}`))

	unsupported := mustSchema(t, []byte(`{
		"$schema":"https://json-schema.org/draft/2019-09/schema",
		"type":"object"
	}`))
	if _, err := validator.Validate(context.Background(), unsupported, value); err == nil {
		t.Fatal("Validate() accepted unsupported dialect")
	}

	remote := mustSchema(t, []byte(`{"$ref":"https://example.test/schema.json"}`))
	if _, err := validator.Validate(context.Background(), remote, value); err == nil {
		t.Fatal("Validate() resolved remote reference without an explicit loader")
	}
}

func mustSchema(t *testing.T, raw []byte) catalogdomain.Schema {
	t.Helper()
	schema, err := catalogdomain.NewSchema(raw)
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

func mustValue(t *testing.T, raw []byte) invocationdomain.JSONValue {
	t.Helper()
	value, err := invocationdomain.NewJSONValue(raw)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
