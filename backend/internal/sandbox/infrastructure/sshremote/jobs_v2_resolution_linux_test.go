package sshremote

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	"syscall"
)

// Only a disposable Linux filesystem and local commands are touched. No
// SSH host, systemd, user Credential, or privileged Sandbox is used.
func TestSSHJobsV2LinuxRecoveryLockAndDelayedLaunch(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("requires Linux util-linux flock and GNU coreutils")
	}
	for _, program := range []string{"flock", "sync", "sha256sum", "mv", "cut"} {
		if _, err := exec.LookPath(program); err != nil {
			t.Skipf("missing %s", program)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	root := t.TempDir()
	id := sandboxdomain.ID("sbx_" + strings.Repeat("2", 32))
	j := &Jobs{root: root, sandboxID: id, store: &jobStore{root: root, sandboxID: id}}
	hash := func(raw []byte) string { h := sha256.Sum256(raw); return hex.EncodeToString(h[:]) }
	type scenario struct {
		dir, script string
		req         v2RemoteResolution
		sideEffect  string
	}
	makeScenario := func(key string) scenario {
		t.Helper()
		output := filepath.Join(root, "side-effect-"+key[:2])
		plan, err := j.planV2(JobRequest{IdempotencyKey: key, Command: "printf x >> " + quoted(output)})
		if err != nil {
			t.Fatal(err)
		}
		plan.manifest.RunGeneration = 1
		dir := j.store.taskPath(plan.manifest.TaskID)
		if err = os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		manifest, e := json.Marshal(plan.manifest)
		if e != nil {
			t.Fatal(e)
		}
		launch := []byte("v2:" + plan.manifest.TaskID + ":" + plan.manifest.AttemptID + ":" + plan.manifest.CommandDigest + "\n")
		for name, data := range map[string][]byte{"manifest.json": manifest, "run.sh": []byte(plan.script), "launch.intent": launch} {
			if err = os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
				t.Fatal(err)
			}
		}
		reasonDigest := hash([]byte("approved by isolated test fixture"))
		marker, e := json.Marshal(v2ResolutionMarker{Version: 2, TaskID: plan.manifest.TaskID, Disposition: "aborted_before_launch",
			ManifestHash: hash(manifest), ScriptHash: hash([]byte(plan.script)), AttemptID: plan.manifest.AttemptID,
			EvidenceFingerprint: strings.Repeat("f", 64), Actor: "local:test-fixture", ReasonDigest: reasonDigest})
		if e != nil {
			t.Fatal(e)
		}
		return scenario{dir: dir, script: filepath.Join(dir, "run.sh"), sideEffect: output,
			req: v2RemoteResolution{TaskDir: dir, ManifestHash: hash(manifest), ScriptHash: hash([]byte(plan.script)),
				LaunchHash: hash(launch), LaunchPresent: true, Marker: marker}}
	}
	run := func(args ...string) (string, error) {
		t.Helper()
		// #nosec G204 -- only test-generated fixed SSH backend gate/recovery shell.
		out, err := exec.CommandContext(ctx, args[0], args[1:]...).CombinedOutput()
		return string(out), err
	}
	t.Run("resolution_first_late_gate_blocked", func(t *testing.T) {
		s := makeScenario(strings.Repeat("a", 32))
		command, err := v2RecoveryShell(s.req)
		if err != nil {
			t.Fatal(err)
		}
		out, err := run("/bin/sh", "-c", command)
		if err != nil || out != "mender-v2-resolved" {
			t.Fatalf("locked resolution: %q %v", out, err)
		}
		proof, err := v2ResolutionProofShell(s.dir, hash(s.req.Marker))
		if err != nil {
			t.Fatal(err)
		}
		if out, err = run("/bin/sh", "-c", proof); err != nil || out != "mender-v2-durable" {
			t.Fatalf("resolution file and directory fsync not proven: %q %v", out, err)
		}
		_, err = run("/bin/sh", s.script)
		if err != nil {
			t.Fatalf("late gated unit should exit without execution: %v", err)
		}
		if _, err = os.Stat(s.sideEffect); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("resolved task executed user command: %v", err)
		}
		if _, err = os.Stat(filepath.Join(s.dir, "start.intent")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("late gate committed start after resolution: %v", err)
		}
		if _, err = run("/bin/sh", "-c", command); err == nil {
			t.Fatal("resolution marker overwritten or duplicated")
		}
	})
	t.Run("gate_first_resolution_denied", func(t *testing.T) {
		s := makeScenario(strings.Repeat("b", 32))
		if out, err := run("/bin/sh", s.script); err != nil {
			t.Fatalf("gate start %q %v", out, err)
		}
		command, err := v2RecoveryShell(s.req)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = run("/bin/sh", "-c", command); err == nil {
			t.Fatal("resolved a started/completed command")
		}
		output, err := os.ReadFile(s.sideEffect) // #nosec G304 -- test-owned output path
		if err != nil || string(output) != "x" {
			t.Fatalf("unexpected single user effect %q %v", output, err)
		}
		if _, err = os.Stat(filepath.Join(s.dir, "resolution.json")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("false v2 resolution: %v", err)
		}
	})
	t.Run("held_launch_lock_blocks_recovery", func(t *testing.T) {
		s := makeScenario(strings.Repeat("c", 32))
		lock, err := os.OpenFile(filepath.Join(s.dir, "launch.lock"), os.O_WRONLY|os.O_CREATE, 0600) // #nosec G304 -- isolated file
		if err != nil {
			t.Fatal(err)
		}
		if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			t.Fatal(err)
		}
		command, err := v2RecoveryShell(s.req)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = run("/bin/sh", "-c", command); err == nil {
			t.Fatal("recovery overtook active gate lock")
		}
		if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); err != nil {
			t.Fatal(err)
		}
		if err = lock.Close(); err != nil {
			t.Fatal(err)
		}
		if out, err := run("/bin/sh", "-c", command); err != nil || out != "mender-v2-resolved" {
			t.Fatalf("recovery after release: %q %v", out, err)
		}
	})
	t.Run("changed_launch_intent_rejected", func(t *testing.T) {
		s := makeScenario(strings.Repeat("d", 32))
		if err := os.WriteFile(filepath.Join(s.dir, "launch.intent"), []byte("changed"), 0600); err != nil {
			t.Fatal(err)
		}
		command, err := v2RecoveryShell(s.req)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = run("/bin/sh", "-c", command); err == nil {
			t.Fatal("stale launch evidence was accepted")
		}
	})
	t.Run("durability_proof_requires_same_lock_and_digest", func(t *testing.T) {
		s := makeScenario(strings.Repeat("e", 32))
		command, err := v2RecoveryShell(s.req)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = run("/bin/sh", "-c", command); err != nil {
			t.Fatal(err)
		}
		proof, err := v2ResolutionProofShell(s.dir, hash(s.req.Marker))
		if err != nil {
			t.Fatal(err)
		}
		lockPath := filepath.Join(s.dir, "launch.lock")
		// #nosec G304 -- isolated, test-owned lock inode.
		held, err := os.OpenFile(lockPath, os.O_RDWR, 0600)
		if err != nil {
			t.Fatal(err)
		}
		if err = syscall.Flock(int(held.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			t.Fatal(err)
		}
		if _, err = run("/bin/sh", "-c", proof); err == nil {
			t.Fatal("durability proof bypassed running launch gate")
		}
		if err = syscall.Flock(int(held.Fd()), syscall.LOCK_UN); err != nil {
			t.Fatal(err)
		}
		if err = held.Close(); err != nil {
			t.Fatal(err)
		}
		if out, e := run("/bin/sh", "-c", proof); e != nil || out != "mender-v2-durable" {
			t.Fatalf("remote durable proof not established: %q %v", out, e)
		}
		// A content mismatch cannot be fsynced into a false affirmative.
		if _, e := run("/bin/sh", "-c", strings.Replace(proof, hash(s.req.Marker), strings.Repeat("0", 64), 1)); e == nil {
			t.Fatal("wrong marker digest passed durability proof")
		}
	})
}
