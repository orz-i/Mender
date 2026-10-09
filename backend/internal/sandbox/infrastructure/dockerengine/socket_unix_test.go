//go:build !windows

package dockerengine

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func localSocketFixture(t *testing.T) (EngineEndpoint, string) {
	t.Helper()
	// macOS UNIX socket names are short; creating under an ordinary home-owned
	// directory avoids /tmp parent symlinks while keeping test paths bounded.
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	directory, err := os.MkdirTemp(home, ".mender-docker-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	dir, err := filepath.EvalSymlinks(directory)
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(dir, "daemon.sock")
	address, err := net.ResolveUnixAddr("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.ListenUnix("unix", address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	endpoint, err := ParseEngineEndpoint("unix://" + socket)
	if err != nil {
		t.Fatal(err)
	}
	return endpoint, socket
}
func TestDockerSocketPreflightRequiresRealUnixSocketAndSafeComponents(t *testing.T) {
	endpoint, socket := localSocketFixture(t)
	if err := endpoint.PreflightSocket(); err != nil {
		t.Fatalf("valid UNIX socket: %v", err)
	}
	nonSocket := filepath.Join(filepath.Dir(socket), "regular-file")
	if err := os.WriteFile(nonSocket, []byte("no daemon"), 0o600); err != nil {
		t.Fatal(err)
	}
	ordinary, err := ParseEngineEndpoint("unix://" + nonSocket)
	if err != nil {
		t.Fatal(err)
	}
	if err := ordinary.PreflightSocket(); err == nil {
		t.Fatal("accepted non-socket file")
	}
	alias := filepath.Join(filepath.Dir(socket), "socket-link")
	if err := os.Symlink(socket, alias); err != nil {
		t.Skip(err)
	}
	linked, err := ParseEngineEndpoint("unix://" + alias)
	if err != nil {
		t.Fatal(err)
	}
	if err := linked.PreflightSocket(); err == nil {
		t.Fatal("accepted symlink to Docker socket")
	}
}
func TestDockerSocketRejectsWritableAncestor(t *testing.T) {
	endpoint, _ := localSocketFixture(t)
	dir := filepath.Dir(endpoint.SocketPath())
	// #nosec G302 -- test deliberately makes a private temporary directory unsafe to verify denial.
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() {
		// #nosec G302 -- restore the test-only temporary directory owner mode before cleanup.
		_ = os.Chmod(dir, 0o700)
	})
	if err := endpoint.PreflightSocket(); err == nil {
		t.Fatal("accepted world-writable socket ancestor")
	}
}
