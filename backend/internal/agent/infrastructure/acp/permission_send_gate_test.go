package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

// A still-active request must resolve through the fenced send path and echo
// ONLY the originally advertised one-shot option ids, for both the owner
// allow and reject decisions. A durable "allow_always" id present in the
// Agent's offered envelope may never reach a response frame.
func TestACPStillActiveDispatchEchoesOnlyOriginallyOfferedOneShotOptions(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "note.txt")
	if err := os.WriteFile(target, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		toolCallID string
		decision   agentdomain.ApprovalChoice
		wantID     string
		otherID    string
	}{
		{"gate-allow", agentdomain.ApprovalAllowOnce, "offered-once", "offered-reject"},
		{"gate-reject", agentdomain.ApprovalReject, "offered-reject", "offered-once"},
	} {
		t.Run(tc.toolCallID, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			var output bytes.Buffer
			wire := &stream{
				stdin: &output, permissionSession: "gate-fixture",
				permissionRoot: root, seenRequests: map[string]struct{}{},
				permissionErrors: make(chan error, 2),
				permissionRequester: func(context.Context, agentapplication.PermissionRequest) (agentapplication.PermissionDecision, error) {
					return agentapplication.PermissionDecision{Choice: tc.decision}, nil
				},
			}
			raw := fmt.Sprintf(`{"sessionId":"gate-fixture","toolCall":{"toolCallId":%q,"kind":"read","locations":[{"path":%q}],"rawInput":{"path":%q}},"options":[{"optionId":"offered-once","kind":"allow_once"},{"optionId":"offered-reject","kind":"reject_once"},{"optionId":"never-persist","kind":"allow_always"}]}`,
				tc.toolCallID, target, target)
			if err := wire.dispatchPermission(ctx, rpcMessage{
				JSONRPC: "2.0", ID: json.RawMessage("51"), Method: "session/request_permission",
				Params: json.RawMessage(raw),
			}); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(2 * time.Second)
			for wire.pendingPermissions.Load() != 0 && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			if wire.pendingPermissions.Load() != 0 {
				t.Fatal("still-active ACP permission goroutine leaked")
			}
			text := output.String()
			if strings.Count(text, "\n") != 1 {
				t.Fatalf("still-active ACP permission must send exactly one response: %q", text)
			}
			want, other := fmt.Sprintf(`"optionId":%q,`, tc.wantID), fmt.Sprintf(`"optionId":%q`, tc.otherID)
			if !strings.Contains(text, want) || !strings.Contains(text, `"outcome":"selected"`) {
				t.Fatalf("still-active ACP permission did not select the offered %s: %q", tc.wantID, text)
			}
			if other != "" && strings.Contains(text, other) {
				t.Fatalf("still-active ACP permission echoed an unselected offered id: %q", text)
			}
			if strings.Contains(text, "never-persist") || strings.Contains(text, "allow_always") {
				t.Fatalf("still-active ACP permission selected a durable option: %q", text)
			}
			select {
			case err := <-wire.permissionErrors:
				t.Fatalf("still-active ACP permission reported a failure: %v", err)
			default:
			}
		})
	}
}

// Both the gated permission response and an unrelated live ACP frame wait for
// the same stream write lock. The Task cancels while the permission response
// is parked behind that lock, before the lock is released. Regardless of the
// acquisition order the live frame is delivered and the late one-shot is not;
// concurrent lock, buffer and context use is checked by the Race Detector.
func TestACPPermissionSendGateUnderConcurrentCancellationAndSend(t *testing.T) {
	for i := 0; i < 8; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		started := make(chan struct{})
		var output bytes.Buffer
		wire := &stream{
			stdin: &output, permissionSession: "race-fixture",
			permissionRoot: t.TempDir(), seenRequests: map[string]struct{}{},
			permissionErrors: make(chan error, 2),
			permissionRequester: func(context.Context, agentapplication.PermissionRequest) (agentapplication.PermissionDecision, error) {
				close(started)
				return agentapplication.PermissionDecision{
					Choice: agentdomain.ApprovalAllowOnce, AuthorizedByPolicy: true,
				}, nil
			},
		}
		wire.sendMu.Lock()
		raw := fmt.Sprintf(`{"sessionId":"race-fixture","toolCall":{"toolCallId":"race-%d","kind":"execute"},"options":[{"optionId":"once","kind":"allow_once"},{"optionId":"no","kind":"reject_once"}]}`, i)
		if err := wire.dispatchPermission(ctx, rpcMessage{
			JSONRPC: "2.0", ID: json.RawMessage(fmt.Sprintf("%d", i)), Method: "session/request_permission",
			Params: json.RawMessage(raw),
		}); err != nil {
			wire.sendMu.Unlock()
			cancel()
			t.Fatal(err)
		}
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			wire.sendMu.Unlock()
			cancel()
			t.Fatal("ACP permission requester never started")
		}
		// An unrelated live frame keeps contending for the shared writer lock
		// while the permission response is parked behind the same lock.
		noise := make(chan error, 1)
		go func() {
			noise <- wire.send(map[string]any{"jsonrpc": "2.0", "method": "mender/contention"})
		}()
		// Cancel strictly before releasing the protected writer lock.
		cancel()
		wire.sendMu.Unlock()
		select {
		case err := <-noise:
			if err != nil {
				t.Fatalf("unrelated ACP frame should not be blocked: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("unrelated ACP frame was starved behind the lock")
		}
		deadline := time.Now().Add(2 * time.Second)
		for wire.pendingPermissions.Load() != 0 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if wire.pendingPermissions.Load() != 0 {
			t.Fatal("cancelled ACP permission goroutine leaked")
		}
		text := output.String()
		if !strings.Contains(text, `mender/contention`) {
			t.Fatalf("live ACP frame sharing the lock was lost: %q", text)
		}
		if strings.Contains(text, `"outcome":"selected"`) || strings.Contains(text, `"optionId":"once"`) {
			t.Fatalf("late permission selection was sent to a cancelled Agent: %q", text)
		}
		select {
		case err := <-wire.permissionErrors:
			t.Fatalf("late suppression was reported as an authorization failure: %v", err)
		default:
		}
	}
}
