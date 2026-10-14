package sshremote

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"strings"
	"unicode/utf8"

	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// v2GatePlan is the only executable job preparation plan. Real host
// fsync/flock preflight and the Core submission lease are mandatory before
// creating TaskID material or submitting to the managed systemd unit.
type v2GatePlan struct {
	manifest v2GateManifest
	script   string
}
type v2GateManifest struct {
	Version       int    `json:"version"`
	TaskID        string `json:"taskId"`
	SandboxID     string `json:"sandboxId"`
	Unit          string `json:"unit"`
	CommandDigest string `json:"commandDigest"`
	ScriptDigest  string `json:"scriptDigest"`
	AttemptID     string `json:"attemptId"`
	RunGeneration uint64 `json:"runGeneration"`
}

// planV2 is called only for an exclusive new reservation, never on replay.
func (j *Jobs) planV2(request JobRequest) (v2GatePlan, error) {
	if j == nil || j.store == nil || request.Validate() != nil {
		return v2GatePlan{}, ErrJobInvalid
	}
	if _, err := sandboxdomain.ParseID(j.sandboxID.String()); err != nil {
		return v2GatePlan{}, ErrJobInvalid
	}
	if !path.IsAbs(j.root) || path.Clean(j.root) != j.root {
		return v2GatePlan{}, ErrJobInvalid
	}
	var attempt [16]byte
	if _, err := rand.Read(attempt[:]); err != nil {
		return v2GatePlan{}, fmt.Errorf("allocate SSH v2 attempt: %w", err)
	}
	taskID := j.identity(request.IdempotencyKey)
	commandHash := sha256.Sum256([]byte(request.Command))
	digest := hex.EncodeToString(commandHash[:])
	attemptID := hex.EncodeToString(attempt[:])
	script, err := gatedJobScriptV2(j.root, j.store.taskPath(taskID), taskID, digest, attemptID, request.Command)
	if err != nil {
		return v2GatePlan{}, err
	}
	scriptHash := sha256.Sum256([]byte(script))
	return v2GatePlan{
		manifest: v2GateManifest{Version: 2, TaskID: taskID, SandboxID: j.sandboxID.String(), Unit: j.unit(taskID),
			CommandDigest: digest, ScriptDigest: hex.EncodeToString(scriptHash[:]), AttemptID: attemptID},
		script: script,
	}, nil
}

// gatedJobScriptV2 is intentionally Linux-specific (util-linux flock and GNU
// sync/mv), not a portable shell fallback. It carries no credential or
// caller-selected Sandbox routing field. It does not itself assert durability:
// R3b must verify the remote filesystem and support before it may be enabled.
//
// The first user command is BELOW a same-inode remote exclusive lock, a
// resolution check, and a durably committed start marker. The gate never
// permits a command to run after an explicit resolution was persisted under
// that same lock, including after a delayed systemd-run call.
func gatedJobScriptV2(root, taskDir, taskID, commandDigest, attemptID, command string) (string, error) {
	if validateJobID(taskID) != nil || !path.IsAbs(root) || path.Clean(root) != root ||
		!path.IsAbs(taskDir) || path.Clean(taskDir) != taskDir ||
		!strings.HasPrefix(taskDir, root+jobStorageDir+"/") || path.Base(taskDir) != taskID ||
		len(commandDigest) != 64 || len(attemptID) != 32 || strings.Trim(commandDigest, "0123456789abcdef") != "" ||
		strings.Trim(attemptID, "0123456789abcdef") != "" ||
		len(command) == 0 || len(command) > maxJobCommandBytes || !utf8.ValidString(command) || strings.TrimSpace(command) == "" || strings.ContainsRune(command, 0) {
		return "", ErrJobInvalid
	}
	lock := quoted(path.Join(taskDir, "launch.lock"))
	resolved := quoted(path.Join(taskDir, "resolution.json"))
	started := quoted(path.Join(taskDir, "start.intent"))
	prepared := quoted(path.Join(taskDir, "start.intent.tmp"))
	result := quoted(path.Join(taskDir, "exit"))
	temporary := quoted(path.Join(taskDir, "exit.tmp"))
	task := quoted(taskDir)
	marker := quoted("v2:" + taskID + ":" + attemptID + ":" + commandDigest)
	return "#!/bin/sh\n" +
		"umask 077\nset -u\n" +
		"cd " + quoted(root) + " || exit 125\n" +
		"exec 9>> " + lock + " || exit 125\n" +
		"flock -x 9 || exit 125\n" +
		// Existing resolved task suppresses even a late systemd unit.
		"if [ -e " + resolved + " ] || [ -L " + resolved + " ]; then exit 0; fi\n" +
		"if [ -e " + started + " ] || [ -L " + started + " ] || [ -e " + prepared + " ] || [ -L " + prepared + " ]; then exit 125; fi\n" +
		// Exclusive marker must be durable before any user-command side effect.
		"set -C\n" +
		"printf '%s\\n' " + marker + " > " + prepared + " || exit 125\n" +
		"set +C\n" +
		"sync -f " + prepared + " || exit 125\n" +
		"mv -T " + prepared + " " + started + " || exit 125\n" +
		"sync -f " + task + " || exit 125\n" +
		"finish() { rc=$?; printf '%s\\n' \"$rc\" > " + temporary + " || return; " +
		"sync -f " + temporary + " || return; " +
		"mv -T " + temporary + " " + result + " || return; " +
		"sync -f " + task + " || return; }\n" +
		"trap finish EXIT\ntrap 'exit 143' TERM\n" +
		"(ulimit -f 8192; exec /bin/sh -c " + quoted(command) + ") > " + quoted(path.Join(taskDir, "stdout")) +
		" 2> " + quoted(path.Join(taskDir, "stderr")) + "\n" +
		"exit $?\n", nil
}
