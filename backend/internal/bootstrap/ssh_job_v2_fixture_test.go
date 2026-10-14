package bootstrap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// Native v2 fixture journals use only disposable SFTP-root files.
// No test writes an obsolete protocol, starts systemd or touches the Keychain.
// This helper is solely for proving Core governance against v2 evidence.
type fixtureSSHJobV2 struct {
	ID    string
	Dir   string
	Start string
}

func seedSSHJobV2(t *testing.T, root string, id sandboxdomain.ID, taskID string, terminal bool) fixtureSSHJobV2 {
	t.Helper()
	if len(taskID) != 37 || taskID[:5] != "sjob_" {
		t.Fatal("invalid fixed task ID")
	}
	base := root + ".mender-ssh-jobs-v2"
	dir := filepath.Join(base, id.String(), taskID)
	for _, path := range []string{base, filepath.Join(base, id.String()), filepath.Join(base, id.String(), ".retired-ids"), dir} {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	script := []byte("#!/bin/sh\n# test-only flock -x 9 resolution.json start.intent sync -f\n")
	scriptDigest := sha256.Sum256(script)
	cmdDigest := sha256.Sum256([]byte("test-only-native-v2"))
	attempt := "0123456789abcdef0123456789abcdef"
	command := hex.EncodeToString(cmdDigest[:])
	manifest := struct {
		Version       int    `json:"version"`
		TaskID        string `json:"taskId"`
		SandboxID     string `json:"sandboxId"`
		Unit          string `json:"unit"`
		CommandDigest string `json:"commandDigest"`
		ScriptDigest  string `json:"scriptDigest"`
		AttemptID     string `json:"attemptId"`
		RunGeneration uint64 `json:"runGeneration"`
	}{Version: 2, TaskID: taskID, SandboxID: id.String(), Unit: "mender-" + taskID + ".service",
		CommandDigest: command, ScriptDigest: hex.EncodeToString(scriptDigest[:]), AttemptID: attempt, RunGeneration: 1}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	for name, raw := range map[string][]byte{"manifest.json": data, "run.sh": script} {
		if err = os.WriteFile(filepath.Join(dir, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	start := "v2:" + taskID + ":" + attempt + ":" + command + "\n"
	if terminal {
		if err = os.WriteFile(filepath.Join(dir, "start.intent"), []byte(start), 0600); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, "exit"), []byte("0\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return fixtureSSHJobV2{ID: taskID, Dir: dir, Start: start}
}
func enrollSSHJobV2Fixture(t *testing.T, ctx context.Context, r *Runtime, id sandboxdomain.ID, caller gov.Context) {
	t.Helper()
	run, err := r.Sandboxes.GetRun(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	token, err := r.Sandboxes.store.ReserveSSHJobSubmit(ctx, id, run.Generation, caller)
	if err != nil {
		t.Fatal(err)
	}
	if err = r.Sandboxes.store.ReleaseExecution(ctx, id, token); err != nil {
		t.Fatal(err)
	}
}
