package domain

import (
	"errors"
	"testing"
	"time"
)

func sampleSpec(t *testing.T) Spec {
	t.Helper()
	id, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	return Spec{ID: id, Name: "Reviewer", Backend: BackendCodex, PluginID: BackendCodex.PluginID(),
		ExtensionID: "main", WorkspaceRoot: t.TempDir(), Isolation: "restricted_host",
		OwnerKind: "local_user", OwnerRef: "os:501", CreatedAt: time.Now()}
}

func TestAgentIdentityAndSafety(t *testing.T) {
	spec := sampleSpec(t)
	agent, err := NewAgent(spec)
	if err != nil {
		t.Fatal(err)
	}
	if !agent.OwnedBy("local_user", "os:501") {
		t.Fatal("owner lost")
	}
	bad := spec
	bad.Backend = BackendGrok
	if _, err := NewAgent(bad); !errors.Is(err, ErrInvalid) {
		t.Fatalf("wrong binding accepted: %v", err)
	}
	bad = spec
	bad.WorkspaceRoot = "relative"
	if _, err := NewAgent(bad); !errors.Is(err, ErrInvalid) {
		t.Fatalf("relative root accepted: %v", err)
	}
	bad = spec
	bad.PluginID = "io.mender.agent.evil"
	if _, err := NewAgent(bad); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown plugin accepted: %v", err)
	}
}

func TestOpenCodeBackendHasDistinctExactBuiltinIdentity(t *testing.T) {
	spec := sampleSpec(t)
	spec.Backend = BackendOpenCode
	spec.PluginID = BackendOpenCode.PluginID()
	if spec.PluginID != OpenCodePluginID {
		t.Fatal("wrong OpenCode plugin identity")
	}
	if _, err := NewAgent(spec); err != nil {
		t.Fatalf("OpenCode resource rejected: %v", err)
	}
	spec.PluginID = GrokPluginID
	if _, err := NewAgent(spec); !errors.Is(err, ErrInvalid) {
		t.Fatalf("OpenCode masqueraded as Grok: %v", err)
	}
}
func TestTaskStateIsMonotonic(t *testing.T) {
	if err := NextStatus(TaskQueued, TaskRunning); err != nil {
		t.Fatal(err)
	}
	if err := NextStatus(TaskRunning, TaskUnknown); err != nil {
		t.Fatal(err)
	}
	if err := NextStatus(TaskCompleted, TaskRunning); !errors.Is(err, ErrConflict) {
		t.Fatal("terminal task reopened")
	}
	if err := NextStatus(TaskQueued, TaskCompleted); !errors.Is(err, ErrConflict) {
		t.Fatal("task skipped running")
	}
	id, err := NewTaskID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseTaskID(string(id)); err != nil {
		t.Fatal(err)
	}
}
