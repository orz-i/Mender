package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestSelectModelUsesAdvertisedACPv1Options(t *testing.T) {
	session := json.RawMessage(`{"configOptions":[{"id":"model","category":"model","type":"select","currentValue":"default","options":[{"value":"default","name":"Default"},{"value":"test-model","name":"Test"}]}]}`)
	updates := make(chan rpcMessage, 1)
	updates <- rpcMessage{JSONRPC: "2.0", ID: json.RawMessage("4"), Result: json.RawMessage(`{"configOptions":[{"id":"model","category":"model","type":"select","currentValue":"test-model","options":[{"value":"test-model","name":"Test"}]}]}`)}
	var sent bytes.Buffer
	wire := stream{stdin: &sent, updates: updates}
	if err := selectModel(context.Background(), &wire, "session-id", session, "test-model"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sent.String(), `"method":"session/set_config_option"`) || !strings.Contains(sent.String(), `"value":"test-model"`) {
		t.Fatalf("unexpected ACP config RPC: %s", sent.String())
	}
}
func TestSelectModelRejectsUnadvertisedAndUnconfirmed(t *testing.T) {
	cases := []struct {
		name    string
		session string
		result  string
	}{
		{"not advertised", `{"configOptions":[]}`, ""},
		{"not listed", `{"configOptions":[{"id":"model","category":"model","type":"select","options":[{"value":"other"}]}]}`, ""},
		{"not confirmed", `{"configOptions":[{"id":"model","category":"model","type":"select","options":[{"value":"test-model"}]}]}`, `{"configOptions":[{"id":"model","type":"select","currentValue":"other"}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var sent bytes.Buffer
			updates := make(chan rpcMessage, 1)
			if tc.result != "" {
				updates <- rpcMessage{JSONRPC: "2.0", ID: json.RawMessage("4"), Result: json.RawMessage(tc.result)}
			}
			wire := stream{stdin: &sent, updates: updates}
			if err := selectModel(context.Background(), &wire, "sess", json.RawMessage(tc.session), "test-model"); err == nil {
				t.Fatal("unadvertised/unconfirmed model accepted")
			}
			if tc.result == "" && sent.Len() != 0 {
				t.Fatal("an unadvertised model emitted ACP RPC")
			}
		})
	}
}
