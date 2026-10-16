package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/orz-i/Mender/backend/internal/catalog/domain"
)

var (
	ErrInvalidToolCursor = errors.New("catalog: invalid tool page cursor")
	ErrStaleToolCursor   = errors.New("catalog: tool page cursor is stale")
)

const maxToolPageOffset = 10_000_000

// ToolPager is a compact read-only catalog port, not an executor.
type ToolPager interface {
	CatalogRevision(context.Context) (int64, error)
	PagedBrowseTools(context.Context, int, *domain.ProviderID, int) ([]ToolListItem, error)
	PagedSearchTools(context.Context, string, int, ToolSearchScope, int) ([]ToolListItem, error)
}
type PageToolsRequest struct {
	Mode           string // browse or search
	Query          string
	Limit          int
	Cursor         string
	ProviderID     *domain.ProviderID
	Profile        string
	AllowedToolIDs []domain.ToolID
}
type ToolPage struct {
	Tools      []ToolListItem
	Offset     int
	NextCursor string
}

type PageTools struct{ pager ToolPager }

func NewPageTools(pager ToolPager) PageTools { return PageTools{pager: pager} }

type pageCursor struct {
	Version  int    `json:"v"`
	Offset   int    `json:"o"`
	Revision int64  `json:"r"`
	Scope    string `json:"s"`
}

func pageScope(request PageToolsRequest) string {
	provider := ""
	if request.ProviderID != nil {
		provider = request.ProviderID.String()
	}
	allowed := make([]string, 0, len(request.AllowedToolIDs))
	for _, id := range request.AllowedToolIDs {
		allowed = append(allowed, id.String())
	}
	slices.Sort(allowed)
	marker := "unrestricted"
	if request.AllowedToolIDs != nil {
		marker = "restricted"
	}
	payload, _ := json.Marshal([]any{request.Mode, request.Query, request.Limit, provider, request.Profile, marker, allowed})
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func decodePageCursor(raw, scope string) (pageCursor, error) {
	if len(raw) > 4096 {
		return pageCursor{}, ErrInvalidToolCursor
	}
	payload, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return pageCursor{}, ErrInvalidToolCursor
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var result pageCursor
	if err = decoder.Decode(&result); err != nil {
		return pageCursor{}, ErrInvalidToolCursor
	}
	if err = decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return pageCursor{}, ErrInvalidToolCursor
	}
	if result.Version != 1 || result.Scope != scope || result.Revision < 0 ||
		result.Offset < 1 || result.Offset > maxToolPageOffset {
		return pageCursor{}, ErrInvalidToolCursor
	}
	return result, nil
}

func (uc PageTools) Execute(ctx context.Context, request PageToolsRequest) (ToolPage, error) {
	result := ToolPage{Tools: []ToolListItem{}}
	if request.Limit < 1 || request.Limit > 100 {
		return result, ErrInvalidSearchLimit
	}
	request.Query = strings.TrimSpace(request.Query)
	if request.Mode == "search" {
		if request.Query == "" {
			return result, ErrInvalidSearchQuery
		}
	} else if request.Mode != "browse" || request.Query != "" ||
		request.Profile != "" || request.AllowedToolIDs != nil {
		return result, ErrInvalidToolCursor
	}
	for _, id := range request.AllowedToolIDs {
		if _, err := domain.ParseToolID(id.String()); err != nil {
			return result, fmt.Errorf("page tools: allowed id: %w", err)
		}
	}
	scope := pageScope(request)
	revision, err := uc.pager.CatalogRevision(ctx)
	if err != nil {
		return result, fmt.Errorf("page tools: revision: %w", err)
	}
	offset := 0
	if request.Cursor != "" {
		cursor, decodeErr := decodePageCursor(request.Cursor, scope)
		if decodeErr != nil {
			return result, decodeErr
		}
		if cursor.Revision != revision {
			return result, ErrStaleToolCursor
		}
		offset = cursor.Offset
	}
	var items []ToolListItem
	if request.Mode == "browse" {
		items, err = uc.pager.PagedBrowseTools(ctx, request.Limit+1, request.ProviderID, offset)
	} else {
		items, err = uc.pager.PagedSearchTools(ctx, request.Query, request.Limit+1,
			ToolSearchScope{ProviderID: request.ProviderID, AllowedToolIDs: slices.Clone(request.AllowedToolIDs)}, offset)
	}
	if err != nil {
		return result, fmt.Errorf("page tools: list: %w", err)
	}
	current, err := uc.pager.CatalogRevision(ctx)
	if err != nil {
		return result, fmt.Errorf("page tools: revision after list: %w", err)
	}
	if current != revision {
		return result, ErrStaleToolCursor
	}
	if len(items) > request.Limit {
		items = items[:request.Limit]
		cursor, _ := json.Marshal(pageCursor{Version: 1, Offset: offset + len(items), Revision: revision, Scope: scope})
		result.NextCursor = base64.RawURLEncoding.EncodeToString(cursor)
	}
	result.Offset = offset
	result.Tools = items
	return result, nil
}
