package localfs

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	installationapplication "github.com/orz-i/Mender/backend/internal/providerplugin/installation/application"
	installationdomain "github.com/orz-i/Mender/backend/internal/providerplugin/installation/domain"
	"github.com/orz-i/Mender/backend/internal/providerplugin/manifest"
)

const (
	defaultMaxFiles = 4096
	defaultMaxBytes = int64(256 * 1024 * 1024)
	digestPreamble  = "mender-provider-package-tree-v1\x00"
)

// Limits bound local package ingestion before data enters Mender-managed storage.
type Limits struct {
	MaxFiles int
	MaxBytes int64
}

// DefaultLimits returns the local package ingestion limits used by Mender.
func DefaultLimits() Limits {
	return Limits{
		MaxFiles: defaultMaxFiles,
		MaxBytes: defaultMaxBytes,
	}
}

// Store owns immutable local package directories beneath one managed root.
type Store struct {
	root   string
	goos   string
	goarch string
	limits Limits
}

// New constructs a package store for the current platform.
func New(root string) (*Store, error) {
	return newStore(root, runtime.GOOS, runtime.GOARCH, DefaultLimits())
}

func newStore(root, goos, goarch string, limits Limits) (*Store, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("new plugin package store: root must not be empty")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("new plugin package store: resolve root: %w", err)
	}
	if strings.TrimSpace(goos) == "" || strings.TrimSpace(goarch) == "" {
		return nil, fmt.Errorf("new plugin package store: platform must not be empty")
	}
	if limits.MaxFiles < 1 || limits.MaxBytes < 1 {
		return nil, fmt.Errorf("new plugin package store: limits must be positive")
	}
	return &Store{
		root:   absolute,
		goos:   goos,
		goarch: goarch,
		limits: limits,
	}, nil
}

