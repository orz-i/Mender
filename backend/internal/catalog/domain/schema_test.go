package domain

import (
	"bytes"
	"testing"
)

func TestSchemaSupportsObjectAndBooleanRoots(t *testing.T) {
	t.Parallel()

	for _, raw := range [][]byte{
		[]byte(`{"type":"object","properties":{"name":{"type":"string"}}}`),
		[]byte("true"),
		[]byte("false"),
	} {
		if _, err := NewSchema(raw); err != nil {
			t.Errorf("NewSchema(%s) error = %v", raw, err)
		}
	}

	for _, raw := range [][]byte{
		nil,
		[]byte("null"),
		[]byte(`"string"`),
		[]byte(`[]`),
		[]byte(`{"broken"`),
	} {
		if _, err := NewSchema(raw); err == nil {
			t.Errorf("NewSchema(%s) expected error", raw)
		}
	}
}

func TestSchemaCopiesInputAndOutputBytes(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"type":"object"}`)
	schema, err := NewSchema(raw)
	if err != nil {
		t.Fatalf("NewSchema() error = %v", err)
	}

	raw[0] = '['
	if got := schema.Bytes(); !bytes.Equal(got, []byte(`{"type":"object"}`)) {
		t.Fatalf("Schema mutated through constructor input: %s", got)
	}

	exposed := schema.Bytes()
	exposed[0] = '['
	if got := schema.Bytes(); !bytes.Equal(got, []byte(`{"type":"object"}`)) {
		t.Fatalf("Schema mutated through Bytes(): %s", got)
	}
}
