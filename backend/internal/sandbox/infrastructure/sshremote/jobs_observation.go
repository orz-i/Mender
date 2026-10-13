package sshremote

import (
	"context"

	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// QuiescentJobs performs plugin-owned remote task inventory reconciliation for
// an already admitted, exclusively fenced Sandbox lifecycle operation.
// The caller owns the authority/fence; the plugin owns the task contents and
// no-unknown/no-running proof. Reads never launch or repeat systemd units.
// It works for a stopped Sandbox: Open() intentionally requires Ready, so
// lifecycle checking uses the exact pinned SSH configuration instead.
func (b *Backend) QuiescentJobs(ctx context.Context, id sandboxdomain.ID) error {
	if b == nil || b.Templates == nil || b.Dial.Credentials == nil {
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
	stopCancellationClose := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopCancellationClose()
	jobs, err := newJobs(id, t.Root, conn.SFTP, systemdJobRunner{client: conn.SSH, user: t.User})
	if err != nil {
		return err
	}
	return jobs.Quiescent(ctx)
}
