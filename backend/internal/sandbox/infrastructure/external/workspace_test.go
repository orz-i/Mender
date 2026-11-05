package external

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"
	sandboxv1 "github.com/orz-i/Mender/sdk/sandboxplugin/go/protocol/v1"
	"google.golang.org/grpc"
)

const readResultJSON = `{"path":"README.txt","content":"fixture","startLine":1,"endLine":1,"truncated":false}`

type workspaceClient struct {
	sandboxv1.SandboxPluginServiceClient
	reply        *sandboxv1.ExecuteWorkspaceResponse
	err          error
	request      *sandboxv1.ExecuteWorkspaceRequest
	calls        int
	closeRequest *sandboxv1.CloseSessionRequest
	closeContext context.Context
	closeErr     error
}

func (c *workspaceClient) ExecuteWorkspace(ctx context.Context, in *sandboxv1.ExecuteWorkspaceRequest, _ ...grpc.CallOption) (*sandboxv1.ExecuteWorkspaceResponse, error) {
	c.request, c.calls = in, c.calls+1
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return c.reply, c.err
}

func (c *workspaceClient) CloseSession(ctx context.Context, in *sandboxv1.CloseSessionRequest, _ ...grpc.CallOption) (*sandboxv1.CloseSessionResponse, error) {
	c.closeContext, c.closeRequest = ctx, in
	return &sandboxv1.CloseSessionResponse{}, c.closeErr
}

func workspaceSession(raw string) (*Session, *workspaceClient) {
	client := &workspaceClient{reply: &sandboxv1.ExecuteWorkspaceResponse{ContentJson: []byte(raw)}}
	return &Session{service: client, id: "session:fixture", capabilities: sandboxdomain.Capabilities{
		Read: true, Search: true, Edit: true, Shell: true,
	}}, client
}

func TestTypedWorkspaceRetainsV1EdgeWhitespaceAndSizeOrder(t *testing.T) {
	t.Parallel()
	for _, whitespace := range []string{" \n\t", "\u00a0", "\u2003"} {
		session, _ := workspaceSession(whitespace + readResultJSON + whitespace)
		result, err := session.Read(context.Background(), workspace.ReadRequest{Path: "README.txt"})
		if err != nil || result.Content != "fixture" || result.CanonicalBytes != len(readResultJSON) {
			t.Fatalf("v1 edge whitespace %q: %+v %v", whitespace, result, err)
		}
	}
	session, _ := workspaceSession(strings.Repeat("\u2003", maxResultJSON/3+1) + readResultJSON)
	_, err := session.Read(context.Background(), workspace.ReadRequest{Path: "README.txt"})
	assertOrdinaryWorkspaceError(t, err)
	if !strings.Contains(err.Error(), "result too large") {
		t.Fatalf("trim bypassed wire cap: %v", err)
	}
}

func TestTypedWorkspaceArgumentCapRetainsOriginalV1Measurement(t *testing.T) {
	t.Parallel()
	for _, value := range []string{strings.Repeat("<", 256*1024), strings.Repeat("\u2028", 200*1024)} {
		original := `{"path":"a","operation":"create","content":"` + value + `"}`
		session, client := workspaceSession(`{"path":"a","operation":"create","replacements":0,"bytesBefore":0,"bytesAfter":1}`)
		request := workspace.EditRequest{Path: "a", Operation: "create", Content: value, CanonicalBytes: len(original)}
		if _, err := session.Edit(context.Background(), request); err != nil {
			t.Fatalf("typed expansion changed v1 cap: %v", err)
		}
		if client.calls != 1 || len(client.request.GetArgumentsJson()) <= maxWorkspaceArguments {
			t.Fatal("fixture did not cross encoded cap")
		}
		var decoded workspace.EditRequest
		if err := json.Unmarshal(client.request.GetArgumentsJson(), &decoded); err != nil || decoded.Content != value {
			t.Fatal("request content changed")
		}
		request.CanonicalBytes = maxWorkspaceArguments + 1
		if _, err := session.Edit(context.Background(), request); err == nil || client.calls != 1 {
			t.Fatal("original cap bypassed")
		}
		request.CanonicalBytes = 0
		if _, err := session.Edit(context.Background(), request); err == nil || client.calls != 1 {
			t.Fatal("unmeasured typed request bypassed fallback cap")
		}
	}
}

