package infrastructure

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	defaultMaxExtractedFiles = 4096
	defaultMaxExtractedBytes = int64(256 * 1024 * 1024) // 256MB
)

// Extractor performs Zip-Slip and path-traversal safe tar.gz archive decompression.
type Extractor struct {
	maxFiles int
	maxBytes int64
}

// NewExtractor returns an Extractor with default security bounds.
func NewExtractor() *Extractor {
	return &Extractor{
		maxFiles: defaultMaxExtractedFiles,
		maxBytes: defaultMaxExtractedBytes,
	}
}

// ExtractTarGz extracts an archive file into the designated destination directory with Zip-Slip protection.
func (e *Extractor) ExtractTarGz(ctx context.Context, archivePath string, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("extractor: open archive %s: %w", archivePath, err)
	}
	defer func() { _ = f.Close() }()

	return e.ExtractTarGzReader(ctx, f, destDir)
}

// ExtractTarGzReader extracts a tar.gz stream into destDir while enforcing path and size safety invariants.
func (e *Extractor) ExtractTarGzReader(ctx context.Context, r io.Reader, destDir string) error {
	destAbs, err := filepath.Abs(destDir)
	if err != nil {
		return fmt.Errorf("extractor: resolve destDir: %w", err)
	}

	gzReader, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("extractor: gzip reader: %w", err)
	}
	defer func() { _ = gzReader.Close() }()

	tarReader := tar.NewReader(gzReader)

	fileCount := 0
	var totalBytes int64

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("extractor: tar read next: %w", err)
		}

		// Security: Validate file name and prevent Zip-Slip / path traversal
		cleanName := path.Clean(header.Name)
		if cleanName == "." {
			continue
		}
		if path.IsAbs(cleanName) || strings.HasPrefix(cleanName, "/") || strings.HasPrefix(cleanName, "\\") {
			return fmt.Errorf("extractor: security violation: absolute path rejected: %q", header.Name)
		}
		if cleanName == ".." || strings.HasPrefix(cleanName, "../") || strings.Contains(cleanName, "\x00") {
			return fmt.Errorf("extractor: security violation: path traversal rejected: %q", header.Name)
		}

		targetPath := filepath.Join(destAbs, filepath.FromSlash(cleanName))
		targetAbs, err := filepath.Abs(targetPath)
		if err != nil {
			return fmt.Errorf("extractor: resolve target path: %w", err)
		}

		inside, err := pathWithin(destAbs, targetAbs)
		if err != nil || !inside {
			return fmt.Errorf("extractor: security violation: path escapes target directory: %q", header.Name)
		}

		// Type restrictions: only regular files and directories allowed
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetAbs, 0o700); err != nil {
				return fmt.Errorf("extractor: mkdir %s: %w", targetAbs, err)
			}

		case tar.TypeReg, tar.TypeRegA:
			fileCount++
			if fileCount > e.maxFiles {
				return fmt.Errorf("extractor: archive exceeds maximum file count limit (%d files)", e.maxFiles)
			}

			if header.Size < 0 || header.Size > e.maxBytes-totalBytes {
				return fmt.Errorf("extractor: archive exceeds maximum extracted size limit (%d bytes)", e.maxBytes)
			}
			totalBytes += header.Size

			parentDir := filepath.Dir(targetAbs)
			if err := os.MkdirAll(parentDir, 0o700); err != nil {
				return fmt.Errorf("extractor: mkdir parent %s: %w", parentDir, err)
			}

			// Mode: ensure writable for extraction, preserve executable bit
			perm := os.FileMode(0o600)
			if header.FileInfo().Mode().Perm()&0o111 != 0 {
				perm = 0o700
			}

			outFile, err := os.OpenFile(targetAbs, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
			if err != nil {
				return fmt.Errorf("extractor: create file %s: %w", targetAbs, err)
			}

			copied, err := io.CopyN(outFile, tarReader, header.Size)
			closeErr := outFile.Close()
			if err != nil {
				return fmt.Errorf("extractor: write content to %s: %w", targetAbs, err)
			}
			if copied != header.Size {
				return fmt.Errorf("extractor: short write to %s: copied %d of %d", targetAbs, copied, header.Size)
			}
			if closeErr != nil {
				return fmt.Errorf("extractor: close file %s: %w", targetAbs, closeErr)
			}

		case tar.TypeSymlink, tar.TypeLink:
			return fmt.Errorf("extractor: security violation: links not permitted in packages: %q", header.Name)

		default:
			return fmt.Errorf("extractor: security violation: unsupported file type flag %d for %q", header.Typeflag, header.Name)
		}
	}

	return nil
}

func pathWithin(parent, child string) (bool, error) {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false, err
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))), nil
}