// InstallLocal validates and atomically copies one local package directory.
func (s *Store) InstallLocal(
	ctx context.Context,
	sourcePath string,
	id installationdomain.InstallationID,
) (installationapplication.ManagedPackage, error) {
	if _, err := installationdomain.ParseInstallationID(id.String()); err != nil {
		return installationapplication.ManagedPackage{}, fmt.Errorf(
			"install local plugin package: %w",
			err,
		)
	}
	if err := s.ensureRoot(); err != nil {
		return installationapplication.ManagedPackage{}, err
	}

	source, err := filepath.Abs(strings.TrimSpace(sourcePath))
	if err != nil {
		return installationapplication.ManagedPackage{}, fmt.Errorf(
			"install local plugin package: resolve source: %w",
			err,
		)
	}
	if strings.TrimSpace(sourcePath) == "" {
		return installationapplication.ManagedPackage{}, fmt.Errorf(
			"install local plugin package: source path must not be empty",
		)
	}
	if inside, err := pathWithin(s.root, source); err != nil {
		return installationapplication.ManagedPackage{}, fmt.Errorf(
			"install local plugin package: compare source path: %w",
			err,
		)
	} else if inside {
		return installationapplication.ManagedPackage{}, fmt.Errorf(
			"install local plugin package: source must be outside managed plugin root",
		)
	}

	sourceInfo, err := os.Lstat(source)
	if err != nil {
		return installationapplication.ManagedPackage{}, fmt.Errorf(
			"install local plugin package: source: %w",
			err,
		)
	}
	if sourceInfo.Mode()&os.ModeSymlink != 0 || !sourceInfo.IsDir() {
		return installationapplication.ManagedPackage{}, fmt.Errorf(
			"install local plugin package: source must be a real directory",
		)
	}

	sourceSnapshot, err := s.inspect(ctx, source)
	if err != nil {
		return installationapplication.ManagedPackage{}, fmt.Errorf(
			"install local plugin package: inspect source: %w",
			err,
		)
	}

	destination := filepath.Join(s.root, id.String())
	if _, err := os.Lstat(destination); err == nil {
		return installationapplication.ManagedPackage{}, fmt.Errorf(
			"install local plugin package: installation directory already exists",
		)
	} else if !errors.Is(err, os.ErrNotExist) {
		return installationapplication.ManagedPackage{}, fmt.Errorf(
			"install local plugin package: inspect destination: %w",
			err,
		)
	}

	staging, err := os.MkdirTemp(s.root, ".staging-"+id.String()+"-")
	if err != nil {
		return installationapplication.ManagedPackage{}, fmt.Errorf(
			"install local plugin package: create staging directory: %w",
			err,
		)
	}
	renamed := false
	defer func() {
		if !renamed {
			_ = makeTreeWritable(staging)
			_ = os.RemoveAll(staging)
		}
	}()

	if err := s.copySnapshot(ctx, source, staging, sourceSnapshot.files); err != nil {
		return installationapplication.ManagedPackage{}, err
	}
	if err := hardenTree(staging, sourceSnapshot.files); err != nil {
		return installationapplication.ManagedPackage{}, fmt.Errorf(
			"install local plugin package: harden staging package: %w",
			err,
		)
	}

	stagedSnapshot, err := s.inspect(ctx, staging)
	if err != nil {
		return installationapplication.ManagedPackage{}, fmt.Errorf(
			"install local plugin package: verify staging package: %w",
			err,
		)
	}
	if stagedSnapshot.digest != sourceSnapshot.digest {
		return installationapplication.ManagedPackage{}, fmt.Errorf(
			"install local plugin package: package changed while being copied",
		)
	}
	if stagedSnapshot.parsed.PluginID != sourceSnapshot.parsed.PluginID ||
		stagedSnapshot.parsed.Version != sourceSnapshot.parsed.Version {
		return installationapplication.ManagedPackage{}, fmt.Errorf(
			"install local plugin package: staged manifest identity changed",
		)
	}

	if err := os.Rename(staging, destination); err != nil {
		return installationapplication.ManagedPackage{}, fmt.Errorf(
			"install local plugin package: publish package: %w",
			err,
		)
	}
	renamed = true

	return installationapplication.ManagedPackage{
		Manifest:        stagedSnapshot.parsed,
		ManifestJSON:    append([]byte(nil), stagedSnapshot.manifestJSON...),
		Digest:          stagedSnapshot.digest,
		SourceReference: source,
	}, nil
}

// Resolve verifies one managed package immediately before process launch.
func (s *Store) Resolve(
	ctx context.Context,
	installation installationdomain.Installation,
) (installationapplication.ResolvedPackage, error) {
	if err := installation.Validate(); err != nil {
		return installationapplication.ResolvedPackage{}, fmt.Errorf(
			"resolve local plugin package: installation: %w",
			err,
		)
	}
	if err := s.ensureRoot(); err != nil {
		return installationapplication.ResolvedPackage{}, err
	}
	packageRoot := filepath.Join(s.root, installation.ID().String())
	info, err := os.Lstat(packageRoot)
	if err != nil {
		return installationapplication.ResolvedPackage{}, fmt.Errorf(
			"resolve local plugin package: package root: %w",
			err,
		)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return installationapplication.ResolvedPackage{}, fmt.Errorf(
			"resolve local plugin package: package root must be a real directory",
		)
	}
	snapshot, err := s.inspect(ctx, packageRoot)
	if err != nil {
		return installationapplication.ResolvedPackage{}, fmt.Errorf(
			"resolve local plugin package: verify package: %w",
			err,
		)
	}
	if snapshot.digest != installation.Digest() {
		return installationapplication.ResolvedPackage{}, fmt.Errorf(
			"resolve local plugin package: package digest mismatch",
		)
	}
	if snapshot.parsed.PluginID != installation.PluginID().String() ||
		snapshot.parsed.Version != installation.PackageVersion() {
		return installationapplication.ResolvedPackage{}, fmt.Errorf(
			"resolve local plugin package: manifest identity mismatch",
		)
	}
	entrypoint, err := snapshot.parsed.Entrypoint(s.goos, s.goarch)
	if err != nil {
		return installationapplication.ResolvedPackage{}, fmt.Errorf(
			"resolve local plugin package: entrypoint: %w",
			err,
		)
	}
	checksum, err := executableChecksum(
		ctx,
		packageRoot,
		filepath.FromSlash(entrypoint),
	)
	if err != nil {
		return installationapplication.ResolvedPackage{}, fmt.Errorf(
			"resolve local plugin package: executable checksum: %w",
			err,
		)
	}
	return installationapplication.ResolvedPackage{
		Manifest:           snapshot.parsed,
		EntrypointPath:     filepath.Join(packageRoot, filepath.FromSlash(entrypoint)),
		WorkingDirectory:   packageRoot,
		ExecutableChecksum: checksum,
	}, nil
}

