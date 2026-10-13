package bootstrap

import (
	"context"
	"errors"
	"time"

	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxapp "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sshremote "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sshremote"
)

// VerifySSHJobResource is a side-effect-free, exact-pinned capability check
// used by the host-owned Provider Facade on configuration/Tool discovery.
// It reads no secrets and never dials the SSH host.
func (s *Sandboxes) VerifySSHJobResource(ctx context.Context, id sandboxdomain.ID) error {
	_, err := s.VerifiedSSHJobContracts(ctx, id)
	if err != nil {
		return err
	}
	run, err := s.store.GetRun(ctx, id)
	if err != nil {
		return err
	}
	if run.State == sandboxdomain.StateDestroying || run.State == sandboxdomain.StateDestroyed {
		return sandboxdomain.ErrConflict
	}
	return nil
}

func (s *Sandboxes) authorizeSSHJob(ctx context.Context, id sandboxdomain.ID, caller gov.Context, cap sandboxdomain.Capability, requiresReady bool) (sandboxdomain.Run, error) {
	if s == nil || s.store == nil || s.sshRemote == nil {
		return sandboxdomain.Run{}, sandboxdomain.ErrBackendUnavailable
	}
	if cap != sandboxdomain.CapabilityJobSubmit && cap != sandboxdomain.CapabilityJobObserve && cap != sandboxdomain.CapabilityJobCancel {
		return sandboxdomain.Run{}, sandboxdomain.ErrCapabilityUnavailable
	}
	if err := s.VerifySSHJobResource(ctx, id); err != nil {
		return sandboxdomain.Run{}, err
	}
	item, err := s.store.Get(ctx, id)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	run, err := s.store.GetRun(ctx, id)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	if requiresReady && run.State != sandboxdomain.StateReady {
		return sandboxdomain.Run{}, sandboxdomain.ErrConflict
	}
	if !requiresReady && run.State != sandboxdomain.StateReady && run.State != sandboxdomain.StateStopped {
		return sandboxdomain.Run{}, sandboxdomain.ErrConflict
	}
	// sandbox.acquire admission checks exact trusted Profile Service identity,
	// capability-specific grant, Policy and audit before remote SSH/credentials.
	if err := s.admission.AdmitAcquire(ctx, caller, sandboxapp.Placement{Sandbox: item, Run: run, Required: cap}); err != nil {
		return sandboxdomain.Run{}, err
	}
	return run, nil
}

// SSHJobStatus and SSHJobResult are read-only observation paths: unlike
// submit/cancel they do NOT acquire sandbox_execution_leases. An old UNCERTAIN
// lease must not hide the plugin's ability to reconcile an existing TaskID.
func (s *Sandboxes) SSHJobStatus(ctx context.Context, id sandboxdomain.ID, caller gov.Context, taskID string) (sshremote.JobSnapshot, error) {
	if _, err := s.authorizeSSHJob(ctx, id, caller, sandboxdomain.CapabilityJobObserve, false); err != nil {
		return sshremote.JobSnapshot{}, err
	}
	var snapshot sshremote.JobSnapshot
	err := s.sshRemote.WithJobs(ctx, id, func(j *sshremote.Jobs) error { var err error; snapshot, err = j.Status(ctx, taskID); return err })
	return snapshot, err
}
func (s *Sandboxes) SSHJobResult(ctx context.Context, id sandboxdomain.ID, caller gov.Context, taskID string) (sshremote.JobSnapshot, error) {
	if _, err := s.authorizeSSHJob(ctx, id, caller, sandboxdomain.CapabilityJobObserve, false); err != nil {
		return sshremote.JobSnapshot{}, err
	}
	var result sshremote.JobSnapshot
	err := s.sshRemote.WithJobs(ctx, id, func(j *sshremote.Jobs) error { var err error; result, err = j.Result(ctx, taskID); return err })
	return result, err
}
func (s *Sandboxes) SSHJobLogs(ctx context.Context, id sandboxdomain.ID, caller gov.Context, taskID, stream string, cursor int64, limit int) (sshremote.JobLogs, error) {
	if _, err := s.authorizeSSHJob(ctx, id, caller, sandboxdomain.CapabilityJobObserve, false); err != nil {
		return sshremote.JobLogs{}, err
	}
	var result sshremote.JobLogs
	err := s.sshRemote.WithJobs(ctx, id, func(j *sshremote.Jobs) error {
		var err error
		result, err = j.Logs(ctx, taskID, stream, cursor, limit)
		return err
	})
	return result, err
}

// Cancel requests an execution-side effect and therefore takes a SHORT,
// Profile-grant-checked SQLite lease before any SSH connection. Observers do
// not gain cancellation permission. No terminal status is fabricated after a
// lost stop acknowledgment.
func (s *Sandboxes) SSHJobCancel(ctx context.Context, id sandboxdomain.ID, caller gov.Context, taskID string) (snapshot sshremote.JobSnapshot, retErr error) {
	run, err := s.authorizeSSHJob(ctx, id, caller, sandboxdomain.CapabilityJobCancel, true)
	if err != nil {
		return snapshot, err
	}
	token, err := s.store.ReserveExecution(ctx, id, run.Generation, caller, sandboxdomain.CapabilityJobCancel)
	if err != nil {
		return snapshot, err
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		retErr = errors.Join(retErr, s.store.ReleaseExecution(releaseCtx, id, token))
	}()
	err = s.sshRemote.WithJobs(ctx, id, func(j *sshremote.Jobs) error { var err error; snapshot, err = j.Cancel(ctx, taskID); return err })
	return snapshot, err
}