func TestTypedWorkspaceOperationsPreserveV1Fields(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name       string
		capability string
		arguments  string
		reply      string
		call       func(*Session) (any, error)
		want       any
	}{
		{
			name: "read", capability: "file.read", arguments: `{"path":"README.txt","offset":2,"limit":3}`,
			reply: readResultJSON,
			call: func(s *Session) (any, error) {
				return s.Read(context.Background(), workspace.ReadRequest{Path: "README.txt", Offset: 2, Limit: 3})
			},
			want: workspace.ReadResult{Path: "README.txt", Content: "fixture", StartLine: 1, EndLine: 1},
		},
		{
			name: "search", capability: "file.search",
			arguments: `{"mode":"content","path":"docs","query":"needle","pattern":"*.txt","regex":true,"caseSensitive":true,"maxResults":2}`,
			reply:     `{"mode":"content","matches":[{"path":"a.txt"},{"path":"b.txt","line":2,"text":""}],"truncated":true}`,
			call: func(s *Session) (any, error) {
				return s.Search(context.Background(), workspace.SearchRequest{Mode: "content", Path: "docs", Query: "needle",
					Pattern: "*.txt", Regex: true, CaseSensitive: true, MaxResults: 2})
			},
			want: workspace.SearchResult{Mode: "content", Matches: []workspace.SearchMatch{
				{Path: "a.txt"}, {Path: "b.txt", Line: 2, Text: workspace.PresentText("")},
			}, Truncated: true},
		},
		{
			name: "edit", capability: "file.edit",
			arguments: `{"path":"a.txt","operation":"replace","content":"new","edits":[{"oldText":"old","newText":"new","expectedReplacements":2}]}`,
			reply:     `{"path":"a.txt","operation":"replace","replacements":2,"bytesBefore":10,"bytesAfter":12}`,
			call: func(s *Session) (any, error) {
				return s.Edit(context.Background(), workspace.EditRequest{Path: "a.txt", Operation: "replace", Content: "new",
					Edits: []workspace.TextEdit{{OldText: "old", NewText: "new", ExpectedReplacements: 2}}})
			},
			want: workspace.EditResult{Path: "a.txt", Operation: "replace", Replacements: 2, BytesBefore: 10, BytesAfter: 12},
		},
		{
			name: "shell", capability: "process.shell", arguments: `{"command":"echo fixture","cwd":"docs","timeoutMs":1000}`,
			reply: `{"exitCode":-1,"stdout":"out","stderr":"err","timedOut":true,"truncated":true}`,
			call: func(s *Session) (any, error) {
				return s.Shell(context.Background(), workspace.ShellRequest{Command: "echo fixture", Cwd: "docs", TimeoutMS: 1000})
			},
			want: workspace.ShellResult{ExitCode: -1, Stdout: "out", Stderr: "err", TimedOut: true, Truncated: true},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			session, client := workspaceSession(test.reply)
			got, err := test.call(session)
			if err != nil {
				t.Fatal(err)
			}
			got = assertResultSize(t, got, len(strings.TrimSpace(test.reply)))
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("typed operation = %#v, %v; want %#v", got, err, test.want)
			}
			if client.calls != 1 || client.request.GetSessionId() != "session:fixture" || client.request.GetCapability() != test.capability {
				t.Fatalf("v1 request = %+v, calls=%d", client.request, client.calls)
			}
			assertEqualJSON(t, client.request.GetArgumentsJson(), []byte(test.arguments))
		})
	}
}

