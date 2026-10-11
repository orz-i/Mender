package sqlite

import (
	"context"
	"fmt"
	"time"

	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// TransitionExternalRun fences resource generation and concurrent durable
// leases in the SAME SQLite CAS as the observed state transition. Successful
// final operations record immutable lifecycle audit in that transaction.
func (s *Store) TransitionExternalRun(ctx context.Context, expected sandboxdomain.Run,
	nextState sandboxdomain.State, externalID string, action gov.Action,
	caller gov.Context, audit bool) (sandboxdomain.Run, error) {
	if err := caller.Validate(); err != nil {
		return sandboxdomain.Run{}, err
	}
	if err := action.Validate(); err != nil {
		return sandboxdomain.Run{}, err
	}
	next, err := expected.Transition(nextState, externalID)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	defer func() { _ = tx.Rollback() }()
	subject := caller.Principal()
	statement := `UPDATE sandbox_runs SET generation=?,state=?,external_id=?,updated_at=?
        WHERE sandbox_id=? AND generation=? AND state=? AND external_id=?
        AND NOT EXISTS(SELECT 1 FROM sandbox_execution_leases WHERE sandbox_id=?)
        AND EXISTS(SELECT 1 FROM sandboxes WHERE id=? AND backend='external' AND owner_kind=? AND owner_ref=?)`
	args := []any{next.Generation, string(next.State), next.ExternalID, time.Now().UTC().Format(time.RFC3339Nano),
		expected.SandboxID.String(), expected.Generation, string(expected.State), expected.ExternalID,
		expected.SandboxID.String(), expected.SandboxID.String(), string(subject.Kind()), subject.Reference()}
	if action == gov.ActionSandboxDestroy {
		// Provider references are checked in the same CAS as the run state.
		// This query exists only when the Provider inventory exists; otherwise
		// destruction fails closed, not silently bypassing reference checks.
		statement += ` AND NOT EXISTS (
            SELECT 1 FROM provider_configs p
            WHERE p.plugin_id='io.mender.provider.workspace' AND p.config_version=2
            AND p.plugin_installation_id IS NULL
            AND CASE WHEN json_valid(p.config_json) THEN json_extract(p.config_json,'$.sandboxId') END=?
        )`
		args = append(args, expected.SandboxID.String())
	}
	outcome, err := tx.ExecContext(ctx, statement, args...)
	if err != nil {
		return sandboxdomain.Run{}, fmt.Errorf("sandbox external run CAS: %w", err)
	}
	count, err := outcome.RowsAffected()
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	if count != 1 {
		return sandboxdomain.Run{}, sandboxdomain.ErrConflict
	}
	if audit {
		scope := caller.Scope()
		if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_lifecycle_events(sandbox_id,action,previous_state,next_state,
            generation,principal_kind,principal_ref,origin,scope_kind,scope_ref,finished_at)
            VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
			expected.SandboxID.String(), string(action), string(expected.State), string(next.State),
			next.Generation, string(subject.Kind()), subject.Reference(), string(caller.Origin()), string(scope.Kind()),
			scope.Reference(), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return sandboxdomain.Run{}, fmt.Errorf("sandbox external lifecycle audit: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return sandboxdomain.Run{}, err
	}
	return next, nil
}