func executableChecksum(
	ctx context.Context,
	rootPath string,
	relative string,
) ([]byte, error) {
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	file, err := root.Open(relative)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("entrypoint is not a regular file")
	}
	hasher := sha256.New()
	buffer := make([]byte, 32*1024)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		count, readErr := file.Read(buffer)
		if count > 0 {
			_, _ = hasher.Write(buffer[:count])
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
	}
	return hasher.Sum(nil), nil
}

// Remove removes one managed package directory. It is idempotent for compensation.
func (s *Store) Remove(
	ctx context.Context,
	id installationdomain.InstallationID,
) error {
	if _, err := installationdomain.ParseInstallationID(id.String()); err != nil {
		return fmt.Errorf("remove local plugin package: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.ensureRoot(); err != nil {
		return err
	}
	destination := filepath.Join(s.root, id.String())
	if err := makeTreeWritable(destination); err != nil {
		return fmt.Errorf("remove local plugin package: prepare package files: %w", err)
	}
	if err := os.RemoveAll(destination); err != nil {
		return fmt.Errorf("remove local plugin package: %w", err)
	}
	return nil
}

type packageFile struct {
	relative string
	size     int64
	mode     fs.FileMode
}

type packageSnapshot struct {
	files        []packageFile
	manifestJSON []byte
	parsed       manifest.Manifest
	digest       string
}

func (s *Store) inspect(
	ctx context.Context,
	rootPath string,
) (packageSnapshot, error) {
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return packageSnapshot{}, err
	}
	defer func() { _ = root.Close() }()

	files := make([]packageFile, 0)
	var total int64
	err = filepath.WalkDir(rootPath, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if current == rootPath {
			return nil
		}
		relative, err := filepath.Rel(rootPath, current)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if relative == ".." || strings.HasPrefix(relative, "../") {
			return fmt.Errorf("path escapes package root: %s", relative)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic links are not allowed: %s", relative)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("non-regular files are not allowed: %s", relative)
		}
		if len(files) >= s.limits.MaxFiles {
			return fmt.Errorf("package contains more than %d files", s.limits.MaxFiles)
		}
		if info.Size() < 0 || info.Size() > s.limits.MaxBytes-total {
			return fmt.Errorf("package exceeds %d bytes", s.limits.MaxBytes)
		}
		total += info.Size()
		files = append(files, packageFile{
			relative: relative,
			size:     info.Size(),
			mode:     info.Mode(),
		})
		return nil
	})
	if err != nil {
		return packageSnapshot{}, err
	}

	slices.SortFunc(files, func(a, b packageFile) int {
		return strings.Compare(a.relative, b.relative)
	})
	manifestIndex := slices.IndexFunc(files, func(file packageFile) bool {
		return file.relative == manifest.FileName
	})
	if manifestIndex < 0 {
		return packageSnapshot{}, fmt.Errorf("package is missing %s", manifest.FileName)
	}
	manifestJSON, err := root.ReadFile(filepath.FromSlash(manifest.FileName))
	if err != nil {
		return packageSnapshot{}, fmt.Errorf("read manifest: %w", err)
	}
	parsed, err := manifest.Parse(manifestJSON)
	if err != nil {
		return packageSnapshot{}, err
	}
	entrypoint, err := parsed.Entrypoint(s.goos, s.goarch)
	if err != nil {
		return packageSnapshot{}, err
	}
	entrypointIndex := slices.IndexFunc(files, func(file packageFile) bool {
		return file.relative == entrypoint
	})
	if entrypointIndex < 0 {
		return packageSnapshot{}, fmt.Errorf(
			"manifest entrypoint does not exist: %s",
			entrypoint,
		)
	}
	if s.goos != "windows" && files[entrypointIndex].mode.Perm()&0o111 == 0 {
		return packageSnapshot{}, fmt.Errorf(
			"manifest entrypoint is not executable: %s",
			entrypoint,
		)
	}

	digest, err := digestFiles(ctx, root, files)
	if err != nil {
		return packageSnapshot{}, err
	}
	return packageSnapshot{
		files:        files,
		manifestJSON: append([]byte(nil), manifestJSON...),
		parsed:       parsed,
		digest:       digest,
	}, nil
}

