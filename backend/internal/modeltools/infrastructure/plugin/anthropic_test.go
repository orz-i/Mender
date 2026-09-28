package plugin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestAnthropicWebSearchNormalizesServerToolResult(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-api-key"); got != "test-secret" {
			http.Error(w, "bad auth", http.StatusUnauthorized)
			return
		}
		if got := r.Header.Get("anthropic-version"); got != anthropicVersion {
			http.Error(w, "bad version", http.StatusBadRequest)
			return
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request["system"] != webSearchInstruction {
			t.Fatalf("system = %#v", request["system"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"stop_reason":"end_turn",
			"content":[
				{"type":"text","text":"I will search first."},
				{
					"type":"server_tool_use",
					"id":"srv_1",
					"name":"web_search",
					"input":{"query":"Mender release"}
				},
				{
					"type":"web_search_tool_result",
					"tool_use_id":"srv_1",
					"content":[
						{
							"type":"web_search_result",
							"url":"https://example.com/release",
							"title":"Release",
							"encrypted_content":"do-not-expose"
						}
					]
				},
				{
					"type":"text",
					"text":"Mender release answer.",
					"citations":[
						{
							"type":"web_search_result_location",
							"url":"https://example.com/release",
							"title":"Release",
							"encrypted_index":"internal",
							"cited_text":"text"
						}
					]
				}
			]
		}`))
	}))
	t.Cleanup(server.Close)

	engine := newAnthropicEngine("claude-test")
	engine.endpoint = server.URL
	result, err := engine.WebSearch(
		t.Context(),
		WebSearchRequest{Query: "latest Mender release"},
		[]byte("test-secret"),
	)
	if err != nil {
		t.Fatalf("WebSearch() error = %v", err)
	}
	if result.Answer != "Mender release answer." {
		t.Fatalf("answer = %q", result.Answer)
	}
	if len(result.Sources) != 1 ||
		result.Sources[0].URL != "https://example.com/release" {
		t.Fatalf("sources = %+v", result.Sources)
	}
	if len(result.Queries) != 1 || result.Queries[0] != "Mender release" {
		t.Fatalf("queries = %+v", result.Queries)
	}
	raw := string(mustJSON(t, result))
	if strings.Contains(raw, "do-not-expose") ||
		strings.Contains(raw, "internal") ||
		strings.Contains(raw, "I will search first") {
		t.Fatalf("normalized result leaked transient provider state: %s", raw)
	}
}

func TestAnthropicWebSearchContinuesPauseTurnWithRawContent(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		messages, _ := request["messages"].([]any)
		if call == 1 {
			if len(messages) != 1 {
				t.Fatalf("first messages = %#v", messages)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"stop_reason":"pause_turn",
				"content":[
					{
						"type":"server_tool_use",
						"id":"srv_1",
						"name":"web_search",
						"input":{"query":"q"}
					},
					{
						"type":"web_search_tool_result",
						"tool_use_id":"srv_1",
						"content":[
							{
								"type":"web_search_result",
								"url":"https://example.com/a",
								"title":"A",
								"encrypted_content":"preserve-me"
							}
						]
					}
				]
			}`))
			return
		}

		if len(messages) != 2 {
			t.Fatalf("continuation messages = %#v", messages)
		}
		assistant, _ := messages[1].(map[string]any)
		rawAssistant := string(mustJSON(t, assistant))
		if !strings.Contains(rawAssistant, "preserve-me") {
			t.Fatalf("continuation lost encrypted content: %s", rawAssistant)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"stop_reason":"end_turn",
			"content":[
				{"type":"text","text":"Final answer.","citations":[{"type":"web_search_result_location","url":"https://example.com/a","title":"A"}]}
			]
		}`))
	}))
	t.Cleanup(server.Close)

	engine := newAnthropicEngine("claude-test")
	engine.endpoint = server.URL
	result, err := engine.WebSearch(
		t.Context(),
		WebSearchRequest{Query: "query"},
		[]byte("secret"),
	)
	if err != nil {
		t.Fatalf("WebSearch() error = %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls = %d, want 2", calls.Load())
	}
	if result.Answer != "Final answer." {
		t.Fatalf("answer = %q", result.Answer)
	}
	if strings.Contains(string(mustJSON(t, result)), "preserve-me") {
		t.Fatal("canonical result leaked encrypted continuation state")
	}
}

func TestAnthropicWebSearchRequiresHostedCall(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"stop_reason":"end_turn",
			"content":[{"type":"text","text":"memory answer"}]
		}`))
	}))
	t.Cleanup(server.Close)

	engine := newAnthropicEngine("claude-test")
	engine.endpoint = server.URL
	_, err := engine.WebSearch(
		t.Context(),
		WebSearchRequest{Query: "query"},
		[]byte("secret"),
	)
	if err == nil || !IsToolError(err) {
		t.Fatalf("WebSearch() error = %v, want ToolError", err)
	}
}

func TestAnthropicWebSearchMapsToolResultError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"stop_reason":"end_turn",
			"content":[
				{"type":"server_tool_use","id":"srv_1","name":"web_search","input":{"query":"q"}},
				{"type":"web_search_tool_result","tool_use_id":"srv_1","content":{"type":"web_search_tool_result_error","error_code":"rate_limit"}}
			]
		}`))
	}))
	t.Cleanup(server.Close)

	engine := newAnthropicEngine("claude-test")
	engine.endpoint = server.URL
	_, err := engine.WebSearch(
		t.Context(),
		WebSearchRequest{Query: "query"},
		[]byte("secret"),
	)
	if err == nil || !IsToolError(err) {
		t.Fatalf("WebSearch() error = %v, want ToolError", err)
	}
}
