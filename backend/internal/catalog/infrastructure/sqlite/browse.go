package sqlite

import (
	"context"
	"fmt"

	"github.com/orz-i/Mender/backend/internal/catalog/application"
	"github.com/orz-i/Mender/backend/internal/catalog/domain"
)

func (s *Store) BrowseTools(
	ctx context.Context,
	limit int,
	providerID *domain.ProviderID,
) ([]application.ToolListItem, error) {
	query := `SELECT id, provider_id, name, title, description
		FROM tools
		ORDER BY provider_id, name, id
		LIMIT ?`
	args := []any{limit}
	if providerID != nil {
		query = `SELECT id, provider_id, name, title, description
			FROM tools
			WHERE provider_id = ?
			ORDER BY name, id
			LIMIT ?`
		args = []any{providerID.String(), limit}
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("browse tools: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]application.ToolListItem, 0, limit)
	for rows.Next() {
		var rawToolID, rawProviderID, name, title, description string
		if err := rows.Scan(
			&rawToolID,
			&rawProviderID,
			&name,
			&title,
			&description,
		); err != nil {
			return nil, fmt.Errorf("browse tools: scan: %w", err)
		}
		toolID, err := domain.ParseToolID(rawToolID)
		if err != nil {
			return nil, fmt.Errorf("browse tools: parse tool id: %w", err)
		}
		id, err := domain.ParseProviderID(rawProviderID)
		if err != nil {
			return nil, fmt.Errorf("browse tools: parse provider id: %w", err)
		}
		items = append(items, application.ToolListItem{
			ToolID:      toolID,
			ProviderID:  id,
			Name:        name,
			Title:       title,
			Description: description,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("browse tools: rows: %w", err)
	}
	return items, nil
}

func (s *Store) ListProviderToolCounts(
	ctx context.Context,
) (map[domain.ProviderID]int, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT provider_id, COUNT(*)
		FROM tools
		GROUP BY provider_id`,
	)
	if err != nil {
		return nil, fmt.Errorf("list provider tool counts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	counts := make(map[domain.ProviderID]int)
	for rows.Next() {
		var rawProviderID string
		var count int
		if err := rows.Scan(&rawProviderID, &count); err != nil {
			return nil, fmt.Errorf("list provider tool counts: scan: %w", err)
		}
		providerID, err := domain.ParseProviderID(rawProviderID)
		if err != nil {
			return nil, fmt.Errorf("list provider tool counts: parse provider id: %w", err)
		}
		counts[providerID] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list provider tool counts: rows: %w", err)
	}
	return counts, nil
}
