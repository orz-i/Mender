package sshremote

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/pkg/sftp"
)

type fixtureV2Runner struct {
	fixtureJobRunner
	muV2         sync.Mutex
	verifyErr    error
	syncErr      error
	recoveryErr  error
	recovered    int
	recoveryHold chan struct{}
	locked       chan struct{}
}

func (f *fixtureV2Runner) VerifyV2(_ context.Context, _ string) error           { return f.verifyErr }
func (f *fixtureV2Runner) PersistV2Directory(_ context.Context, _ string) error { return f.syncErr }
func (f *fixtureV2Runner) ResolveV2UnderLock(_ context.Context, r v2RemoteResolution) error {
	if f.recoveryErr != nil {
		return f.recoveryErr
	}
	lock, err := os.OpenFile(filepath.Join(r.TaskDir, "launch.lock"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return ErrJobUnknown
	}
	defer func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) }()
	if f.locked != nil {
		select {
		case f.locked <- struct{}{}:
		default:
		}
	}
	if f.recoveryHold != nil {
		<-f.recoveryHold
	}
	sha := func(name string) (string, error) {
		// #nosec G304 -- disposable in-process SFTP fixture, never a user path.
		raw, e := os.ReadFile(filepath.Join(r.TaskDir, name))
		if e != nil {
			return "", e
		}
		h := sha256.Sum256(raw)
		return hex.EncodeToString(h[:]), nil
	}
	for name, want := range map[string]string{"manifest.json": r.ManifestHash, "run.sh": r.ScriptHash} {
		got, e := sha(name)
		if e != nil || got != want {
			return ErrJobUnknown
		}
	}
	if r.LaunchPresent {
		got, e := sha("launch.intent")
		if e != nil || got != r.LaunchHash {
			return ErrJobUnknown
		}
	} else if _, e := os.Lstat(filepath.Join(r.TaskDir, "launch.intent")); !errors.Is(e, os.ErrNotExist) {
		return ErrJobUnknown
	}
	for _, name := range []string{"start.intent", "start.intent.tmp", "exit", "exit.tmp", "resolution.json", "resolution.tmp"} {
		if _, e := os.Lstat(filepath.Join(r.TaskDir, name)); !errors.Is(e, os.ErrNotExist) {
			return ErrJobUnknown
		}
	}
	p := filepath.Join(r.TaskDir, "resolution.json")
	// #nosec G304 -- test-only private task directory in disposable SSH fixture.
	file, e := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	n, e := file.Write(r.Marker)
	if e != nil || n != len(r.Marker) {
		_ = file.Close()
		return ErrJobUnknown
	}
	if e = file.Sync(); e != nil {
		_ = file.Close()
		return e
	}
	if e = file.Close(); e != nil {
		return e
	}
	directory, e := os.Open(r.TaskDir)
	if e != nil {
		return e
	}
	e = directory.Sync()
	_ = directory.Close()
	if e != nil {
		return e
	}
	f.muV2.Lock()
	f.recovered++
	f.muV2.Unlock()
	return nil
}

func fixtureV2Jobs(t *testing.T) (*Jobs, func() *Jobs, *fixtureV2Runner) {
	t.Helper()
	runner := &fixtureV2Runner{}
	jobs, reconnect := fixtureJobs(t, runner)
	// Test-only durable injection models successful fsync@openssh.com. This
	// must never be configured by production NewJobs / Runtime.
	jobs.store.v2SyncFixture = func(*sftp.File) error { return nil }
	makeReconnect := func() *Jobs {
		j := reconnect()
		j.store.v2SyncFixture = func(*sftp.File) error { return nil }
		return j
	}
	return jobs, makeReconnect, runner
}

