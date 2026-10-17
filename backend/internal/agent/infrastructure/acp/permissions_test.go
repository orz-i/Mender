package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
				if req.SessionID != "fixture-1" || req.ToolCallID != "fixture-tool" || req.Kind != "edit" ||
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
		if _, err := parseACPApproval(json.RawMessage(raw), "fixture-1"); err == nil {
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
