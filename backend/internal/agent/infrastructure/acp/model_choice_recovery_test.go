package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

func TestACPConfiguredChoiceFailsClosedWithoutAnOutboundSelection(t *testing.T) {
	t.Parallel()
	cases := []struct {
		kind, requested, raw string
		sentinel             error
	}{
		{"model", "requested-model", `{"configOptions":[{"id":"model","category":"model","type":"select","currentValue":"other","options":[{"value":"other"}]}]}`, errACPModelNotAdvertised},
		{"reasoning_effort", "xhigh", `{"configOptions":[{"id":"reasoning_effort","category":"thought_level","type":"select","currentValue":"high","options":[{"value":"high"}]}]}`, errACPReasoningNotAdvertised},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			var outbound bytes.Buffer
			wire := &stream{stdin: &outbound}
			_, err := setACPChoice(context.Background(), wire, "fixture-session", json.RawMessage(tc.raw), tc.kind, tc.requested, 4)
			if !errors.Is(err, tc.sentinel) || !errors.Is(err, agentdomain.ErrUnavailable) {
				t.Fatalf("unadvertised configured choice must be rejected: %v", err)
			}
			if outbound.Len() != 0 {
				t.Fatal("unadvertised model or effort sent a configuration RPC")
			}
			stageErr := tagChoiceConfigurationError(err)
			if strings.Contains(stageErr.Error(), tc.requested) {
				t.Fatal("owner-facing stage leaked the model/effort value")
			}
		})
	}
}

func TestACPConfiguredChoiceRequiresExplicitConfirmation(t *testing.T) {
	const session = `{"configOptions":[{"id":"model","category":"model","type":"select","currentValue":"other","options":[{"value":"other"},{"value":"selected"}]}]}`
	var outbound bytes.Buffer
	updates := make(chan rpcMessage, 1)
	updates <- rpcMessage{
		JSONRPC: "2.0", ID: json.RawMessage("4"),
		Result: json.RawMessage(`{"configOptions":[{"id":"model","category":"model","type":"select","currentValue":"other"}]}`),
	}
	wire := &stream{stdin: &outbound, updates: updates}
	_, err := setACPChoice(context.Background(), wire, "fixture-session", json.RawMessage(session), "model", "selected", 4)
	if !errors.Is(err, errACPChoiceNotConfirmed) || !errors.Is(err, agentdomain.ErrUnavailable) {
		t.Fatalf("wrong ACP option confirmation was not rejected: %v", err)
	}
	if !strings.Contains(outbound.String(), `"value":"selected"`) {
		t.Fatal("test did not send the requested model choice")
	}
}

func TestACPConfiguredChoiceRejectsRemoteRPCWithoutErrorDisclosure(t *testing.T) {
	var outbound bytes.Buffer
	updates := make(chan rpcMessage, 1)
	updates <- rpcMessage{
		JSONRPC: "2.0", ID: json.RawMessage("4"),
		Error: &rpcError{Code: -32001, Message: "SIMULATED_PRIVATE_RPC_PAYLOAD"},
	}
	wire := &stream{stdin: &outbound, updates: updates}
	const session = `{"configOptions":[{"id":"model","category":"model","type":"select","currentValue":"other","options":[{"value":"other"},{"value":"selected"}]}]}`
	_, err := setACPChoice(context.Background(), wire, "fixture-session", json.RawMessage(session), "model", "selected", 4)
	if !errors.Is(err, errACPChoiceRequestFailed) {
		t.Fatalf("remote ACP configuration rejection lost the fixed cause: %v", err)
	}
	fixed := tagChoiceConfigurationError(err)
	var staged interface{ SafeAgentFailureStage() string }
	if !errors.As(fixed, &staged) || staged.SafeAgentFailureStage() != stageConfigRequestFailed ||
		strings.Contains(fixed.Error(), "SIMULATED_PRIVATE_RPC_PAYLOAD") {
		t.Fatalf("unsafe ACP configuration Task stage: %v", fixed)
	}
}
