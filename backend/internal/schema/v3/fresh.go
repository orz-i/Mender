package v3

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// An empty, hash-pinned v3 schema with pinned native Tool contracts and
// zero Agent/Provider/Sandbox/Profile/Credential/Task user records. This is the
// first-run schema baseline, not a legacy runtime or a data migration route.
// The compressed SQLite image is portable; it is opened only when no DB exists.
//
//go:embed assets/fresh-typed-v3.sqlite.gz
var freshTypedV3 []byte

// SHA-256 of the checked, uncompressed, empty v3 SQLite schema image.
// Unexpected mutations to the pinned image fail closed before any resource
// is activated or a user database is opened.
const freshTypedV3Hash = "59c31b89d397bf3d565100b0362659ea43824b152423377c0c62ea1e024df9b0"

// EnsureFreshDatabase provisions a NEW schema only. It will never open,
// migrate, truncate or replace a pre-existing database (including old v2).
func EnsureFreshDatabase(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if st, err := os.Lstat(absolute); err == nil {
		if !st.Mode().IsRegular() {
			return fmt.Errorf("v3 bootstrap: existing database must be a regular file")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("v3 bootstrap: inspect target: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0700); err != nil {
		return err
	}
	// #nosec G304 -- absolute is the explicitly selected new database path; O_EXCL refuses existing files.
	target, err := os.OpenFile(absolute, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("v3 bootstrap: create new database exclusively: %w", err)
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.Remove(absolute)
		}
	}()
	gz, err := gzip.NewReader(bytes.NewReader(freshTypedV3))
	if err != nil {
		_ = target.Close()
		return fmt.Errorf("v3 bootstrap: verify schema image: %w", err)
	}
	h := sha256.New()
	count, copyErr := io.CopyN(io.MultiWriter(target, h), gz, 8<<20)
	if copyErr != nil && !errors.Is(copyErr, io.EOF) {
		_ = gz.Close()
		_ = target.Close()
		return copyErr
	}
	if count == 0 || count == 8<<20 || fmt.Sprintf("%x", h.Sum(nil)) != freshTypedV3Hash {
		_ = gz.Close()
		_ = target.Close()
		return fmt.Errorf("v3 bootstrap: schema image checksum mismatch")
	}
	if err := gz.Close(); err != nil {
		_ = target.Close()
		return err
	}
	if err := target.Sync(); err != nil {
		_ = target.Close()
		return err
	}
	if err := target.Close(); err != nil {
		return err
	}
	complete = true
	return nil
}
