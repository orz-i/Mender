package application

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/orz-i/Mender/backend/internal/catalog/domain"
)

// ToolSearchScope restricts lexical discovery without changing ranking semantics.
//
// A nil AllowedToolIDs slice means unrestricted. A non-nil empty slice means
// that no Tool is visible.
type ToolSearchScope struct {
	SourceRef      *domain.SourceRef
	AllowedToolIDs []domain.ToolID
}

// ToolSearch finds canonical Tools ordered by relevance.
type ToolSearch interface {
	SearchTools(
		context.Context,
		string,
		int,
		ToolSearchScope,
	) ([]domain.Tool, error)
}

// SearchToolsRequest defines lexical discovery constraints.
type SearchToolsRequest struct {
	Query          string
	Limit          int
	SourceRef      *domain.SourceRef
	AllowedToolIDs []domain.ToolID
}

// ToolSearchResult is a compact application projection for discovery results.
type ToolSearchResult struct {
	Rank        int
	ToolID      domain.ToolID
	SourceRef   domain.SourceRef
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

	if request.SourceRef != nil {
		if err := request.SourceRef.Validate(); err != nil {
			return nil, fmt.Errorf("search tools: invalid SourceRef: %w", err)
		}
	}
	for _, id := range request.AllowedToolIDs {
		if _, err := domain.ParseToolID(id.String()); err != nil {
			return nil, fmt.Errorf("search tools: allowed tool id: %w", err)
		}
	}

	tools, err := uc.search.SearchTools(ctx, query, request.Limit, ToolSearchScope{
		SourceRef:      request.SourceRef,
		AllowedToolIDs: slices.Clone(request.AllowedToolIDs),
	})
	if err != nil {
		return nil, fmt.Errorf("search tools: %w", err)
	}

	results := make([]ToolSearchResult, 0, len(tools))
	for i, tool := range tools {
		results = append(results, ToolSearchResult{
			Rank:        i + 1,
			ToolID:      tool.ID(),
			SourceRef:   tool.SourceRef(),
			Name:        tool.Name(),
			Title:       tool.Title(),
			Description: tool.Description(),
		})
	}
	return results, nil
}