func TestTypedWorkspaceDoesNotInjectV1RequestDefaults(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		operation string
		arguments string
	}{
		{"read", `{"path":"README.txt"}`},
		{"search", `{"mode":"files"}`},
		{"edit", `{"path":"a.txt","operation":"replace","edits":[{"oldText":"old","newText":"new"}]}`},
		{"shell", `{"command":"echo fixture"}`},
	} {
		t.Run(test.operation, func(t *testing.T) {
			session, client := workspaceSession(validResultFor(test.operation))
			if _, err := callWorkspace(session, context.Background(), test.operation); err != nil {
				t.Fatal(err)
			}
			assertEqualJSON(t, client.request.GetArgumentsJson(), []byte(test.arguments))
		})
	}
}

func TestTypedWorkspacePreservesExplicitZeroValueFields(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		in   any
		want string
	}{
		{"read zeros", workspace.ReadRequest{Path: "a", Present: workspace.FieldOffset | workspace.FieldLimit},
			`{"path":"a","offset":0,"limit":0}`},
		{"search empty and false", workspace.SearchRequest{Mode: "content", Present: workspace.FieldPath |
			workspace.FieldQuery | workspace.FieldPattern | workspace.FieldRegex | workspace.FieldCaseSensitive | workspace.FieldMaxResults},
			`{"mode":"content","path":"","query":"","pattern":"","regex":false,"caseSensitive":false,"maxResults":0}`},
		{"edit empty fields", workspace.EditRequest{Path: "a", Operation: "create", Present: workspace.FieldContent | workspace.FieldEdits},
			`{"path":"a","operation":"create","content":"","edits":[]}`},
		{"edit explicit replacement zero", workspace.EditRequest{Path: "a", Operation: "replace",
			Edits: []workspace.TextEdit{{OldText: "old", NewText: "", Present: workspace.FieldExpectedReplacements}}},
			`{"path":"a","operation":"replace","edits":[{"oldText":"old","newText":"","expectedReplacements":0}]}`},
		{"shell zeros", workspace.ShellRequest{Command: "echo fixture", Present: workspace.FieldCwd | workspace.FieldTimeoutMS},
			`{"command":"echo fixture","cwd":"","timeoutMs":0}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := marshalWorkspaceRequest(test.in)
			if err != nil {
				t.Fatal(err)
			}
			assertEqualJSON(t, got, []byte(test.want))
		})
	}
}

func TestTypedWorkspaceRejectsSchemaErrorsBeforeZeroValueDecoding(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name      string
		operation string
		reply     string
	}{
		{"not object", "read", `[]`},
		{"null object", "read", `null`},
		{"missing field", "read", `{"path":"a","content":"","startLine":0,"endLine":0}`},
		{"unknown field", "read", `{"path":"a","content":"","startLine":0,"endLine":0,"truncated":false,"extra":1}`},
		{"wrong case", "read", `{"Path":"a","content":"","startLine":0,"endLine":0,"truncated":false}`},
		{"null string", "read", `{"path":"a","content":null,"startLine":0,"endLine":0,"truncated":false}`},
		{"null number", "read", `{"path":"a","content":"","startLine":null,"endLine":0,"truncated":false}`},
		{"wrong type", "read", `{"path":"a","content":"","startLine":0,"endLine":0,"truncated":"false"}`},
		{"negative line", "read", `{"path":"a","content":"","startLine":-1,"endLine":0,"truncated":false}`},
		{"fractional line", "read", `{"path":"a","content":"","startLine":1.5,"endLine":2,"truncated":false}`},
		{"out of range line", "read", `{"path":"a","content":"","startLine":9223372036854775808,"endLine":0,"truncated":false}`},
		{"invalid search mode", "search", `{"mode":"unknown","matches":[],"truncated":false}`},
		{"null matches", "search", `{"mode":"files","matches":null,"truncated":false}`},
		{"wrong matches", "search", `{"mode":"files","matches":{},"truncated":false}`},
		{"null match", "search", `{"mode":"files","matches":[null],"truncated":false}`},
		{"missing match path", "search", `{"mode":"files","matches":[{"line":1}],"truncated":false}`},
		{"unknown match field", "search", `{"mode":"files","matches":[{"path":"a","extra":1}],"truncated":false}`},
		{"wrong match case", "search", `{"mode":"files","matches":[{"Path":"a"}],"truncated":false}`},
		{"null match text", "search", `{"mode":"content","matches":[{"path":"a","text":null}],"truncated":false}`},
		{"wrong match text", "search", `{"mode":"content","matches":[{"path":"a","text":1}],"truncated":false}`},
		{"zero explicit match line", "search", `{"mode":"content","matches":[{"path":"a","line":0}],"truncated":false}`},
		{"invalid edit operation", "edit", `{"path":"a","operation":"append","replacements":0,"bytesBefore":0,"bytesAfter":0}`},
		{"negative replacements", "edit", `{"path":"a","operation":"delete","replacements":-1,"bytesBefore":0,"bytesAfter":0}`},
		{"negative byte count", "edit", `{"path":"a","operation":"delete","replacements":0,"bytesBefore":0,"bytesAfter":-1}`},
		{"missing zero byte count", "edit", `{"path":"a","operation":"delete","replacements":0,"bytesBefore":0}`},
		{"invalid shell exit", "shell", `{"exitCode":-2,"stdout":"","stderr":"","timedOut":false,"truncated":false}`},
		{"missing false shell flag", "shell", `{"exitCode":0,"stdout":"","stderr":"","truncated":false}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			session, _ := workspaceSession(test.reply)
			_, err := callWorkspace(session, context.Background(), test.operation)
			var invalid *workspace.InvalidResultError
			if !errors.As(err, &invalid) {
				t.Fatalf("schema error = %v; want InvalidResultError", err)
			}
			if invalid.CanonicalBytes != len(strings.TrimSpace(test.reply)) {
				t.Fatalf("schema failure lost original result bytes: %d, want %d", invalid.CanonicalBytes, len(strings.TrimSpace(test.reply)))
			}
		})
	}
}

