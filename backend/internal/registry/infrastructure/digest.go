package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	// TreeDigestPreamble is the canonical preamble used to calculate Mender package tree digests.
	TreeDigestPreamble = "mender-plugin-package-tree-v2\x00"
)

type digestFileEntry struct {
	relative string
	size     int64
	mode     fs.FileMode
}

// CalculateTreeDigest walks rootDir and computes the deterministic tree digest matching localfs.
func CalculateTreeDigest(ctx context.Context, rootDir string) (string, error) {
	rootAbs, err := filepath.Abs(rootDir)
	if err != nil {
		return "", fmt.Errorf("tree digest: resolve root: %w", err)
	}

	entries := make([]digestFileEntry, 0)
	var totalBytes int64

	err = filepath.WalkDir(rootAbs, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if current == rootAbs {
			return nil
		}

		rel, err := filepath.Rel(rootAbs, current)
		if err != nil {
			return err
		}
		relSlash := filepath.ToSlash(rel)
		if relSlash == ".." || strings.HasPrefix(relSlash, "../") {
			return fmt.Errorf("tree digest: path escapes root: %s", relSlash)
		}

		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("tree digest: symbolic links not allowed: %s", relSlash)
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("tree digest: non-regular file not allowed: %s", relSlash)
		}

		totalBytes += info.Size()
		entries = append(entries, digestFileEntry{
			relative: relSlash,
			size:     info.Size(),
			mode:     info.Mode(),
		})
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("tree digest: walk: %w", err)
	}

	// Sort files strictly lexicographically by forward-slash relative path
	slices.SortFunc(entries, func(a, b digestFileEntry) int {
		return strings.Compare(a.relative, b.relative)
	})

	hasher := sha256.New()
	_, _ = io.WriteString(hasher, TreeDigestPreamble)

	var scratch [8]byte
	for _, item := range entries {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		binary.BigEndian.PutUint64(scratch[:], uint64(len(item.relative)))
		_, _ = hasher.Write(scratch[:])
		_, _ = io.WriteString(hasher, item.relative)

		if item.mode.Perm()&0o111 != 0 {
			_, _ = hasher.Write([]byte{1})
		} else {
			_, _ = hasher.Write([]byte{0})
		}

		if item.size < 0 {
			return "", fmt.Errorf("tree digest: negative file size for %s", item.relative)
		}
		_, _ = fmt.Fprintf(hasher, "%d", item.size)
		_, _ = hasher.Write([]byte{0})

		filePath := filepath.Join(rootAbs, filepath.FromSlash(item.relative))
		f, err := os.Open(filePath)
		if err != nil {
			return "", fmt.Errorf("tree digest: open %s: %w", item.relative, err)
		}

		copied, copyErr := io.CopyN(hasher, f, item.size)
		closeErr := f.Close()
		if copyErr != nil {
			return "", fmt.Errorf("tree digest: hash %s: %w", item.relative, copyErr)
		}
		if copied != item.size {
			return "", fmt.Errorf("tree digest: short read on %s", item.relative)
		}
		if closeErr != nil {
			return "", fmt.Errorf("tree digest: close %s: %w", item.relative, closeErr)
		}
	}

	return "sha256:" + hex.EncodeToString(hasher.Sum(nil)), nil
}

// VerifyTreeDigest calculates the tree digest of rootDir and verifies it against the expected tree digest.
func VerifyTreeDigest(ctx context.Context, rootDir string, expectedDigest string) error {
	cleanExpected := strings.ToLower(strings.TrimSpace(expectedDigest))
	calculated, err := CalculateTreeDigest(ctx, rootDir)
	if err != nil {
		return err
	}
	if calculated != cleanExpected {
		return fmt.Errorf("tree digest mismatch: got %s, want %s", calculated, cleanExpected)
	}
	return nil
}
