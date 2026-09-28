package identity

import (
	"bytes"
	"testing"
)

func TestNewInstallationID(t *testing.T) {
	t.Parallel()

	generator := &Generator{
		reader: bytes.NewReader(bytes.Repeat([]byte{0xab}, randomIDBytes)),
	}
	id, err := generator.NewInstallationID()
	if err != nil {
		t.Fatalf("NewInstallationID() error = %v", err)
	}
	const want = "pinst_abababababababababababababababab"
	if id.String() != want {
		t.Fatalf("NewInstallationID() = %q, want %q", id, want)
	}
}