func TestTypedWorkspaceSchemaFailureRetainsTrimmedV1ByteCount(t *testing.T) {
	t.Parallel()
	raw := "\n  { \"path\" : \"a\", \"content\" : \"" + strings.Repeat("x", 1<<20) + "\" }  \n"
	session, _ := workspaceSession(raw)
	_, err := session.Read(context.Background(), workspace.ReadRequest{Path: "a"})
	var invalid *workspace.InvalidResultError
	if !errors.As(err, &invalid) || invalid.CanonicalBytes != len(strings.TrimSpace(raw)) || invalid.CanonicalBytes <= 1<<20 {
		t.Fatalf("schema failure size = %+v, %v", invalid, err)
	}
}

func TestTypedWorkspacePreservesIntegralJSONNumbers(t *testing.T) {
	t.Parallel()
	session, _ := workspaceSession(`{"path":"a","content":"","startLine":0.0,"endLine":1e0,"truncated":false}`)
	read, err := session.Read(context.Background(), workspace.ReadRequest{Path: "a"})
	if err != nil || read.StartLine != 0 || read.EndLine != 1 {
		t.Fatalf("integral decimal/exponent read = %+v, %v", read, err)
	}
	session, _ = workspaceSession(`{"path":"a","operation":"replace","replacements":2.0,"bytesBefore":9007199254740993,"bytesAfter":9e1}`)
	edit, err := session.Edit(context.Background(), workspace.EditRequest{Path: "a", Operation: "replace"})
	if err != nil || edit.Replacements != 2 || edit.BytesBefore != 9007199254740993 || edit.BytesAfter != 90 {
		t.Fatalf("integral/precise edit counts = %+v, %v", edit, err)
	}
}

