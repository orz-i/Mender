package sshremote

import (
	"context"

	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// WithJobs is the SSH plugin's backend-owned recovery/observation port. The
// caller must establish exact resource identity, Job Profile grants and
// sandbox.acquire Policy/audit BEFORE invocation. It takes no global Sandbox
// execution lease: an uncertain prior Submit cannot block read-only recovery.
// The callback cannot choose another resource or arbitrary backend.
func (b *Backend) WithJobs(ctx context.Context, id sandboxdomain.ID, execute func(*Jobs) error) error {
	if b == nil || b.Templates == nil || b.Dial.Credentials == nil || execute == nil {
		return ErrJobUnavailable
	}
	if _, err := sandboxdomain.ParseID(id.String()); err != nil {
		return ErrJobInvalid
	}
	t, err := b.Templates.GetSSHRemoteTemplate(ctx, id)
	if err != nil || t.User != "root" {
		return ErrJobUnavailable
	}
	conn, err := b.Dial.Connect(ctx, t)
	if err != nil {
		return ErrJobUnknown
	}
	defer func() { _ = conn.Close() }()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	jobs, err := newJobs(id, t.Root, conn.SFTP, systemdJobRunner{client: conn.SSH, user: t.User})
	if err != nil {
		return err
	}
	return execute(jobs)
}

// InspectJobRecovery is an owner-admin read-only evidence port. The host
// must perform a separate owner-only sandbox.reconcile Policy admission
// before any SSH connection. Inspection never reserves/releases a lease.
func (b *Backend) InspectJobRecovery(ctx context.Context, id sandboxdomain.ID) (RecoveryReport, error) {
	var report RecoveryReport
	err := b.WithJobs(ctx, id, func(j *Jobs) error {
		var inspectErr error
		report, inspectErr = j.InspectRecovery(ctx)
		return inspectErr
	})
	return report, err
}

// ResolveJobRecovery is called only after Core owner authorization and an
// exact exclusive recovery lease. The SSH plugin owns both evidence and the
// remote execution-point lock; the caller cannot choose a filesystem path.
func (b *Backend) ResolveJobRecovery(ctx context.Context, id sandboxdomain.ID, request V2ResolveRequest) (RecoveryEvidence, error) {
	var evidence RecoveryEvidence
	err := b.WithJobs(ctx, id, func(j *Jobs) error {
		var operationErr error
		evidence, operationErr = j.ResolveV2UnderFence(ctx, request)
		return operationErr
	})
	return evidence, err
}

// QuiescentJobs is a read-only plugin-owned proof after Core has reserved an
// exclusive lifecycle fence. It never launches units or changes Task states.
func (b *Backend) QuiescentJobs(ctx context.Context, id sandboxdomain.ID) error {
	return b.WithJobs(ctx, id, func(jobs *Jobs) error { return jobs.Quiescent(ctx) })
}
