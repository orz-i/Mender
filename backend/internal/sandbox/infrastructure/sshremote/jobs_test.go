package sshremote

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

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
		engine, err := newJobs(sandboxdomain.ID("sbx_00000000000000000000000000000011"), template.Root, conn.SFTP, runner)
		if err != nil {
			t.Fatal(err)
		}
		return engine
	}
	return connect(), connect
}
func TestSSHDurableJobsIdempotencyStatusLogsAndRestart(t *testing.T) {
	fake := &fixtureJobRunner{}
	jobs, reconnect := fixtureJobs(t, fake)
	key := "0123456789abcdef0123456789abcdef"
	ctx := context.Background()
	request := JobRequest{IdempotencyKey: key, Command: "echo test-side-effect"}
	first, err := jobs.Submit(ctx, request)
	if err != nil || first.State != JobRunning || !jobIDPattern.MatchString(first.TaskID) {
		t.Fatalf("first submit: %+v %v", first, err)
	}
	next, err := jobs.Submit(ctx, request)
	if err != nil || next.TaskID != first.TaskID || next.State != JobRunning || fake.starts != 1 {
		t.Fatalf("non-idempotent retry: %+v %v start=%d", next, err, fake.starts)
	}
	conflict, err := jobs.Submit(ctx, JobRequest{IdempotencyKey: key, Command: "echo DIFFERENT"})
	if !errors.Is(err, ErrJobConflict) || conflict.TaskID != first.TaskID || fake.starts != 1 {
		t.Fatalf("conflict executed: %+v %v", conflict, err)
	}
	home := jobs.store.taskPath(first.TaskID)
	// #nosec G304 -- home is inside this test's private temporary SSH fixture.
	if data, err := os.ReadFile(filepath.Join(home, "run.sh")); err != nil || !strings.Contains(string(data), "echo test-side-effect") {
		t.Fatalf("missing durable script: %v", err)
	}
	for _, name := range []string{"manifest.json", "run.sh"} {
		info, e := os.Stat(filepath.Join(home, name))
		if e != nil || info.Mode().Perm()&0077 != 0 {
			t.Fatalf("unsafe job file permissions %s %v %v", name, info, e)
		}
	}
	if info, e := os.Stat(home); e != nil || info.Mode().Perm()&0077 != 0 {
		t.Fatalf("unsafe task directory %v %v", info, e)
	}
	if err = os.WriteFile(filepath.Join(home, "stdout"), []byte("hello world"), 0600); err != nil {
		t.Fatal(err)
	}
	log, e := jobs.Logs(ctx, first.TaskID, "stdout", 0, 5)
	if e != nil || log.Content != "hello" || log.Cursor != 5 || !log.Truncated {
		t.Fatalf("first logs %+v %v", log, e)
	}
	log, e = jobs.Logs(ctx, first.TaskID, "stdout", log.Cursor, 16)
	if e != nil || log.Content != " world" || log.Cursor != 11 || log.Truncated {
		t.Fatalf("resume logs %+v %v", log, e)
	}
	if err = os.WriteFile(filepath.Join(home, "exit"), []byte("0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	restarted := reconnect()
	done, err := restarted.Status(ctx, first.TaskID)
	if err != nil || done.State != JobCompleted || done.ExitCode == nil || *done.ExitCode != 0 || fake.starts != 1 {
		t.Fatalf("restart lost result: %+v %v", done, err)
	}
	if err = restarted.Quiescent(ctx); err != nil {
		t.Fatalf("terminal proof failed: %v", err)
	}
	if err = restarted.PruneOutput(ctx, first.TaskID); err != nil {
		t.Fatal(err)
	}
	again, err := restarted.Submit(ctx, request)
	if err != nil || again.State != JobCompleted || fake.starts != 1 {
		t.Fatalf("prune destroyed idempotency tombstone: %+v %v", again, err)
	}
}
func TestSSHDurableJobsLostAcknowledgementAndUnknownNoReplay(t *testing.T) {
	fake := &fixtureJobRunner{lostAck: true}
	jobs, reconnect := fixtureJobs(t, fake)
	ctx := context.Background()
	request := JobRequest{IdempotencyKey: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Command: "echo only-once"}
	first, err := jobs.Submit(ctx, request)
	if !errors.Is(err, ErrJobUnknown) || first.State != JobUnknown || fake.starts != 1 {
		t.Fatalf("ACK loss: %+v %v", first, err)
	}
	restored := reconnect()
	same, err := restored.Submit(ctx, request)
	if err != nil || same.State != JobRunning || fake.starts != 1 {
		t.Fatalf("ACK replay: %+v %v start=%d", same, err, fake.starts)
	}
	// A reservation with no complete manifest is never considered safe to replay.
	pendingKey := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	pendingID := jobs.identity(pendingKey)
	if err = os.Mkdir(jobs.store.taskPath(pendingID), 0700); err != nil {
		t.Fatal(err)
	}
	uncertain, e := restored.Submit(ctx, JobRequest{IdempotencyKey: pendingKey, Command: "echo unknown"})
	if !errors.Is(e, ErrJobUnknown) || uncertain.State != JobUnknown || fake.starts != 1 {
		t.Fatalf("partial reservation replayed: %+v %v", uncertain, e)
	}
	if err = restored.Quiescent(ctx); !errors.Is(err, ErrJobUnknown) {
		t.Fatalf("uncertain job did not fence lifecycle: %v", err)
	}
}
func TestSSHDurableJobsCancelDoesNotFabricateTermination(t *testing.T) {
	fake := &fixtureJobRunner{}
	jobs, _ := fixtureJobs(t, fake)
	ctx := context.Background()
	req := JobRequest{IdempotencyKey: "cccccccccccccccccccccccccccccccc", Command: "sleep 30"}
	first, err := jobs.Submit(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	cancel, err := jobs.Cancel(ctx, first.TaskID)
	if err != nil || !cancel.CancelRequested || cancel.State != JobUnknown || fake.stops != 1 {
		t.Fatalf("cancel falsely terminal: %+v %v", cancel, err)
	}
	if err = jobs.Quiescent(ctx); !errors.Is(err, ErrJobUnknown) {
		t.Fatalf("unconfirmed stop permitted lifecycle: %v", err)
	}
	// Only the durable remote wrapper exit is proof of terminal state.
	if err = os.WriteFile(filepath.Join(jobs.store.taskPath(first.TaskID), "exit"), []byte("143\n"), 0600); err != nil {
		t.Fatal(err)
	}
	stopped, err := jobs.Status(ctx, first.TaskID)
	if err != nil || stopped.State != JobCancelled || !stopped.CancelRequested {
		t.Fatalf("no confirmed cancellation %+v %v", stopped, err)
	}
}
func TestSSHDurableJobsValidateKeysAndRefuseSymlink(t *testing.T) {
	for _, key := range []string{"", "not-random", "AAAA0000111122223333444455556666", strings.Repeat("0", 33)} {
		if (JobRequest{IdempotencyKey: key, Command: "echo hi"}).Validate() == nil {
			t.Fatalf("accepted invalid key %q", key)
		}
	}
	if (JobRequest{IdempotencyKey: strings.Repeat("a", 32), Command: strings.Repeat("x", maxJobCommandBytes+1)}).Validate() == nil {
		t.Fatal("oversized command")
	}
	fake := &fixtureJobRunner{}
	jobs, _ := fixtureJobs(t, fake)
	// Directory symlink must never redirect persistent task material to a
	// workspace-external destination. None of the fake runner methods is used.
	other := t.TempDir()
	if err := os.Symlink(other, filepath.Join(jobs.root, jobStorageDir)); err != nil {
		t.Fatal(err)
	}
	_, err := jobs.Submit(context.Background(), JobRequest{IdempotencyKey: strings.Repeat("1", 32), Command: "echo nope"})
	if err == nil || fake.starts != 0 {
		t.Fatalf("symlink task store accepted: %v", err)
	}
}

// This runner starts only a test-owned child process under a temp workspace;
// it does not invoke systemd, SSH to a real machine, or touch user credentials.
type localFixtureRunner struct {
	mu     sync.Mutex
	starts int
	cmds   map[string]*exec.Cmd
}

func (r *localFixtureRunner) Start(_ context.Context, unit, script string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cmds == nil {
		r.cmds = map[string]*exec.Cmd{}
	}
	command := exec.Command("/bin/sh", script) // #nosec G204 -- script is written exclusively in a test-owned temporary workspace
	if err := command.Start(); err != nil {
		return err
	}
	r.cmds[unit] = command
	r.starts++
	go func() { _ = command.Wait() }()
	return nil
}
func (r *localFixtureRunner) Observe(_ context.Context, unit string) (JobState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.cmds[unit]; ok {
		return JobRunning, nil
	}
	return JobUnknown, nil
}
func (r *localFixtureRunner) Stop(_ context.Context, unit string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if command := r.cmds[unit]; command != nil && command.Process != nil {
		_ = command.Process.Kill()
	}
	return nil
}
func TestSSHDurableJobScriptSurvivesSSHDisconnect(t *testing.T) {
	runner := &localFixtureRunner{}
	jobs, reconnect := fixtureJobs(t, runner)
	req := JobRequest{IdempotencyKey: "dddddddddddddddddddddddddddddddd", Command: "sleep 0.4; printf 'finished after disconnect\\n'"}
	ctx := context.Background()
	started, err := jobs.Submit(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	// A subsequent SSH connection observes the same remote path and result.
	restored := reconnect()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		result, e := restored.Status(ctx, started.TaskID)
		if e == nil && result.State == JobCompleted {
			logs, e := restored.Logs(ctx, started.TaskID, "stdout", 0, 1024)
			if e != nil || logs.Content != "finished after disconnect\n" || runner.starts != 1 {
				t.Fatalf("result %+v %v", logs, e)
			}
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("detached script did not produce a durable completion file")
}