func TestV2JournalUnknownNeverReplaysOnLostAck(t *testing.T) {
	ctx := context.Background()
	j, again, runner := fixtureV2Jobs(t)
	runner.lostAck = true
	req := JobRequest{IdempotencyKey: strings.Repeat("8", 32), Command: "echo run-once"}
	if err := j.Prepare(ctx); err != nil {
		t.Fatal(err)
	}
	first, e := j.SubmitV2(ctx, req, 1)
	if !errors.Is(e, ErrJobUnknown) || first.TaskID == "" || runner.starts != 1 {
		t.Fatalf("expected lost-ACK without replay: %+v %v", first, e)
	}
	reopened := again()
	snapshot, e := reopened.SubmitV2(ctx, req, 1)
	if e != nil || snapshot.TaskID != first.TaskID || runner.starts != 1 {
		t.Fatalf("retried v2 side effects %+v %v", snapshot, e)
	}
	other, e := reopened.SubmitV2(ctx, JobRequest{IdempotencyKey: req.IdempotencyKey, Command: "echo DIFFERENT"}, 1)
	if !errors.Is(e, ErrJobConflict) || other.TaskID != first.TaskID {
		t.Fatalf("v2 conflicting key: %+v %v", other, e)
	}
	version, e := j.store.manifestVersion(ctx, first.TaskID)
	if e != nil || version != 2 {
		t.Fatalf("expected v2 on disk: %d %v", version, e)
	}
	inspection, e := reopened.InspectRecovery(ctx)
	if e != nil || len(inspection.Records) != 1 || inspection.Records[0].Phase != "prelaunch_candidate" || !inspection.Records[0].Recoverable {
		t.Fatalf("lost ACK evidence: %+v %v", inspection, e)
	}
}

func TestV2ResolveUnderLockStaleEvidenceAndOperationalQuiescence(t *testing.T) {
	ctx := context.Background()
	j, reconnect, runner := fixtureV2Jobs(t)
	if err := j.Prepare(ctx); err != nil {
		t.Fatal(err)
	}
	req := JobRequest{IdempotencyKey: strings.Repeat("a", 32), Command: "echo do-not-run"}
	snapshot, e := j.SubmitV2(ctx, req, 1)
	if e != nil {
		t.Fatal(e)
	}
	before, e := j.InspectRecovery(ctx)
	if e != nil || len(before.Records) != 1 || !before.Records[0].Recoverable {
		t.Fatalf("candidate: %+v %v", before, e)
	}
	if e = j.Quiescent(ctx); !errors.Is(e, ErrJobUnknown) {
		t.Fatalf("unresolved v2 allowed lifecycle: %v", e)
	}
	wrong := V2ResolveRequest{TaskID: snapshot.TaskID, Fingerprint: strings.Repeat("f", 64), Generation: 1, Actor: "owner:fixture", Reason: "reviewed prelaunch"}
	if _, e = j.ResolveV2UnderFence(ctx, wrong); !errors.Is(e, ErrJobConflict) {
		t.Fatalf("stale fingerprint resolved: %v", e)
	}
	good := wrong
	good.Fingerprint = before.Records[0].Fingerprint
	resolved, e := reconnect().ResolveV2UnderFence(ctx, good)
	if e != nil || resolved.Phase != "resolved_before_launch" || resolved.State != JobUnknown || resolved.Activity != "quiescent_proved" {
		t.Fatalf("fenced recovery result %+v %v", resolved, e)
	}
	if e = j.Quiescent(ctx); e != nil {
		t.Fatalf("operational v2 resolution did not prove quiescence: %v", e)
	}
	status, e := j.Status(ctx, snapshot.TaskID)
	if e != nil || status.State != JobUnknown || status.ExitCode != nil {
		t.Fatalf("resolved fabricated shell result: %+v %v", status, e)
	}
	if _, e = j.ResolveV2UnderFence(ctx, good); !errors.Is(e, ErrJobConflict) {
		t.Fatalf("same recovery executed twice: %v", e)
	}
	runner.muV2.Lock()
	count := runner.recovered
	runner.muV2.Unlock()
	if count != 1 {
		t.Fatalf("multiple resolutions accepted: %d", count)
	}
	if runner.starts != 1 {
		t.Fatalf("recovery replayed user command: starts=%d", runner.starts)
	}
}

