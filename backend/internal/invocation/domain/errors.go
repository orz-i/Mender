package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type ValidationError struct {
	Field  string
	Reason string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Reason)
}

func validationError(field, reason string) error {
	return ValidationError{Field: field, Reason: reason}
}

func validateSchema(raw []byte) (any, error) {
	trimmed := bytes.TrimSpace(raw)
	var value any
	if err := json.Unmarshal(trimmed, &value); err != nil {
		return nil, validationError("input_schema", "must be valid JSON")
	}
	switch value.(type) {
	case map[string]any, bool:
		return value, nil
	default:
		return nil, validationError("input_schema", "root must be an object or boolean")
	}
}

func cloneBytes(raw []byte) []byte {
	if len(raw) == 0 {
		return nil
	}
	return bytes.Clone(raw)
}
