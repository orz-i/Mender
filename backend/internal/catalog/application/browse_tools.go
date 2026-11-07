package application

import (
	"context"
	"fmt"

	"github.com/orz-i/Mender/backend/internal/catalog/domain"
)

type ToolListItem struct {
	ToolID      domain.ToolID
	SourceRef   domain.SourceRef
	Name        string
	Title       string
	Description string
}

type ToolBrowser interface {
	BrowseTools(
		context.Context,
		int,
		*domain.SourceRef,
	) ([]ToolListItem, error)
}

type BrowseToolsRequest struct {
	Limit     int
	SourceRef *domain.SourceRef
}

type BrowseTools struct {
	browser ToolBrowser
}

func NewBrowseTools(browser ToolBrowser) BrowseTools {
	return BrowseTools{browser: browser}
}

func (uc BrowseTools) Execute(
	ctx context.Context,
	request BrowseToolsRequest,
) ([]ToolListItem, error) {
	if request.Limit < 1 || request.Limit > 100 {
		return nil, ErrInvalidSearchLimit
	}
	if request.SourceRef != nil {
		if err := request.SourceRef.Validate(); err != nil {
			return nil, fmt.Errorf("browse tools: invalid SourceRef: %w", err)
		}
	}
	items, err := uc.browser.BrowseTools(ctx, request.Limit, request.SourceRef)
	if err != nil {
		return nil, fmt.Errorf("browse tools: %w", err)
	}
	return items, nil
}
