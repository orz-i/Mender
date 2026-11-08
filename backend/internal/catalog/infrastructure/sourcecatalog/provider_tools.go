package sourcecatalog

import (
	"context"
	"fmt"

	application "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
)

// ListProviderTools retains the existing ProviderToolRepository application
// contract, backed by exact typed provenance rather than Provider ancestry.
func (r *Repository) ListProviderTools(ctx context.Context, providerID catalog.ProviderID) ([]catalog.Tool, error) {
	source, err := catalog.ProviderSource(providerID)
	if err != nil {
		return nil, err
	}
	return r.ListSourceTools(ctx, source)
}

// ReplaceProviderTools is intentionally fail-closed on retired ToolIDs.
// Provider discovery can refresh an exact source snapshot, but cannot silently
// remove Profile/Policy-selected ToolIDs; that needs explicit reviewed cleanup.
// The existing SyncProviderTools use case is reused unmodified.
func (r *Repository) ReplaceProviderTools(ctx context.Context, providerID catalog.ProviderID, tools []catalog.Tool) error {
	source, err := catalog.ProviderSource(providerID)
	if err != nil {
		return err
	}
	for _, tool := range tools {
		if tool.SourceRef() != source {
			return fmt.Errorf("%w: Tool %s is not owned by %s", ErrSourceIdentityConflict, tool.ID(), source)
		}
	}
	return r.UpsertSourceSnapshot(ctx, source, tools)
}

// ListProviderToolCounts counts ONLY actual Provider-owned tools, never Agent
// or Native sources. No bogus Provider rows are synthesized for discovery.
func (r *Repository) ListProviderToolCounts(ctx context.Context) (map[catalog.ProviderID]int, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT source_id,COUNT(*) FROM tools
 WHERE source_kind='provider' GROUP BY source_id ORDER BY source_id`)
	if err != nil {
		return nil, fmt.Errorf("typed Catalog Provider Tool counts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	counts := make(map[catalog.ProviderID]int)
	for rows.Next() {
		var raw string
		var count int
		if err := rows.Scan(&raw, &count); err != nil {
			return nil, err
		}
		id, err := catalog.ParseProviderID(raw)
		if err != nil {
			return nil, err
		}
		counts[id] = count
	}
	return counts, rows.Err()
}

var _ application.ProviderCreator = (*Repository)(nil)
var _ application.ProviderUpdater = (*Repository)(nil)
var _ application.ProviderGetter = (*Repository)(nil)
var _ application.ProviderLister = (*Repository)(nil)
var _ application.ProviderNamespaceFinder = (*Repository)(nil)
var _ application.ProviderRemover = (*Repository)(nil)
var _ application.ProviderToolRepository = (*Repository)(nil)
var _ application.ProviderToolCountLister = (*Repository)(nil)
