package jsonschema

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	gojsonschema "github.com/google/jsonschema-go/jsonschema"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
)

const (
	draft7HTTP     = "http://json-schema.org/draft-07/schema#"
	draft7HTTPS    = "https://json-schema.org/draft-07/schema#"
	draft2020HTTPS = "https://json-schema.org/draft/2020-12/schema"
)

// Validator evaluates canonical JSON values against canonical Catalog schemas.
// Resolve(nil) deliberately has no remote loader, so validation never fetches
// arbitrary external $ref targets.
type Validator struct{}

func (Validator) Validate(
	ctx context.Context,
	schema catalogdomain.Schema,
	value invocationdomain.JSONValue,
) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	var parsed gojsonschema.Schema
	if err := json.Unmarshal(schema.Bytes(), &parsed); err != nil {
		return false, fmt.Errorf("decode JSON Schema: %w", err)
	}
	if !supportedDialect(parsed.Schema) {
		return false, fmt.Errorf("unsupported JSON Schema dialect %q", parsed.Schema)
	}

	resolved, err := parsed.Resolve(nil)
	if err != nil {
		return false, fmt.Errorf("resolve JSON Schema: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(value.Bytes()))
	var instance any
	if err := decoder.Decode(&instance); err != nil {
		return false, fmt.Errorf("decode validated JSON value: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}

	if err := resolved.Validate(instance); err != nil {
		return false, nil
	}
	return true, nil
}

func supportedDialect(value string) bool {
	switch value {
	case "", draft7HTTP, draft7HTTPS, draft2020HTTPS:
		return true
	default:
		return false
	}
}
