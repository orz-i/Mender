package plugin

import (
	"context"
	"net/http"
	"strings"
)

const openAIResponsesEndpoint = "https://api.openai.com/v1/responses"

type openAIEngine struct {
	model    string
	endpoint string
	http     engineHTTPClient
}

func newOpenAIEngine(model string) *openAIEngine {
	return &openAIEngine{
		model:    model,
		endpoint: openAIResponsesEndpoint,
		http:     newEngineHTTPClient(),
	}
}

func (e *openAIEngine) WebSearch(
	ctx context.Context,
	request WebSearchRequest,
	credential []byte,
) (SearchResult, error) {
	type tool struct {
		Type string `json:"type"`
	}
	type payload struct {
		Model        string   `json:"model"`
		Instructions string   `json:"instructions"`
		Input        string   `json:"input"`
		Tools        []tool   `json:"tools"`
		ToolChoice   string   `json:"tool_choice"`
		Include      []string `json:"include,omitempty"`
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
	type action struct {
		Type    string   `json:"type"`
		Query   string   `json:"query"`
		Queries []string `json:"queries"`
	}
	type output struct {
		Type    string         `json:"type"`
		Action  action         `json:"action"`
		Content []contentBlock `json:"content"`
	}
	type response struct {
		Output []output `json:"output"`
	}

	headers := make(http.Header)
	headers.Set("Authorization", "Bearer "+string(credential))
	var decoded response
	err := e.http.postJSON(
		ctx,
		e.endpoint,
		headers,
		payload{
			Model:        e.model,
			Instructions: webSearchInstruction,
			Input:        request.Query,
			Tools:        []tool{{Type: "web_search"}},
			ToolChoice:   "required",
			Include:      []string{"web_search_call.action.sources"},
		},
		&decoded,
	)
	if err != nil {
		return SearchResult{}, err
	}

	searchExecuted := false
	var answerParts []string
	var sources []Source
	var queries []string

	for _, item := range decoded.Output {
		switch item.Type {
		case "web_search_call":
			searchExecuted = true
			if query := strings.TrimSpace(item.Action.Query); query != "" {
				queries = append(queries, query)
			}
			queries = append(queries, item.Action.Queries...)

		case "message":
			for _, block := range item.Content {
				if block.Type != "output_text" {
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

func (e *openAIEngine) Close() error {
	if e != nil {
		e.http.closeIdleConnections()
	}
	return nil
}

var _ EngineAdapter = (*openAIEngine)(nil)