func TestWorkspaceIntegerNormalizationBoundsExponentWork(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		raw   string
		want  int64
		valid bool
	}{
		{"0e999999999", 0, true},
		{"-0e-999999999", 0, true},
		{"0e99999999999999999999999999999999999999999", 0, true},
		{"1e999999999", 0, false},
		{"1e-999999999", 0, true},
		{"1e9223372036854775807", 0, false},
		{"1e-9223372036854775808", 0, true},
		{"10e-1", 1, true},
		{"0.00010e4", 1, true},
		{"1.001e2", 0, false},
		{"1.00000000000000001", 1, true},
		{"9223372036854775807.0", 9223372036854775807, true},
		{"-9.223372036854775808e18", -9223372036854775808, true},
		{"9223372036854775808", 0, false},
		{"-9223372036854775809", 0, false},
	} {
		got, valid := resultInteger(test.raw)
		if valid != test.valid || (valid && got != test.want) {
			t.Fatalf("resultInteger(%q) = %d, %v; want %d, %v", test.raw, got, valid, test.want, test.valid)
		}
	}
	// A large mantissa can cancel its exponent without constructing a large
	// power of ten. The work remains bounded by the received message bytes.
	const zeros = 1 << 20
	if got, valid := resultInteger("1" + strings.Repeat("0", zeros) + "e-1048576"); !valid || got != 1 {
		t.Fatalf("long zero tail = %d, %v; want 1, true", got, valid)
	}
	session, _ := workspaceSession(`{"path":"a","content":"","startLine":0e999999999,"endLine":-0e-999999999,"truncated":false}`)
	if result, err := session.Read(context.Background(), workspace.ReadRequest{Path: "a"}); err != nil || result.StartLine != 0 || result.EndLine != 0 {
		t.Fatalf("zero with extreme exponents = %+v, %v", result, err)
	}
}

func TestTypedWorkspaceRetainsOrdinaryTransportAndJSONErrors(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"", "!", "{} {}", "1e1000", strings.Repeat("x", maxResultJSON+1)} {
		session, _ := workspaceSession(raw)
		_, err := session.Read(context.Background(), workspace.ReadRequest{Path: "a"})
		assertOrdinaryWorkspaceError(t, err)
	}
	session, client := workspaceSession(readResultJSON)
	client.reply = nil
	_, err := session.Read(context.Background(), workspace.ReadRequest{Path: "a"})
	assertOrdinaryWorkspaceError(t, err)

	rpcErr := errors.New("transport unavailable")
	session, client = workspaceSession(readResultJSON)
	client.err = rpcErr
	_, err = session.Read(context.Background(), workspace.ReadRequest{Path: "a"})
	if !errors.Is(err, rpcErr) {
		t.Fatalf("RPC error chain lost: %v", err)
	}
	assertOrdinaryWorkspaceError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = session.Read(ctx, workspace.ReadRequest{Path: "a"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RPC context cancellation lost: %v", err)
	}
}

func TestTypedWorkspacePreservesWireSizeLimits(t *testing.T) {
	t.Parallel()
	session, client := workspaceSession(readResultJSON)
	path := strings.Repeat("a", maxWorkspaceArguments-len(`{"path":""}`))
	if _, err := session.Read(context.Background(), workspace.ReadRequest{Path: path}); err != nil {
		t.Fatalf("exact argument limit rejected: %v", err)
	}
	if len(client.request.GetArgumentsJson()) != maxWorkspaceArguments {
		t.Fatal("test did not exercise exact argument limit")
	}
	_, err := session.Read(context.Background(), workspace.ReadRequest{Path: path + "a"})
	assertOrdinaryWorkspaceError(t, err)
	if client.calls != 1 {
		t.Fatal("oversized arguments reached external service")
	}
	base := `{"path":"a","content":"","startLine":0,"endLine":0,"truncated":false}`
	raw := strings.Replace(base, `"content":""`, `"content":"`+strings.Repeat("a", maxResultJSON-len(base))+`"`, 1)
	session, client = workspaceSession(raw)
	if len(raw) != maxResultJSON {
		t.Fatal("test did not exercise exact result limit")
	}
	if _, err := session.Read(context.Background(), workspace.ReadRequest{Path: "a"}); err != nil {
		t.Fatalf("exact result limit rejected: %v", err)
	}
	client.reply.ContentJson = append(client.reply.ContentJson, ' ')
	_, err = session.Read(context.Background(), workspace.ReadRequest{Path: "a"})
	assertOrdinaryWorkspaceError(t, err)
}

