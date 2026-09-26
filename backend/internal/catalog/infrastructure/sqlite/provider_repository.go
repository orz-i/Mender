package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/orz-i/Mender/backend/internal/catalog/application"
	"github.com/orz-i/Mender/backend/internal/catalog/domain"
)

// CreateProvider persists a newly registered Provider.
func (s *Store) CreateProvider(ctx context.Context, provider domain.Provider) error {
	result, err := s.db.ExecContext(
		ctx,
		`INSERT INTO providers(
			id, namespace, display_name, description, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(namespace) DO NOTHING`,
		provider.ID().String(),
		provider.Namespace().String(),
		provider.DisplayName(),
		provider.Description(),
		formatTime(provider.CreatedAt()),
		formatTime(provider.UpdatedAt()),
	)
	if err != nil {
		return fmt.Errorf("create provider: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("create provider: rows affected: %w", err)
	}
	if affected == 0 {
		return application.ErrNamespaceConflict
	}
	return nil
}

// GetProvider loads one Provider by stable Mender identity.
func (s *Store) GetProvider(
	ctx context.Context,
	id domain.ProviderID,
) (domain.Provider, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT id, namespace, display_name, description, created_at, updated_at
		FROM providers
		WHERE id = ?`,
		id.String(),
	)

	provider, err := scanProvider(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Provider{}, application.ErrProviderNotFound
	}
	if err != nil {
		return domain.Provider{}, fmt.Errorf("get provider: %w", err)
	}
	return provider, nil
}

type rowScanner interface {
	Scan(...any) error
}

func scanProvider(scanner rowScanner) (domain.Provider, error) {
	var (
		rawID        string
		rawNamespace string
		displayName  string
		description  string
		rawCreated   string
		rawUpdated   string
	)
	if err := scanner.Scan(
		&rawID,
		&rawNamespace,
		&displayName,
		&description,
		&rawCreated,
		&rawUpdated,
	); err != nil {
		return domain.Provider{}, err
	}

	id, err := domain.ParseProviderID(rawID)
	if err != nil {
		return domain.Provider{}, fmt.Errorf("parse provider id: %w", err)
	}
	namespace, err := domain.NewNamespace(rawNamespace)
	if err != nil {
		return domain.Provider{}, fmt.Errorf("parse provider namespace: %w", err)
	}
	createdAt, err := parseTime(rawCreated)
	if err != nil {
		return domain.Provider{}, fmt.Errorf("parse provider created_at: %w", err)
	}
	updatedAt, err := parseTime(rawUpdated)
	if err != nil {
		return domain.Provider{}, fmt.Errorf("parse provider updated_at: %w", err)
	}

	return domain.RestoreProvider(
		id,
		namespace,
		displayName,
		description,
		createdAt,
		updatedAt,
	)
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, value)
}

func (s *Store) ListProviders(ctx context.Context) ([]domain.Provider, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, namespace, display_name, description, created_at, updated_at
		FROM providers
		ORDER BY namespace, id`,
	)
	if err != nil {
		return nil, fmt.Errorf("list providers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	providers := make([]domain.Provider, 0)
	for rows.Next() {
		provider, scanErr := scanProvider(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list providers: %w", scanErr)
		}
		providers = append(providers, provider)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list providers: rows: %w", err)
	}
	return providers, nil
}

func (s *Store) FindProviderByNamespace(
	ctx context.Context,
	namespace domain.Namespace,
) (domain.Provider, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT id, namespace, display_name, description, created_at, updated_at
		FROM providers
		WHERE namespace = ?`,
		namespace.String(),
	)
	provider, err := scanProvider(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Provider{}, application.ErrProviderNotFound
	}
	if err != nil {
		return domain.Provider{}, fmt.Errorf("find provider by namespace: %w", err)
	}
	return provider, nil
}

func (s *Store) RemoveProvider(ctx context.Context, id domain.ProviderID) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM providers WHERE id = ?", id.String())
	if err != nil {
		return fmt.Errorf("remove provider: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("remove provider: rows affected: %w", err)
	}
	if affected == 0 {
		return application.ErrProviderNotFound
	}
	return nil
}
