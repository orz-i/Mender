package identity

import (
	"bytes"
	"testing"
)

func TestGeneratorProducesDomainValidID(t *testing.T) {
	t.Parallel()

	generator := &Generator{
		reader: bytes.NewReader(bytes.Repeat([]byte{0xcd}, randomIDBytes)),
	}
	id, err := generator.NewProfileID()
	if err != nil {
		t.Fatalf("NewProfileID() error = %v", err)
	}
	if got, want := id.String(), "prof_cdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcd"; got != want {
		t.Fatalf("ProfileID = %q, want %q", got, want)
	}
}
