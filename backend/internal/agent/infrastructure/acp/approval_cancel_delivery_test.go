package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

// A committed approval may complete inside Mender as its Task is cancelled.
// Even if the application reports a selected option, the ACP transport must
// not send a late permission to the now-cancelled Agent subprocess.
func TestACPCancelledTaskNeverSendsLateCommittedPermission(t *testing.T) {
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