func digestFiles(
	ctx context.Context,
	root *os.Root,
	files []packageFile,
) (string, error) {
	hasher := sha256.New()
	_, _ = io.WriteString(hasher, digestPreamble)
	var scratch [8]byte

	for _, item := range files {
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
			return "", fmt.Errorf("package file has negative size: %s", item.relative)
		}
		_, _ = fmt.Fprintf(hasher, "%d", item.size)
		_, _ = hasher.Write([]byte{0})

		file, err := root.Open(filepath.FromSlash(item.relative))
		if err != nil {
			return "", fmt.Errorf("open package file %s: %w", item.relative, err)
		}
		info, err := file.Stat()
		if err != nil {
			_ = file.Close()
			return "", fmt.Errorf("stat package file %s: %w", item.relative, err)
		}
		if !info.Mode().IsRegular() || info.Size() != item.size {
			_ = file.Close()
			return "", fmt.Errorf("package file changed during inspection: %s", item.relative)
		}
		if _, err := io.CopyN(hasher, file, item.size); err != nil {
			_ = file.Close()
			return "", fmt.Errorf("hash package file %s: %w", item.relative, err)
		}
		var extra [1]byte
		count, readErr := file.Read(extra[:])
		closeErr := file.Close()
		if count != 0 || (readErr != nil && !errors.Is(readErr, io.EOF)) {
			return "", fmt.Errorf("package file changed during inspection: %s", item.relative)
		}
		if closeErr != nil {
			return "", fmt.Errorf("close package file %s: %w", item.relative, closeErr)
		}
	}
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil)), nil
}

