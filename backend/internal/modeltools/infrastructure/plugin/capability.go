package plugin

import (
	"context"
	"fmt"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
)

var (
	webSearchInputSchema = []byte(`{
		"type":"object",
		"additionalProperties":false,
		"required":["query"],
		"properties":{
			"query":{
				"type":"string",
				"minLength":1,
				"maxLength":8192
			}
		}
	}`)

	webSearchOutputSchema = []byte(`{
		"type":"object",
		"additionalProperties":false,
		"required":["answer","sources","queries"],
		"properties":{
			"answer":{"type":"string"},
			"sources":{
				"type":"array",
				"items":{
					"type":"object",
					"additionalProperties":false,
					"required":["url","title"],
					"properties":{
						"url":{"type":"string"},
						"title":{"type":"string"}
					}
				}
			},
			"queries":{
				"type":"array",
				"items":{"type":"string"}
			}
		}
	}`)
)

// WebSearchRequest is the normalized input to provider-hosted web search.
type WebSearchRequest struct {
	Query string
}

// EngineAdapter hides one provider's hosted-tool wire protocol.
type EngineAdapter interface {
	WebSearch(
		context.Context,
		WebSearchRequest,
		[]byte,
	) (SearchResult, error)
	Close() error
}

func discoveredCapabilityTools(config Config) []catalogapplication.DiscoveredTool {
	if !config.HasCapability(CapabilityWebSearch) {
		return nil
	}
	engineLabel := string(config.Engine)
	return []catalogapplication.DiscoveredTool{
		{
			SourceKey: string(CapabilityWebSearch),
			Name:      string(CapabilityWebSearch),
			Title:     "Web Search",
			Description: fmt.Sprintf(
				"Search the public web through the configured %s model %s and return a grounded answer with sources.",
				engineLabel,
				config.Model,
			),
			InputSchema:  append([]byte(nil), webSearchInputSchema...),
			OutputSchema: append([]byte(nil), webSearchOutputSchema...),
			Tags: []string{
				"model-tools",
				"web-search",
				"engine:" + engineLabel,
			},
		},
	}
}
