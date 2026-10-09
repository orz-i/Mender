package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// LifecycleEvent is an observed successful state transition, distinct from a
// Policy admission record. Neither filesystem contents nor credentials are kept.
type LifecycleEvent struct {
	SandboxID     sandboxdomain.ID
	Action        governancedomain.Action
	Previous      sandboxdomain.State
	Next          sandboxdomain.State
	Generation    uint64
	PrincipalKind governancedomain.PrincipalKind
	PrincipalRef  string
	Origin        governancedomain.Origin
	ScopeKind     governancedomain.ScopeKind
	ScopeRef      string
	FinishedAt    time.Time
}

// ChangeLocalLifecycle applies a permitted local lifecycle operation and its
// audit in one SQLite transaction. It is never used with a remote/container
// backend, and its success does not remove actual host filesystem contents.
func (s *Store) ChangeLocalLifecycle(ctx context.Context, expected sandboxdomain.Run, action governancedomain.Action, caller governancedomain.Context) (sandboxdomain.Run, error) {
	if s == nil || s.db == nil {
		return sandboxdomain.Run{}, sandboxdomain.ErrBackendUnavailable
	}
	if err := caller.Validate(); err != nil {
		return sandboxdomain.Run{}, err
	}
	if _, err := sandboxdomain.NewRun(expected.SandboxID, expected.Generation, expected.State, expected.ExternalID); err != nil {
		return sandboxdomain.Run{}, err
	}
	var intermediate, terminal sandboxdomain.State
	var externalID string
	switch action {
	case governancedomain.ActionSandboxStart:
		if expected.State != sandboxdomain.StateStopped {
			return sandboxdomain.Run{}, sandboxdomain.ErrConflict
		}
		intermediate, terminal = sandboxdomain.StateProvisioning, sandboxdomain.StateReady
		externalID = "local_workspace:" + expected.SandboxID.String()
	case governancedomain.ActionSandboxStop:
		if expected.State != sandboxdomain.StateReady {
			return sandboxdomain.Run{}, sandboxdomain.ErrConflict
		}
		intermediate, terminal = sandboxdomain.StateStopping, sandboxdomain.StateStopped
		externalID = expected.ExternalID
	case governancedomain.ActionSandboxDestroy:
		if expected.State != sandboxdomain.StateStopped && expected.State != sandboxdomain.StateReady {
			return sandboxdomain.Run{}, sandboxdomain.ErrConflict
		}
		intermediate, terminal = sandboxdomain.StateDestroying, sandboxdomain.StateDestroyed
		externalID = expected.ExternalID
	default:
		return sandboxdomain.Run{}, fmt.Errorf("local Sandbox lifecycle: unsupported action %q", action)
	}
	inProgress, err := expected.Transition(intermediate, externalID)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	next, err := inProgress.Transition(terminal, externalID)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return sandboxdomain.Run{}, fmt.Errorf("local Sandbox lifecycle: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var backend, ownerKind, ownerRef string
	err = tx.QueryRowContext(ctx, `SELECT backend,owner_kind,owner_ref FROM sandboxes WHERE id=?`, expected.SandboxID.String()).Scan(&backend, &ownerKind, &ownerRef)
	if errors.Is(err, sql.ErrNoRows) {
		return sandboxdomain.Run{}, sandboxdomain.ErrNotFound
	}
	if err != nil {
		return sandboxdomain.Run{}, fmt.Errorf("local Sandbox lifecycle: inventory: %w", err)
	}
	if backend != string(sandboxdomain.BackendLocalWorkspace) {
		return sandboxdomain.Run{}, sandboxdomain.ErrBackendUnavailable
	}
	principal := caller.Principal()
	if ownerKind != string(principal.Kind()) || ownerRef != principal.Reference() {
		return sandboxdomain.Run{}, sandboxdomain.ErrNotAuthorized
	}
	var active int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sandbox_execution_leases WHERE sandbox_id=?`, expected.SandboxID.String()).Scan(&active); err != nil {
		return sandboxdomain.Run{}, fmt.Errorf("local Sandbox lifecycle: leases: %w", err)
	}
	if active != 0 {
		return sandboxdomain.Run{}, sandboxdomain.ErrConflict
	}
	if action == governancedomain.ActionSandboxDestroy {
		// Validate references INSIDE the same transaction as the Run CAS. The
		// workspace-binding triggers installed by Runtime also prevent new v2
		// references to destroyed resources.
		var dependent int
		err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM provider_configs WHERE plugin_id='io.mender.provider.workspace' AND plugin_installation_id IS NULL AND config_version=2 AND CASE WHEN json_valid(config_json) THEN json_extract(config_json,'$.sandboxId') END=?`, expected.SandboxID.String()).Scan(&dependent)
		if err != nil {
			return sandboxdomain.Run{}, fmt.Errorf("local Sandbox lifecycle: Provider reference check unavailable: %w", err)
		}
		if dependent != 0 {
			return sandboxdomain.Run{}, sandboxdomain.ErrConflict
		}
	}
	outcome, err := tx.ExecContext(ctx, `UPDATE sandbox_runs SET generation=?,state=?,external_id=?,updated_at=? WHERE sandbox_id=? AND generation=? AND state=? AND external_id=? AND NOT EXISTS (SELECT 1 FROM sandbox_execution_leases WHERE sandbox_id=?)`, next.Generation, string(next.State), next.ExternalID, time.Now().UTC().Format(time.RFC3339Nano), expected.SandboxID.String(), expected.Generation, string(expected.State), expected.ExternalID, expected.SandboxID.String())
	if err != nil {
		return sandboxdomain.Run{}, fmt.Errorf("local Sandbox lifecycle: transition: %w", err)
	}
	changed, err := outcome.RowsAffected()
	if err != nil {
		return sandboxdomain.Run{}, fmt.Errorf("local Sandbox lifecycle: state update: %w", err)
	}
	if changed != 1 {
		return sandboxdomain.Run{}, sandboxdomain.ErrConflict
	}
	scope := caller.Scope()
	_, err = tx.ExecContext(ctx, `INSERT INTO sandbox_lifecycle_events(sandbox_id,action,previous_state,next_state,generation,principal_kind,principal_ref,origin,scope_kind,scope_ref,finished_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, expected.SandboxID.String(), string(action), string(expected.State), string(next.State), next.Generation, string(principal.Kind()), principal.Reference(), string(caller.Origin()), string(scope.Kind()), scope.Reference(), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return sandboxdomain.Run{}, fmt.Errorf("local Sandbox lifecycle: audit: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return sandboxdomain.Run{}, fmt.Errorf("local Sandbox lifecycle: commit: %w", err)
	}
	return next, nil
}

func (s *Store) ListLifecycleEvents(ctx context.Context, id sandboxdomain.ID, limit int) ([]LifecycleEvent, error) {
	if _, err := sandboxdomain.ParseID(id.String()); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 500 {
		return nil, fmt.Errorf("sandbox lifecycle events: limit must be 1..500")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT action,previous_state,next_state,generation,principal_kind,principal_ref,origin,scope_kind,scope_ref,finished_at FROM sandbox_lifecycle_events WHERE sandbox_id=? ORDER BY event_id DESC LIMIT ?`, id.String(), limit)
	if err != nil {
		return nil, fmt.Errorf("list Sandbox lifecycle events: %w", err)
	}
	defer func() { _ = rows.Close() }()
	records := make([]LifecycleEvent, 0)
	for rows.Next() {
		var r LifecycleEvent
		var action, previous, next, principalKind, origin, scopeKind, finished string
		r.SandboxID = id
		if err := rows.Scan(&action, &previous, &next, &r.Generation, &principalKind, &r.PrincipalRef, &origin, &scopeKind, &r.ScopeRef, &finished); err != nil {
			return nil, fmt.Errorf("read Sandbox lifecycle event: %w", err)
		}
		r.Action = governancedomain.Action(action)
		r.Previous = sandboxdomain.State(previous)
		r.Next = sandboxdomain.State(next)
		r.PrincipalKind = governancedomain.PrincipalKind(principalKind)
		r.Origin = governancedomain.Origin(origin)
		r.ScopeKind = governancedomain.ScopeKind(scopeKind)
		if err := r.Action.Validate(); err != nil {
			return nil, err
		}
		if err := r.Previous.Validate(); err != nil {
			return nil, err
		}
		if err := r.Next.Validate(); err != nil {
			return nil, err
		}
		if r.FinishedAt, err = time.Parse(time.RFC3339Nano, finished); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list Sandbox lifecycle events: %w", err)
	}
	return records, nil
}
