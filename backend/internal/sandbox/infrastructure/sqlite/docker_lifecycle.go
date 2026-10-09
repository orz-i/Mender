package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sand "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// TransitionIfUnleased is the persistent admission fence for Docker side
// effects. One atomic SQLite transaction checks active leases and Provider
// references before changing the Run from ready/stopped to a blocking state.
func (s *Store) TransitionIfUnleased(ctx context.Context, expected sand.Run, to sand.State, externalID string, blockProvider bool) (sand.Run, error) {
	if s == nil || s.db == nil {
		return sand.Run{}, sand.ErrBackendUnavailable
	}
	next, err := expected.Transition(to, externalID)
	if err != nil {
		return sand.Run{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return sand.Run{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var active int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sandbox_execution_leases WHERE sandbox_id=?`, expected.SandboxID.String()).Scan(&active); err != nil {
		return sand.Run{}, err
	}
	if active != 0 {
		return sand.Run{}, sand.ErrConflict
	}
	if blockProvider {
		var dependents int
		err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM provider_configs WHERE plugin_id='io.mender.provider.workspace' AND plugin_installation_id IS NULL AND config_version=2 AND CASE WHEN json_valid(config_json) THEN json_extract(config_json,'$.sandboxId') END=?`, expected.SandboxID.String()).Scan(&dependents)
		if err != nil {
			return sand.Run{}, fmt.Errorf("docker Sandbox reference check: %w", err)
		}
		if dependents != 0 {
			return sand.Run{}, sand.ErrConflict
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE sandbox_runs SET generation=?,state=?,external_id=?,updated_at=? WHERE sandbox_id=? AND generation=? AND state=? AND external_id=? AND NOT EXISTS(SELECT 1 FROM sandbox_execution_leases WHERE sandbox_id=?)`, next.Generation, string(next.State), next.ExternalID, time.Now().UTC().Format(time.RFC3339Nano), expected.SandboxID.String(), expected.Generation, string(expected.State), expected.ExternalID, expected.SandboxID.String())
	if err != nil {
		return sand.Run{}, fmt.Errorf("docker Sandbox lifecycle CAS: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return sand.Run{}, sand.ErrConflict
	}
	if err := tx.Commit(); err != nil {
		return sand.Run{}, err
	}
	return next, nil
}

// FinishDockerLifecycle records a successful, observed Docker transition and
// its durable audit in one SQLite transaction, not before an Engine outcome.
func (s *Store) FinishDockerLifecycle(ctx context.Context, expected sand.Run, to sand.State, externalID string, action gov.Action, caller gov.Context) (sand.Run, error) {
	if s == nil || s.db == nil {
		return sand.Run{}, sand.ErrBackendUnavailable
	}
	if err := caller.Validate(); err != nil {
		return sand.Run{}, err
	}
	switch action {
	case gov.ActionSandboxStart, gov.ActionSandboxStop, gov.ActionSandboxDestroy:
	default:
		return sand.Run{}, fmt.Errorf("docker lifecycle action invalid")
	}
	next, err := expected.Transition(to, externalID)
	if err != nil {
		return sand.Run{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return sand.Run{}, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `UPDATE sandbox_runs SET generation=?,state=?,external_id=?,updated_at=? WHERE sandbox_id=? AND generation=? AND state=? AND external_id=?`, next.Generation, string(next.State), next.ExternalID, time.Now().UTC().Format(time.RFC3339Nano), expected.SandboxID.String(), expected.Generation, string(expected.State), expected.ExternalID)
	if err != nil {
		return sand.Run{}, fmt.Errorf("docker lifecycle commit state: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		return sand.Run{}, sand.ErrConflict
	}
	subject := caller.Principal()
	scope := caller.Scope()
	_, err = tx.ExecContext(ctx, `INSERT INTO sandbox_lifecycle_events(sandbox_id,action,previous_state,next_state,generation,principal_kind,principal_ref,origin,scope_kind,scope_ref,finished_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, expected.SandboxID.String(), string(action), string(expected.State), string(next.State), next.Generation, string(subject.Kind()), subject.Reference(), string(caller.Origin()), string(scope.Kind()), scope.Reference(), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return sand.Run{}, fmt.Errorf("docker lifecycle audit: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return sand.Run{}, err
	}
	return next, nil
}

// ProviderReferenceCount can support proactive UI warnings; the actual
// reference guard remains in TransitionIfUnleased's transaction.
func (s *Store) ProviderReferenceCount(ctx context.Context, id sand.ID) (int, error) {
	var total int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM provider_configs WHERE plugin_id='io.mender.provider.workspace' AND plugin_installation_id IS NULL AND config_version=2 AND CASE WHEN json_valid(config_json) THEN json_extract(config_json,'$.sandboxId') END=?`, id.String()).Scan(&total)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return total, nil
}
