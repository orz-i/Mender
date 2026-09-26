package sqlite

import (
	"context"
	"fmt"
	"strings"

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

// SearchTools performs initial FTS5/BM25 lexical discovery.
func (s *Store) SearchTools(
	ctx context.Context,
	query string,
	limit int,
	providerID *domain.ProviderID,
) ([]domain.Tool, error) {
	match := ftsQuery(query)
	if match == "" {
		return []domain.Tool{}, nil
	}

	statement := searchToolsQuery
	args := []any{match, limit}
	if providerID != nil {
		statement = searchProviderToolsQuery
		args = []any{match, providerID.String(), limit}
	}

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
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search tools: rows: %w", err)
	}

	return tools, nil
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