func TestV2RecoveryRejectsUnsafePartialAndStartEvidence(t *testing.T) {
	for _, tc := range []struct{ name, file, data string }{
		{"start_intent", "start.intent", "unexpected"},
		{"partial_start", "start.intent.tmp", "partial"},
		{"partial_terminal", "exit.tmp", "partial"},
		{"symlink_resolution", "resolution.json", "symlink"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			j, _, _ := fixtureV2Jobs(t)
			if e := j.Prepare(ctx); e != nil {
				t.Fatal(e)
			}
			snapshot, e := j.SubmitV2(ctx, JobRequest{IdempotencyKey: strings.Repeat("c", 32), Command: "echo noop"}, 1)
			if e != nil {
				t.Fatal(e)
			}
			dst := filepath.Join(j.store.taskPath(snapshot.TaskID), tc.file)
			if tc.data == "symlink" {
				e = os.Symlink(filepath.Join(t.TempDir(), "absent"), dst)
			} else {
				e = os.WriteFile(dst, []byte(tc.data), 0600)
			}
			if e != nil {
				t.Fatal(e)
			}
			report, e := j.InspectRecovery(ctx)
			if e == nil && len(report.Records) == 1 && report.Records[0].Recoverable {
				t.Fatalf("unsafe evidence presented as recoverable: %+v", report)
			}
			req := V2ResolveRequest{TaskID: snapshot.TaskID, Generation: 1, Fingerprint: strings.Repeat("0", 64), Actor: "owner:fixture", Reason: "reviewed recovery"}
			if _, e := j.ResolveV2UnderFence(ctx, req); e == nil {
				t.Fatal("unsafe v2 record resolved")
			}
		})
	}
}

func TestV2DurabilityPrerequisitesNeverFallBackToV1(t *testing.T) {
	ctx := context.Background()
	j, _, runner := fixtureV2Jobs(t)
	if e := j.Prepare(ctx); e != nil {
		t.Fatal(e)
	}
	j.store.v2SyncFixture = nil // fixture SFTP offers no fsync@openssh.com.
	_, e := j.SubmitV2(ctx, JobRequest{IdempotencyKey: strings.Repeat("e", 32), Command: "echo no"}, 1)
	if !errors.Is(e, ErrJobUnavailable) {
		t.Fatalf("unsupported v2 fsync admitted: %v", e)
	}
	if runner.starts != 0 {
		t.Fatal("failed v2 preflight executed command")
	}
	j.store.v2SyncFixture = func(*sftp.File) error { return errors.New("fsync lost") }
	snapshot, e := j.SubmitV2(ctx, JobRequest{IdempotencyKey: strings.Repeat("f", 32), Command: "echo no"}, 1)
	if !errors.Is(e, ErrJobUnknown) || snapshot.TaskID == "" || runner.starts != 0 {
		t.Fatalf("uncertain fsync replayed: %+v %v", snapshot, e)
	}
	if e = j.Quiescent(ctx); !errors.Is(e, ErrJobUnknown) {
		t.Fatalf("lost fsync allowed quiescence: %v", e)
	}
}

func TestV2ForgedResolutionFingerprintNeverClearsQuiescence(t *testing.T) {
	ctx := context.Background()
	j, _, _ := fixtureV2Jobs(t)
	if err := j.Prepare(ctx); err != nil {
		t.Fatal(err)
	}
	req := JobRequest{IdempotencyKey: strings.Repeat("1", 32), Command: "echo no-auto-clear"}
	snap, err := j.SubmitV2(ctx, req, 1)
	if err != nil {
		t.Fatal(err)
	}
	ev, err := j.store.readV2Evidence(ctx, snap.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	marker := v2ResolutionMarker{
		Version: 2, TaskID: snap.TaskID, Disposition: "aborted_before_launch", ManifestHash: ev.manifestHash,
		ScriptHash: ev.scriptHash, AttemptID: ev.manifest.AttemptID, EvidenceFingerprint: strings.Repeat("d", 64),
		Actor: "local:forged", ReasonDigest: strings.Repeat("a", 64),
	}
	payload, err := json.Marshal(marker)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(j.store.taskPath(snap.TaskID), "resolution.json"), payload, 0600); err != nil {
		t.Fatal(err)
	}
	if err = j.Quiescent(ctx); !errors.Is(err, ErrJobUnknown) {
		t.Fatalf("forged resolution passed Quiescent: %v", err)
	}
	report, err := j.InspectRecovery(ctx)
	if err != nil || len(report.Records) != 1 || report.Records[0].Recoverable ||
		report.Records[0].Activity != "uncertain" {
		t.Fatalf("forged resolution made Task recoverable: %+v %v", report, err)
	}
}
