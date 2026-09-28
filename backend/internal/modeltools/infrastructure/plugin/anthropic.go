package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const (
	anthropicMessagesEndpoint  = "https://api.anthropic.com/v1/messages"
	anthropicVersion           = "2023-06-01"
	anthropicWebSearchToolType = "web_search_20260318"
	anthropicMaxTokens         = 2048
	anthropicContinuationLimit = 3
)

type anthropicEngine struct {
	model    string
	endpoint string
	http     engineHTTPClient
}

func newAnthropicEngine(model string) *anthropicEngine {
	return &anthropicEngine{
		model:    model,
		endpoint: anthropicMessagesEndpoint,
		http:     newEngineHTTPClient(),
	}
}

func (e *anthropicEngine) WebSearch(
	ctx context.Context,
	request WebSearchRequest,
	credential []byte,
) (SearchResult, error) {
	type tool struct {
		Type           string   `json:"type"`
		Name           string   `json:"name"`
		AllowedCallers []string `json:"allowed_callers,omitempty"`
	}
	type message struct {
		Role    string `json:"role"`
		Content any    `json:"content"`
	}
	type payload struct {
		Model     string    `json:"model"`
		MaxTokens int       `json:"max_tokens"`
		System    string    `json:"system"`
		Messages  []message `json:"messages"`
		Tools     []tool    `json:"tools"`
	}
	type response struct {
		Content    []json.RawMessage `json:"content"`
		StopReason string            `json:"stop_reason"`
	}

	headers := make(http.Header)
	headers.Set("x-api-key", string(credential))
	headers.Set("anthropic-version", anthropicVersion)

	tools := []tool{{
		Type:           anthropicWebSearchToolType,
		Name:           "web_search",
		AllowedCallers: []string{"direct"},
	}}
	messages := []message{{
		Role:    "user",
		Content: request.Query,
	}}

	var (
		searchExecuted bool
		answerParts    []string
		sources        []Source
		queries        []string
	)

	for attempt := 0; ; attempt++ {
		var decoded response
		err := e.http.postJSON(
			ctx,
			e.endpoint,
			headers,
			payload{
				Model:     e.model,
				MaxTokens: anthropicMaxTokens,
				System:    webSearchInstruction,
				Messages:  messages,
				Tools:     tools,
			},
			&decoded,
		)
		if err != nil {
			return SearchResult{}, err
		}

		for _, rawBlock := range decoded.Content {
			blockSearch, resetAnswer, blockText, blockSources, blockQueries, blockErr :=
				parseAnthropicContentBlock(rawBlock)
			if blockErr != nil {
				return SearchResult{}, blockErr
			}
			if blockSearch {
				searchExecuted = true
			}
			if resetAnswer {
				answerParts = nil
			}
			if blockText != "" {
				answerParts = append(answerParts, blockText)
			}
			sources = append(sources, blockSources...)
			queries = append(queries, blockQueries...)
		}

		if decoded.StopReason != "pause_turn" {
			if decoded.StopReason == "tool_use" {
				return SearchResult{}, fmt.Errorf(
					"anthropic web search unexpectedly requested a client tool",
				)
			}
			break
		}
		if attempt >= anthropicContinuationLimit {
			return SearchResult{}, fmt.Errorf(
				"anthropic web search continuation exceeded %d attempts",
				anthropicContinuationLimit,
			)
		}
		messages = append(messages, message{
			Role:    "assistant",
			Content: decoded.Content,
		})
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

func parseAnthropicContentBlock(
	raw json.RawMessage,
) (
	searchExecuted bool,
	resetAnswer bool,
	text string,
	sources []Source,
	queries []string,
	err error,
) {
	var envelope struct {
		Type string `json:"type"`
	}
	if decodeErr := json.Unmarshal(raw, &envelope); decodeErr != nil {
		return false, false, "", nil, nil, fmt.Errorf(
			"decode Anthropic content block: invalid JSON",
		)
	}

	switch envelope.Type {
	case "server_tool_use":
		var block struct {
			Name  string `json:"name"`
			Input struct {
				Query string `json:"query"`
			} `json:"input"`
		}
		if decodeErr := json.Unmarshal(raw, &block); decodeErr != nil {
			return false, false, "", nil, nil, fmt.Errorf(
				"decode Anthropic server tool use: invalid JSON",
			)
		}
		if block.Name != "web_search" {
			return false, false, "", nil, nil, nil
		}
		query := strings.TrimSpace(block.Input.Query)
		if query != "" {
			queries = append(queries, query)
		}
		return true, false, "", nil, queries, nil

	case "web_search_tool_result":
		var block struct {
			Content json.RawMessage `json:"content"`
		}
		if decodeErr := json.Unmarshal(raw, &block); decodeErr != nil {
			return false, false, "", nil, nil, fmt.Errorf(
				"decode Anthropic web search result: invalid JSON",
			)
		}
		resultSources, toolErr := parseAnthropicSearchResultContent(block.Content)
		if toolErr != nil {
			return false, false, "", nil, nil, toolErr
		}
		return false, true, "", resultSources, nil, nil

	case "text":
		var block struct {
			Text      string `json:"text"`
			Citations []struct {
				Type  string `json:"type"`
				URL   string `json:"url"`
				Title string `json:"title"`
			} `json:"citations"`
		}
		if decodeErr := json.Unmarshal(raw, &block); decodeErr != nil {
			return false, false, "", nil, nil, fmt.Errorf(
				"decode Anthropic text block: invalid JSON",
			)
		}
		for _, citation := range block.Citations {
			if citation.Type != "web_search_result_location" {
				continue
			}
			sources = append(sources, Source{
				URL:   citation.URL,
				Title: citation.Title,
			})
		}
		return false, false, strings.TrimSpace(block.Text), sources, nil, nil

	default:
		return false, false, "", nil, nil, nil
	}
}

func parseAnthropicSearchResultContent(
	raw json.RawMessage,
) ([]Source, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	var results []struct {
		Type  string `json:"type"`
		URL   string `json:"url"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal(raw, &results); err == nil {
		sources := make([]Source, 0, len(results))
		for _, result := range results {
			if result.Type != "web_search_result" {
				continue
			}
			sources = append(sources, Source{
				URL:   result.URL,
				Title: result.Title,
			})
		}
		return sources, nil
	}

	var failure struct {
		Type      string `json:"type"`
		ErrorCode string `json:"error_code"`
	}
	if err := json.Unmarshal(raw, &failure); err == nil {
		if strings.Contains(strings.ToLower(failure.Type), "error") ||
			strings.TrimSpace(failure.ErrorCode) != "" {
			return nil, newToolError("hosted web search reported an execution error")
		}
	}

	return nil, fmt.Errorf("decode Anthropic web search result content: unsupported shape")
}

var _ EngineAdapter = (*anthropicEngine)(nil)
