package sqlite

import (
	"context"
	"database/sql"
	"encoding/hex"
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
	return s.changePassiveLifecycle(ctx, expected, action, caller, sandboxdomain.BackendLocalWorkspace, "local_workspace:", "")
}

// SSH stop/start/destroy only changes the local authority binding. It does
// not mutate, reboot or delete any resource on the remote SSH host.
func (s *Store) ChangeSSHRemoteLifecycle(ctx context.Context, expected sandboxdomain.Run, action governancedomain.Action, caller governancedomain.Context) (sandboxdomain.Run, error) {
	return s.changePassiveLifecycle(ctx, expected, action, caller, sandboxdomain.BackendSSHRemote, "ssh_remote:", "")
}

// ChangeSSHRemoteLifecycleFenced consumes a previously reserved exclusive
// lifecycle fence in the SAME transaction that changes the Sandbox Run.
// The plugin's remote no-active-Jobs proof must precede this call.
func (s *Store) ChangeSSHRemoteLifecycleFenced(ctx context.Context, expected sandboxdomain.Run,
	action governancedomain.Action, caller governancedomain.Context, token string) (sandboxdomain.Run, error) {
	if len(token) != 32 {
		return sandboxdomain.Run{}, sandboxdomain.ErrConflict
	}
	if _, err := hex.DecodeString(token); err != nil {
		return sandboxdomain.Run{}, sandboxdomain.ErrConflict
	}
	if action != governancedomain.ActionSandboxStop && action != governancedomain.ActionSandboxDestroy {
		return sandboxdomain.Run{}, sandboxdomain.ErrConflict
	}
	return s.changePassiveLifecycle(ctx, expected, action, caller, sandboxdomain.BackendSSHRemote, "ssh_remote:", token)
}
func (s *Store) changePassiveLifecycle(ctx context.Context, expected sandboxdomain.Run, action governancedomain.Action, caller governancedomain.Context, backendKind sandboxdomain.BackendKind, prefix, fenceToken string) (sandboxdomain.Run, error) {
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
		externalID = prefix + expected.SandboxID.String()
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
	if backend != string(backendKind) {
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
	// A permanent SSH Job obligation never represents task state. The plugin
	// must prove quiescence after an exclusive fence reservation and the
	// Run state CAS must consume that exact fence atomically.
	if backendKind == sandboxdomain.BackendSSHRemote && action != governancedomain.ActionSandboxStart {
		var managed int
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sandbox_ssh_job_obligations WHERE sandbox_id=?)`, expected.SandboxID.String()).Scan(&managed); err != nil {
			return sandboxdomain.Run{}, err
		}
		if (managed != 0) != (fenceToken != "") {
			return sandboxdomain.Run{}, fmt.Errorf("SSH lifecycle: job obligation requires matching fence: %w", sandboxdomain.ErrConflict)
		}
	}
	if fenceToken == "" && active != 0 {
		return sandboxdomain.Run{}, sandboxdomain.ErrConflict
	}
	if fenceToken != "" {
		var held int
		if active != 1 {
			return sandboxdomain.Run{}, fmt.Errorf("SSH lifecycle: exclusive fence unavailable: %w", sandboxdomain.ErrConflict)
		}
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sandbox_execution_leases WHERE sandbox_id=? AND lease_token=? AND generation=? AND principal_kind=? AND principal_ref=?)`,
			expected.SandboxID.String(), fenceToken, expected.Generation, string(principal.Kind()), principal.Reference()).Scan(&held); err != nil || held != 1 {
			return sandboxdomain.Run{}, fmt.Errorf("SSH lifecycle: invalid fence token or generation: %w", sandboxdomain.ErrConflict)
		}
	}
	if action == governancedomain.ActionSandboxDestroy {
		// Resource ownership and raw Provider config references are both
		// checked within the Run CAS transaction. FK restrictions alone cannot
		// protect a logical Destroy because the Sandbox row is not deleted.
		// A Jobs facade is as authoritative a reference as Workspace.
		var dependent int
		err = tx.QueryRowContext(ctx, `SELECT
			(SELECT COUNT(*) FROM sandbox_tool_suite_bindings WHERE sandbox_id=?) +
			(SELECT COUNT(*) FROM sandbox_job_suite_bindings WHERE sandbox_id=?) +
			(SELECT COUNT(*) FROM provider_configs
			 WHERE plugin_installation_id IS NULL
			   AND ((plugin_id='io.mender.provider.workspace' AND config_version=2)
			     OR (plugin_id='io.mender.provider.sandbox-jobs' AND config_version=1))
			   AND CASE WHEN json_valid(config_json) THEN json_extract(config_json,'$.sandboxId') END=?)`,
			expected.SandboxID.String(), expected.SandboxID.String(), expected.SandboxID.String()).Scan(&dependent)
		if err != nil {
			return sandboxdomain.Run{}, fmt.Errorf("local Sandbox lifecycle: Tool Suite reference check unavailable: %w", err)
		}
		if dependent != 0 {
			return sandboxdomain.Run{}, sandboxdomain.ErrConflict
		}
	}
	leasePredicate := "AND NOT EXISTS (SELECT 1 FROM sandbox_execution_leases WHERE sandbox_id=?)"
	if fenceToken != "" {
		leasePredicate = "AND EXISTS (SELECT 1 FROM sandbox_execution_leases WHERE sandbox_id=? AND lease_token=?)"
	}
	args := []any{next.Generation, string(next.State), next.ExternalID, time.Now().UTC().Format(time.RFC3339Nano), expected.SandboxID.String(), expected.Generation, string(expected.State), expected.ExternalID, expected.SandboxID.String()}
	if fenceToken != "" {
		args = append(args, fenceToken)
	}
	outcome, err := tx.ExecContext(ctx, `UPDATE sandbox_runs SET generation=?,state=?,external_id=?,updated_at=? WHERE sandbox_id=? AND generation=? AND state=? AND external_id=? `+leasePredicate, args...)
	if err != nil {
		return sandboxdomain.Run{}, fmt.Errorf("local Sandbox lifecycle: transition: %w", err)
	}
	changed, err := outcome.RowsAffected()
	if err != nil {
		return sandboxdomain.Run{}, fmt.Errorf("local Sandbox lifecycle: state update: %w", err)
	}
	if changed != 1 {
		return sandboxdomain.Run{}, fmt.Errorf("sandbox lifecycle: stale run or execution fence: %w", sandboxdomain.ErrConflict)
	}
	scope := caller.Scope()
	_, err = tx.ExecContext(ctx, `INSERT INTO sandbox_lifecycle_events(sandbox_id,action,previous_state,next_state,generation,principal_kind,principal_ref,origin,scope_kind,scope_ref,finished_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, expected.SandboxID.String(), string(action), string(expected.State), string(next.State), next.Generation, string(principal.Kind()), principal.Reference(), string(caller.Origin()), string(scope.Kind()), scope.Reference(), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return sandboxdomain.Run{}, fmt.Errorf("local Sandbox lifecycle: audit: %w", err)
	}
	if fenceToken != "" {
		released, releaseErr := tx.ExecContext(ctx, `DELETE FROM sandbox_execution_leases WHERE sandbox_id=? AND lease_token=?`, expected.SandboxID.String(), fenceToken)
		if releaseErr != nil || !oneRow(released) {
			return sandboxdomain.Run{}, fmt.Errorf("SSH lifecycle: unable to consume fence: %w", sandboxdomain.ErrConflict)
		}
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
