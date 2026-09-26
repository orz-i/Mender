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

const searchToolsUnboundedQuery = `SELECT
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
ORDER BY bm25(tools_fts), t.id`

const searchProviderToolsUnboundedQuery = `SELECT
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
ORDER BY bm25(tools_fts), t.id`

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
	if scope.AllowedToolIDs != nil && len(scope.AllowedToolIDs) == 0 {
		return []domain.Tool{}, nil
	}

	statement, args := searchStatement(match, limit, scope)
	rows, err := s.db.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, fmt.Errorf("search tools: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	allowed := make(map[domain.ToolID]struct{}, len(scope.AllowedToolIDs))
	for _, id := range scope.AllowedToolIDs {
		allowed[id] = struct{}{}
	}

	tools := make([]domain.Tool, 0, limit)
	for rows.Next() {
		tool, scanErr := scanTool(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("search tools: %w", scanErr)
		}
		if scope.AllowedToolIDs != nil {
			if _, visible := allowed[tool.ID()]; !visible {
				continue
			}
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

func searchStatement(
	match string,
	limit int,
	scope application.ToolSearchScope,
) (string, []any) {
	if scope.AllowedToolIDs != nil {
		if scope.ProviderID != nil {
			return searchProviderToolsUnboundedQuery, []any{match, scope.ProviderID.String()}
		}
		return searchToolsUnboundedQuery, []any{match}
	}

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
