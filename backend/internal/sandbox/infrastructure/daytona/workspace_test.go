package daytona

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	workspaceapplication "github.com/orz-i/Mender/backend/internal/workspace/application"
	workspacedomain "github.com/orz-i/Mender/backend/internal/workspace/domain"
)

func TestDaytonaWorkspaceReadSearchEditShellShareOneRemoteFS(t *testing.T) {
	remote := &fakeRemote{root: "/home/remote", files: map[string][]byte{}}
	client := &fakeRemoteClient{remote: remote}
	s := &session{client: client, remote: remote, root: remote.root, template: validTemplate()}
	ctx := context.Background()
	created, err := s.Edit(ctx, workspaceapplication.NormalizeEdit(workspacedomain.EditRequest{Path: "draft.txt", Operation: "create", Content: "hello remote\n"}))
	if err != nil || created.Operation != "create" {
		t.Fatalf("create %+v, %v", created, err)
	}
	read, err := s.Read(ctx, workspaceapplication.NormalizeRead(workspacedomain.ReadRequest{Path: "draft.txt"}))
	if err != nil || read.Content != "hello remote" {
		t.Fatalf("read %+v, %v", read, err)
	}
	search, err := s.Search(ctx, workspaceapplication.NormalizeSearch(workspacedomain.SearchRequest{Mode: "content", Query: "remote"}))
	if err != nil || len(search.Matches) != 1 {
		t.Fatalf("search %+v, %v", search, err)
	}
	renamed, err := s.Edit(ctx, workspaceapplication.NormalizeEdit(workspacedomain.EditRequest{Path: "draft.txt", Operation: "replace", Edits: []workspacedomain.TextEdit{{OldText: "hello", NewText: "goodbye"}}}))
	if err != nil || renamed.Replacements != 1 {
		t.Fatalf("replace %+v, %v", renamed, err)
	}
	updated, err := s.Read(ctx, workspaceapplication.NormalizeRead(workspacedomain.ReadRequest{Path: "draft.txt"}))
	if err != nil || updated.Content != "goodbye remote" {
		t.Fatalf("not same remote file %+v, %v", updated, err)
	}
	shell, err := s.Shell(ctx, workspaceapplication.NormalizeShell(workspacedomain.ShellRequest{Command: "pwd"}))
	if err != nil || shell.Stdout != "approved remote execution" || shell.Stderr != "" || shell.TimedOut || shell.Truncated {
		t.Fatalf("remote shell %+v, %v", shell, err)
	}
	deleted, err := s.Edit(ctx, workspaceapplication.NormalizeEdit(workspacedomain.EditRequest{Path: "draft.txt", Operation: "delete"}))
	if err != nil || deleted.Operation != "delete" {
		t.Fatalf("delete %+v, %v", deleted, err)
	}
	if _, ok := remote.files["/home/remote/draft.txt"]; ok {
		t.Fatal("remote delete did not modify same filesystem")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if client.closeCalls != 1 {
		t.Fatal("SDK client not closed")
	}
}

func TestDaytonaWorkspaceRejectsTraversalAndOversized(t *testing.T) {
	remote := &fakeRemote{root: "/home/remote", files: map[string][]byte{"/home/remote/big.txt": []byte(strings.Repeat("x", maxReadBytes+1))}}
	s := &session{client: &fakeRemoteClient{remote: remote}, remote: remote, root: remote.root, template: validTemplate()}
	for _, bad := range []string{"../etc/passwd", "/etc/passwd", "unsafe; touch x", "sub//file", "bad\\path"} {
		_, err := s.Read(context.Background(), workspaceapplication.NormalizeRead(workspacedomain.ReadRequest{Path: bad}))
		var tool *workspacedomain.ToolError
		if !errors.As(err, &tool) {
			t.Fatalf("unsafe path %q: %v", bad, err)
		}
	}
	if remote.execCalls != 0 {
		t.Fatal("lexically unsafe paths reached remote execution")
	}
	_, err := s.Read(context.Background(), workspaceapplication.NormalizeRead(workspacedomain.ReadRequest{Path: "big.txt"}))
	var tool *workspacedomain.ToolError
	if !errors.As(err, &tool) {
		t.Fatalf("oversized content did not produce a Tool failure: %v", err)
	}
	if remote.execCalls == 0 {
		t.Fatal("file scope checks were skipped")
	}
}

func TestDaytonaTypedReadAndSearchRetainEmptyTextSemantics(t *testing.T) {
	t.Parallel()
	remote := &fakeRemote{root: "/home/remote", files: map[string][]byte{
		"/home/remote/empty.txt": {}, "/home/remote/line.txt": []byte("\n"),
	}}
	s := &session{client: &fakeRemoteClient{remote: remote}, remote: remote, root: remote.root}
	read, err := s.Read(context.Background(), workspaceapplication.NormalizeRead(workspacedomain.ReadRequest{Path: "empty.txt"}))
	if err != nil || read.Content != "" || read.StartLine != 1 || read.EndLine != 1 {
		t.Fatalf("Daytona empty-file lines changed: %+v, %v", read, err)
	}
	search, err := s.Search(context.Background(), workspaceapplication.NormalizeSearch(workspacedomain.SearchRequest{
		Mode: "content", Query: "^$", Regex: true, Pattern: "line.txt",
	}))
	if err != nil || len(search.Matches) != 2 {
		t.Fatalf("Daytona trailing empty search line changed: %+v, %v", search, err)
	}
	for _, match := range search.Matches {
		if match.Text == nil || *match.Text != "" {
			t.Fatalf("Daytona empty content text was omitted: %+v", match)
		}
	}
}

func TestDaytonaTypedShellRetainsRemoteTimeoutAndOutputBounds(t *testing.T) {
	t.Parallel()
	base := &fakeRemote{root: "/home/remote", files: map[string][]byte{}}
	remote := &workspaceRemote{RemoteSandbox: base}
	userCalls := 0
	remote.exec = func(ctx context.Context, command, cwd string, timeout time.Duration) (RemoteExec, error) {
		if command != "user-command" {
			return base.Exec(ctx, command, cwd, timeout)
		}
		userCalls++
		deadline, ok := ctx.Deadline()
		if !ok || timeout != 100*time.Millisecond || time.Until(deadline) <= timeout || cwd != base.root {
			t.Fatalf("Daytona remote timeout/context/cwd changed: deadline=%v timeout=%v cwd=%q", deadline, timeout, cwd)
		}
		return RemoteExec{Output: strings.Repeat("x", maxOutputBytes+1)}, nil
	}
	s := &session{client: &fakeRemoteClient{remote: base}, remote: remote, root: base.root}
	for _, timeout := range []int64{1, 99} {
		_, err := s.Shell(context.Background(), workspaceapplication.NormalizeShell(workspacedomain.ShellRequest{Command: "user-command", TimeoutMS: timeout}))
		var tool *workspacedomain.ToolError
		if !errors.As(err, &tool) {
			t.Fatalf("Daytona lower timeout bound accepted %d: %v", timeout, err)
		}
	}
	_, err := s.Shell(context.Background(), workspaceapplication.NormalizeShell(workspacedomain.ShellRequest{Command: "user-command", TimeoutMS: 100}))
	var tool *workspacedomain.ToolError
	if !errors.As(err, &tool) || userCalls != 1 {
		t.Fatalf("oversized output was silently truncated or invalid timeout reached shell: calls=%d err=%v", userCalls, err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("known output rejection retained execution lease: %v", err)
	}
}

func TestDaytonaTypedUncertainShellAndCleanupRetainLease(t *testing.T) {
	t.Parallel()
	for _, stage := range []string{"shell", "cleanup"} {
		t.Run(stage, func(t *testing.T) {
			base := &fakeRemote{root: "/home/remote", files: map[string][]byte{}}
			remote := &workspaceRemote{RemoteSandbox: base}
			remote.exec = func(ctx context.Context, command, cwd string, timeout time.Duration) (RemoteExec, error) {
				if command == "user-command" {
					return RemoteExec{}, errors.New("untrusted vendor transport detail")
				}
				if strings.HasPrefix(command, "rm -f -- ") {
					return RemoteExec{ExitCode: 1}, nil
				}
				return base.Exec(ctx, command, cwd, timeout)
			}
			client := &fakeRemoteClient{remote: base}
			s := &session{client: client, remote: remote, root: base.root}
			if stage == "shell" {
				_, err := s.Shell(context.Background(), workspaceapplication.NormalizeShell(workspacedomain.ShellRequest{Command: "user-command"}))
				var tool *workspacedomain.ToolError
				if !errors.Is(err, sandboxdomain.ErrBackendUnavailable) || errors.As(err, &tool) || strings.Contains(err.Error(), "untrusted") {
					t.Fatalf("uncertain remote error downgraded or leaked: %v", err)
				}
			} else {
				result, err := s.Edit(context.Background(), workspaceapplication.NormalizeEdit(workspacedomain.EditRequest{
					Path: "draft.txt", Operation: "create", Content: "approved content",
				}))
				if err != nil || result.Operation != "create" {
					t.Fatalf("fixture did not reach post-commit cleanup: %+v, %v", result, err)
				}
			}
			if err := s.Close(); !errors.Is(err, sandboxdomain.ErrConflict) || client.closeCalls != 1 {
				t.Fatalf("uncertainty released lease or skipped client cleanup: %v closes=%d", err, client.closeCalls)
			}
		})
	}
}

type workspaceRemote struct {
	RemoteSandbox
	exec func(context.Context, string, string, time.Duration) (RemoteExec, error)
}

func (r *workspaceRemote) Exec(ctx context.Context, command, cwd string, timeout time.Duration) (RemoteExec, error) {
	return r.exec(ctx, command, cwd, timeout)
}

func TestDaytonaSDKVendorTransportDenySSRF(t *testing.T) {
	calls := 0
	transport := vendorTransport{base: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header)}, nil
	})}
	for _, url := range []string{"http://app.daytona.io/api", "https://127.0.0.1/internal", "https://app.daytona.io.evil.test", "https://localhost", "https://app.daytona.io:8443/api"} {
		request, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := transport.RoundTrip(request)
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		if err == nil {
			t.Fatalf("SSRF accepted %s", url)
		}
	}
	good, err := http.NewRequest(http.MethodGet, "https://proxy.app.daytona.io/api", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := transport.RoundTrip(good)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil || calls != 1 {
		t.Fatalf("allowed vendor endpoint rejected: %v calls=%d", err, calls)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
