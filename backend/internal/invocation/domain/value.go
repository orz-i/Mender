package domain

import (
	"bytes"
	"encoding/json"
)

// JSONValue is an immutable protocol-independent JSON value.
type JSONValue struct {
	raw []byte
}

// NewJSONValue validates and copies exactly one JSON value.
func NewJSONValue(raw []byte) (JSONValue, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return JSONValue{}, validationError("json", "must not be empty")
	}

	var value any
	if err := json.Unmarshal(trimmed, &value); err != nil {
		return JSONValue{}, validationError("json", "must contain exactly one valid JSON value")
	}

	return JSONValue{raw: bytes.Clone(trimmed)}, nil
}

// Bytes returns a defensive copy.
func (v JSONValue) Bytes() []byte {
	return bytes.Clone(v.raw)
}

// IsZero reports whether the value has not been initialized.
func (v JSONValue) IsZero() bool {
	return len(v.raw) == 0
}
