package plugin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIWebSearchNormalizesHostedResult(t *testing.T) {
	t.Parallel()

	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-secret" {
			http.Error(w, "bad auth", http.StatusUnauthorized)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"output":[
				{
					"type":"reasoning",
					"summary":[{"type":"summary_text","text":"private reasoning"}]
				},
				{
					"type":"web_search_call",
					"action":{
						"type":"search",
						"query":"Mender latest release"
					}
				},
				{
					"type":"message",
					"content":[
						{
							"type":"output_text",
							"text":"Mender release answer.",
							"annotations":[
								{
									"type":"url_citation",
									"url":"https://example.com/release",
									"title":"Release"
								},
								{
									"type":"url_citation",
									"url":"https://example.com/release",
									"title":"Duplicate"
								}
							]
						}
					]
				}
			]
		}`))
	}))
	t.Cleanup(server.Close)

	engine := newOpenAIEngine("gpt-test")
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
	if len(result.Queries) != 1 || result.Queries[0] != "Mender latest release" {
		t.Fatalf("queries = %+v", result.Queries)
	}

	if captured["model"] != "gpt-test" {
		t.Fatalf("model = %#v", captured["model"])
	}
	if captured["input"] != "latest Mender release" {
		t.Fatalf("input = %#v", captured["input"])
	}
	if captured["instructions"] != webSearchInstruction {
		t.Fatalf("instructions = %#v", captured["instructions"])
	}
	if captured["tool_choice"] != "required" {
		t.Fatalf("tool_choice = %#v", captured["tool_choice"])
	}
	if strings.Contains(string(mustJSON(t, result)), "private reasoning") {
		t.Fatal("normalized result leaked reasoning")
	}
}

func TestOpenAIWebSearchRequiresHostedSearchCall(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"output":[
				{
					"type":"message",
					"content":[{"type":"output_text","text":"answer from memory","annotations":[]}]
				}
			]
		}`))
	}))
	t.Cleanup(server.Close)

	engine := newOpenAIEngine("gpt-test")
	engine.endpoint = server.URL
	_, err := engine.WebSearch(
		t.Context(),
		WebSearchRequest{Query: "query"},
		[]byte("secret"),
	)
	if err == nil || !IsToolError(err) ||
		!strings.Contains(err.Error(), "not executed") {
		t.Fatalf("WebSearch() error = %v, want ToolError", err)
	}
}

func TestOpenAIWebSearchRejectsMissingGroundedAnswer(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"output":[
				{"type":"web_search_call","action":{"type":"search","query":"q"}}
			]
		}`))
	}))
	t.Cleanup(server.Close)

	engine := newOpenAIEngine("gpt-test")
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

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
