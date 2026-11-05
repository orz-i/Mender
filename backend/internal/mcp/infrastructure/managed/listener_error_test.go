package managed

import (
	"errors"
	"net"
	"os"
	"testing"
)

func TestManagedMCPRotationOnlyRecognizesActualAddressInUse(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = occupied.Close() })
	duplicate, err := net.Listen("tcp", occupied.Addr().String())
	if err == nil {
		_ = duplicate.Close()
		t.Fatal("duplicate bind unexpectedly succeeded")
	}
	if !managedMCPAddressInUse(err) {
		t.Fatalf("platform address-in-use error was not recognized: %v", err)
	}
	for _, unrelated := range []error{nil, os.ErrPermission, net.ErrClosed, errors.New("address already in use")} {
		if managedMCPAddressInUse(unrelated) {
			t.Fatalf("unrelated bind failure would trigger rotation: %v", unrelated)
		}
	}
}
