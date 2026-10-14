package sshremote

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// A fake JobRunner is an in-memory systemd service for the test-owned SFTP
// fixture. It has no privileged shell or real SSH side effects.
type fixtureJobRunner struct {
	mu      sync.Mutex
	starts  int
	stops   int
	state   JobState
	lostAck bool
}

func (f *fixtureJobRunner) Start(_ context.Context, _, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.starts++
	f.state = JobRunning
	if f.lostAck {
		return ErrJobUnknown
	}
	return nil
}
func (f *fixtureJobRunner) Observe(_ context.Context, _ string) (JobState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state, nil
}
func (f *fixtureJobRunner) Stop(_ context.Context, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stops++
	f.state = JobUnknown
	return nil
}
func fixtureJobs(t *testing.T, runner JobRunner) (*Jobs, func() *Jobs) {
	t.Helper()
	template, cred := sshFixture(t)
	dial := Dialer{Credentials: cred}
	connect := func() *Jobs {
		t.Helper()
		conn, err := dial.Connect(context.Background(), template)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = conn.Close() })
		jobs, err := newJobs(sandboxdomain.ID("sbx_00000000000000000000000000000011"), template.Root, conn.SFTP, runner)
		if err != nil {
			t.Fatal(err)
		}
		return jobs
	}
	return connect(), connect
}
func TestSSHDurableJobsSessionRequiresExplicitRootAccount(t *testing.T) {
	template, credentials := sshFixture(t)
	connection, err := (Dialer{Credentials: credentials}).Connect(context.Background(), template)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = connection.Close() }()
	item := &Session{conn: connection, root: template.Root, sandboxID: sandboxdomain.ID("sbx_00000000000000000000000000000011"), remoteUser: "agent"}
	if _, err = item.Jobs(); !errors.Is(err, ErrJobUnavailable) {
		t.Fatalf("non-root account accepted: %v", err)
	}
	item.remoteUser = "root"
	if _, err = item.Jobs(); err != nil {
		t.Fatalf("root account unavailable: %v", err)
	}
}
func TestSystemdJobRunnerStrictUnitNaming(t *testing.T) {
	for _, value := range []string{"mender-job.service", "mender-sjob_abc.service", "mender-sjob_" + strings.Repeat("0", 32) + ".target", "x" + strings.Repeat("a", 40) + ".service"} {
		if validUnit(value) {
			t.Fatalf("accepted arbitrary unit %q", value)
		}
	}
	if !validUnit("mender-sjob_" + strings.Repeat("a", 32) + ".service") {
		t.Fatal("bounded service identity rejected")
	}
}
func TestSSHJobsV2ValidateRequestAndSymlink(t *testing.T) {
	for _, key := range []string{"", "not-random", "AAAA0000111122223333444455556666", strings.Repeat("0", 33)} {
		if (JobRequest{IdempotencyKey: key, Command: "echo hi"}).Validate() == nil {
			t.Fatalf("invalid key %q accepted", key)
		}
	}
	if (JobRequest{IdempotencyKey: strings.Repeat("a", 32), Command: strings.Repeat("x", maxJobCommandBytes+1)}).Validate() == nil {
		t.Fatal("oversized command accepted")
	}
	jobs, _, runner := fixtureV2Jobs(t)
	if err := os.Symlink(t.TempDir(), jobs.root+jobStorageDir); err != nil {
		t.Fatal(err)
	}
	_, err := jobs.Submit(context.Background(), JobRequest{IdempotencyKey: strings.Repeat("1", 32), Command: "echo no"}, 1)
	if err == nil || runner.starts != 0 {
		t.Fatalf("symlinked task store accepted: %v", err)
	}
}
func TestSSHJobsV2StatusLogsAndCancellation(t *testing.T) {
	ctx := context.Background()
	jobs, reconnect, runner := fixtureV2Jobs(t)
	if err := jobs.Prepare(ctx); err != nil {
		t.Fatal(err)
	}
	req := JobRequest{IdempotencyKey: strings.Repeat("a", 32), Command: "echo test-only"}
	first, err := jobs.Submit(ctx, req, 1)
	if err != nil || first.TaskID == "" {
		t.Fatalf("submit failed: %+v %v", first, err)
	}
	manifest, _, err := jobs.store.readV2Manifest(ctx, first.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	start := "v2:" + first.TaskID + ":" + manifest.AttemptID + ":" + manifest.CommandDigest + "\n"
	dir := jobs.store.taskPath(first.TaskID)
	if err = os.WriteFile(filepath.Join(dir, "start.intent"), []byte(start), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "stdout"), []byte("hello from v2"), 0600); err != nil {
		t.Fatal(err)
	}
	log, err := jobs.Logs(ctx, first.TaskID, "stdout", 0, 5)
	if err != nil || log.Content != "hello" || log.Cursor != 5 || !log.Truncated {
		t.Fatalf("bounded log: %+v %v", log, err)
	}
	resumed := reconnect()
	log, err = resumed.Logs(ctx, first.TaskID, "stdout", log.Cursor, 512)
	if err != nil || log.Content != " from v2" || log.Truncated {
		t.Fatalf("resumed log: %+v %v", log, err)
	}
	running, err := resumed.Status(ctx, first.TaskID)
	if err != nil || running.State != JobRunning {
		t.Fatalf("start evidence not observed: %+v %v", running, err)
	}
	stopped, err := resumed.Cancel(ctx, first.TaskID)
	if err != nil || !stopped.CancelRequested || stopped.State != JobUnknown || runner.stops != 1 {
		t.Fatalf("cancel fabricated terminal: %+v %v", stopped, err)
	}
	if err = resumed.Quiescent(ctx); !errors.Is(err, ErrJobUnknown) {
		t.Fatalf("cancelled-but-unproved task allowed lifecycle: %v", err)
	}
	if err = os.WriteFile(filepath.Join(dir, "exit"), []byte("143\n"), 0600); err != nil {
		t.Fatal(err)
	}
	done, err := resumed.Status(ctx, first.TaskID)
	if err != nil || done.State != JobCancelled || done.ExitCode == nil || *done.ExitCode != 143 {
		t.Fatalf("cancel result not verified: %+v %v", done, err)
	}
	if err = resumed.Quiescent(ctx); err != nil {
		t.Fatalf("terminal evidence rejected: %v", err)
	}
}
func TestSSHJobsV2RetiredIDsPreventReplayWithoutTaskData(t *testing.T) {
	ctx := context.Background()
	jobs, reconnect, runner := fixtureV2Jobs(t)
	if err := jobs.Prepare(ctx); err != nil {
		t.Fatal(err)
	}
	req := JobRequest{IdempotencyKey: strings.Repeat("b", 32), Command: "echo never-execute-retired"}
	id := jobs.identity(req.IdempotencyKey)
	marker := jobs.store.retiredMarkerPath(id)
	if err := os.WriteFile(marker, []byte(retiredMarker), 0600); err != nil {
		t.Fatal(err)
	}
	snap, err := jobs.Submit(ctx, req, 1)
	if !errors.Is(err, ErrJobConflict) || snap.TaskID != id || runner.starts != 0 {
		t.Fatalf("retired ID replayed: %+v %v", snap, err)
	}
	reopened := reconnect()
	_, err = reopened.Submit(ctx, req, 1)
	if !errors.Is(err, ErrJobConflict) {
		t.Fatalf("restarted runtime forgot retired TaskID: %v", err)
	}
	report, err := reopened.InspectRecovery(ctx)
	if err != nil || len(report.Records) != 0 {
		t.Fatalf("retired marker exposed as running task: %+v %v", report, err)
	}
	if err = reopened.Quiescent(ctx); err != nil {
		t.Fatalf("retired-only inventory blocked lifecycle: %v", err)
	}
	if err = os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink("/tmp/forged-retired-marker", marker); err != nil {
		t.Fatal(err)
	}
	_, err = reopened.Submit(ctx, req, 1)
	if !errors.Is(err, ErrJobUnknown) {
		t.Fatalf("symlinked retired marker permitted replay: %v", err)
	}
}
