package sourcecatalog

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	application "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
)

// One Catalog paging use case handles v3 SourceRef queries, cursor checks
// and revision fences. This is not another discovery or execution pipeline.
func (r *Repository) CatalogRevision(ctx context.Context) (int64, error) {
	var revision int64
	if err := r.db.QueryRowContext(ctx, "SELECT revision FROM catalog_tool_revision WHERE id=1").Scan(&revision); err != nil {
		return 0, fmt.Errorf("typed Catalog revision: %w", err)
	}
	return revision, nil
}

func (r *Repository) PagedBrowseTools(ctx context.Context, limit int, source *catalog.SourceRef, offset int) ([]application.ToolListItem, error) {
	if source != nil && source.Validate() != nil {
		return nil, fmt.Errorf("typed Catalog browse: invalid SourceRef")
	}
	query := "SELECT id,source_kind,source_id,name,title,description FROM tools"
	args := []any{}
	if source != nil {
		query += " WHERE source_kind=? AND source_id=?"
		args = append(args, string(source.Kind()), source.ID())
	}
	query += " ORDER BY source_kind,source_id,name,id LIMIT ? OFFSET ?"
	args = append(args, limit, offset)
	return r.compactRows(ctx, query, args...)
}

func (r *Repository) BrowseTools(ctx context.Context, limit int, source *catalog.SourceRef) ([]application.ToolListItem, error) {
	return r.PagedBrowseTools(ctx, limit, source, 0)
}

func (r *Repository) PagedSearchTools(ctx context.Context, text string, limit int,
	scope application.ToolSearchScope, offset int,
) ([]application.ToolListItem, error) {
	query, args, err := typedSearchStatement(text, limit, scope, offset, compactSourceColumns)
	if err != nil {
		return nil, err
	}
	if query == "" {
		return []application.ToolListItem{}, nil
	}
	return r.compactRows(ctx, query, args...)
}

func (r *Repository) SearchTools(ctx context.Context, text string, limit int,
	scope application.ToolSearchScope,
) ([]catalog.Tool, error) {
	query, args, err := typedSearchStatement(text, limit, scope, 0, fullSourceColumns)
	if err != nil {
		return nil, err
	}
	if query == "" {
		return []catalog.Tool{}, nil
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	tools := []catalog.Tool{}
	for rows.Next() {
		tool, err := scanSourceTool(rows)
		if err != nil {
			return nil, err
		}
		tools = append(tools, tool)
	}
	return tools, rows.Err()
}

const compactSourceColumns = "t.id,t.source_kind,t.source_id,t.name,t.title,t.description"
const fullSourceColumns = "t.id,t.source_kind,t.source_id,t.source_key,t.name,t.title,t.description,t.input_schema,t.output_schema,t.tags_json,t.created_at,t.updated_at"

// Shared compact/full FTS query, restricted by exact SourceRef and an optional
// allowlist BEFORE LIMIT. All values are bound SQL parameters.
func typedSearchStatement(text string, limit int, scope application.ToolSearchScope, offset int, columns string) (string, []any, error) {
	if scope.SourceRef != nil && scope.SourceRef.Validate() != nil {
		return "", nil, fmt.Errorf("typed Catalog search: invalid SourceRef")
	}
	if scope.AllowedToolIDs != nil && len(scope.AllowedToolIDs) == 0 {
		return "", nil, nil
	}
	terms := []string{}
	for _, item := range strings.Fields(text) {
		item = strings.ReplaceAll(item, "\"", "\"\"")
		if item != "" {
			terms = append(terms, "\""+item+"\""+"*")
		}
	}
	if len(terms) == 0 {
		return "", nil, nil
	}
	query := "SELECT " + columns + " FROM tools_fts JOIN tools t ON t.id=tools_fts.tool_id WHERE tools_fts MATCH ?"
	args := []any{strings.Join(terms, " AND ")}
	if scope.SourceRef != nil {
		query += " AND t.source_kind=? AND t.source_id=?"
		args = append(args, string(scope.SourceRef.Kind()), scope.SourceRef.ID())
	}
	if scope.AllowedToolIDs != nil {
		ids := []string{}
		for _, id := range scope.AllowedToolIDs {
			if _, err := catalog.ParseToolID(id.String()); err != nil {
				return "", nil, err
			}
			ids = append(ids, id.String())
		}
		allowed, err := json.Marshal(ids)
		if err != nil {
			return "", nil, err
		}
		query += " AND t.id IN (SELECT value FROM json_each(?))"
		args = append(args, string(allowed))
	}
	term := strings.ToLower(strings.TrimSpace(text))
	query += ` ORDER BY CASE
 WHEN lower(t.name)=? THEN 0
 WHEN instr(lower(t.name),?)>0 THEN 1
 WHEN lower(t.title)=? THEN 2
 WHEN instr(lower(t.title),?)>0 THEN 3
 WHEN instr(lower(t.tags_json),?)>0 THEN 4
 ELSE 5 END,
 bm25(tools_fts,0.0,0.0,0.0,12.0,8.0,1.0,6.0),t.id LIMIT ? OFFSET ?`
	args = append(args, term, term, term, term, term, limit, offset)
	return query, args, nil
}

func (r *Repository) compactRows(ctx context.Context, query string, args ...any) ([]application.ToolListItem, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := []application.ToolListItem{}
	for rows.Next() {
		var rawID, kind, owner, name, title, description string
		if err := rows.Scan(&rawID, &kind, &owner, &name, &title, &description); err != nil {
			return nil, err
		}
		id, err := catalog.ParseToolID(rawID)
		if err != nil {
			return nil, err
		}
		source, err := catalog.NewSourceRef(catalog.SourceKind(kind), owner)
		if err != nil {
			return nil, err
		}
		items = append(items, application.ToolListItem{ToolID: id, SourceRef: source, Name: name, Title: title, Description: description})
	}
	return items, rows.Err()
}

var _ application.ToolPager = (*Repository)(nil)
var _ application.ToolSearch = (*Repository)(nil)
var _ application.ToolBrowser = (*Repository)(nil)
