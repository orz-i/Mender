package domain

import (
	"bytes"
	"encoding/json"
)

// Schema is an immutable JSON Schema document.
//
// JSON Schema 2020-12 allows object and boolean schemas. Protocol-specific
// dialect negotiation remains outside the Catalog domain.
type Schema struct {
	raw []byte
}

// NewSchema validates and copies a JSON Schema document.
func NewSchema(raw []byte) (Schema, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return Schema{}, validationError("schema", "must not be empty")
	}

	var value any
	if err := json.Unmarshal(trimmed, &value); err != nil {
		return Schema{}, validationError("schema", "must be valid JSON")
	}

	switch value.(type) {
	case map[string]any, bool:
	default:
		return Schema{}, validationError("schema", "root must be an object or boolean")
	}

	return Schema{raw: bytes.Clone(trimmed)}, nil
}

// Bytes returns a copy of the JSON Schema bytes.
func (s Schema) Bytes() []byte {
	return bytes.Clone(s.raw)
}

// IsZero reports whether no schema has been initialized.
func (s Schema) IsZero() bool {
	return len(s.raw) == 0
}

func cloneOptionalSchema(schema *Schema) *Schema {
	if schema == nil {
		return nil
	}

	cloned, err := NewSchema(schema.Bytes())
	if err != nil {
		panic("catalog domain: cloning validated schema: " + err.Error())
	}
	return &cloned
}
