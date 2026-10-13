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

// QuiescentJobs is a read-only plugin-owned proof after Core has reserved an
// exclusive lifecycle fence. It never launches units or changes Task states.
func (b *Backend) QuiescentJobs(ctx context.Context, id sandboxdomain.ID) error {
	return b.WithJobs(ctx, id, func(jobs *Jobs) error { return jobs.Quiescent(ctx) })
}
