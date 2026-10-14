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

// SubmitSSHJob is invoked only by the reviewed SSH Jobs Provider Facade.
// It uses a distinct JobSubmit grant and Sandbox Policy, never Workspace Shell
// authority. The plugin, not Mender Core, owns TaskID, runner and recovery.
func (s *Sandboxes) SubmitSSHJob(ctx context.Context, id sandboxdomain.ID,
	caller gov.Context, request sshremote.JobRequest) (snapshot sshremote.JobSnapshot, retErr error) {
	if s == nil || s.sshRemote == nil || s.store == nil {
		return snapshot, sandboxdomain.ErrBackendUnavailable
	}
	if err := request.Validate(); err != nil {
		return snapshot, err
	}
	item, err := s.store.Get(ctx, id)
	if err != nil {
		return snapshot, err
	}
	if item.Backend() != sandboxdomain.BackendSSHRemote || item.Spec().Binding != sandboxdomain.BuiltinBinding(sandboxdomain.BackendSSHRemote) {
		return snapshot, sandboxdomain.ErrBackendUnavailable
	}
	if err := caller.Validate(); err != nil {
		return snapshot, err
	}
	run, err := s.store.GetRun(ctx, id)
	if err != nil {
		return snapshot, err
	}
	if run.State != sandboxdomain.StateReady {
		return snapshot, sandboxdomain.ErrConflict
	}
	// Fast local rejection avoids remote session/credential access for paused
	// admissions. The atomic SQLite Reserve below is still authoritative when
	// another Runtime pauses between this read and a live SSH handshake.
	paused, err := s.store.SSHJobSubmissionsPaused(ctx, id)
	if err != nil {
		return snapshot, err
	}
	if paused {
		return snapshot, sandboxdomain.ErrConflict
	}
	// Host-owned approval/audit before any network or Credential resolution.
	if _, err := s.VerifiedSSHJobContracts(ctx, id); err != nil {
		return snapshot, err
	}
	if err := s.admission.AdmitAcquire(ctx, caller, sandboxapp.Placement{Sandbox: item, Run: run, Required: sandboxdomain.CapabilityJobSubmit}); err != nil {
		return snapshot, err
	}
	session, err := s.sshRemote.Open(ctx, item, run)
	if err != nil {
		return snapshot, err
	}
	defer func() { retErr = errors.Join(retErr, session.Close()) }()
	typed, ok := session.(*sshremote.Session)
	if !ok {
		return snapshot, sandboxdomain.ErrBackendUnavailable
	}
	jobs, err := typed.Jobs()
	if err != nil {
		return snapshot, err
	}
	stopCancellationClose := context.AfterFunc(ctx, func() { _ = session.Close() })
	defer stopCancellationClose()
	// The storage sibling must be safely creatable before enrolling the host
	// obligation; preparation has no TaskID or executable side effect.
	if err := jobs.Prepare(ctx); err != nil {
		return snapshot, err
	}
	// Production accepts only the execution-fenced v2 protocol. Missing
	// remote fsync/flock capability fails closed before admission.
	if err := jobs.CheckV2(ctx); err != nil {
		return snapshot, err
	}
	// Reserve and mark a non-sensitive lifecycle obligation AFTER successful
	// transport verification but BEFORE first remote task storage mutation.
	// SQLite's unique execution lease serializes submissions against Stop/Destroy.
	token, err := s.store.ReserveSSHJobSubmit(ctx, id, run.Generation, caller)
	if err != nil {
		return snapshot, err
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		retErr = errors.Join(retErr, s.store.ReleaseExecution(releaseCtx, id, token))
	}()
	return jobs.Submit(ctx, request, run.Generation)
}
