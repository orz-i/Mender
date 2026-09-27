package identity

import (
	"bytes"
	"testing"
)

func TestGeneratorProducesStableRuleIDShape(t *testing.T) {
	t.Parallel()

	generator := &Generator{
		reader: bytes.NewReader([]byte{
			0, 1, 2, 3, 4, 5, 6, 7,
			8, 9, 10, 11, 12, 13, 14, 15,
		}),
	}
	id, err := generator.NewRuleID()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := id.String(), "rule_000102030405060708090a0b0c0d0e0f"; got != want {
		t.Fatalf("NewRuleID() = %q, want %q", got, want)
	}
}
