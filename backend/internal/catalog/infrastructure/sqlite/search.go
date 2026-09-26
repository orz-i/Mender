package sqlite

import (
	"context"
	"fmt"
	"strings"

	"github.com/orz-i/Mender/backend/internal/catalog/application"
	"github.com/orz-i/Mender/backend/internal/catalog/domain"
)

const searchToolsQuery = `SELECT
	t.id,
	t.provider_id,
	t.source_key,
	t.name,
	t.title,
	t.description,
	t.input_schema,
	t.output_schema,
	t.tags_json,
	t.created_at,
	t.updated_at
FROM tools_fts
JOIN tools AS t ON t.id = tools_fts.tool_id
WHERE tools_fts MATCH ?
ORDER BY bm25(tools_fts), t.id
LIMIT ?`

const searchProviderToolsQuery = `SELECT
	t.id,
	t.provider_id,
	t.source_key,
	t.name,
	t.title,
	t.description,
	t.input_schema,
	t.output_schema,
	t.tags_json,
	t.created_at,
	t.updated_at
FROM tools_fts
JOIN tools AS t ON t.id = tools_fts.tool_id
WHERE tools_fts MATCH ?
	AND tools_fts.provider_id = ?
ORDER BY bm25(tools_fts), t.id
LIMIT ?`

const searchToolIDsUnboundedQuery = `SELECT tool_id
FROM tools_fts
WHERE tools_fts MATCH ?
ORDER BY bm25(tools_fts), tool_id`

const searchProviderToolIDsUnboundedQuery = `SELECT tool_id
FROM tools_fts
WHERE tools_fts MATCH ?
	AND provider_id = ?
ORDER BY bm25(tools_fts), tool_id`

// SearchTools performs FTS5/BM25 lexical discovery.
func (s *Store) SearchTools(
	ctx context.Context,
	query string,
	limit int,
	scope application.ToolSearchScope,
) ([]domain.Tool, error) {
	match := ftsQuery(query)
	if match == "" {
		return []domain.Tool{}, nil
	}
	if scope.AllowedToolIDs != nil {
		if len(scope.AllowedToolIDs) == 0 {
			return []domain.Tool{}, nil
		}
		return s.searchAllowedTools(ctx, match, limit, scope)
	}

	statement, args := searchStatement(match, limit, scope)
	rows, err := s.db.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, fmt.Errorf("search tools: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	tools := make([]domain.Tool, 0, limit)
	for rows.Next() {
		tool, scanErr := scanTool(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("search tools: %w", scanErr)
		}
		tools = append(tools, tool)
		if len(tools) == limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search tools: rows: %w", err)
	}

	return tools, nil
}

func (s *Store) searchAllowedTools(
	ctx context.Context,
	match string,
	limit int,
	scope application.ToolSearchScope,
) ([]domain.Tool, error) {
	statement := searchToolIDsUnboundedQuery
	args := []any{match}
	if scope.ProviderID != nil {
		statement = searchProviderToolIDsUnboundedQuery
		args = append(args, scope.ProviderID.String())
	}

	rows, err := s.db.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, fmt.Errorf("search allowed tools: %w", err)
	}
	defer func() { _ = rows.Close() }()

	allowed := make(map[string]domain.ToolID, len(scope.AllowedToolIDs))
	for _, id := range scope.AllowedToolIDs {
		allowed[id.String()] = id
	}

	matched := make([]domain.ToolID, 0, limit)
	for rows.Next() {
		var rawID string
		if err := rows.Scan(&rawID); err != nil {
			return nil, fmt.Errorf("search allowed tools: scan id: %w", err)
		}
		id, visible := allowed[rawID]
		if !visible {
			continue
		}
		matched = append(matched, id)
		if len(matched) == limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search allowed tools: rows: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("search allowed tools: close rows: %w", err)
	}

	tools := make([]domain.Tool, 0, len(matched))
	for _, id := range matched {
		tool, err := s.GetTool(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("search allowed tools: load %s: %w", id, err)
		}
		tools = append(tools, tool)
	}
	return tools, nil
}

func searchStatement(
	match string,
	limit int,
	scope application.ToolSearchScope,
) (string, []any) {
	if scope.ProviderID != nil {
		return searchProviderToolsQuery, []any{match, scope.ProviderID.String(), limit}
	}
	return searchToolsQuery, []any{match, limit}
}

func ftsQuery(query string) string {
	fields := strings.Fields(query)
	terms := make([]string, 0, len(fields))

	for _, field := range fields {
		escaped := strings.ReplaceAll(field, `"`, `""`)
		if escaped == "" {
			continue
		}
		terms = append(terms, `"`+escaped+`"*`)
	}
	return strings.Join(terms, " AND ")
}
