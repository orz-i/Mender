package dockerengine

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"strconv"
	"strings"
)

// Structured file operations are fixed non-root in-container commands. Docker
// Engine archive GET/HEAD/PUT cannot access tmpfs files under ReadonlyRootfs;
// using the fixed Exec stream keeps that guarantee while avoiding host access.
func (s *workspaceSession) verifyPath(ctx context.Context, relative string, wantDir, allowMissingFinal bool) error {
	if err := s.verifyPart(ctx, "/workspace", true, false); err != nil {
		return err
	}
	if relative == "." {
		if wantDir {
			return nil
		}
		return fmt.Errorf("docker Workspace root is not a file")
	}
	parts := strings.Split(relative, "/")
	for index := range parts {
		final := index == len(parts)-1
		abs := containerPath(strings.Join(parts[:index+1], "/"))
		if err := s.verifyPart(ctx, abs, !final || wantDir, final && allowMissingFinal); err != nil {
			return err
		}
	}
	return nil
}
func (s *workspaceSession) verifyPart(ctx context.Context, abs string, dir, missing bool) error {
	var command string
	switch {
	case missing:
		command = "test ! -e \"$1\" && test ! -L \"$1\""
	case dir:
		command = "test -d \"$1\" && test ! -L \"$1\""
	default:
		command = "test -f \"$1\" && test ! -L \"$1\""
	}
	exit, err := s.runFixed(ctx, []string{"/bin/sh", "-c", command, "mender", abs})
	if err != nil {
		return err
	}
	if exit != 0 {
		return fmt.Errorf("docker Workspace path absent, symlinked or wrong type")
	}
	return nil
}
func (s *workspaceSession) readFile(ctx context.Context, relative string, maxBytes int) ([]byte, error) {
	if maxBytes < 1 || maxBytes > maxDockerEditBytes {
		return nil, fmt.Errorf("docker Workspace read cap invalid")
	}
	if err := s.verifyPath(ctx, relative, false, false); err != nil {
		return nil, err
	}
	command := "test -f \"$1\" && test ! -L \"$1\" && cat -- \"$1\""
	result, err := s.runExecOutput(ctx, workspacePath, []string{"/bin/sh", "-c", command, "mender", containerPath(relative)}, maxBytes+1)
	if err != nil {
		return nil, err
	}
	if result.ExitCode != 0 || result.Truncated || len(result.Stdout) > maxBytes {
		return nil, fmt.Errorf("docker Workspace read exceeded limits or file changed")
	}
	return []byte(result.Stdout), nil
}

// listFilePaths uses NUL-delimited fixed in-container find; no caller code is
// executed. Follow neither file nor directory symlinks, cap output and files.
func (s *workspaceSession) listFilePaths(ctx context.Context, relative string) ([]string, bool, error) {
	if err := s.verifyPath(ctx, relative, true, false); err != nil {
		return nil, false, err
	}
	command := "test -d \"$1\" && test ! -L \"$1\" && find \"$1\" -type f -print0"
	result, err := s.runExecOutput(ctx, workspacePath, []string{"/bin/sh", "-c", command, "mender", containerPath(relative)}, 2*1024*1024)
	if err != nil {
		return nil, false, err
	}
	if result.ExitCode != 0 {
		return nil, false, fmt.Errorf("docker Workspace search listing failed")
	}
	truncated := result.Truncated
	paths := strings.Split(result.Stdout, "\x00")
	out := make([]string, 0, min(len(paths), 10000))
	prefix := "/workspace/"
	for _, abs := range paths {
		if abs == "" {
			continue
		}
		if len(out) >= 10000 {
			truncated = true
			break
		}
		if !strings.HasPrefix(abs, prefix) {
			return nil, false, fmt.Errorf("docker Workspace listing escaped root")
		}
		name, err := relativePath(strings.TrimPrefix(abs, prefix), false)
		if err != nil {
			return nil, false, err
		}
		if strings.HasPrefix(name, ".git/") || strings.Contains(name, "/.git/") {
			continue
		}
		out = append(out, name)
	}
	return out, truncated, nil
}

// stageFile is the only write payload channel. Staging has a random name in
// the already verified writable tmpfs, fixed argv and post-transfer hash.
func (s *workspaceSession) stageFile(ctx context.Context, parent string, content []byte) (string, error) {
	if len(content) > maxDockerEditBytes {
		return "", fmt.Errorf("docker Workspace edit payload too large")
	}
	var random [12]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	staged := path.Join(containerPath(parent), ".mender-stage-"+hex.EncodeToString(random[:]))
	command := "umask 077; test ! -e \"$2\" && head -c \"$1\" > \"$2\""
	result, err := s.runExecStream(ctx, workspacePath, []string{"/bin/sh", "-c", command, "mender", strconv.Itoa(len(content)), staged}, content)
	if err != nil {
		s.uncertain.Store(true)
		return "", err
	}
	if result.ExitCode != 0 {
		s.uncertain.Store(true)
		return "", fmt.Errorf("docker Workspace file staging failed")
	}
	hash := sha256.Sum256(content)
	code, err := s.runFixed(ctx, []string{"/bin/sh", "-c", "test \"$(sha256sum \"$1\" | cut -d' ' -f1)\" = \"$2\"", "mender", staged, hex.EncodeToString(hash[:])})
	if err != nil || code != 0 {
		s.uncertain.Store(true)
		return "", fmt.Errorf("docker Workspace staged file integrity failed")
	}
	return staged, nil
}
func (s *workspaceSession) cleanupStage(ctx context.Context, staged string) {
	if staged == "" {
		return
	}
	_, err := s.runFixed(ctx, []string{"/bin/sh", "-c", "rm -f -- \"$1\"", "mender", staged})
	if err != nil {
		s.uncertain.Store(true)
	}
}
