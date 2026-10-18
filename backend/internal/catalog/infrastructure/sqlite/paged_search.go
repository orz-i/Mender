package sqlite

import (
	"context"
	"fmt"

	"github.com/orz-i/Mender/backend/internal/catalog/application"
	"github.com/orz-i/Mender/backend/internal/catalog/domain"
)

// PagedSearchTools reads only compact metadata. Schema bytes are loaded
// exclusively by Tool detail/invocation paths, never list/search browsing.
func (s *Store) PagedSearchTools(
	ctx context.Context, query string, limit int,
	scope application.ToolSearchScope, offset int,
) ([]application.ToolListItem, error) {
	match := ftsQuery(query)
	if match == "" || (scope.AllowedToolIDs != nil && len(scope.AllowedToolIDs) == 0) {
		return []application.ToolListItem{}, nil
	}
	statement := `SELECT t.id,t.provider_id,t.name,t.title,t.description
  FROM tools_fts JOIN tools t ON t.id=tools_fts.tool_id
  WHERE tools_fts MATCH ?`
	args := []any{match}
	if scope.ProviderID != nil {
		statement += " AND tools_fts.provider_id=?"
		args = append(args, scope.ProviderID.String())
	}
	if scope.AllowedToolIDs != nil {
		encoded, err := encodeAllowedToolIDs(scope.AllowedToolIDs)
		if err != nil {
			return nil, fmt.Errorf("paged tool search: allowed IDs: %w", err)
		}
		statement += " AND t.id IN (SELECT value FROM json_each(?))"
		args = append(args, encoded)
	}
	statement += "\n" + ftsRankOrder + "\nLIMIT ? OFFSET ?"
	args = append(args, ftsRankingArgs(query)...)
	args = append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, fmt.Errorf("paged tool search: query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	items := make([]application.ToolListItem, 0, limit)
	for rows.Next() {
		var rawID, rawProvider, name, title, description string
		if err = rows.Scan(&rawID, &rawProvider, &name, &title, &description); err != nil {
			return nil, fmt.Errorf("paged tool search: scan: %w", err)
		}
		toolID, parseErr := domain.ParseToolID(rawID)
		if parseErr != nil {
			return nil, fmt.Errorf("paged tool search: ToolID: %w", parseErr)
		}
		providerID, parseErr := domain.ParseProviderID(rawProvider)
		if parseErr != nil {
			return nil, fmt.Errorf("paged tool search: ProviderID: %w", parseErr)
		}
		items = append(items, application.ToolListItem{
			ToolID: toolID, ProviderID: providerID, Name: name, Title: title, Description: description,
		})
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("paged tool search: rows: %w", err)
	}
	return items, nil
}
