package sshremote

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

func TestSSHJobsV2PlanHasNoRemoteEffectsAndOrdersExecutionGate(t *testing.T) {
	jobs, _ := fixtureJobs(t, &fixtureJobRunner{})
	key := strings.Repeat("3", 32)
	command := "printf 'guarded action' > /tmp/test-owned-example"
	req := JobRequest{IdempotencyKey: key, Command: command}
	first, err := jobs.planV2(req)
	if err != nil {
		t.Fatal(err)
	}
	again, err := jobs.planV2(req)
	if err != nil {
		t.Fatal(err)
	}
	if first.manifest.Version != 2 || first.manifest.TaskID != jobs.identity(key) ||
		first.manifest.Unit != jobs.unit(first.manifest.TaskID) ||
		first.manifest.ScriptDigest == "" || first.manifest.CommandDigest == "" ||
		first.manifest.AttemptID == again.manifest.AttemptID || first.script == again.script {
		t.Fatalf("v2 attempt identity invalid: %+v", first.manifest)
	}
	digest := sha256.Sum256([]byte(first.script))
	if first.manifest.ScriptDigest != hex.EncodeToString(digest[:]) {
		t.Fatal("v2 manifest lost verified gate script hash")
	}
	checks := []string{
		"exec 9>>", "flock -x 9", "resolution.json", "start.intent",
		"sync -f", "mv -T", "trap finish EXIT", "exec /bin/sh -c",
	}
	last := -1
	for _, part := range checks {
		at := strings.Index(first.script, part)
		if at < last || at < 0 {
			t.Fatalf("v2 gate ordering missing %q: previous=%d next=%d", part, last, at)
		}
		last = at
	}
	// Neither plan nor inspecting source definitions writes remote records,
	// starts systemd or delegates authority to Mender Core.
	ids, err := jobs.store.sftp.ReadDir(jobs.store.basePath())
	if err == nil && len(ids) > 0 {
		t.Fatalf("v2 planning wrote remote task directory: %d", len(ids))
	}
}

func TestSSHJobsV2GateRejectsInvalidPreparedIdentity(t *testing.T) {
	id := "sjob_" + strings.Repeat("a", 32)
	digest := strings.Repeat("f", 64)
	attempt := strings.Repeat("a", 32)
	root := "/tmp/mender-fixture"
	folder := root + jobStorageDir + "/sbx_" + strings.Repeat("0", 32) + "/" + id
	for _, test := range []struct{ name, root, folder, id, digest, attempt, command string }{
		{"invalidTask", root, folder, "other", digest, attempt, "echo"},
		{"unboundedRoot", "/tmp/../etc", folder, id, digest, attempt, "echo"},
		{"wrongDirectory", root, "/etc/other", id, digest, attempt, "echo"},
		{"invalidDigest", root, folder, id, "not-a-hash", attempt, "echo"},
		{"invalidAttempt", root, folder, id, digest, "oops", "echo"},
		{"oversizedCommand", root, folder, id, digest, attempt, strings.Repeat("x", maxJobCommandBytes+1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := gatedJobScriptV2(test.root, test.folder, test.id, test.digest, test.attempt, test.command); !errors.Is(err, ErrJobInvalid) {
				t.Fatalf("accepted invalid v2 gate: %v", err)
			}
		})
	}
}

// Local Linux-only integration: runs test-owned shell commands exclusively in
// a temporary workspace. No SSH host, real systemd, or user command is used.
// On macOS there is no util-linux flock and a live Rocky check is still needed.
func TestSSHJobsV2ExecutionGateStopsLateLaunchAndDoubleExecution(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("util-linux flock and GNU sync -f integration requires Linux")
	}
	for _, program := range []string{"flock", "sync", "mv"} {
		if _, err := exec.LookPath(program); err != nil {
			t.Skipf("%s unavailable", program)
		}
	}
	root := t.TempDir()
	sid := sandboxdomain.ID("sbx_" + strings.Repeat("b", 32))
	j := &Jobs{sandboxID: sid, root: root, store: &jobStore{root: root, sandboxID: sid}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	run := func(script string) (int, error) {
		command := exec.CommandContext(ctx, "/bin/sh", script) // #nosec G204 -- only a test-owned generated script
		err := command.Run()
		if err == nil {
			return 0, nil
		}
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode(), nil
		}
		return -1, err
	}
	materialize := func(key, command string) (v2GatePlan, string) {
		t.Helper()
		plan, err := j.planV2(JobRequest{IdempotencyKey: key, Command: command})
		if err != nil {
			t.Fatal(err)
		}
		dir := j.store.taskPath(plan.manifest.TaskID)
		if err = os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		script := filepath.Join(dir, "run.sh")
		if err = os.WriteFile(script, []byte(plan.script), 0600); err != nil {
			t.Fatal(err)
		}
		return plan, script
	}
	// Recoverer may have persisted resolution before a delayed systemd unit
	// runs. The script must suppress user code even if dispatch arrives later.
	suppressed := filepath.Join(root, "suppressed")
	before, lateScript := materialize(strings.Repeat("e", 32), "printf 'x' >> "+quoted(suppressed))
	if err := os.WriteFile(filepath.Join(j.store.taskPath(before.manifest.TaskID), "resolution.json"), []byte("test-owned-resolution"), 0600); err != nil {
		t.Fatal(err)
	}
	if status, err := run(lateScript); err != nil || status != 0 {
		t.Fatalf("late gate: %d %v", status, err)
	}
	if _, err := os.Stat(suppressed); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("delayed resolved task ran user code: %v", err)
	}
	if _, err := os.Stat(filepath.Join(j.store.taskPath(before.manifest.TaskID), "start.intent")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("suppressed task recorded user execution: %v", err)
	}
	// A successful first execution must leave a durable start marker and a
	// terminal file. Even an accidental repeated systemd dispatch cannot
	// execute a second time for the same immutable TaskID.
	sideEffect := filepath.Join(root, "side-effect")
	allowed, script := materialize(strings.Repeat("f", 32), "printf 'x' >> "+quoted(sideEffect))
	if status, err := run(script); err != nil || status != 0 {
		t.Fatalf("first v2 execution: %d %v", status, err)
	}
	taskDir := j.store.taskPath(allowed.manifest.TaskID)
	// #nosec G304 -- local Linux-only test reads its own temporary task marker.
	marker, err := os.ReadFile(filepath.Join(taskDir, "start.intent"))
	if err != nil || !strings.Contains(string(marker), allowed.manifest.AttemptID) {
		t.Fatalf("start marker unverified: %q %v", marker, err)
	}
	// #nosec G304 -- local Linux-only test reads its own temporary terminal marker.
	if result, err := os.ReadFile(filepath.Join(taskDir, "exit")); err != nil || string(result) != "0\n" {
		t.Fatalf("terminal result: %q %v", result, err)
	}
	if status, err := run(script); err != nil || status == 0 {
		t.Fatalf("second execution should be fenced: %d %v", status, err)
	}
	// #nosec G304 -- local Linux-only test reads its own temporary output artifact.
	effect, err := os.ReadFile(sideEffect)
	if err != nil || string(effect) != "x" {
		t.Fatalf("duplicate user side effect: %q %v", effect, err)
	}
}
