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

// submitSSHJob is an INTERNAL owner-only integration seam until Stage C adds
// dedicated job-specific Profile/Policy/Tool authorization. It must never be
// projected directly as an MCP/HTTP/CLI Tool or use Workspace's shell grants
// as authority for detached jobs. The SSH plugin, not this method, owns tasks.
func (s *Sandboxes) submitSSHJob(ctx context.Context, id sandboxdomain.ID,
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
	authorized := sandboxapp.IsGrantManager(caller, item)
	if !authorized && caller.Origin() == gov.OriginInternal && caller.Principal().Kind() == gov.PrincipalLocalUser && caller.Scope().Kind() == gov.ScopeCatalog {
		authorized = sandboxapp.IsOwner(caller, item)
	}
	if !authorized {
		return snapshot, sandboxdomain.ErrNotAuthorized
	}
	run, err := s.store.GetRun(ctx, id)
	if err != nil {
		return snapshot, err
	}
	if run.State != sandboxdomain.StateReady {
		return snapshot, sandboxdomain.ErrConflict
	}
	// Host-owned approval/audit before any network or Credential resolution.
	if err := s.admission.AdmitAction(ctx, caller, item, gov.ActionSandboxAcquire); err != nil {
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
	return jobs.Submit(ctx, request)
}
