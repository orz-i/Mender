package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/orz-i/Mender/backend/internal/catalog/domain"
)

// ToolSearch finds canonical Tools ordered by relevance.
type ToolSearch interface {
	SearchTools(
		context.Context,
		string,
		int,
		*domain.ProviderID,
	) ([]domain.Tool, error)
}

// SearchToolsRequest defines initial lexical discovery constraints.
type SearchToolsRequest struct {
	Query      string
	Limit      int
	ProviderID *domain.ProviderID
}

// ToolSearchResult is a compact application projection for discovery results.
type ToolSearchResult struct {
	Rank        int
	ToolID      domain.ToolID
	ProviderID  domain.ProviderID
	Name        string
	Title       string
	Description string
}

// SearchTools performs lexical Tool discovery through an application-owned port.
type SearchTools struct {
	search ToolSearch
}

func NewSearchTools(search ToolSearch) SearchTools {
	return SearchTools{search: search}
}

func (uc SearchTools) Execute(
	ctx context.Context,
	request SearchToolsRequest,
) ([]ToolSearchResult, error) {
	query := strings.TrimSpace(request.Query)
	if query == "" {
		return nil, ErrInvalidSearchQuery
	}
	if request.Limit < 1 || request.Limit > 100 {
		return nil, ErrInvalidSearchLimit
	}

	tools, err := uc.search.SearchTools(ctx, query, request.Limit, request.ProviderID)
	if err != nil {
		return nil, fmt.Errorf("search tools: %w", err)
	}

	results := make([]ToolSearchResult, 0, len(tools))
	for i, tool := range tools {
		results = append(results, ToolSearchResult{
			Rank:        i + 1,
			ToolID:      tool.ID(),
			ProviderID:  tool.ProviderID(),
			Name:        tool.Name(),
			Title:       tool.Title(),
			Description: tool.Description(),
		})
	}
	return results, nil
}