func TestTypedWorkspaceRejectsUnavailableCapabilitiesBeforeRPC(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"read", "search", "edit", "shell"} {
		t.Run(operation, func(t *testing.T) {
			session, client := workspaceSession(validResultFor(operation))
			session.capabilities = sandboxdomain.Capabilities{}
			if _, err := callWorkspace(session, context.Background(), operation); !errors.Is(err, sandboxdomain.ErrCapabilityUnavailable) {
				t.Fatalf("unsupported operation = %v", err)
			}
			if client.calls != 0 {
				t.Fatal("unsupported operation reached RPC")
			}
			for _, unavailable := range []*Session{nil, {}} {
				if _, err := callWorkspace(unavailable, context.Background(), operation); !errors.Is(err, sandboxdomain.ErrBackendUnavailable) {
					t.Fatalf("unavailable session = %v", err)
				}
			}
		})
	}
}

func TestTypedWorkspaceCloseRetainsBoundedIndependentContextAndError(t *testing.T) {
	t.Parallel()
	session, client := workspaceSession(readResultJSON)
	closeErr := errors.New("session cleanup failed")
	client.closeErr = closeErr
	started := time.Now()
	if err := session.Close(); !errors.Is(err, closeErr) {
		t.Fatalf("Close error = %v; want original %v", err, closeErr)
	}
	deadline, ok := client.closeContext.Deadline()
	if !ok || deadline.Before(started) || deadline.After(started.Add(closeTimeout+time.Second)) || client.closeRequest.GetSessionId() != "session:fixture" {
		t.Fatalf("Close lost bounded identity/context: deadline=%v request=%+v", deadline, client.closeRequest)
	}
	if err := (*Session)(nil).Close(); err != nil {
		t.Fatal(err)
	}
}

func callWorkspace(session *Session, ctx context.Context, operation string) (any, error) {
	switch operation {
	case "read":
		return session.Read(ctx, workspace.ReadRequest{Path: "README.txt"})
	case "search":
		return session.Search(ctx, workspace.SearchRequest{Mode: "files"})
	case "edit":
		return session.Edit(ctx, workspace.EditRequest{Path: "a.txt", Operation: "replace",
			Edits: []workspace.TextEdit{{OldText: "old", NewText: "new"}}})
	case "shell":
		return session.Shell(ctx, workspace.ShellRequest{Command: "echo fixture"})
	default:
		panic("unknown test operation")
	}
}

func validResultFor(operation string) string {
	switch operation {
	case "read":
		return readResultJSON
	case "search":
		return `{"mode":"files","matches":[],"truncated":false}`
	case "edit":
		return `{"path":"a.txt","operation":"replace","replacements":1,"bytesBefore":3,"bytesAfter":3}`
	case "shell":
		return `{"exitCode":0,"stdout":"","stderr":"","timedOut":false,"truncated":false}`
	default:
		panic("unknown test operation")
	}
}

func assertOrdinaryWorkspaceError(t *testing.T, err error) {
	t.Helper()
	var invalid *workspace.InvalidResultError
	var tool *workspace.ToolError
	if err == nil || errors.As(err, &invalid) || errors.As(err, &tool) {
		t.Fatalf("expected ordinary execution error, got %v", err)
	}
}

func assertEqualJSON(t *testing.T, got, want []byte) {
	t.Helper()
	var gotValue, wantValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(want, &wantValue); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("v1 arguments = %s; want %s", got, want)
	}
}

func assertResultSize(t *testing.T, result any, want int) any {
	t.Helper()
	var got int
	switch value := result.(type) {
	case workspace.ReadResult:
		got, value.CanonicalBytes = value.CanonicalBytes, 0
		result = value
	case workspace.SearchResult:
		got, value.CanonicalBytes = value.CanonicalBytes, 0
		result = value
	case workspace.EditResult:
		got, value.CanonicalBytes = value.CanonicalBytes, 0
		result = value
	case workspace.ShellResult:
		got, value.CanonicalBytes = value.CanonicalBytes, 0
		result = value
	default:
		t.Fatalf("unexpected result type %T", result)
	}
	if got != want {
		t.Fatalf("original v1 result bytes = %d; want %d", got, want)
	}
	return result
}
