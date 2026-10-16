package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

func TestACPReasoningEffortAdvertisedAndConfirmedAfterModel(t *testing.T) {
	session := json.RawMessage(`{"configOptions":[{"id":"model","category":"model","type":"select","currentValue":"old","options":[{"value":"new"}]},{"id":"reasoning_effort","category":"thought_level","type":"select","currentValue":"low","options":[{"value":"low"}]}]}`)
	updates := make(chan rpcMessage, 2)
	updates <- rpcMessage{JSONRPC: "2.0", ID: json.RawMessage("4"), Result: json.RawMessage(`{"configOptions":[{"id":"model","category":"model","type":"select","currentValue":"new","options":[{"value":"new"}]},{"id":"reasoning_effort","category":"thought_level","type":"select","currentValue":"medium","options":[{"value":"medium"},{"value":"xhigh"}]}]}`)}
	updates <- rpcMessage{JSONRPC: "2.0", ID: json.RawMessage("5"), Result: json.RawMessage(`{"configOptions":[{"id":"model","category":"model","type":"select","currentValue":"new","options":[{"value":"new"}]},{"id":"reasoning_effort","category":"thought_level","type":"select","currentValue":"xhigh","options":[{"value":"xhigh"}]}]}`)}
	var sent bytes.Buffer
	wire := stream{stdin: &sent, updates: updates}
	if err := applySessionChoices(context.Background(), &wire, "fixture", session, "new", "xhigh"); err != nil {
		t.Fatal(err)
	}
	calls := strings.Split(strings.TrimSpace(sent.String()), "\n")
	if len(calls) != 2 || !strings.Contains(calls[0], `"configId":"model"`) || !strings.Contains(calls[1], `"configId":"reasoning_effort"`) {
		t.Fatalf("wrong ACP option ordering %s", sent.String())
	}
	raw := json.RawMessage(`{"configOptions":[{"id":"reasoning_effort","category":"thought_level","type":"select","currentValue":"high","options":[{"value":"xhigh","name":"Extra high"},{"value":"high","name":"High"}]}]}`)
	result, err := ReadModelChoices(raw)
	if err != nil || result.ReasoningEffort == nil || result.ReasoningEffort.Current != "high" || len(result.ReasoningEffort.Choices) != 2 {
		t.Fatalf("thought-level-only session not projected %+v err %v", result, err)
	}
}
func TestACPReasoningUnsupportedFailsClosed(t *testing.T) {
	for _, raw := range []string{
		`{"configOptions":[{"id":"model","category":"model","type":"select","options":[{"value":"new"}]}]}`,
		`{"configOptions":[{"id":"reasoning_effort","category":"thought_level","type":"select","options":[{"value":"low"}]}]}`,
	} {
		var output bytes.Buffer
		wire := stream{stdin: &output, updates: make(chan rpcMessage)}
		if err := applySessionChoices(context.Background(), &wire, "fixture", json.RawMessage(raw), "", "xhigh"); err == nil {
			t.Fatalf("unavailable effort accepted: %s", raw)
		}
		if output.Len() != 0 {
			t.Fatal("sent ACP command without advertisement")
		}
	}
	for _, v := range []string{"XHIGH", " high", "high;rm", "../high", strings.Repeat("x", 65)} {
		if err := agentdomain.ValidateReasoningEffort(v); err == nil {
			t.Fatalf("invalid value allowed %q", v)
		}
	}
}
func TestACPReasoningUnconfirmedFailsClosed(t *testing.T) {
	raw := json.RawMessage(`{"configOptions":[{"id":"reasoning_effort","category":"thought_level","type":"select","options":[{"value":"high"}]}]}`)
	updates := make(chan rpcMessage, 1)
	updates <- rpcMessage{JSONRPC: "2.0", ID: json.RawMessage("5"), Result: json.RawMessage(`{"configOptions":[{"id":"reasoning_effort","category":"thought_level","type":"select","currentValue":"low","options":[{"value":"high"}]}]}`)}
	var output bytes.Buffer
	wire := stream{stdin: &output, updates: updates}
	if err := applySessionChoices(context.Background(), &wire, "fixture", raw, "", "high"); err == nil {
		t.Fatal("unconfirmed thought level accepted")
	}
}
