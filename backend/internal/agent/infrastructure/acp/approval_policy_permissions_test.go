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

func TestACPUnreviewableNativeActionOnlyReceivesOfferedOnceFromVerifiedOwnerPolicy(t *testing.T) {
	for _, test := range []struct {
		name         string
		policy       bool
		wantSelected bool
	}{
		{"no policy cannot bypass", false, false},
		{"explicit policy selects original offered once", true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			var output bytes.Buffer
			stream := &stream{
				stdin:             &output,
				permissionSession: "task-session",
				permissionRoot:    t.TempDir(),
				seenRequests:      map[string]struct{}{},
				permissionErrors:  make(chan error, 2),
				permissionRequester: func(context.Context, agentapplication.PermissionRequest) (agentapplication.PermissionDecision, error) {
					return agentapplication.PermissionDecision{Choice: agentdomain.ApprovalAllowOnce, AuthorizedByPolicy: test.policy}, nil
				},
			}
			raw := `{"sessionId":"task-session","toolCall":{"toolCallId":"edit-without-preview","kind":"execute"},"options":[{"optionId":"one-action-only","kind":"allow_once"},{"optionId":"rejected","kind":"reject_once"},{"optionId":"never-use-this","kind":"allow_always"}]}`
			if err := stream.dispatchPermission(ctx, rpcMessage{JSONRPC: "2.0", ID: json.RawMessage("10"), Method: "session/request_permission", Params: json.RawMessage(raw)}); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(2 * time.Second)
			for stream.pendingPermissions.Load() != 0 && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			if stream.pendingPermissions.Load() != 0 {
				t.Fatal("permission goroutine stuck")
			}
			result := output.String()
			selected := strings.Contains(result, `"outcome":"selected"`) && strings.Contains(result, `"optionId":"one-action-only"`)
			if selected != test.wantSelected {
				t.Fatalf("policy provenance gate was bypassed: selected=%t", selected)
			}
			if strings.Contains(result, "never-use-this") {
				t.Fatal("ACP allowed a durable allow_always choice")
			}
		})
	}
}