func (s *Store) copySnapshot(
	ctx context.Context,
	sourcePath string,
	stagingPath string,
	files []packageFile,
) error {
	source, err := os.OpenRoot(sourcePath)
	if err != nil {
		return fmt.Errorf("install local plugin package: open source root: %w", err)
	}
	defer func() { _ = source.Close() }()
	staging, err := os.OpenRoot(stagingPath)
	if err != nil {
		return fmt.Errorf("install local plugin package: open staging root: %w", err)
	}
	defer func() { _ = staging.Close() }()

	for _, item := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		relativeOS := filepath.FromSlash(item.relative)
		parent := filepath.Dir(relativeOS)
		if parent != "." {
			if err := staging.MkdirAll(parent, 0o700); err != nil {
				return fmt.Errorf(
					"install local plugin package: create directory for %s: %w",
					item.relative,
					err,
				)
			}
		}
		input, err := source.Open(relativeOS)
		if err != nil {
			return fmt.Errorf(
				"install local plugin package: open %s: %w",
				item.relative,
				err,
			)
		}
		info, err := input.Stat()
		if err != nil {
			_ = input.Close()
			return fmt.Errorf(
				"install local plugin package: stat %s: %w",
				item.relative,
				err,
			)
		}
		if !info.Mode().IsRegular() || info.Size() != item.size {
			_ = input.Close()
			return fmt.Errorf(
				"install local plugin package: source changed during copy: %s",
				item.relative,
			)
		}

		output, err := staging.OpenFile(relativeOS, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			_ = input.Close()
			return fmt.Errorf(
				"install local plugin package: create %s: %w",
				item.relative,
				err,
			)
		}
		_, copyErr := io.CopyN(output, input, item.size)
		syncErr := output.Sync()
		closeOutputErr := output.Close()
		var extra [1]byte
		count, readErr := input.Read(extra[:])
		closeInputErr := input.Close()
		if copyErr != nil {
			return fmt.Errorf(
				"install local plugin package: copy %s: %w",
				item.relative,
				copyErr,
			)
		}
		if count != 0 || (readErr != nil && !errors.Is(readErr, io.EOF)) {
			return fmt.Errorf(
				"install local plugin package: source changed during copy: %s",
				item.relative,
			)
		}
		if syncErr != nil {
			return fmt.Errorf(
				"install local plugin package: sync %s: %w",
				item.relative,
				syncErr,
			)
		}
		if closeOutputErr != nil {
			return fmt.Errorf(
				"install local plugin package: close %s: %w",
				item.relative,
				closeOutputErr,
			)
		}
		if closeInputErr != nil {
			return fmt.Errorf(
				"install local plugin package: close source %s: %w",
				item.relative,
				closeInputErr,
			)
		}
	}
	return nil
}

func hardenTree(rootPath string, files []packageFile) error {
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()

	for _, item := range files {
		mode := os.FileMode(0o400)
		if item.mode.Perm()&0o111 != 0 {
			mode = 0o500
		}
		if err := root.Chmod(filepath.FromSlash(item.relative), mode); err != nil {
			return err
		}
	}

	directories, err := collectDirectories(rootPath)
	if err != nil {
		return err
	}
	slices.Reverse(directories)
	for _, directory := range directories {
		if err := root.Chmod(filepath.FromSlash(directory), 0o500); err != nil {
			return err
		}
	}
	return nil
}

func makeTreeWritable(rootPath string) error {
	info, err := os.Lstat(rootPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()

	files := make([]string, 0)
	directories := make([]string, 0)
	if err := filepath.WalkDir(rootPath, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(rootPath, current)
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if entry.IsDir() {
			directories = append(directories, relative)
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			files = append(files, relative)
		}
		return nil
	}); err != nil {
		return err
	}
	for _, file := range files {
		if err := root.Chmod(file, 0o600); err != nil {
			return err
		}
	}
	for _, directory := range directories {
		if err := root.Chmod(directory, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func collectDirectories(rootPath string) ([]string, error) {
	directories := make([]string, 0)
	err := filepath.WalkDir(rootPath, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(rootPath, current)
		if err != nil {
			return err
		}
		directories = append(directories, relative)
		return nil
	})
	return directories, err
}

func (s *Store) ensureRoot() error {
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return fmt.Errorf("plugin package store: create managed root: %w", err)
	}
	info, err := os.Lstat(s.root)
	if err != nil {
		return fmt.Errorf("plugin package store: inspect managed root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("plugin package store: managed root must be a real directory")
	}
	//nolint:gosec // 0700 is owner-only; directory traversal requires the execute bit.
	if err := os.Chmod(s.root, 0o700); err != nil {
		return fmt.Errorf("plugin package store: protect managed root: %w", err)
	}
	return nil
}

func pathWithin(parent, child string) (bool, error) {
	relative, err := filepath.Rel(parent, child)
	if err != nil {
		return false, err
	}
	return relative == "." ||
		(relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))), nil
}
