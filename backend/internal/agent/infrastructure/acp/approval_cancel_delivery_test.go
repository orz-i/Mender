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

// A committed approval may complete inside Mender while its Task cancels.
// When cancellation is already visible before the callback returns, ACP
// must not send the late one-shot option. This test does NOT prove an atomic
// check-and-write gate for cancellation concurrent with the final send.
func TestACPPreCancelledTaskDoesNotSendCommittedPermission(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	release := make(chan struct{})
	var output bytes.Buffer
	wire := &stream{
		stdin: &output, permissionSession: "cancelled-fixture",
		permissionRoot: t.TempDir(), seenRequests: map[string]struct{}{},
		permissionErrors: make(chan error, 2),
		permissionRequester: func(context.Context, agentapplication.PermissionRequest) (agentapplication.PermissionDecision, error) {
			close(started)
			<-release
			// Simulates a one-shot selection already committed by the
			// application while its surrounding Task is being cancelled.
			return agentapplication.PermissionDecision{
				Choice: agentdomain.ApprovalAllowOnce, AuthorizedByPolicy: true,
			}, nil
		},
	}
	raw := `{"sessionId":"cancelled-fixture","toolCall":{"toolCallId":"late-code","kind":"execute"},"options":[{"optionId":"once","kind":"allow_once"},{"optionId":"no","kind":"reject_once"}]}`
	if err := wire.dispatchPermission(ctx, rpcMessage{
		JSONRPC: "2.0", ID: json.RawMessage("11"), Method: "session/request_permission",
		Params: json.RawMessage(raw),
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("ACP permission requester never started")
	}
	cancel()
	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for wire.pendingPermissions.Load() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if wire.pendingPermissions.Load() != 0 {
		t.Fatal("cancelled ACP permission goroutine leaked")
	}
	if strings.Contains(output.String(), `"outcome":"selected"`) || strings.Contains(output.String(), `"optionId":"once"`) {
		t.Fatal("ACP sent a committed but late permission to a cancelled Agent")
	}
}

func TestACPPermissionSendRechecksCancellationAfterAcquiringWriterLock(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var output bytes.Buffer
	wire := &stream{stdin: &output}

	wire.sendMu.Lock()
	started := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		close(started)
		finished <- wire.sendPermission(ctx, map[string]any{
			"jsonrpc": "2.0", "id": 41,
			"result": map[string]any{"outcome": map[string]string{
				"outcome": "selected", "optionId": "once",
			}},
		})
	}()
	<-started
	// The ACP permission cannot write while another frame holds sendMu.
	// Cancellation must be rechecked when that lock becomes available.
	cancel()
	wire.sendMu.Unlock()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled ACP permission send returned %v; want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled ACP permission send was stuck behind the lock")
	}
	if output.Len() != 0 {
		t.Fatalf("ACP wrote a selected permission after cancellation: %q", output.String())
	}
}

func TestACPLivePermissionSendPreservesOfferedOneShotChoice(t *testing.T) {
	var output bytes.Buffer
	wire := &stream{stdin: &output}
	if err := wire.sendPermission(context.Background(), map[string]any{
		"jsonrpc": "2.0", "id": 42,
		"result": map[string]any{"outcome": map[string]string{
			"outcome": "selected", "optionId": "offered-once",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"optionId":"offered-once"`) ||
		!strings.Contains(output.String(), `"outcome":"selected"`) {
		t.Fatalf("live ACP permission choice missing from response: %s", output.String())
	}
	if strings.Contains(output.String(), "allow_always") {
		t.Fatal("ACP permission transport invented a persistent approval")
	}
}

func TestACPCancelledPermissionBlockedBehindAnotherResponse(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	requestReady := make(chan struct{})
	var output bytes.Buffer
	wire := &stream{
		stdin:             &output,
		permissionSession: "blocked-writer-fixture",
		permissionRoot:    t.TempDir(),
		seenRequests:      map[string]struct{}{},
		permissionErrors:  make(chan error, 2),
		permissionRequester: func(context.Context, agentapplication.PermissionRequest) (agentapplication.PermissionDecision, error) {
			close(requestReady)
			return agentapplication.PermissionDecision{
				Choice:             agentdomain.ApprovalAllowOnce,
				AuthorizedByPolicy: true,
			}, nil
		},
	}
	wire.sendMu.Lock()
	raw := `{"sessionId":"blocked-writer-fixture","toolCall":{"toolCallId":"execute-under-cancellation","kind":"execute"},"options":[{"optionId":"once","kind":"allow_once"},{"optionId":"no","kind":"reject_once"}]}`
	if err := wire.dispatchPermission(ctx, rpcMessage{
		JSONRPC: "2.0", ID: json.RawMessage("43"), Method: "session/request_permission",
		Params: json.RawMessage(raw),
	}); err != nil {
		wire.sendMu.Unlock()
		t.Fatal(err)
	}
	select {
	case <-requestReady:
	case <-time.After(2 * time.Second):
		wire.sendMu.Unlock()
		t.Fatal("permission requester never reached the pending send")
	}
	cancel()
	wire.sendMu.Unlock()
	deadline := time.Now().Add(2 * time.Second)
	for wire.pendingPermissions.Load() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if wire.pendingPermissions.Load() != 0 {
		t.Fatal("cancelled permission goroutine leaked")
	}
	if strings.Contains(output.String(), `"outcome":"selected"`) {
		t.Fatal("late execute approval was sent after task cancellation")
	}
	select {
	case <-wire.permissionErrors:
		t.Fatal("Task cancellation was incorrectly reported as a new permission failure")
	default:
	}
}
