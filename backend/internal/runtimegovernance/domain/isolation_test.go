package domain

import "testing"

func TestDefaultIsolationIncludesTunnelClient(t *testing.T) {
	t.Parallel()

	hostOnly := DefaultIsolation(false)
	if hostOnly.WorkspaceShell() != IsolationHost ||
		hostOnly.ExternalProvider() != IsolationHost ||
		hostOnly.TunnelClient() != IsolationHost {
		t.Fatalf("host-only default = %#v", hostOnly.Spec())
	}

	restricted := DefaultIsolation(true)
	if restricted.WorkspaceShell() != IsolationRestrictedHost ||
		restricted.ExternalProvider() != IsolationHost ||
		restricted.TunnelClient() != IsolationRestrictedHost {
		t.Fatalf("restricted default = %#v", restricted.Spec())
	}
}

func TestIsolationRequiresTunnelClientMode(t *testing.T) {
	t.Parallel()

	_, err := NewIsolation(IsolationSpec{
		WorkspaceShell:   IsolationHost,
		ExternalProvider: IsolationHost,
	})
	if err == nil {
		t.Fatal("NewIsolation accepted missing TunnelClient mode")
	}
}
