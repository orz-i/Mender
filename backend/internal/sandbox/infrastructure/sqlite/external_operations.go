package sqlite

import (
	"context"
	"fmt"
	"strings"
	"time"

	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// BeginExternalOperation persists a never-reused operation key while verifying
// the exact currently fenced Run generation and owner. No remote call is allowed
// if the insert fails; a process crash cannot erase this evidence.
func (s *Store) BeginExternalOperation(ctx context.Context, run sandboxdomain.Run,
	action gov.Action, caller gov.Context, operationID string) error {
	if _, err := sandboxdomain.NewRun(run.SandboxID, run.Generation, run.State, run.ExternalID); err != nil {
		return err
	}
	if err := action.Validate(); err != nil {
		return err
	}
	if err := caller.Validate(); err != nil {
		return err
	}
	if len(operationID) < 12 || len(operationID) > 80 || !strings.HasPrefix(operationID, "sbop_") {
		return fmt.Errorf("sandbox external operation id invalid")
	}
	switch action {
	case gov.ActionSandboxProvision, gov.ActionSandboxStart, gov.ActionSandboxStop,
		gov.ActionSandboxDestroy, gov.ActionSandboxReconcile:
	default:
		return fmt.Errorf("unsupported external lifecycle operation %s", action)
	}
	subject := caller.Principal()
	outcome, err := s.db.ExecContext(ctx, `INSERT INTO sandbox_external_operations(operation_id,sandbox_id,generation,action,state,created_at)
        SELECT ?,s.id,r.generation,?,'issued',? FROM sandboxes s
        JOIN sandbox_runs r ON r.sandbox_id=s.id
        WHERE s.id=? AND s.backend='external' AND r.generation=? AND r.state=? AND r.external_id=?
          AND s.owner_kind=? AND s.owner_ref=?
          AND NOT EXISTS(SELECT 1 FROM sandbox_execution_leases l WHERE l.sandbox_id=s.id)`,
		operationID, string(action), time.Now().UTC().Format(time.RFC3339Nano),
		run.SandboxID.String(), run.Generation, string(run.State), run.ExternalID,
		string(subject.Kind()), subject.Reference())
	if err != nil {
		return fmt.Errorf("persist external operation: %w", err)
	}
	count, err := outcome.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return sandboxdomain.ErrConflict
	}
	return nil
}

func (s *Store) FinishExternalOperation(ctx context.Context, operationID string, uncertain bool) error {
	status := "complete"
	if uncertain {
		status = "uncertain"
	}
	outcome, err := s.db.ExecContext(ctx, `UPDATE sandbox_external_operations
        SET state=?,finished_at=? WHERE operation_id=? AND state='issued'`,
		status, time.Now().UTC().Format(time.RFC3339Nano), operationID)
	if err != nil {
		return err
	}
	count, err := outcome.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return sandboxdomain.ErrConflict
	}
	return nil
}
