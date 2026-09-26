package identity

import (
	"bytes"
	"testing"
)

func TestGeneratorProducesDomainValidIDs(t *testing.T) {
	t.Parallel()

	random := append(
		bytes.Repeat([]byte{0x01}, randomIDBytes),
		bytes.Repeat([]byte{0xab}, randomIDBytes)...,
	)
	generator := &Generator{reader: bytes.NewReader(random)}

	providerID, err := generator.NewProviderID()
	if err != nil {
		t.Fatalf("NewProviderID() error = %v", err)
	}
	if got, want := providerID.String(), "prov_01010101010101010101010101010101"; got != want {
		t.Fatalf("ProviderID = %q, want %q", got, want)
	}

	toolID, err := generator.NewToolID()
	if err != nil {
		t.Fatalf("NewToolID() error = %v", err)
	}
	if got, want := toolID.String(), "tool_abababababababababababababababab"; got != want {
		t.Fatalf("ToolID = %q, want %q", got, want)
	}
}
