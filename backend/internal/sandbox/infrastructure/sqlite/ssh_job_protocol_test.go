package sqlite

import (
	"context"
	"errors"
	"testing"

	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

func TestSSHJobV2ActivationPersistsAndFencesOldDispatchers(t *testing.T) {
	ctx := context.Background()
	filename, id, run, owner := newFencedSSHFixture(t)
	first, err := Open(ctx, filename)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	other, err := Open(ctx, filename)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.Close() }()
	if version, err := first.SSHJobProtocol(ctx, id); err != nil || version != SSHJobProtocolLegacy {
		t.Fatalf("expected legacy before explicit activation: %d %v", version, err)
	}
	if err = first.EnableSSHJobV2(ctx, run, owner); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("unpaused activation succeeded: %v", err)
	}
	token, err := first.ReserveSSHJobSubmit(ctx, id, run.Generation, owner, SSHJobProtocolLegacy)
	if err != nil {
		t.Fatal(err)
	}
	if err = other.SetSSHJobSubmissionsPaused(ctx, id, run.Generation, true, owner); err != nil {
		t.Fatal(err)
	}
	if err = other.EnableSSHJobV2(ctx, run, owner); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("active v1 lease was overtaken: %v", err)
	}
	if err = first.ReleaseExecution(ctx, id, token); err != nil {
		t.Fatal(err)
	}
	wrong, _ := gov.NewPrincipal(gov.PrincipalLocalUser, "os:not-owner")
	outsider, _ := gov.NewContext(gov.OriginCLI, wrong, gov.CatalogScope())
	if err = first.EnableSSHJobV2(ctx, run, outsider); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("different owner activated v2: %v", err)
	}
	if err = first.EnableSSHJobV2(ctx, run, owner); err != nil {
		t.Fatal(err)
	}
	if err = other.EnableSSHJobV2(ctx, run, owner); err != nil {
		t.Fatalf("idempotent cross-runtime activation: %v", err)
	}
	if version, err := other.SSHJobProtocol(ctx, id); err != nil || version != SSHJobProtocolV2 {
		t.Fatalf("committed v2 invisible in second process: %d %v", version, err)
	}
	var count int
	err = first.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sandbox_ssh_job_protocol_events WHERE sandbox_id=?`, id.String()).Scan(&count)
	if err != nil || count != 1 {
		t.Fatalf("one-way activation audit: %d %v", count, err)
	}
	if _, err = first.db.ExecContext(ctx, `UPDATE sandbox_ssh_job_protocols SET protocol=1 WHERE sandbox_id=?`, id.String()); err == nil {
		t.Fatal("protocol downgraded by SQL")
	}
	if _, err = first.db.ExecContext(ctx, `DELETE FROM sandbox_ssh_job_protocols WHERE sandbox_id=?`, id.String()); err == nil {
		t.Fatal("protocol activation tombstone deleted")
	}
	if err = other.SetSSHJobSubmissionsPaused(ctx, id, run.Generation, false, owner); err != nil {
		t.Fatal(err)
	}
	// A v1 caller that inspected the protocol BEFORE activation must now be
	// rejected atomically when it finally tries to reserve its submission.
	if _, err = first.ReserveSSHJobSubmit(ctx, id, run.Generation, owner, SSHJobProtocolLegacy); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("old v1 writer admitted AFTER durable v2: %v", err)
	}
	v2Token, err := other.ReserveSSHJobSubmit(ctx, id, run.Generation, owner, SSHJobProtocolV2)
	if err != nil {
		t.Fatalf("new v2 writer denied: %v", err)
	}
	if err = other.ReleaseExecution(ctx, id, v2Token); err != nil {
		t.Fatal(err)
	}
	if err = first.Close(); err != nil {
		t.Fatal(err)
	}
	first = nil
	restart, err := Open(ctx, filename)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = restart.Close() }()
	if version, err := restart.SSHJobProtocol(ctx, id); err != nil || version != SSHJobProtocolV2 {
		t.Fatalf("restart lost permanent v2 activation: %d %v", version, err)
	}
}

func TestSSHJobV2ActivationRequiresReadyRunAndCannotSkipOwner(t *testing.T) {
	ctx := context.Background()
	filename, id, run, owner := newFencedSSHFixture(t)
	store, err := Open(ctx, filename)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	if err = store.SetSSHJobSubmissionsPaused(ctx, id, run.Generation, true, owner); err != nil {
		t.Fatal(err)
	}
	fake := run
	fake.Generation++ // exact generation guard before any database side effect
	if err = store.EnableSSHJobV2(ctx, fake, owner); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("stale Run activated protocol: %v", err)
	}
	servicePrincipal, _ := gov.NewPrincipal(gov.PrincipalService, "service:forbidden")
	profileCaller, _ := gov.NewContext(gov.OriginMCP, servicePrincipal, gov.CatalogScope())
	if err = store.EnableSSHJobV2(ctx, run, profileCaller); !errors.Is(err, sandboxdomain.ErrNotAuthorized) {
		t.Fatalf("MCP service activated v2: %v", err)
	}
}
