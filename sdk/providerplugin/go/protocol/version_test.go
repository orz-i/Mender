package protocol

import (
	"slices"
	"testing"
)

func TestSupportedVersionsReturnsCopy(t *testing.T) {
	t.Parallel()

	got := SupportedVersions()
	if !slices.Equal(got, []uint32{Version1}) {
		t.Fatalf("SupportedVersions() = %v", got)
	}
	got[0] = 99
	if SupportedVersions()[0] != Version1 {
		t.Fatal("SupportedVersions() exposed mutable state")
	}
}

func TestNegotiateSelectsMutualVersion(t *testing.T) {
	t.Parallel()

	got, err := Negotiate([]uint32{99, Version1, 0})
	if err != nil {
		t.Fatalf("Negotiate() error = %v", err)
	}
	if got != Version1 {
		t.Fatalf("Negotiate() = %d, want %d", got, Version1)
	}
}

func TestNegotiateRejectsIncompatiblePeer(t *testing.T) {
	t.Parallel()

	if _, err := Negotiate([]uint32{2, 3}); err == nil {
		t.Fatal("Negotiate() error = nil")
	}
}
