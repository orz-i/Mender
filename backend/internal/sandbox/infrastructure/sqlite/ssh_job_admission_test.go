package sqlite

import (
	"context"
	"errors"
	"sync"
	"testing"

	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

func TestSSHJobPausePersistsAndAtomicallyDeniesSubmissionsAcrossRuntimes(t *testing.T) {
	filename, id, run, owner := newFencedSSHFixture(t)
	ctx := context.Background()
	first, err := Open(ctx, filename)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := Open(ctx, filename)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	paused, err := first.SSHJobSubmissionsPaused(ctx, id)
	if err != nil || paused {
		t.Fatalf("initial pause: %t %v", paused, err)
	}
	if err = first.SetSSHJobSubmissionsPaused(ctx, id, run.Generation, true, owner); err != nil {
		t.Fatal(err)
	}
	if err = first.SetSSHJobSubmissionsPaused(ctx, id, run.Generation, true, owner); err != nil {
		t.Fatal(err)
	}
	paused, err = second.SSHJobSubmissionsPaused(ctx, id)
	if err != nil || !paused {
		t.Fatalf("pause not durable across runtimes: %t %v", paused, err)
	}
	if _, err = second.ReserveSSHJobSubmit(ctx, id, run.Generation, owner); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("new job passed paused admission: %v", err)
	}
	managed, err := second.SSHJobManaged(ctx, id)
	if err != nil || managed {
		t.Fatalf("denied submit created task obligation: %t %v", managed, err)
	}
	var events int
	if err = first.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sandbox_ssh_job_admission_events WHERE sandbox_id=?`, id.String()).Scan(&events); err != nil || events != 1 {
		t.Fatalf("idempotent pause wrote duplicate events: %d %v", events, err)
	}
	otherPrincipal, _ := gov.NewPrincipal(gov.PrincipalLocalUser, "os:imposter")
	other, _ := gov.NewContext(gov.OriginCLI, otherPrincipal, gov.CatalogScope())
	if err = second.SetSSHJobSubmissionsPaused(ctx, id, run.Generation, false, other); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("other owner resumed: %v", err)
	}
	if err = second.SetSSHJobSubmissionsPaused(ctx, id, run.Generation, false, owner); err != nil {
		t.Fatal(err)
	}
	paused, err = first.SSHJobSubmissionsPaused(ctx, id)
	if err != nil || paused {
		t.Fatalf("resume not propagated: %t %v", paused, err)
	}
	token, err := first.ReserveSSHJobSubmit(ctx, id, run.Generation, owner)
	if err != nil {
		t.Fatal(err)
	}
	// Pause can be committed while an older admitted Submit is still using
	// its short lease. It blocks subsequent submits, not the older effect.
	if err = second.SetSSHJobSubmissionsPaused(ctx, id, run.Generation, true, owner); err != nil {
		t.Fatal(err)
	}
	if err = first.ReleaseExecution(ctx, id, token); err != nil {
		t.Fatal(err)
	}
	if _, err = first.ReserveSSHJobSubmit(ctx, id, run.Generation, owner); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("admission resumed implicitly after older lease finished: %v", err)
	}
	if _, err = second.ChangeSSHRemoteLifecycle(ctx, run, gov.ActionSandboxStop, owner); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("pause replaced required remote quiescence proof: %v", err)
	}
	if err = first.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sandbox_ssh_job_admission_events WHERE sandbox_id=?`, id.String()).Scan(&events); err != nil || events != 3 {
		t.Fatalf("missing pause/resume audit: %d %v", events, err)
	}
}

func TestSSHJobPauseRacesFirstSubmissionWithoutLosingState(t *testing.T) {
	for i := 0; i < 12; i++ {
		filename, id, run, owner := newFencedSSHFixture(t)
		ctx := context.Background()
		one, err := Open(ctx, filename)
		if err != nil {
			t.Fatal(err)
		}
		two, err := Open(ctx, filename)
		if err != nil {
			_ = one.Close()
			t.Fatal(err)
		}
		start := make(chan struct{})
		var wg sync.WaitGroup
		var tok string
		var submitErr, pauseErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			tok, submitErr = one.ReserveSSHJobSubmit(ctx, id, run.Generation, owner)
		}()
		go func() {
			defer wg.Done()
			<-start
			pauseErr = two.SetSSHJobSubmissionsPaused(ctx, id, run.Generation, true, owner)
		}()
		close(start)
		wg.Wait()
		if pauseErr != nil {
			t.Fatalf("pause must succeed or explicitly surface retriable DB error: %v", pauseErr)
		}
		paused, err := one.SSHJobSubmissionsPaused(ctx, id)
		if err != nil || !paused {
			t.Fatalf("pause lost: %t %v", paused, err)
		}
		if submitErr == nil {
			if tok == "" {
				t.Fatal("accepted submission missing short lease")
			}
			if err = one.ReleaseExecution(ctx, id, tok); err != nil {
				t.Fatal(err)
			}
		}
		if _, err = two.ReserveSSHJobSubmit(ctx, id, run.Generation, owner); !errors.Is(err, sandboxdomain.ErrConflict) {
			t.Fatalf("iteration %d: post-pause submission accepted: %v", i, err)
		}
		_ = one.Close()
		_ = two.Close()
	}
}
