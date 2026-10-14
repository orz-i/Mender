package bootstrap

import (
	"context"
	"errors"
	"time"

	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sandboxsqlite "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sqlite"
	sshremote "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sshremote"
)

type SSHJobResolutionRequest struct {
	TaskID      string `json:"taskId"`
	Fingerprint string `json:"fingerprint"`
	Generation  uint64 `json:"generation"`
	Reason      string `json:"reason"`
}
type SSHJobResolutionReceipt struct {
	SandboxID  sandboxdomain.ID           `json:"sandboxId"`
	Generation uint64                     `json:"generation"`
	Evidence   sshremote.RecoveryEvidence `json:"evidence"`
}

// ResolveSSHJobRecovery is a deliberately separate owner-administrative
// operation, NEVER a Profile Tool, MCP Task, Agent delegation or Core job loop.
// Backend owns v2 execution markers; Core only authorizes the exact resource,
// acquires the short CAS lease and records a minimized decision audit.
func (s *Sandboxes) ResolveSSHJobRecovery(ctx context.Context, id sandboxdomain.ID,
	req SSHJobResolutionRequest, caller gov.Context) (receipt SSHJobResolutionReceipt, retErr error) {
	actor := string(caller.Principal().Kind()) + ":" + caller.Principal().Reference()
	backendRequest := sshremote.V2ResolveRequest{TaskID: req.TaskID,
		Fingerprint: req.Fingerprint, Generation: req.Generation, Reason: req.Reason, Actor: actor}
	if err := backendRequest.Validate(); err != nil {
		return receipt, err
	}
	item, run, err := s.sshJobOwner(ctx, id, caller)
	if err != nil {
		return receipt, err
	}
	if run.Generation != req.Generation {
		return receipt, sandboxdomain.ErrConflict
	}
	// Policy/approval before credential access, SSH dial and any remote changes.
	if err = s.admission.AdmitAction(ctx, caller, item, gov.ActionSandboxReconcile); err != nil {
		return receipt, err
	}
	paused, err := s.store.SSHJobSubmissionsPaused(ctx, id)
	if err != nil {
		return receipt, err
	}
	if !paused {
		return receipt, sandboxdomain.ErrConflict
	}
	token, err := s.store.ReserveSSHJobRecovery(ctx, run, caller)
	if err != nil {
		return receipt, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		retErr = errors.Join(retErr, s.store.ReleaseExecution(cleanup, id, token))
	}()
	evidence, operationErr := s.sshRemote.ResolveJobRecovery(ctx, id, backendRequest)
	outcome := "resolved"
	if operationErr != nil {
		outcome = "unknown"
		if errors.Is(operationErr, sshremote.ErrJobConflict) {
			outcome = "conflict"
		}
	}
	// Even if remote commit succeeded, a missing authoritative host audit
	// prevents a claimed success. The operator may inspect before retrying.
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	auditErr := s.store.RecordSSHJobRecoveryAttempt(auditCtx, sandboxsqlite.SSHRecoveryAuditInput{
		SandboxID: id, TaskID: req.TaskID, Fingerprint: req.Fingerprint, Reason: req.Reason,
		Generation: run.Generation, Outcome: outcome, Token: token, Caller: caller,
	})
	if auditErr != nil {
		return receipt, errors.Join(sshremote.ErrJobUnknown, auditErr)
	}
	if operationErr != nil {
		return receipt, operationErr
	}
	return SSHJobResolutionReceipt{SandboxID: id, Generation: run.Generation, Evidence: evidence}, nil
}
