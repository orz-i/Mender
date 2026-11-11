package plugin

import (
	"context"
	"errors"
	"testing"

	invocation "github.com/orz-i/Mender/backend/internal/invocation/domain"
	workspaceapp "github.com/orz-i/Mender/backend/internal/workspace/application"
	workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"
)

type typedSessionFixture struct {
	calls  int
	err    error
	read   workspace.ReadRequest
	search workspace.SearchRequest
	edit   workspace.EditRequest
	shell  workspace.ShellRequest
}

func (s *typedSessionFixture) Read(_ context.Context, in workspace.ReadRequest) (workspace.ReadResult, error) {
	s.calls++
	s.read = in
	return workspace.ReadResult{Path: in.Path, Content: "ok"}, s.err
}
func (s *typedSessionFixture) Search(_ context.Context, in workspace.SearchRequest) (workspace.SearchResult, error) {
	s.search = in
	return workspace.SearchResult{Mode: in.Mode, Matches: []workspace.SearchMatch{}}, nil
}
func (s *typedSessionFixture) Edit(_ context.Context, in workspace.EditRequest) (workspace.EditResult, error) {
	s.edit = in
	return workspace.EditResult{Path: in.Path, Operation: in.Operation}, nil
}
func (s *typedSessionFixture) Shell(_ context.Context, in workspace.ShellRequest) (workspace.ShellResult, error) {
	s.shell = in
	return workspace.ShellResult{}, nil
}
func (*typedSessionFixture) Close() error { return nil }

func TestTypedToolAdapterPreservesFailureClassesAndCleanupPriority(t *testing.T) {
	transport := errors.New("transport lost")
	cleanup := errors.New("cleanup uncertain")
	for _, test := range []struct {
		name       string
		run, close error
		source     invocation.FailureSource
		code       invocation.FailureCode
	}{
		{name: "success"},
		{name: "tool", run: &workspace.ToolError{Message: "safe failure"}, source: invocation.FailureSourceTool, code: invocation.FailureToolError},
		{name: "invalid output", run: &workspace.InvalidResultError{Message: "bad output"}, source: invocation.FailureSourceResultGuard, code: invocation.FailureOutputSchemaMismatch},
		{name: "invalid and oversized output", run: &workspace.InvalidResultError{Message: "bad output", CanonicalBytes: 1001}, source: invocation.FailureSourceResultGuard, code: invocation.FailureResultTooLarge},
		{name: "transport", run: transport},
		{name: "tool and cleanup", run: &workspace.ToolError{Message: "safe failure"}, close: cleanup},
		{name: "transport and cleanup", run: transport, close: cleanup},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := &typedSessionFixture{err: test.run}
			closed := 0
			l := workspaceapp.NewLease(s, func() error { closed++; return test.close }, workspaceapp.ExecutionConstraints{})
			args, err := invocation.NewJSONValue([]byte(`{"path":"a"}`))
			if err != nil {
				t.Fatal(err)
			}
			out, err := executeTyped(context.Background(), l, workspace.Read, args, 1000)
			if closed != 1 || s.calls != 1 {
				t.Fatalf("lifetime %d/%d", closed, s.calls)
			}
			if test.close != nil || errors.Is(test.run, transport) {
				if err == nil || out.Kind() != "" {
					t.Fatalf("operational failure became outcome: %+v %v", out, err)
				}
				if test.close != nil && !errors.Is(err, cleanup) {
					t.Fatal(err)
				}
				if errors.Is(test.run, transport) && !errors.Is(err, transport) {
					t.Fatal(err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if test.source != "" {
				if out.FailureSource() != test.source || out.FailureCode() != test.code {
					t.Fatalf("classification changed: %+v", out)
				}
			} else if out.Kind() != invocation.OutcomeKindComplete || s.read.Path != "a" {
				t.Fatalf("success/defaults %+v %+v", out, s.read)
			}
		})
	}
}

func TestTypedToolDecodeFailureAndCancellationReleaseWithoutExecuting(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		s := &typedSessionFixture{}
		closed := 0
		l := workspaceapp.NewLease(s, func() error { closed++; return nil }, workspaceapp.ExecutionConstraints{})
		args, err := invocation.NewJSONValue([]byte(`{"path":"a","unknown":1}`))
		if err != nil {
			t.Fatal(err)
		}
		ctx, stop := context.WithCancel(context.Background())
		defer stop()
		if cancel {
			stop()
		}
		out, err := executeTyped(ctx, l, workspace.Read, args, 1000)
		if closed != 1 || s.calls != 0 {
			t.Fatalf("failed request executed/leaked %d/%d", s.calls, closed)
		}
		if cancel {
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation %v", err)
			}
		} else if err != nil || out.FailureCode() != invocation.FailureToolError {
			t.Fatalf("decode classification %+v %v", out, err)
		}
	}
}

func TestTypedAdapterPreservesExplicitOptionalValuesForV1(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		s := &typedSessionFixture{}
		invoke := func(operation workspace.Operation, raw string) {
			t.Helper()
			args, err := invocation.NewJSONValue([]byte(raw))
			if err != nil {
				t.Fatal(err)
			}
			lease := workspaceapp.NewLease(s, func() error { return nil }, workspaceapp.ExecutionConstraints{})
			if _, err := executeTyped(context.Background(), lease, operation, args, 1000); err != nil {
				t.Fatal(err)
			}
			var size int
			switch operation {
			case workspace.Search:
				size = s.search.CanonicalBytes
			case workspace.Edit:
				size = s.edit.CanonicalBytes
			case workspace.Shell:
				size = s.shell.CanonicalBytes
			}
			if size != len(raw) {
				t.Fatalf("source argument measurement lost: %d want %d", size, len(raw))
			}
		}
		search := `{"mode":"content","query":"x"}`
		edit := `{"path":"a","operation":"create"}`
		shell := `{"command":"echo"}`
		if explicit {
			search = `{"mode":"content","query":"x","path":"","pattern":"","regex":false,"caseSensitive":false}`
			edit = `{"path":"a","operation":"create","content":"","edits":[]}`
			shell = `{"command":"echo","cwd":""}`
		}
		invoke(workspace.Search, search)
		invoke(workspace.Edit, edit)
		invoke(workspace.Shell, shell)
		for _, field := range []workspace.Fields{workspace.FieldPath, workspace.FieldPattern, workspace.FieldRegex, workspace.FieldCaseSensitive} {
			if s.search.Present.Has(field) != explicit {
				t.Fatalf("search presence lost explicit=%v: %+v", explicit, s.search)
			}
		}
		if !s.search.Present.Has(workspace.FieldQuery) || s.edit.Present.Has(workspace.FieldContent) != explicit || s.edit.Present.Has(workspace.FieldEdits) != explicit || s.shell.Present.Has(workspace.FieldCwd) != explicit {
			t.Fatalf("optional presence lost: %+v %+v %+v", s.search, s.edit, s.shell)
		}
		if s.search.Path != "" || s.search.MaxResults != 0 || s.shell.Cwd != "" || s.shell.TimeoutMS != 0 {
			t.Fatal("v1 values gained defaults")
		}
	}
}
