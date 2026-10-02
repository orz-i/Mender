package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	runtimegovernanceapplication "github.com/orz-i/Mender/backend/internal/runtimegovernance/application"
	runtimegovernancedomain "github.com/orz-i/Mender/backend/internal/runtimegovernance/domain"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(ctx context.Context, path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("open runtime governance sqlite: path must not be empty")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("open runtime governance sqlite: absolute path: %w", err)
	}
	dsn := (&url.URL{Scheme: "file", Path: absolute}).String() + "?_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open runtime governance sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db}
	if err := migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) GetBudget(ctx context.Context) (runtimegovernancedomain.Budget, error) {
	var spec runtimegovernancedomain.BudgetSpec
	err := s.db.QueryRowContext(
		ctx,
		`SELECT
			max_invocations,
			max_principal_invocations,
			max_provider_invocations,
			max_tool_invocations,
			max_provider_activations,
			max_execution_timeout_ms,
			max_argument_bytes,
			max_result_bytes
		FROM runtime_budget_settings
		WHERE singleton_id = 1`,
	).Scan(
		&spec.MaxInvocations,
		&spec.MaxPrincipalInvocations,
		&spec.MaxProviderInvocations,
		&spec.MaxToolInvocations,
		&spec.MaxProviderActivations,
		&spec.MaxExecutionTimeoutMS,
		&spec.MaxArgumentBytes,
		&spec.MaxResultBytes,
	)
	if err != nil {
		return runtimegovernancedomain.Budget{}, fmt.Errorf("get runtime budget: %w", err)
	}
	value, err := runtimegovernancedomain.NewBudget(spec)
	if err != nil {
		return runtimegovernancedomain.Budget{}, fmt.Errorf("get runtime budget: invalid persisted value: %w", err)
	}
	return value, nil
}

func (s *Store) SetBudget(ctx context.Context, value runtimegovernancedomain.Budget) error {
	if err := value.Validate(); err != nil {
		return fmt.Errorf("set runtime budget: %w", err)
	}
	spec := value.Spec()
	result, err := s.db.ExecContext(
		ctx,
		`UPDATE runtime_budget_settings SET
			max_invocations = ?,
			max_principal_invocations = ?,
			max_provider_invocations = ?,
			max_tool_invocations = ?,
			max_provider_activations = ?,
			max_execution_timeout_ms = ?,
			max_argument_bytes = ?,
			max_result_bytes = ?,
			updated_at = ?
		WHERE singleton_id = 1`,
		spec.MaxInvocations,
		spec.MaxPrincipalInvocations,
		spec.MaxProviderInvocations,
		spec.MaxToolInvocations,
		spec.MaxProviderActivations,
		spec.MaxExecutionTimeoutMS,
		spec.MaxArgumentBytes,
		spec.MaxResultBytes,
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("set runtime budget: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("set runtime budget: rows affected: %w", err)
	}
	if rows != 1 {
		return fmt.Errorf("set runtime budget: singleton row is missing")
	}
	return nil
}

var _ runtimegovernanceapplication.Repository = (*Store)(nil)
