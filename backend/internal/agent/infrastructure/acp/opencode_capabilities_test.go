package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestOpenCodeACPModelEffortAndModeStayInAgentSession(t *testing.T) {
	session := json.RawMessage(`{"configOptions":[
  {"id":"model","category":"model","type":"select","currentValue":"opencode/ll-alpha-free",
   "options":[{"value":"opencode/ll-alpha-free","name":"Free"},{"value":"anthropic/claude-sonnet","name":"Sonnet"}]},
  {"id":"effort","category":"thought_level","type":"select","currentValue":"default",
   "options":[{"value":"default"},{"value":"low"},{"value":"medium"},{"value":"high"},{"value":"xhigh"},{"value":"max"}]},
  {"id":"mode","category":"mode","type":"select","currentValue":"build",
   "options":[{"value":"build"},{"value":"plan"}]}
 ]}`)
	choices, err := ReadModelChoices(session)
	if err != nil || len(choices.Models) != 2 || choices.Models[0].ID != "opencode/ll-alpha-free" ||
		choices.ReasoningEffort == nil || len(choices.ReasoningEffort.Choices) != 6 {
		t.Fatalf("OpenCode ACP model/effort capabilities invalid: %+v %v", choices, err)
	}
	updates := make(chan rpcMessage, 2)
	updates <- rpcMessage{JSONRPC: "2.0", ID: json.RawMessage("4"),
		Result: json.RawMessage(`{"configOptions":[
   {"id":"model","category":"model","type":"select","currentValue":"anthropic/claude-sonnet",
    "options":[{"value":"anthropic/claude-sonnet"}]},
   {"id":"effort","category":"thought_level","type":"select","currentValue":"default",
    "options":[{"value":"low"},{"value":"medium"},{"value":"high"}]},
   {"id":"mode","category":"mode","type":"select","currentValue":"build","options":[{"value":"build"}]}
  ]}`)}
	updates <- rpcMessage{JSONRPC: "2.0", ID: json.RawMessage("5"),
		Result: json.RawMessage(`{"configOptions":[
   {"id":"model","category":"model","type":"select","currentValue":"anthropic/claude-sonnet",
    "options":[{"value":"anthropic/claude-sonnet"}]},
   {"id":"effort","category":"thought_level","type":"select","currentValue":"high",
    "options":[{"value":"low"},{"value":"high"}]}
  ]}`)}
	var sent bytes.Buffer
	wire := stream{stdin: &sent, updates: updates}
	if err := applySessionChoices(context.Background(), &wire, "session-abc", session, "anthropic/claude-sonnet", "high"); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(sent.String()), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], `"configId":"model"`) ||
		!strings.Contains(lines[1], `"configId":"effort"`) ||
		strings.Contains(sent.String(), `"configId":"mode"`) {
		t.Fatalf("Mender hardcoded model thought-level/mode IDs: %s", sent.String())
	}
}

func TestOpenCodeACPRejectsStaleModelSpecificThoughtLevel(t *testing.T) {
	session := json.RawMessage(`{"configOptions":[
 {"id":"model","category":"model","type":"select","currentValue":"opencode/default","options":[{"value":"opencode/default"},{"value":"test/limited"}]},
 {"id":"effort","category":"thought_level","type":"select","currentValue":"default","options":[{"value":"xhigh"}]}
 ]}`)
	updates := make(chan rpcMessage, 1)
	updates <- rpcMessage{JSONRPC: "2.0", ID: json.RawMessage("4"),
		Result: json.RawMessage(`{"configOptions":[
   {"id":"model","category":"model","type":"select","currentValue":"test/limited","options":[{"value":"test/limited"}]},
   {"id":"effort","category":"thought_level","type":"select","currentValue":"default","options":[{"value":"low"},{"value":"medium"}]}
  ]}`)}
	var sent bytes.Buffer
	wire := stream{stdin: &sent, updates: updates}
	if err := applySessionChoices(context.Background(), &wire, "session-abc", session, "test/limited", "xhigh"); err == nil {
		t.Fatal("outdated OpenCode model effort accepted")
	}
	if strings.Count(sent.String(), `"method":"session/set_config_option"`) != 1 {
		t.Fatal("unadvertised effort was sent to ACP Agent")
	}
}
