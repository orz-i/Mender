package toolhardcut

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	sqlite "modernc.org/sqlite"
)

// BackupSnapshot uses the SQLite Online Backup API to create a consistent
// *new* file at destination (never overwrite). It compares an assessment
// before and after backup, so a concurrent writer cannot silently alter the
// migration manifest. It does not execute any migration or modify source.
//
// This requires a fully quiesced Runtime for cutover. A failed or changed
// snapshot is deleted; a successful backup is operator-managed and immutable
// for the purposes of subsequent migration planning.
func BackupSnapshot(ctx context.Context, sourcePath, destinationPath string) (Report, error) {
	if ctx.Err() != nil {
		return Report{}, ctx.Err()
	}
	source, err := filepath.Abs(sourcePath)
	if err != nil {
		return Report{}, err
	}
	destination, err := filepath.Abs(destinationPath)
	if err != nil {
		return Report{}, err
	}
	if source == destination || strings.TrimSpace(destinationPath) == "" {
		return Report{}, fmt.Errorf("SQLite backup requires a distinct explicit destination")
	}
	initial, err := AssessPath(ctx, source)
	if err != nil {
		return Report{}, fmt.Errorf("assess before SQLite backup: %w", err)
	}
	if _, err := os.Stat(filepath.Dir(destination)); err != nil {
		return Report{}, fmt.Errorf("backup directory must already exist: %w", err)
	}
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		if _, err := os.Lstat(destination + suffix); err == nil {
			return Report{}, fmt.Errorf("backup or SQLite journal already exists")
		} else if !os.IsNotExist(err) {
			return Report{}, err
		}
	}
	// #nosec G304 -- explicit destination, verified absent, opened with O_EXCL and 0600.
	reserved, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return Report{}, fmt.Errorf("refuse existing backup destination: %w", err)
	}
	if err = reserved.Close(); err != nil {
		_ = os.Remove(destination)
		return Report{}, err
	}
	success := false
	defer func() {
		if !success {
			for _, suffix := range []string{"-wal", "-shm", "-journal", ""} {
				_ = os.Remove(destination + suffix)
			}
		}
	}()
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(source)}
	query := u.Query()
	query.Set("mode", "ro")
	query.Add("_pragma", "query_only(1)")
	query.Add("_pragma", "busy_timeout(5000)")
	u.RawQuery = query.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return Report{}, err
	}
	db.SetMaxOpenConns(1)
	defer func() { _ = db.Close() }()
	conn, err := db.Conn(ctx)
	if err != nil {
		return Report{}, err
	}
	defer func() { _ = conn.Close() }()
	target := url.URL{Scheme: "file", Path: filepath.ToSlash(destination)}
	err = conn.Raw(func(driverConn any) (retErr error) {
		copier, ok := driverConn.(interface {
			NewBackup(string) (*sqlite.Backup, error)
		})
		if !ok {
			return fmt.Errorf("SQLite connection does not support Online Backup API")
		}
		backup, err := copier.NewBackup(target.String())
		if err != nil {
			return err
		}
		// Finish closes the destination handle, including on interrupted copies.
		defer func() { retErr = errors.Join(retErr, backup.Finish()) }()
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			remaining, err := backup.Step(128)
			if err != nil {
				return err
			}
			if !remaining {
				return nil
			}
		}
	})
	if err != nil {
		return Report{}, fmt.Errorf("SQLite online backup: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	// A restore artifact must be one self-contained SQLite file, including
	// sources that were originally in WAL mode. Checkpoint the independent
	// destination; never alter the source journal configuration.
	copyDB, err := sql.Open("sqlite", destination)
	if err != nil {
		return Report{}, err
	}
	var mode string
	modeErr := copyDB.QueryRowContext(ctx, "PRAGMA journal_mode=DELETE").Scan(&mode)
	closeErr := copyDB.Close()
	if err := errors.Join(modeErr, closeErr); err != nil {
		return Report{}, err
	}
	if mode != "delete" {
		return Report{}, fmt.Errorf("backup journal checkpoint was not confirmed")
	}
	copied, err := AssessPath(ctx, destination)
	if err != nil {
		return Report{}, fmt.Errorf("verify SQLite backup: %w", err)
	}
	if initial.Snapshot.DatabaseFingerprint != copied.Snapshot.DatabaseFingerprint ||
		initial.ManifestDigest != copied.ManifestDigest {
		return Report{}, fmt.Errorf("SQLite backup source changed between assessment and copy")
	}
	// #nosec G304 -- only opens the just-created O_EXCL backup destination for fsync.
	handle, err := os.Open(destination)
	if err != nil {
		return Report{}, err
	}
	syncErr := handle.Sync()
	closeErr = handle.Close()
	if err = errors.Join(syncErr, closeErr); err != nil {
		return Report{}, err
	}
	success = true
	return copied, nil
}
