package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/orz-i/Mender/backend/internal/catalog/application"
	"github.com/orz-i/Mender/backend/internal/catalog/domain"
)

// Name/title/tag priority must precede BM25: a spammy external description
// repeating one term hundreds of times can otherwise outrank an exact Tool
// name even with large BM25 field weights. All inputs remain SQL parameters.
// Within each lexical tier, FTS5 BM25 and canonical ToolID break ties.
const ftsRankOrder = `ORDER BY CASE
 WHEN lower(t.name) = ? THEN 0
 WHEN instr(lower(t.name), ?) > 0 THEN 1
 WHEN lower(t.title) = ? THEN 2
 WHEN instr(lower(t.title), ?) > 0 THEN 3
 WHEN instr(lower(t.tags_json), ?) > 0 THEN 4
 ELSE 5 END,
 bm25(tools_fts, 0.0, 0.0, 12.0, 8.0, 1.0, 6.0), t.id`

func ftsRankingArgs(query string) []any {
	term := strings.ToLower(strings.TrimSpace(query))
	return []any{term, term, term, term, term}
}

const searchToolsStatement = `SELECT
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
WHERE tools_fts MATCH ?`

// SearchTools performs FTS5/BM25 lexical discovery.
func (s *Store) SearchTools(
	ctx context.Context,
	query string,
	limit int,
	scope application.ToolSearchScope,
) ([]domain.Tool, error) {
	return s.pagedSearchToolDomain(ctx, query, limit, scope, 0)
}

func (s *Store) pagedSearchToolDomain(
	ctx context.Context, query string, limit int,
	scope application.ToolSearchScope, offset int,
) ([]domain.Tool, error) {
	match := ftsQuery(query)
	if match == "" || (scope.AllowedToolIDs != nil && len(scope.AllowedToolIDs) == 0) {
		return []domain.Tool{}, nil
	}

	statement, args, err := searchStatement(match, query, limit, offset, scope)
	if err != nil {
		return nil, fmt.Errorf("search tools: %w", err)
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
		if len(tools) == limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search tools: rows: %w", err)
	}

	return tools, nil
}

// searchStatement builds one parameterized search query. Any Profile
// allowance is restricted with a JSON1 IN predicate, evaluated together with
// the provider filter before ranking, LIMIT and OFFSET, so pagination can
// never bypass the scope.
func searchStatement(
	match string, query string, limit int, offset int, scope application.ToolSearchScope,
) (string, []any, error) {
	statement := searchToolsStatement
	args := []any{match}
	if scope.ProviderID != nil {
		statement += `
	AND tools_fts.provider_id = ?`
		args = append(args, scope.ProviderID.String())
	}
	if scope.AllowedToolIDs != nil {
		allowed, err := encodeAllowedToolIDs(scope.AllowedToolIDs)
		if err != nil {
			return "", nil, fmt.Errorf("allowed IDs: %w", err)
		}
		statement += `
	AND t.id IN (SELECT value FROM json_each(?))`
		args = append(args, allowed)
	}
	statement += "\n" + ftsRankOrder + `
LIMIT ? OFFSET ?`
	args = append(args, ftsRankingArgs(query)...)
	args = append(args, limit, offset)
	return statement, args, nil
}

// encodeAllowedToolIDs renders Profile allowances as one JSON array parameter
// so ToolIDs are bound, not interpolated into SQL.
func encodeAllowedToolIDs(allowed []domain.ToolID) (string, error) {
	encoded := make([]string, 0, len(allowed))
	for _, id := range allowed {
		encoded = append(encoded, id.String())
	}
	payload, err := json.Marshal(encoded)
	if err != nil {
		return "", fmt.Errorf("encode allowed tool ids: %w", err)
	}
	return string(payload), nil
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
