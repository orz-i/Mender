package application

import (
	"context"
	"errors"
	"reflect"
	"testing"

	workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"
)

type sessionFixture struct {
	read   workspace.ReadRequest
	search workspace.SearchRequest
	edit   workspace.EditRequest
	shell  workspace.ShellRequest
	calls  int
	err    error
}

func (s *sessionFixture) Read(_ context.Context, r workspace.ReadRequest) (workspace.ReadResult, error) {
	s.calls++
	s.read = r
	return workspace.ReadResult{Path: r.Path, Content: "read"}, s.err
}
func (s *sessionFixture) Search(_ context.Context, r workspace.SearchRequest) (workspace.SearchResult, error) {
	s.calls++
	s.search = r
	return workspace.SearchResult{Mode: r.Mode, Matches: []workspace.SearchMatch{}}, s.err
}
func (s *sessionFixture) Edit(_ context.Context, r workspace.EditRequest) (workspace.EditResult, error) {
	s.calls++
	s.edit = r
	return workspace.EditResult{Path: r.Path}, s.err
}
func (s *sessionFixture) Shell(_ context.Context, r workspace.ShellRequest) (workspace.ShellResult, error) {
	s.calls++
	s.shell = r
	return workspace.ShellResult{Stdout: "shell"}, s.err
}
func (*sessionFixture) Close() error { return nil }

func TestOperationsPassThroughAndCloseOriginalLease(t *testing.T) {
	ctx := context.Background()
	s := &sessionFixture{}
	closed := 0
	newLease := func() *Lease { return NewLease(s, func() error { closed++; return nil }, ExecutionConstraints{}) }
	r := newLease()
	if _, err := r.Read(ctx, workspace.ReadRequest{Path: "./a"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if s.read.Path != "./a" || closed != 1 {
		t.Fatalf("read=%+v closes=%d", s.read, closed)
	}
	if _, err := newLease().Search(ctx, workspace.SearchRequest{Mode: "files"}); err != nil {
		t.Fatal(err)
	}
	if s.search.Mode != "files" {
		t.Fatalf("search=%+v", s.search)
	}
	input := workspace.EditRequest{Path: "a", Operation: "replace", Edits: []workspace.TextEdit{{OldText: "a", NewText: "b"}}}
	before := append([]workspace.TextEdit(nil), input.Edits...)
	if _, err := newLease().Edit(ctx, input); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(input.Edits, before) {
		t.Fatalf("edit mutated caller: %+v %+v", input, s.edit)
	}
	if _, err := newLease().Shell(ctx, workspace.ShellRequest{Command: "echo"}); err != nil {
		t.Fatal(err)
	}
	if s.shell.Command != "echo" || closed != 4 || s.calls != 4 {
		t.Fatalf("shell=%+v closes=%d calls=%d", s.shell, closed, s.calls)
	}
}

func TestExternalDefaultsAndExplicitValuesRemainUnchanged(t *testing.T) {
	s := &sessionFixture{}
	l := NewLease(s, func() error { return nil }, ExecutionConstraints{})
	if _, err := l.Read(context.Background(), workspace.ReadRequest{Path: "a"}); err != nil {
		t.Fatal(err)
	}
	if s.read.Offset != 0 || s.read.Limit != 0 {
		t.Fatalf("v1 omissions changed: %+v", s.read)
	}
	r := NormalizeRead(workspace.ReadRequest{Offset: -1, Limit: 17})
	if r.Offset != -1 || r.Limit != 17 {
		t.Fatalf("validation hidden by defaults: %+v", r)
	}
	sh := NormalizeShell(workspace.ShellRequest{Cwd: "./x", TimeoutMS: 99})
	if sh.Cwd != "./x" || sh.TimeoutMS != 99 {
		t.Fatalf("explicit shell changed: %+v", sh)
	}
}

func TestFailureAndCancellationAlwaysCloseWithoutRetry(t *testing.T) {
	closeErr := errors.New("uncertain remote cleanup")
	for _, runErr := range []error{nil, context.Canceled, &workspace.ToolError{Message: "expected failure"}, &workspace.InvalidResultError{Message: "invalid result"}} {
		for _, cleanup := range []error{nil, closeErr} {
			s := &sessionFixture{err: runErr}
			closed := 0
			l := NewLease(s, func() error { closed++; return cleanup }, ExecutionConstraints{})
			_, err := l.Edit(context.Background(), workspace.EditRequest{Path: "a", Operation: "delete"})
			if cleanup != nil {
				var typed *CleanupError
				if !errors.As(err, &typed) || !errors.Is(err, closeErr) {
					t.Fatalf("cleanup lost: %v", err)
				}
				if errors.Is(runErr, context.Canceled) && !errors.Is(err, context.Canceled) {
					t.Fatalf("cancel lost: %v", err)
				}
			} else if !errors.Is(err, runErr) {
				t.Fatalf("run classification changed: %v", err)
			}
			_ = l.Close()
			if closed != 1 || s.calls != 1 {
				t.Fatalf("close/retry count %d/%d", closed, s.calls)
			}
		}
	}
}

func TestAbortAndTransportArgumentLimits(t *testing.T) {
	closed := 0
	s := &sessionFixture{}
	l := NewLease(s, func() error { closed++; return nil }, ExecutionConstraints{MaxArgumentBytes: 3})
	if err := l.CheckArgumentSize(3); err != nil {
		t.Fatal(err)
	}
	err := l.Abort(l.CheckArgumentSize(4))
	var tool *workspace.ToolError
	if err == nil || !errors.As(err, &tool) || closed != 1 || s.calls != 0 {
		t.Fatalf("limit classification or cleanup: %v %d %d", err, closed, s.calls)
	}
}
