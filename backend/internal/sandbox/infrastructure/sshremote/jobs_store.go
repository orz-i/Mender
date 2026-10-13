package sshremote

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"

	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	"github.com/pkg/sftp"
)

const jobStorageDir = ".mender-ssh-jobs-v1"

type jobManifest struct {
	Version       int    `json:"version"`
	TaskID        string `json:"taskId"`
	SandboxID     string `json:"sandboxId"`
	CommandDigest string `json:"commandDigest"`
	Unit          string `json:"unit"`
}

type jobStore struct {
	sftp      *sftp.Client
	root      string
	sandboxID sandboxdomain.ID
}

// Task material is placed in a private sibling of the Workspace root.
// Structured Workspace read/search/edit may never reach this directory.
func (s *jobStore) basePath() string            { return path.Join(s.root+jobStorageDir, s.sandboxID.String()) }
func (s *jobStore) taskPath(id string) string   { return path.Join(s.basePath(), id) }
func (s *jobStore) scriptPath(id string) string { return path.Join(s.taskPath(id), "run.sh") }
func (s *jobStore) checkedDir(p string) error {
	info, err := s.sftp.Lstat(p)
	if err != nil || !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return ErrJobUnavailable
	}
	return nil
}
func (s *jobStore) mkdirPrivate(p string) error {
	err := s.sftp.Mkdir(p)
	if err == nil {
		// Set private mode before placing ANY record or script inside the path.
		if s.sftp.Chmod(p, 0700) != nil {
			return ErrJobUnavailable
		}
	} else if s.checkedDir(p) != nil {
		// Some SFTP servers return a generic Failure for an existing directory;
		// validated directory metadata, not the error text, is authoritative.
		return ErrJobUnavailable
	}
	return s.checkedDir(p)
}
func (s *jobStore) ensureBase(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.mkdirPrivate(s.root + jobStorageDir); err != nil {
		return err
	}
	return s.mkdirPrivate(s.basePath())
}
func (s *jobStore) reserve(ctx context.Context, id string) (bool, error) {
	if validateJobID(id) != nil {
		return false, ErrJobInvalid
	}
	if err := s.ensureBase(ctx); err != nil {
		return false, err
	}
	entries, err := s.sftp.ReadDir(s.basePath())
	if err != nil {
		return false, ErrJobUnavailable
	}
	for _, entry := range entries {
		if validateJobID(entry.Name()) != nil || !entry.IsDir() || entry.Mode()&fs.ModeSymlink != 0 || entry.Mode().Perm()&0077 != 0 {
			return false, ErrJobUnknown
		}
	}
	// Existing reservations always remain queryable, even at the quota ceiling.
	if s.checkedDir(s.taskPath(id)) == nil {
		return false, nil
	}
	if len(entries) >= maxJobRecords {
		return false, ErrJobUnavailable
	}
	err = s.sftp.Mkdir(s.taskPath(id))
	if err == nil {
		if s.sftp.Chmod(s.taskPath(id), 0700) != nil {
			return false, ErrJobUnknown
		}
		return true, s.checkedDir(s.taskPath(id))
	}
	if e := s.checkedDir(s.taskPath(id)); e != nil {
		return false, ErrJobUnavailable
	}
	return false, nil
}
func (s *jobStore) checkTask(ctx context.Context, id string) error {
	if validateJobID(id) != nil {
		return ErrJobInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.checkedDir(s.root + jobStorageDir); err != nil {
		return err
	}
	if err := s.checkedDir(s.basePath()); err != nil {
		return err
	}
	return s.checkedDir(s.taskPath(id))
}
func (s *jobStore) writeExclusive(ctx context.Context, p string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f, err := s.sftp.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY)
	if err != nil {
		return ErrJobUnknown
	}
	// OpenFile's server-side default umask may otherwise create a world-readable
	// file. Fix permissions before writing any command or result metadata.
	if s.sftp.Chmod(p, 0600) != nil {
		_ = f.Close()
		return ErrJobUnknown
	}
	written, writeErr := f.Write(data)
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil || written != len(data) {
		return ErrJobUnknown
	}
	if ctx.Err() != nil {
		return ErrJobUnknown
	}
	return nil
}
func (s *jobStore) writeManifest(ctx context.Context, id string, m jobManifest) error {
	if err := s.checkTask(ctx, id); err != nil {
		return err
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return ErrJobInvalid
	}
	return s.writeExclusive(ctx, path.Join(s.taskPath(id), "manifest.json"), raw)
}
func (s *jobStore) writeScript(ctx context.Context, id, script string) error {
	if err := s.checkTask(ctx, id); err != nil {
		return err
	}
	return s.writeExclusive(ctx, s.scriptPath(id), []byte(script))
}
func (s *jobStore) readFile(ctx context.Context, id, name string, limit int) ([]byte, error) {
	if err := s.checkTask(ctx, id); err != nil {
		return nil, err
	}
	p := path.Join(s.taskPath(id), name)
	info, err := s.sftp.Lstat(p)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > int64(limit) || info.Mode().Perm()&0077 != 0 {
		return nil, ErrJobUnknown
	}
	f, err := s.sftp.Open(p)
	if err != nil {
		return nil, ErrJobUnknown
	}
	defer func() { _ = f.Close() }()
	raw, err := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	if err != nil || len(raw) > limit {
		return nil, ErrJobUnknown
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return raw, nil
}
func (s *jobStore) manifest(ctx context.Context, id string) (jobManifest, error) {
	var m jobManifest
	raw, err := s.readFile(ctx, id, "manifest.json", 1024)
	if err != nil {
		return m, ErrJobUnknown
	}
	if json.Unmarshal(raw, &m) != nil || m.Version != 1 || m.TaskID != id || m.SandboxID != s.sandboxID.String() ||
		m.Unit != "mender-"+id+".service" || len(m.CommandDigest) != 64 {
		return jobManifest{}, ErrJobUnknown
	}
	return m, nil
}
func (s *jobStore) cancelRequested(ctx context.Context, id string) (bool, error) {
	if err := s.checkTask(ctx, id); err != nil {
		return false, err
	}
	info, err := s.sftp.Lstat(path.Join(s.taskPath(id), "cancel"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil || !info.Mode().IsRegular() {
		return false, ErrJobUnknown
	}
	return true, nil
}
func (s *jobStore) markCancel(ctx context.Context, id string) error {
	if err := s.checkTask(ctx, id); err != nil {
		return err
	}
	p := path.Join(s.taskPath(id), "cancel")
	if err := s.writeExclusive(ctx, p, []byte("requested\n")); err != nil {
		exists, e := s.cancelRequested(ctx, id)
		if e == nil && exists {
			return nil
		}
		return ErrJobUnknown
	}
	return nil
}
func (s *jobStore) exitCode(ctx context.Context, id string) (int, bool, error) {
	raw, err := s.readFile(ctx, id, "exit", 16)
	if errors.Is(err, os.ErrNotExist) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, ErrJobUnknown
	}
	code, e := parseExit(strings.TrimSuffix(string(raw), "\n"))
	if e != nil {
		return 0, false, ErrJobUnknown
	}
	return code, true, nil
}
func (s *jobStore) readLog(ctx context.Context, id, name string, cursor int64, limit int) (string, int64, bool, error) {
	if err := s.checkTask(ctx, id); err != nil {
		return "", cursor, false, ErrJobUnknown
	}
	p := path.Join(s.taskPath(id), name)
	info, err := s.sftp.Lstat(p)
	if errors.Is(err, os.ErrNotExist) {
		return "", cursor, false, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return "", cursor, false, ErrJobUnknown
	}
	if cursor > info.Size() {
		return "", cursor, false, ErrJobInvalid
	}
	f, err := s.sftp.Open(p)
	if err != nil {
		return "", cursor, false, ErrJobUnknown
	}
	defer func() { _ = f.Close() }()
	if _, err = f.Seek(cursor, io.SeekStart); err != nil {
		return "", cursor, false, ErrJobUnknown
	}
	raw, err := io.ReadAll(io.LimitReader(f, int64(limit)))
	if err != nil || ctx.Err() != nil {
		return "", cursor, false, ErrJobUnknown
	}
	next := cursor + int64(len(raw))
	return string(raw), next, next < info.Size(), nil
}
func (s *jobStore) list(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := s.checkedDir(s.root + jobStorageDir); err != nil {
		return nil, ErrJobUnknown
	}
	if err := s.checkedDir(s.basePath()); err != nil {
		return nil, ErrJobUnknown
	}
	entries, err := s.sftp.ReadDir(s.basePath())
	if err != nil || len(entries) > maxJobRecords {
		return nil, ErrJobUnknown
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if validateJobID(entry.Name()) != nil || !entry.IsDir() || entry.Mode()&fs.ModeSymlink != 0 || entry.Mode().Perm()&0077 != 0 {
			return nil, ErrJobUnknown
		}
		ids = append(ids, entry.Name())
	}
	return ids, nil
}
func (s *jobStore) pruneOutput(ctx context.Context, id string) error {
	if err := s.checkTask(ctx, id); err != nil {
		return err
	}
	for _, name := range []string{"stdout", "stderr", "run.sh"} {
		if err := ctx.Err(); err != nil {
			return err
		}
		p := path.Join(s.taskPath(id), name)
		if err := s.sftp.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("prune SSH job output: %w", ErrJobUnknown)
		}
	}
	return nil
}
