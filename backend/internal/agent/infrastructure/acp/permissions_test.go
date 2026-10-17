package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

func TestACPExplicitOwnerApprovalAndRejection(t *testing.T) {
	for _, tc := range []struct {
		mode     string
		decision agentdomain.ApprovalChoice
	}{
		{"approve-once", agentdomain.ApprovalAllowOnce},
		{"reject-once", agentdomain.ApprovalReject},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			agent := fixtureAgent(t)
			observed := make(chan agentapplication.PermissionRequest, 1)
			result, err := fixtureRunner(tc.mode).ExecuteConfigured(ctx, agent, "bounded approval fixture",
				agentapplication.RunConfig{WorkspaceRoot: agent.Spec().WorkspaceRoot,
					PermissionRequester: func(_ context.Context, req agentapplication.PermissionRequest) (agentapplication.PermissionDecision, error) {
						observed <- req
						return agentapplication.PermissionDecision{Choice: tc.decision}, nil
					}})
			if err != nil || result != "fixture completed" {
				t.Fatalf("ACP completed=%q error=%v", result, err)
			}
			select {
			case req := <-observed:
				if req.SessionID != "fixture-1" || req.ToolCallID != "fixture-tool" || req.Kind != "read" || req.Target != "fixture.txt" || !req.CanAllow ||
					req.AllowOptionID != "offered-once" || req.RejectOptionID != "offered-reject" {
					t.Fatalf("incorrect bounded permission projection: %+v", req)
				}
			default:
				t.Fatal("owner callback was skipped")
			}
		})
	}
}
func TestACPApprovalDoesNotImplicitlyGrantClientFilesystemMethods(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	agent := fixtureAgent(t)
	result, err := fixtureRunner("approved-client-file").ExecuteConfigured(ctx, agent, "safe fixture",
		agentapplication.RunConfig{WorkspaceRoot: agent.Spec().WorkspaceRoot,
			PermissionRequester: func(context.Context, agentapplication.PermissionRequest) (agentapplication.PermissionDecision, error) {
				return agentapplication.PermissionDecision{Choice: agentdomain.ApprovalAllowOnce}, nil
			}})
	if !errors.Is(err, errClientPermissionDenied) || result != "" {
		t.Fatalf("ACP owner permission silently advertised fs/write: %q %v", result, err)
	}
}
func TestACPRejectsMalformedPermissionEnvelopeAndUnadvertisedChoices(t *testing.T) {
	cases := []string{
		`{}`,
		`{"sessionId":"foreign","toolCall":{"toolCallId":"call-a"},"options":[{"optionId":"approve","kind":"allow_once"},{"optionId":"reject","kind":"reject_once"}]}`,
		`{"sessionId":"fixture-1","toolCall":{"toolCallId":"call-a"},"options":[{"optionId":"approve","kind":"allow_always"},{"optionId":"reject","kind":"reject_once"}]}`,
		`{"sessionId":"fixture-1","toolCall":{"toolCallId":"call-a"},"options":[{"optionId":"duplicate","kind":"allow_once"},{"optionId":"duplicate","kind":"reject_once"}]}`,
		`{"sessionId":"fixture-1","options":[{"optionId":"approve","kind":"allow_once"},{"optionId":"reject","kind":"reject_once"}]}`,
	}
	for _, raw := range cases {
		if _, err := parseACPApproval(json.RawMessage(raw), "fixture-1", t.TempDir()); err == nil {
			t.Fatalf("unsafe ACP approval accepted: %s", raw)
		}
	}
}
func TestACPRejectsDuplicateIncomingPermissionRequestsWithoutOwnerAction(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	requested := make(chan struct{}, 1)
	wire := &stream{
		stdin: &bytes.Buffer{}, permissionSession: "fixture-1",
		permissionErrors: make(chan error, 2),
		seenRequests:     map[string]struct{}{},
		permissionRequester: func(ctx context.Context, req agentapplication.PermissionRequest) (agentapplication.PermissionDecision, error) {
			requested <- struct{}{}
			<-ctx.Done()
			return agentapplication.PermissionDecision{}, ctx.Err()
		},
	}
	raw := `{"sessionId":"fixture-1","toolCall":{"toolCallId":"same-tool","kind":"edit"},"options":[{"optionId":"yes","kind":"allow_once"},{"optionId":"no","kind":"reject_once"}]}`
	message := rpcMessage{JSONRPC: "2.0", ID: json.RawMessage("17"), Method: "session/request_permission", Params: json.RawMessage(raw)}
	if err := wire.dispatchPermission(ctx, message); err != nil {
		t.Fatal(err)
	}
	<-requested
	if err := wire.dispatchPermission(ctx, message); err == nil {
		t.Fatal("duplicate RPC ID and toolCallId admitted")
	}
	message.ID = json.RawMessage("18")
	if err := wire.dispatchPermission(ctx, message); err == nil {
		t.Fatal("same toolCallId with a different RPC ID admitted")
	}
	cancel()
	deadline := time.After(2 * time.Second)
	for wire.pendingPermissions.Load() != 0 {
		select {
		case <-deadline:
			t.Fatal("cancelled permission goroutine leaked")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if strings.Contains(wire.stdin.(*bytes.Buffer).String(), "selected") {
		t.Fatal("cancellation generated an unauthorized affirmative selection")
	}
}
func TestACPReviewTargetRequiresSingleCanonicalWorkspaceFile(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(root, "visible.txt")
	if err := os.WriteFile(inside, []byte("safe fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	relative, allowed := reviewACPFileTarget(root, "read", []permissionLocation{{Path: inside}}, inside)
	if !allowed || relative != "visible.txt" {
		t.Fatalf("expected in-root read: %q %v", relative, allowed)
	}
	_, allowShell := reviewACPFileTarget(root, "execute", []permissionLocation{{Path: inside}}, inside)
	if allowShell {
		t.Fatal("shell approval based on a file hint")
	}
	outside := filepath.Join(t.TempDir(), "private.txt")
	if err := os.WriteFile(outside, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{outside, filepath.Join(root, "escape"), "../private.txt"} {
		if value, allow := reviewACPFileTarget(root, "read", []permissionLocation{{Path: target}}, target); allow {
			t.Fatalf("external/symlink file was allowed %q %q", target, value)
		}
	}
	if _, allowed := reviewACPFileTarget(root, "edit", []permissionLocation{{Path: filepath.Join(root, "safe-new.txt")}}, filepath.Join(root, "safe-new.txt")); allowed {
		t.Fatal("edit without a reviewed diff/executor was allowed")
	}
	if _, allowed := reviewACPFileTarget(root, "read", []permissionLocation{{Path: inside}, {Path: outside}}, inside); allowed {
		t.Fatal("compound target list was misrepresented as one safe file")
	}
}
func TestACPReadPreviewRequiresMatchingRawInputAndReportedLocation(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(root, "first.txt")
	second := filepath.Join(root, "second.txt")
	for _, p := range []string{first, second} {
		if err := os.WriteFile(p, []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// The Agent-reported display location cannot disagree with its own
	// argument payload and still produce a local owner Allow once button.
	for _, item := range []struct{ reported, input string }{
		{first, second}, {second, first}, {first, ""}, {first, filepath.Join(t.TempDir(), "private")},
	} {
		if value, ok := reviewACPFileTarget(root, "read", []permissionLocation{{Path: item.reported}}, item.input); ok {
			t.Fatalf("inconsistent ACP input/display was approved: %q", value)
		}
	}
	short := filepath.Join(root, "short.txt")
	if err := os.Symlink(first, short); err != nil {
		t.Fatal(err)
	}
	if value, ok := reviewACPFileTarget(root, "read", []permissionLocation{{Path: short}}, short); !ok || value != "short.txt" {
		t.Fatalf("relative symlink preview not retained for revalidation: %q %t", value, ok)
	}
}
func TestACPRejectsExcessivePermissionRequestsPerTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	called := false
	wire := &stream{stdin: &bytes.Buffer{}, permissionSession: "fixture-1",
		permissionRoot: t.TempDir(), permissionErrors: make(chan error, 2),
		seenRequests: map[string]struct{}{},
		permissionRequester: func(context.Context, agentapplication.PermissionRequest) (agentapplication.PermissionDecision, error) {
			called = true
			return agentapplication.PermissionDecision{}, nil
		},
	}
	for i := 0; i < 2*maxPermissionRequestsPerTask; i++ {
		wire.seenRequests[fmt.Sprintf("existing-%d", i)] = struct{}{}
	}
	raw := []byte(`{"sessionId":"fixture-1","toolCall":{"toolCallId":"new-tool","kind":"edit"},"options":[{"optionId":"yes","kind":"allow_once"},{"optionId":"no","kind":"reject_once"}]}`)
	message := rpcMessage{JSONRPC: "2.0", ID: json.RawMessage("111"), Method: "session/request_permission", Params: raw}
	if err := wire.dispatchPermission(ctx, message); !errors.Is(err, errClientPermissionDenied) {
		t.Fatalf("task-scoped cap not enforced: %v", err)
	}
	if called {
		t.Fatal("bounded request limit launched a callback")
	}
}
