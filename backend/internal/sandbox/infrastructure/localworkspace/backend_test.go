package localworkspace

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

type testLauncher struct {
	received []sandboxdomain.IsolationClass
}

func (l *testLauncher) Command(ctx context.Context, mode sandboxdomain.IsolationClass, _ string, dir string, command string, env []string) (*exec.Cmd, error) {
	l.received = append(l.received, mode)
	if mode != sandboxdomain.IsolationHost {
		return nil, errors.New("restricted host backend unavailable")
	}
	// #nosec G204 -- test-only launcher deliberately executes a fixed test shell command.
	cmd := exec.CommandContext(ctx, "/bin/sh", "-lc", command)
	cmd.Dir = dir
	cmd.Env = append([]string(nil), env...)
	return cmd, nil
}
func testLocalSandbox(t *testing.T, root string, mode sandboxdomain.IsolationClass) (sandboxdomain.Sandbox, sandboxdomain.Run) {
	t.Helper()
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	id := sandboxdomain.ID("sbx_0123456789abcdef0123456789abcdef")
	item, err := sandboxdomain.NewSandbox(sandboxdomain.Spec{ID: id, Name: "Test", Backend: sandboxdomain.BackendLocalWorkspace, Isolation: mode, LocalRoot: canonical, OwnerKind: "local_user", OwnerRef: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := sandboxdomain.NewRun(id, 1, sandboxdomain.StateReady, "local")
	if err != nil {
		t.Fatal(err)
	}
	return item, run
}
func execute(t *testing.T, instance *Instance, capability sandboxdomain.Capability, input any) invocationdomain.Outcome {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	args, err := invocationdomain.NewJSONValue(raw)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := instance.ExecuteWorkspace(context.Background(), capability, args)
	if err != nil {
		t.Fatal(err)
	}
	return outcome
}
func requireComplete(t *testing.T, o invocationdomain.Outcome) {
	t.Helper()
	if o.Kind() != invocationdomain.OutcomeKindComplete {
		t.Fatalf("want complete got %s: %s", o.Kind(), o.Message())
	}
}
func requireFailure(t *testing.T, o invocationdomain.Outcome) {
	t.Helper()
	if o.Kind() != invocationdomain.OutcomeKindFailed {
		t.Fatalf("want tool error got %s", o.Kind())
	}
}
func TestAllWorkspaceCapabilitiesShareOneRoot(t *testing.T) {
	root := t.TempDir()
	launcher := &testLauncher{}
	backend, err := New(launcher)
	if err != nil {
		t.Fatal(err)
	}
	item, run := testLocalSandbox(t, root, sandboxdomain.IsolationHost)
	opened, err := backend.Open(context.Background(), item, run)
	if err != nil {
		t.Fatal(err)
	}
	instance, ok := opened.(*Instance)
	if !ok {
		t.Fatalf("session %T", opened)
	}
	defer func() { _ = instance.Close() }()
	create := execute(t, instance, sandboxdomain.CapabilityEdit, map[string]any{"path": "draft.txt", "operation": "create", "content": "alpha beta\n"})
	requireComplete(t, create)
	read := execute(t, instance, sandboxdomain.CapabilityRead, map[string]any{"path": "draft.txt"})
	requireComplete(t, read)
	var file struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(read.Content().Bytes(), &file); err != nil {
		t.Fatal(err)
	}
	if file.Content != "alpha beta" {
		t.Fatalf("read content = %q", file.Content)
	}
	search := execute(t, instance, sandboxdomain.CapabilitySearch, map[string]any{"mode": "content", "query": "beta"})
	requireComplete(t, search)
	var found struct {
		Matches []struct {
			Path string `json:"path"`
		} `json:"matches"`
	}
	if err := json.Unmarshal(search.Content().Bytes(), &found); err != nil {
		t.Fatal(err)
	}
	if len(found.Matches) != 1 || found.Matches[0].Path != "draft.txt" {
		t.Fatalf("search %+v", found.Matches)
	}
	shell := execute(t, instance, sandboxdomain.CapabilityShell, map[string]any{"command": "pwd; cat draft.txt"})
	requireComplete(t, shell)
	var result struct {
		ExitCode int    `json:"exitCode"`
		Stdout   string `json:"stdout"`
	}
	if err := json.Unmarshal(shell.Content().Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	canonical, _ := filepath.EvalSymlinks(root)
	if result.ExitCode != 0 || !strings.Contains(result.Stdout, canonical) || !strings.Contains(result.Stdout, "alpha beta") {
		t.Fatalf("shell %+v", result)
	}
	if len(launcher.received) != 1 || launcher.received[0] != sandboxdomain.IsolationHost {
		t.Fatalf("mode %+v", launcher.received)
	}
}
func TestLocalWorkspaceRejectsSymlinkEscapeAndUnknownOperation(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(root, "escape")); err != nil {
		t.Skip(err)
	}
	backend, _ := New(&testLauncher{})
	item, run := testLocalSandbox(t, root, sandboxdomain.IsolationHost)
	opened, err := backend.Open(context.Background(), item, run)
	if err != nil {
		t.Fatal(err)
	}
	instance := opened.(*Instance)
	defer func() { _ = instance.Close() }()
	requireFailure(t, execute(t, instance, sandboxdomain.CapabilityRead, map[string]any{"path": "escape"}))
	requireFailure(t, execute(t, instance, sandboxdomain.CapabilityEdit, map[string]any{"path": "../outside", "operation": "create", "content": "x"}))
	if _, err := instance.ExecuteWorkspace(context.Background(), "unknown", invocationdomain.JSONValue{}); !errors.Is(err, sandboxdomain.ErrCapabilityUnavailable) {
		t.Fatalf("unknown operation: %v", err)
	}
}
func TestRestrictedHostModeIsForwardedAndNeverDowngraded(t *testing.T) {
	launcher := &testLauncher{}
	backend, _ := New(launcher)
	item, run := testLocalSandbox(t, t.TempDir(), sandboxdomain.IsolationRestrictedHost)
	opened, err := backend.Open(context.Background(), item, run)
	if err != nil {
		t.Fatal(err)
	}
	instance := opened.(*Instance)
	defer func() { _ = instance.Close() }()
	_, err = instance.ExecuteWorkspace(context.Background(), sandboxdomain.CapabilityShell, mustJSON(t, map[string]any{"command": "echo must-not-run"}))
	if err == nil || len(launcher.received) != 1 || launcher.received[0] != sandboxdomain.IsolationRestrictedHost {
		t.Fatalf("restricted host silently downgraded: %v; %+v", err, launcher.received)
	}
}
func TestBackendOpenRejectsStaleOrInvalidIdentity(t *testing.T) {
	backend, _ := New(&testLauncher{})
	item, run := testLocalSandbox(t, t.TempDir(), sandboxdomain.IsolationHost)
	stale := run
	stale.State = sandboxdomain.StateStopped
	if _, err := backend.Open(context.Background(), item, stale); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("stale run: %v", err)
	}
	invalid := item.Spec()
	invalid.LocalRoot = t.TempDir() + "/../other"
	malformed := sandboxdomain.Sandbox{}
	if _, err := sandboxdomain.NewSandbox(invalid); err == nil {
		t.Fatal("noncanonical spec allowed")
	}
	if _, err := backend.Open(context.Background(), malformed, run); err == nil {
		t.Fatal("invalid sandbox allowed")
	}
	if _, err := New(nil); err == nil {
		t.Fatal("nil launcher allowed")
	}
}
func mustJSON(t *testing.T, input any) invocationdomain.JSONValue {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	v, err := invocationdomain.NewJSONValue(raw)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
