package domain

import (
	"errors"
	"testing"
)

const validID ID = "sbx_0123456789abcdef0123456789abcdef"

func TestSandboxIdentityAndIsolation(t *testing.T) {
	for _, bad := range []string{"", "sbx_upper", "sbx_0123456789abcdef0123456789abcdeF", "sbx_zz23456789abcdef0123456789abcdef"} {
		if _, err := ParseID(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	good := Spec{ID: validID, Name: "Test", Backend: BackendLocalWorkspace, Binding: BuiltinBinding(BackendLocalWorkspace), Isolation: IsolationHost, LocalRoot: "/tmp/mender", OwnerKind: "local_user", OwnerRef: "owner"}
	if _, err := NewSandbox(good); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		change func(*Spec)
	}{
		{"untrusted host root", func(s *Spec) { s.LocalRoot = "../host" }},
		{"container masquerading as local", func(s *Spec) { s.Isolation = IsolationContainer }},
		{"unknown backend", func(s *Spec) { s.Backend = "fake" }},
		{"no owner", func(s *Spec) { s.OwnerRef = "" }},
		{"remote host mount", func(s *Spec) { s.Backend = BackendDaytona; s.Isolation = IsolationRemote }},
	} {
		t.Run(test.name, func(t *testing.T) {
			item := good
			test.change(&item)
			if _, err := NewSandbox(item); err == nil {
				t.Fatal("accepted invalid sandbox")
			}
		})
	}
}
func TestRunTransitionsAndGeneration(t *testing.T) {
	r, err := NewRun(validID, 1, StateDefined, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.Transition(StateReady, "remote"); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	r, err = r.Transition(StateProvisioning, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.Transition(StateReady, ""); err == nil {
		t.Fatal("ready missing ref")
	}
	r, err = r.Transition(StateReady, "sandbox-one")
	if err != nil {
		t.Fatal(err)
	}
	r, err = r.Transition(StateStopping, "sandbox-one")
	if err != nil {
		t.Fatal(err)
	}
	r, err = r.Transition(StateStopped, "sandbox-one")
	if err != nil {
		t.Fatal(err)
	}
	r, err = r.Transition(StateProvisioning, "")
	if err != nil {
		t.Fatal(err)
	}
	if r.Generation != 2 || r.ExternalID != "" {
		t.Fatalf("new generation not fenced: %+v", r)
	}
	if _, err = r.Transition(StateDestroyed, ""); !errors.Is(err, ErrConflict) {
		t.Fatalf("invalid destroy: %v", err)
	}
}
func TestCapabilitiesExplicit(t *testing.T) {
	cap := Capabilities{Read: true, Shell: true}
	if !cap.Supports(CapabilityRead) || !cap.Supports(CapabilityShell) || cap.Supports(CapabilityEdit) || cap.Supports("unexpected") {
		t.Fatalf("bad capabilities: %+v", cap)
	}
}
