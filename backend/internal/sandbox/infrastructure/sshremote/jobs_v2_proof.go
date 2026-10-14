package sshremote

import (
	"context"
	"path"
	"strings"
)

// v2ResolutionProofShell is not an operation that creates/changes a task
// record. It verifies the exact lock/marker under a nonblocking remote flock,
// then fsyncs BOTH the existing marker and its directory. A marker visible in
// page cache after an interrupted resolution is not treated as durable until
// this independent positive acknowledgement succeeds.
func v2ResolutionProofShell(taskDir, markerDigest string) (string, error) {
	if !path.IsAbs(taskDir) || path.Clean(taskDir) != taskDir ||
		strings.ContainsAny(taskDir, "\r\n\x00") ||
		validateJobID(path.Base(taskDir)) != nil || !hex64(markerDigest) {
		return "", ErrJobInvalid
	}
	dir := quoted(taskDir)
	marker := quoted(path.Join(taskDir, "resolution.json"))
	lock := quoted(path.Join(taskDir, "launch.lock"))
	digest := quoted(markerDigest)

	return "set -eu; " +
		"test -d " + dir + "; test ! -L " + dir + "; " +
		// Do not create a lock inode in an observational proof: a missing or
		// symlinked lock is a broken recovery protocol, not quiescence.
		"test -f " + lock + "; test ! -L " + lock + "; " +
		"exec 9<> " + lock + "; flock -xn 9 || exit 75; " +
		"test -f " + marker + "; test ! -L " + marker + "; " +
		"test \"$(sha256sum " + marker + " | cut -d' ' -f1)\" = " + digest + "; " +
		"sync -f " + marker + "; sync -f " + dir + "; " +
		"test \"$(sha256sum " + marker + " | cut -d' ' -f1)\" = " + digest + "; " +
		"printf '%s' mender-v2-durable", nil
}

func (r systemdJobRunner) ProveV2ResolutionDurable(ctx context.Context, taskDir, markerDigest string) error {
	if r.client == nil || r.user != "root" {
		return ErrJobUnavailable
	}
	// Verify the filesystem of the EXACT task directory before probing:
	// a subdirectory mount must not bypass VerifyV2's reviewed fs allowlist.
	if err := r.VerifyV2(ctx, taskDir); err != nil {
		return ErrJobUnknown
	}
	command, err := v2ResolutionProofShell(taskDir, markerDigest)
	if err != nil {
		return err
	}
	result, err := r.run(ctx, command)
	if err != nil || result != "mender-v2-durable" {
		return ErrJobUnknown
	}
	return nil
}
