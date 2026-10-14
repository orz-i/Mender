package sshremote

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"strings"
)

// VerifyV2 exercises real Linux capabilities on the exact pinned SSH host.
// No environment fallback or remote shell command is accepted from a Tool.
func (r systemdJobRunner) VerifyV2(ctx context.Context, base string) error {
	if r.user != "root" || !path.IsAbs(base) || path.Clean(base) != base || strings.ContainsAny(base, "\r\n") {
		return ErrJobUnavailable
	}
	cmd := "set -eu; " +
		"test \"$(uname -s)\" = Linux; " +
		"flock --version | grep -F 'flock from util-linux' >/dev/null; " +
		"sync --version | grep -F 'GNU coreutils' >/dev/null; " +
		"mv --version | grep -F 'GNU coreutils' >/dev/null; " +
		"command -v sha256sum >/dev/null; " +
		"case \"$(stat -f -c %T " + quoted(base) + ")\" in xfs|ext2/ext3|btrfs) ;; *) exit 70;; esac; " +
		"flock -xn /dev/null true; " +
		"sync -f " + quoted(base) + "; " +
		"printf '%s' mender-v2-ready"
	out, err := r.run(ctx, cmd)
	if err != nil || out != "mender-v2-ready" {
		return ErrJobUnavailable
	}
	return nil
}
func (r systemdJobRunner) PersistV2Directory(ctx context.Context, dir string) error {
	if r.user != "root" || !path.IsAbs(dir) || path.Clean(dir) != dir {
		return ErrJobInvalid
	}
	command := "set -eu; test -d " + quoted(dir) + "; test ! -L " + quoted(dir) + "; sync -f " + quoted(dir) + "; printf '%s' mender-v2-synced"
	out, err := r.run(ctx, command)
	if err != nil || out != "mender-v2-synced" {
		return ErrJobUnknown
	}
	return nil
}

// ResolveV2UnderLock does NOT execute user shell material. Every path is
// derived from a pinned v2 TaskID and Sandbox template; the remote command is
// fixed backend code. Both the delayed gate and this resolver lock FD 9 on
// the same immutable task-scoped inode before observing start/resolution.
func v2RecoveryShell(req v2RemoteResolution) (string, error) {
	if !path.IsAbs(req.TaskDir) || path.Clean(req.TaskDir) != req.TaskDir ||
		!hex64(req.ManifestHash) || !hex64(req.ScriptHash) ||
		(req.LaunchPresent && !hex64(req.LaunchHash)) || (!req.LaunchPresent && req.LaunchHash != "") ||
		len(req.Marker) == 0 || len(req.Marker) > 2048 || strings.ContainsRune(string(req.Marker), 0) {
		return "", ErrJobInvalid
	}
	dir := quoted(req.TaskDir)
	manifest := quoted(path.Join(req.TaskDir, "manifest.json"))
	script := quoted(path.Join(req.TaskDir, "run.sh"))
	lock := quoted(path.Join(req.TaskDir, "launch.lock"))
	started := quoted(path.Join(req.TaskDir, "start.intent"))
	partial := quoted(path.Join(req.TaskDir, "start.intent.tmp"))
	exit := quoted(path.Join(req.TaskDir, "exit"))
	exitPartial := quoted(path.Join(req.TaskDir, "exit.tmp"))
	resolution := quoted(path.Join(req.TaskDir, "resolution.json"))
	resolutionTemp := quoted(path.Join(req.TaskDir, "resolution.tmp"))
	launch := quoted(path.Join(req.TaskDir, "launch.intent"))
	markerHash := sha256.Sum256(req.Marker)
	checkLaunch := "if [ -e " + launch + " ] || [ -L " + launch + " ]; then exit 73; fi; "
	if req.LaunchPresent {
		checkLaunch = "test -f " + launch + "; test ! -L " + launch + "; " +
			"test \"$(sha256sum " + launch + " | cut -d' ' -f1)\" = " + quoted(req.LaunchHash) + "; "
	}
	command := "set -eu; umask 077; " +
		"test -d " + dir + "; test ! -L " + dir + "; " +
		"if [ -L " + lock + " ]; then exit 74; fi; " +
		"exec 9>> " + lock + "; flock -xn 9 || exit 75; " +
		"test -f " + manifest + "; test ! -L " + manifest + "; " +
		"test \"$(sha256sum " + manifest + " | cut -d' ' -f1)\" = " + quoted(req.ManifestHash) + "; " +
		"test -f " + script + "; test ! -L " + script + "; " +
		"test \"$(sha256sum " + script + " | cut -d' ' -f1)\" = " + quoted(req.ScriptHash) + "; " +
		checkLaunch +
		"for p in " + started + " " + partial + " " + exit + " " + exitPartial + " " + resolution + " " + resolutionTemp + "; do " +
		"if [ -e \"$p\" ] || [ -L \"$p\" ]; then exit 76; fi; done; " +
		"set -C; printf '%s' " + quoted(string(req.Marker)) + " > " + resolutionTemp + "; set +C; " +
		"sync -f " + resolutionTemp + "; " +
		"mv -T " + resolutionTemp + " " + resolution + "; " +
		"sync -f " + dir + "; " +
		"test \"$(sha256sum " + resolution + " | cut -d' ' -f1)\" = " + quoted(hex.EncodeToString(markerHash[:])) + "; " +
		"printf '%s' mender-v2-resolved"
	return command, nil
}

func (r systemdJobRunner) ResolveV2UnderLock(ctx context.Context, req v2RemoteResolution) error {
	if r.user != "root" {
		return ErrJobUnavailable
	}
	command, err := v2RecoveryShell(req)
	if err != nil {
		return err
	}
	out, err := r.run(ctx, command)
	if err != nil || out != "mender-v2-resolved" {
		return errors.Join(ErrJobUnknown, fmt.Errorf("v2 remote recovery unconfirmed"))
	}
	return nil
}
