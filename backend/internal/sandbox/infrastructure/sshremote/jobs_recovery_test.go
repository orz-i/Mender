package sshremote

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSSHRecoveryInspectorIsReadOnlyAndNeverClearsLegacyUnknown(t *testing.T) {
	runner := &fixtureJobRunner{}
	jobs, reconnect := fixtureJobs(t, runner)
	ctx := context.Background()
	if err := jobs.Prepare(ctx); err != nil {
		t.Fatal(err)
	}
	incompleteID := jobs.identity(strings.Repeat("a", 32))
	if err := os.Mkdir(jobs.store.taskPath(incompleteID), 0700); err != nil {
		t.Fatal(err)
	}
	manifestID := jobs.identity(strings.Repeat("b", 32))
	if err := os.Mkdir(jobs.store.taskPath(manifestID), 0700); err != nil {
		t.Fatal(err)
	}
	if err := jobs.store.writeManifest(ctx, manifestID, jobManifest{Version: 1, TaskID: manifestID, SandboxID: jobs.sandboxID.String(), CommandDigest: strings.Repeat("f", 64), Unit: jobs.unit(manifestID)}); err != nil {
		t.Fatal(err)
	}
	running, err := jobs.Submit(ctx, JobRequest{IdempotencyKey: strings.Repeat("c", 32), Command: "echo do-not-execute-in-inspection"})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(jobs.store.taskPath(running.TaskID), "exit"), []byte("0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadDir(jobs.store.taskPath(manifestID))
	if err != nil {
		t.Fatal(err)
	}
	reopened := reconnect()
	report, err := reopened.InspectRecovery(ctx)
	if err != nil || len(report.Records) != 3 || report.Verdict != "uncertain" {
		t.Fatalf("inspection: %+v %v", report, err)
	}
	phases := map[string]string{}
	for _, e := range report.Records {
		phases[e.TaskID] = e.Phase
		if e.Fingerprint == "" || e.Recoverable {
			t.Fatalf("legacy record falsely recoverable: %+v", e)
		}
	}
	if phases[incompleteID] != "reservation_only" || phases[manifestID] != "manifest_only" || phases[running.TaskID] != "terminal" {
		t.Fatalf("unexpected recovery phases: %v", phases)
	}
	after, err := os.ReadDir(jobs.store.taskPath(manifestID))
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) || runner.starts != 1 || runner.stops != 0 {
		t.Fatalf("inspection modified remote records or executed code: before=%d after=%d starts=%d stops=%d", len(before), len(after), runner.starts, runner.stops)
	}
	if err = reopened.Quiescent(ctx); !errors.Is(err, ErrJobUnknown) {
		t.Fatalf("inspector cleared uncertain job: %v", err)
	}
	second, err := reopened.InspectRecovery(ctx)
	if err != nil || second.Records[0].Fingerprint != report.Records[0].Fingerprint {
		t.Fatalf("same evidence did not retain fingerprint: %+v %v", second, err)
	}
}

func TestSSHRecoveryInspectorRejectsUnsafeEvidence(t *testing.T) {
	fake := &fixtureJobRunner{}
	jobs, _ := fixtureJobs(t, fake)
	ctx := context.Background()
	key := strings.Repeat("d", 32)
	if _, err := jobs.Submit(ctx, JobRequest{IdempotencyKey: key, Command: "echo noop"}); err != nil {
		t.Fatal(err)
	}
	id := jobs.identity(key)
	script := jobs.store.scriptPath(id)
	if err := os.Remove(script); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/tmp/untrusted-mender-recovery-script", script); err != nil {
		t.Fatal(err)
	}
	report, err := jobs.InspectRecovery(ctx)
	if err != nil || len(report.Records) != 1 || report.Records[0].Phase != "unverified" ||
		report.Records[0].Activity != "uncertain" || report.Records[0].Recoverable {
		t.Fatalf("unsafe symlink was accepted: %+v %v", report, err)
	}
}
