package plugin

import (
	"context"
	"net/http"
	"strings"
)

const geminiInteractionsEndpoint = "https://generativelanguage.googleapis.com/v1beta/interactions"

type geminiEngine struct {
	model    string
	endpoint string
	http     engineHTTPClient
}

func newGeminiEngine(model string) *geminiEngine {
	return &geminiEngine{
		model:    model,
		endpoint: geminiInteractionsEndpoint,
		http:     newEngineHTTPClient(),
	}
}

func (e *geminiEngine) WebSearch(
	ctx context.Context,
	request WebSearchRequest,
	credential []byte,
) (SearchResult, error) {
	type tool struct {
		Type string `json:"type"`
	}
	type payload struct {
		Model             string `json:"model"`
		Input             string `json:"input"`
		SystemInstruction string `json:"system_instruction"`
		Tools             []tool `json:"tools"`
		Store             bool   `json:"store"`
	}
	type annotation struct {
		Type  string `json:"type"`
		URL   string `json:"url"`
		Title string `json:"title"`
	}
	type contentBlock struct {
		Type        string       `json:"type"`
		Text        string       `json:"text"`
		Annotations []annotation `json:"annotations"`
	}
	type arguments struct {
		Queries []string `json:"queries"`
		Query   string   `json:"query"`
	}
	type step struct {
		Type      string         `json:"type"`
		Arguments arguments      `json:"arguments"`
		Content   []contentBlock `json:"content"`
	}
	type response struct {
		Status string `json:"status"`
		Steps  []step `json:"steps"`
	}

	headers := make(http.Header)
	headers.Set("x-goog-api-key", string(credential))

	var decoded response
	err := e.http.postJSON(
		ctx,
		e.endpoint,
		headers,
		payload{
			Model:             e.model,
			Input:             request.Query,
			SystemInstruction: webSearchInstruction,
			Tools:             []tool{{Type: "google_search"}},
			Store:             false,
		},
		&decoded,
	)
	if err != nil {
		return SearchResult{}, err
	}
	if decoded.Status != "" && decoded.Status != "completed" {
		return SearchResult{}, newToolError(
			"hosted web search interaction did not complete",
		)
	}

	searchExecuted := false
	var answerParts []string
	var sources []Source
	var queries []string

	for _, step := range decoded.Steps {
		switch step.Type {
		case "google_search_call":
			searchExecuted = true
			if query := strings.TrimSpace(step.Arguments.Query); query != "" {
				queries = append(queries, query)
			}
			queries = append(queries, step.Arguments.Queries...)

		case "model_output":
			for _, block := range step.Content {
				if block.Type != "text" {
					continue
				}
				if text := strings.TrimSpace(block.Text); text != "" {
					answerParts = append(answerParts, text)
				}
				for _, citation := range block.Annotations {
					if citation.Type != "url_citation" {
						continue
					}
					sources = append(sources, Source{
						URL:   citation.URL,
						Title: citation.Title,
					})
				}
			}
		}
	}

	if !searchExecuted {
		return SearchResult{}, newToolError("hosted web search was not executed")
	}
	normalized, err := (SearchResult{
		Answer:  strings.Join(answerParts, "\n"),
		Sources: sources,
		Queries: queries,
	}).Normalize()
	if err != nil {
		return SearchResult{}, newToolError(
			"hosted web search returned an invalid grounded result",
		)
	}
	return normalized, nil
}

var _ EngineAdapter = (*geminiEngine)(nil)
