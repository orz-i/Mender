package plugin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCrossEngineWebSearchNormalizesToSameCanonicalJSON(t *testing.T) {
	t.Parallel()

	const (
		answer = "Same grounded answer."
		query  = "same provider query"
		url    = "https://example.com/source"
		title  = "Source"
		secret = "shared-test-secret"
	)

	openAIServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"output":[
				{"type":"web_search_call","action":{"type":"search","query":"same provider query"}},
				{"type":"message","content":[{"type":"output_text","text":"Same grounded answer.","annotations":[{"type":"url_citation","url":"https://example.com/source","title":"Source"}]}]}
			]
		}`))
	}))
	t.Cleanup(openAIServer.Close)

	anthropicServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"stop_reason":"end_turn",
			"content":[
				{"type":"server_tool_use","id":"srv_1","name":"web_search","input":{"query":"same provider query"}},
				{"type":"web_search_tool_result","tool_use_id":"srv_1","content":[{"type":"web_search_result","url":"https://example.com/source","title":"Source","encrypted_content":"private"}]},
				{"type":"text","text":"Same grounded answer.","citations":[{"type":"web_search_result_location","url":"https://example.com/source","title":"Source"}]}
			]
		}`))
	}))
	t.Cleanup(anthropicServer.Close)

	geminiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status":"completed",
			"steps":[
				{"type":"google_search_call","arguments":{"queries":["same provider query"]}},
				{"type":"thought","summary":[{"type":"text","text":"private"}]},
				{"type":"model_output","content":[{"type":"text","text":"Same grounded answer.","annotations":[{"type":"url_citation","url":"https://example.com/source","title":"Source"}]}]}
			]
		}`))
	}))
	t.Cleanup(geminiServer.Close)

	openAI := newOpenAIEngine("openai-model")
	openAI.endpoint = openAIServer.URL
	anthropic := newAnthropicEngine("anthropic-model")
	anthropic.endpoint = anthropicServer.URL
	gemini := newGeminiEngine("gemini-model")
	gemini.endpoint = geminiServer.URL

	engines := []EngineAdapter{openAI, anthropic, gemini}
	var canonical string
	for index, engine := range engines {
		result, err := engine.WebSearch(
			t.Context(),
			WebSearchRequest{Query: "caller query"},
			[]byte(secret),
		)
		if err != nil {
			t.Fatalf("engine %d WebSearch() error = %v", index, err)
		}
		if result.Answer != answer ||
			len(result.Sources) != 1 ||
			result.Sources[0].URL != url ||
			result.Sources[0].Title != title ||
			len(result.Queries) != 1 ||
			result.Queries[0] != query {
			t.Fatalf("engine %d result = %+v", index, result)
		}
		raw, err := result.JSON()
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), secret) ||
			strings.Contains(string(raw), "private") {
			t.Fatalf("engine %d canonical JSON leaked provider state: %s", index, raw)
		}
		if index == 0 {
			canonical = string(raw)
		} else if string(raw) != canonical {
			t.Fatalf(
				"engine %d canonical JSON differs:\nfirst=%s\ncurrent=%s",
				index,
				canonical,
				raw,
			)
		}
	}
}

func TestAnthropicContinuationLimitIsBounded(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"stop_reason":"pause_turn",
			"content":[
				{"type":"server_tool_use","id":"srv","name":"web_search","input":{"query":"q"}},
				{"type":"web_search_tool_result","tool_use_id":"srv","content":[{"type":"web_search_result","url":"https://example.com","title":"Example","encrypted_content":"transient"}]}
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
	if err == nil || !strings.Contains(err.Error(), "continuation exceeded") {
		t.Fatalf("WebSearch() error = %v, want continuation limit", err)
	}
	wantCalls := int32(anthropicContinuationLimit + 1)
	if calls.Load() != wantCalls {
		t.Fatalf("calls = %d, want %d", calls.Load(), wantCalls)
	}
	if strings.Contains(err.Error(), "secret") ||
		strings.Contains(err.Error(), "transient") {
		t.Fatalf("continuation error leaked secret/provider state: %v", err)
	}
}

func TestSearchResultCountAndFieldBounds(t *testing.T) {
	t.Parallel()

	t.Run("sources", func(t *testing.T) {
		t.Parallel()
		sources := make([]Source, maxSources+1)
		for index := range sources {
			sources[index] = Source{
				URL:   "https://example.com/" + strings.Repeat("a", index%3),
				Title: "Source",
			}
		}
		_, err := (SearchResult{
			Answer:  "answer",
			Sources: sources,
		}).Normalize()
		if err == nil || !strings.Contains(err.Error(), "sources exceed") {
			t.Fatalf("Normalize() error = %v", err)
		}
	})

	t.Run("queries", func(t *testing.T) {
		t.Parallel()
		queries := make([]string, maxQueries+1)
		for index := range queries {
			queries[index] = "q"
		}
		_, err := (SearchResult{
			Answer:  "answer",
			Queries: queries,
		}).Normalize()
		if err == nil || !strings.Contains(err.Error(), "queries exceed") {
			t.Fatalf("Normalize() error = %v", err)
		}
	})

	t.Run("source title", func(t *testing.T) {
		t.Parallel()
		_, err := (SearchResult{
			Answer: "answer",
			Sources: []Source{{
				URL:   "https://example.com",
				Title: strings.Repeat("x", maxSourceTitleBytes+1),
			}},
		}).Normalize()
		if err == nil || !strings.Contains(err.Error(), "source title exceeds") {
			t.Fatalf("Normalize() error = %v", err)
		}
	})

	t.Run("query bytes", func(t *testing.T) {
		t.Parallel()
		_, err := (SearchResult{
			Answer:  "answer",
			Queries: []string{strings.Repeat("x", maxQueryBytes+1)},
		}).Normalize()
		if err == nil || !strings.Contains(err.Error(), "query exceeds") {
			t.Fatalf("Normalize() error = %v", err)
		}
	})
}

func TestEngineProviderErrorNeverIncludesCredential(t *testing.T) {
	t.Parallel()

	const secret = "model-api-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Echoing the credential in the body simulates a hostile/overly verbose
		// provider error. Mender must not include it in the surfaced error.
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("credential=" + secret))
	}))
	t.Cleanup(server.Close)

	engines := []EngineAdapter{
		func() EngineAdapter {
			engine := newOpenAIEngine("m")
			engine.endpoint = server.URL
			return engine
		}(),
		func() EngineAdapter {
			engine := newAnthropicEngine("m")
			engine.endpoint = server.URL
			return engine
		}(),
		func() EngineAdapter {
			engine := newGeminiEngine("m")
			engine.endpoint = server.URL
			return engine
		}(),
	}
	for index, engine := range engines {
		_, err := engine.WebSearch(
			t.Context(),
			WebSearchRequest{Query: "q"},
			[]byte(secret),
		)
		if err == nil {
			t.Fatalf("engine %d error = nil", index)
		}
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("engine %d error leaked credential: %v", index, err)
		}
		if !strings.Contains(err.Error(), "401 Unauthorized") {
			t.Fatalf("engine %d error = %v", index, err)
		}
	}
}
