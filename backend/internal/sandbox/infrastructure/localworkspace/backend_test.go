package localworkspace

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	workspaceapp "github.com/orz-i/Mender/backend/internal/workspace/application"
	workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"
)

type testLauncher struct {
	received        []sandboxdomain.IsolationClass
	allowRestricted bool
}

func (l *testLauncher) CheckIsolation(mode sandboxdomain.IsolationClass) error {
	if mode == sandboxdomain.IsolationHost || (mode == sandboxdomain.IsolationRestrictedHost && l.allowRestricted) {
		return nil
	}
	return errors.New("required isolation unavailable")
}

func (l *testLauncher) Command(ctx context.Context, mode sandboxdomain.IsolationClass, _ string, dir string, command string, env []string) (*exec.Cmd, error) {
	l.received = append(l.received, mode)
	if mode != sandboxdomain.IsolationHost {
		return nil, errors.New("restricted host backend unavailable")
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		// #nosec G204 -- test-only launcher intentionally executes fixture shell input.
		cmd = exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", command)
	} else {
		// #nosec G204 -- test-only launcher intentionally executes fixture shell input.
		cmd = exec.CommandContext(ctx, "/bin/sh", "-lc", command)
	}
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
	item, err := sandboxdomain.NewSandbox(sandboxdomain.Spec{ID: id, Name: "Test", Backend: sandboxdomain.BackendLocalWorkspace, Binding: sandboxdomain.BuiltinBinding(sandboxdomain.BackendLocalWorkspace), Isolation: mode, LocalRoot: canonical, OwnerKind: "local_user", OwnerRef: "owner"})
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
	var result any
	switch capability {
	case sandboxdomain.CapabilityRead:
		var in workspace.ReadRequest
		if err := json.Unmarshal(raw, &in); err != nil {
			t.Fatal(err)
		}
		result, err = instance.Read(context.Background(), workspaceapp.NormalizeRead(in))
	case sandboxdomain.CapabilitySearch:
		var in workspace.SearchRequest
		if err := json.Unmarshal(raw, &in); err != nil {
			t.Fatal(err)
		}
		result, err = instance.Search(context.Background(), workspaceapp.NormalizeSearch(in))
	case sandboxdomain.CapabilityEdit:
		var in workspace.EditRequest
		if err := json.Unmarshal(raw, &in); err != nil {
			t.Fatal(err)
		}
		result, err = instance.Edit(context.Background(), workspaceapp.NormalizeEdit(in))
	case sandboxdomain.CapabilityShell:
		var in workspace.ShellRequest
		if err := json.Unmarshal(raw, &in); err != nil {
			t.Fatal(err)
		}
		result, err = instance.Shell(context.Background(), workspaceapp.NormalizeShell(in))
	default:
		t.Fatalf("unsupported fixture capability %q", capability)
	}
	var tool *workspace.ToolError
	if errors.As(err, &tool) {
		outcome, failureErr := invocationdomain.Failed(invocationdomain.FailureSourceTool, invocationdomain.FailureToolError, tool.Message)
		if failureErr != nil {
			t.Fatal(failureErr)
		}
		return outcome
	}
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	content, err := invocationdomain.NewJSONValue(encoded)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := invocationdomain.Complete(content)
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
func TestLocalWorkspaceRejectsSymlinkEscapeAndTraversal(t *testing.T) {
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
}
func TestRestrictedHostModeIsForwardedAndNeverDowngraded(t *testing.T) {
	launcher := &testLauncher{allowRestricted: true}
	backend, _ := New(launcher)
	item, run := testLocalSandbox(t, t.TempDir(), sandboxdomain.IsolationRestrictedHost)
	opened, err := backend.Open(context.Background(), item, run)
	if err != nil {
		t.Fatal(err)
	}
	instance := opened.(*Instance)
	defer func() { _ = instance.Close() }()
	_, err = instance.Shell(context.Background(), workspaceapp.NormalizeShell(workspace.ShellRequest{Command: "echo must-not-run"}))
	if err == nil || len(launcher.received) != 1 || launcher.received[0] != sandboxdomain.IsolationRestrictedHost {
		t.Fatalf("restricted host silently downgraded: %v; %+v", err, launcher.received)
	}
}
func TestRestrictedHostBackendUnavailableDeniesFileOperationsToo(t *testing.T) {
	backend, _ := New(&testLauncher{})
	item, run := testLocalSandbox(t, t.TempDir(), sandboxdomain.IsolationRestrictedHost)
	if _, err := backend.Open(context.Background(), item, run); err == nil {
		t.Fatal("restricted_host unavailable: backend still allowed file access")
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

func TestTypedLocalWorkspacePreservesEmptyReadAndSearchFields(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "empty-line.txt"), []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	backend, _ := New(&testLauncher{})
	item, run := testLocalSandbox(t, root, sandboxdomain.IsolationHost)
	opened, err := backend.Open(context.Background(), item, run)
	if err != nil {
		t.Fatal(err)
	}
	instance := opened.(*Instance)
	defer func() { _ = instance.Close() }()
	read, err := instance.Read(context.Background(), workspaceapp.NormalizeRead(workspace.ReadRequest{Path: "./empty-line.txt"}))
	if err != nil || read.Path != "empty-line.txt" || read.Content != "" || read.StartLine != 1 || read.EndLine != 1 || read.Truncated {
		t.Fatalf("empty line read changed: %+v err=%v", read, err)
	}
	beyond, err := instance.Read(context.Background(), workspaceapp.NormalizeRead(workspace.ReadRequest{Path: "./empty-line.txt", Offset: 3}))
	if err != nil || beyond != (workspace.ReadResult{Path: "empty-line.txt"}) {
		t.Fatalf("out-of-range read changed: %+v err=%v", beyond, err)
	}
	search, err := instance.Search(context.Background(), workspaceapp.NormalizeSearch(workspace.SearchRequest{Mode: "content", Query: "^$", Regex: true}))
	if err != nil || len(search.Matches) != 1 || search.Matches[0].Line != 1 || search.Matches[0].Text != nil {
		t.Fatalf("empty snippet presence changed: %+v err=%v", search, err)
	}
	encoded, err := json.Marshal(search)
	if err != nil || strings.Contains(string(encoded), `"text"`) {
		t.Fatalf("empty local snippet is no longer omitted: %s err=%v", encoded, err)
	}
	missing, err := instance.Search(context.Background(), workspaceapp.NormalizeSearch(workspace.SearchRequest{Mode: "content", Query: "missing"}))
	if err != nil || missing.Matches == nil || len(missing.Matches) != 0 {
		t.Fatalf("empty matches must remain an array: %+v err=%v", missing, err)
	}
}

func TestTypedLocalWorkspacePreservesContextAndClosedSessionErrors(t *testing.T) {
	backend, _ := New(&testLauncher{})
	item, run := testLocalSandbox(t, t.TempDir(), sandboxdomain.IsolationHost)
	opened, err := backend.Open(context.Background(), item, run)
	if err != nil {
		t.Fatal(err)
	}
	instance := opened.(*Instance)
	defer func() { _ = instance.Close() }()
	operations := map[string]func(context.Context) error{
		"read": func(ctx context.Context) error {
			_, err := instance.Read(ctx, workspaceapp.NormalizeRead(workspace.ReadRequest{Path: "missing.txt"}))
			return err
		},
		"search": func(ctx context.Context) error {
			_, err := instance.Search(ctx, workspaceapp.NormalizeSearch(workspace.SearchRequest{Mode: "files", Pattern: "*"}))
			return err
		},
		"edit": func(ctx context.Context) error {
			_, err := instance.Edit(ctx, workspaceapp.NormalizeEdit(workspace.EditRequest{Path: "new.txt", Operation: "create"}))
			return err
		},
		"shell": func(ctx context.Context) error {
			_, err := instance.Shell(ctx, workspaceapp.NormalizeShell(workspace.ShellRequest{Command: "must-not-run"}))
			return err
		},
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for name, operation := range operations {
		if err := operation(cancelled); !errors.Is(err, context.Canceled) {
			t.Fatalf("%s lost cancellation error: %v", name, err)
		}
	}
	if err := instance.Close(); err != nil {
		t.Fatal(err)
	}
	for name, operation := range operations {
		if err := operation(context.Background()); !errors.Is(err, sandboxdomain.ErrBackendUnavailable) {
			t.Fatalf("%s accepted a closed session: %v", name, err)
		}
	}
}
