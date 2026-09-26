package identity

import (
	"bytes"
	"testing"
)

func TestGeneratorProducesDomainValidID(t *testing.T) {
	t.Parallel()

	generator := &Generator{
		reader: bytes.NewReader(bytes.Repeat([]byte{0xab}, randomIDBytes)),
	}
	id, err := generator.NewInvocationID()
	if err != nil {
		t.Fatalf("NewInvocationID() error = %v", err)
	}
	if got, want := id.String(), "inv_abababababababababababababababab"; got != want {
		t.Fatalf("InvocationID = %q, want %q", got, want)
	}
}
