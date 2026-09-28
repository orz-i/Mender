package plugin

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSearchResultNormalizeDeterministic(t *testing.T) {
	t.Parallel()

	result := SearchResult{
		Answer: "  grounded answer  ",
		Sources: []Source{
			{URL: "https://b.example/path", Title: "B"},
			{URL: "https://a.example/path", Title: "A"},
			{URL: "https://b.example/path", Title: "Duplicate"},
		},
		Queries: []string{" second ", "first", "first", ""},
	}
	normalized, err := result.Normalize()
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if normalized.Answer != "grounded answer" {
		t.Fatalf("Answer = %q", normalized.Answer)
	}
	if len(normalized.Sources) != 2 ||
		normalized.Sources[0].URL != "https://a.example/path" ||
		normalized.Sources[1].URL != "https://b.example/path" {
		t.Fatalf("Sources = %+v", normalized.Sources)
	}
	if len(normalized.Queries) != 2 ||
		normalized.Queries[0] != "first" ||
		normalized.Queries[1] != "second" {
		t.Fatalf("Queries = %+v", normalized.Queries)
	}

	raw, err := normalized.JSON()
	if err != nil {
		t.Fatal(err)
	}
	var decoded SearchResult
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Answer != normalized.Answer ||
		len(decoded.Sources) != 2 ||
		len(decoded.Queries) != 2 {
		t.Fatalf("decoded = %+v", decoded)
	}
}

func TestSearchResultRejectsInvalidSourceAndBounds(t *testing.T) {
	t.Parallel()

	tests := []SearchResult{
		{Answer: ""},
		{Answer: "ok", Sources: []Source{{URL: "/relative", Title: "x"}}},
		{Answer: "ok", Sources: []Source{{URL: "https://user@example.test", Title: "x"}}},
		{Answer: strings.Repeat("x", maxAnswerBytes+1)},
	}
	for _, result := range tests {
		if _, err := result.Normalize(); err == nil {
			t.Fatalf("Normalize(%+v) error = nil", result)
		}
	}
}

func TestDiscoveredCapabilityToolSchemas(t *testing.T) {
	t.Parallel()

	tools := discoveredCapabilityTools(Config{
		Engine:       EngineGemini,
		Model:        "gemini-model",
		Capabilities: []Capability{CapabilityWebSearch},
	})
	if len(tools) != 1 {
		t.Fatalf("tools len = %d, want 1", len(tools))
	}
	tool := tools[0]
	if tool.SourceKey != "web_search" || tool.Name != "web_search" {
		t.Fatalf("tool identity = %+v", tool)
	}
	if !json.Valid(tool.InputSchema) || !json.Valid(tool.OutputSchema) {
		t.Fatalf("tool schemas are not valid JSON")
	}
	if strings.Contains(string(tool.InputSchema), "credential") ||
		strings.Contains(string(tool.InputSchema), "api_key") {
		t.Fatalf("input schema leaked authentication metadata: %s", tool.InputSchema)
	}
}
