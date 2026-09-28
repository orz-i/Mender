package plugin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGeminiWebSearchNormalizesGroundedInteraction(t *testing.T) {
	t.Parallel()

	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-goog-api-key"); got != "test-secret" {
			http.Error(w, "bad auth", http.StatusUnauthorized)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status":"completed",
			"steps":[
				{
					"type":"thought",
					"summary":[{"type":"text","text":"private thought"}],
					"signature":"private-signature"
				},
				{
					"type":"google_search_call",
					"arguments":{"queries":["Mender release"]}
				},
				{
					"type":"google_search_result",
					"result":[{"search_suggestions":"<html>do-not-expose</html>"}]
				},
				{
					"type":"model_output",
					"content":[
						{
							"type":"text",
							"text":"Mender release answer.",
							"annotations":[
								{
									"type":"url_citation",
									"url":"https://example.com/release",
									"title":"Release"
								}
							]
						}
					]
				}
			]
		}`))
	}))
	t.Cleanup(server.Close)

	engine := newGeminiEngine("gemini-test")
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

	if captured["model"] != "gemini-test" ||
		captured["input"] != "latest Mender release" ||
		captured["system_instruction"] != webSearchInstruction ||
		captured["store"] != false {
		t.Fatalf("request = %#v", captured)
	}
	raw := string(mustJSON(t, result))
	for _, forbidden := range []string{
		"private thought",
		"private-signature",
		"do-not-expose",
	} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("normalized result leaked %q: %s", forbidden, raw)
		}
	}
}

func TestGeminiWebSearchRequiresHostedSearchCall(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status":"completed",
			"steps":[
				{
					"type":"model_output",
					"content":[{"type":"text","text":"memory answer"}]
				}
			]
		}`))
	}))
	t.Cleanup(server.Close)

	engine := newGeminiEngine("gemini-test")
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

func TestGeminiWebSearchRequiresCompletedInteraction(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status":"incomplete",
			"steps":[{"type":"google_search_call","arguments":{"queries":["q"]}}]
		}`))
	}))
	t.Cleanup(server.Close)

	engine := newGeminiEngine("gemini-test")
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
