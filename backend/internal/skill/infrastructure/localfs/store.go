package localfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	skillapplication "github.com/orz-i/Mender/backend/internal/skill/application"
	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
	"github.com/orz-i/Mender/backend/internal/skill/infrastructure/agentskill"
)

// Store owns immutable Skill directories beneath one Mender-managed root.
type Store struct {
	root string
}

// New constructs a managed Skill package store.
func New(root string) (*Store, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("new skill package store: root must not be empty")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("new skill package store: resolve root: %w", err)
	}
	return &Store{root: absolute}, nil
}

// InstallLocal validates and atomically copies one local Agent Skill directory.
func (s *Store) InstallLocal(
	ctx context.Context,
	sourcePath string,
	id skilldomain.InstallationID,
) (skillapplication.ManagedPackage, error) {
	if _, err := skilldomain.ParseInstallationID(id.String()); err != nil {
		return skillapplication.ManagedPackage{}, fmt.Errorf("install local skill package: %w", err)
	}
	if err := s.ensureRoot(); err != nil {
		return skillapplication.ManagedPackage{}, err
	}

	sourcePath = strings.TrimSpace(sourcePath)
	if sourcePath == "" {
		return skillapplication.ManagedPackage{}, fmt.Errorf(
			"install local skill package: source path must not be empty",
		)
	}
	source, err := filepath.Abs(sourcePath)
	if err != nil {
		return skillapplication.ManagedPackage{}, fmt.Errorf(
			"install local skill package: resolve source: %w",
			err,
		)
	}
	if inside, err := pathWithin(s.root, source); err != nil {
		return skillapplication.ManagedPackage{}, fmt.Errorf(
			"install local skill package: compare source path: %w",
			err,
		)
	} else if inside {
		return skillapplication.ManagedPackage{}, fmt.Errorf(
			"install local skill package: source must be outside managed skill root",
		)
	}
	sourceInfo, err := os.Lstat(source)
	if err != nil {
		return skillapplication.ManagedPackage{}, fmt.Errorf(
			"install local skill package: source: %w",
			err,
		)
	}
	if sourceInfo.Mode()&os.ModeSymlink != 0 || !sourceInfo.IsDir() {
		return skillapplication.ManagedPackage{}, fmt.Errorf(
			"install local skill package: source must be a real directory",
		)
	}

	sourceSnapshot, err := agentskill.LoadDirectory(source)
	if err != nil {
		return skillapplication.ManagedPackage{}, fmt.Errorf(
			"install local skill package: inspect source: %w",
			err,
		)
	}

	destination := filepath.Join(s.root, id.String())
	if _, err := os.Lstat(destination); err == nil {
		return skillapplication.ManagedPackage{}, fmt.Errorf(
			"install local skill package: installation directory already exists",
		)
	} else if !errors.Is(err, os.ErrNotExist) {
		return skillapplication.ManagedPackage{}, fmt.Errorf(
			"install local skill package: inspect destination: %w",
			err,
		)
	}

	staging, err := os.MkdirTemp(s.root, ".staging-"+id.String()+"-")
	if err != nil {
		return skillapplication.ManagedPackage{}, fmt.Errorf(
			"install local skill package: create staging directory: %w",
			err,
		)
	}
	published := false
	defer func() {
		if !published {
			_ = makeTreeWritable(staging)
			_ = os.RemoveAll(staging)
		}
	}()

	skillRoot := filepath.Join(staging, sourceSnapshot.Metadata.Name)
	if err := os.Mkdir(skillRoot, 0o700); err != nil {
		return skillapplication.ManagedPackage{}, fmt.Errorf(
			"install local skill package: create staging skill root: %w",
			err,
		)
	}
	if err := copyResources(ctx, source, skillRoot, sourceSnapshot.Resources); err != nil {
		return skillapplication.ManagedPackage{}, err
	}
	stagedSnapshot, err := agentskill.LoadDirectory(skillRoot)
	if err != nil {
		return skillapplication.ManagedPackage{}, fmt.Errorf(
			"install local skill package: verify staging package: %w",
			err,
		)
	}
	if stagedSnapshot.Digest != sourceSnapshot.Digest {
		return skillapplication.ManagedPackage{}, fmt.Errorf(
			"install local skill package: skill changed while being copied",
		)
	}
	if err := hardenTree(staging); err != nil {
		return skillapplication.ManagedPackage{}, fmt.Errorf(
			"install local skill package: harden staging package: %w",
			err,
		)
	}

	if err := os.Rename(staging, destination); err != nil {
		return skillapplication.ManagedPackage{}, fmt.Errorf(
			"install local skill package: publish package: %w",
			err,
		)
	}
	published = true

	return skillapplication.ManagedPackage{
		Snapshot:        stagedSnapshot.Clone(),
		SourceReference: source,
	}, nil
}

// Resolve verifies and returns the immutable managed snapshot.
func (s *Store) Resolve(
	ctx context.Context,
	installation skilldomain.Installation,
) (skilldomain.Snapshot, error) {
	if err := installation.Validate(); err != nil {
		return skilldomain.Snapshot{}, fmt.Errorf("resolve skill package: installation: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return skilldomain.Snapshot{}, err
	}
	if err := s.ensureRoot(); err != nil {
		return skilldomain.Snapshot{}, err
	}
	packageRoot := filepath.Join(s.root, installation.ID().String())
	info, err := os.Lstat(packageRoot)
	if err != nil {
		return skilldomain.Snapshot{}, fmt.Errorf("resolve skill package: package root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return skilldomain.Snapshot{}, fmt.Errorf(
			"resolve skill package: package root must be a real directory",
		)
	}
	skillRoot := filepath.Join(packageRoot, installation.Name())
	snapshot, err := agentskill.LoadDirectory(skillRoot)
	if err != nil {
		return skilldomain.Snapshot{}, fmt.Errorf("resolve skill package: inspect: %w", err)
	}
	if snapshot.Digest != installation.Digest() {
		return skilldomain.Snapshot{}, fmt.Errorf("resolve skill package: digest mismatch")
	}
	return snapshot, nil
}

// Materialize copies one verified exact managed installation into a
// deployment destination. The deployment copy is user-writable; the canonical
// managed installation remains immutable so later edits are detectable drift.
func (s *Store) Materialize(
	ctx context.Context,
	installation skilldomain.Installation,
	destination string,
) error {
	snapshot, err := s.Resolve(ctx, installation)
	if err != nil {
		return fmt.Errorf("materialize skill package: %w", err)
	}
	destination = filepath.Clean(strings.TrimSpace(destination))
	if destination == "." || !filepath.IsAbs(destination) {
		return fmt.Errorf("materialize skill package: destination must be absolute")
	}
	if err := os.Mkdir(destination, 0o700); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf(
				"%w: %s",
				skillapplication.ErrDeploymentTargetExists,
				destination,
			)
		}
		return fmt.Errorf("materialize skill package: create destination: %w", err)
	}
	complete := false
	defer func() {
		if !complete {
			_ = makeTreeWritable(destination)
			_ = os.RemoveAll(destination)
		}
	}()

	source := filepath.Join(
		s.root,
		installation.ID().String(),
		installation.Name(),
	)
	if err := copyResources(ctx, source, destination, snapshot.Resources); err != nil {
		return fmt.Errorf("materialize skill package: %w", err)
	}
	deployed, err := agentskill.LoadDirectory(destination)
	if err != nil {
		return fmt.Errorf("materialize skill package: verify destination: %w", err)
	}
	if deployed.Digest != snapshot.Digest {
		return fmt.Errorf("materialize skill package: destination digest mismatch")
	}
	if err := makeDeploymentWritable(destination, snapshot.Resources); err != nil {
		return fmt.Errorf("materialize skill package: set deployment permissions: %w", err)
	}
	complete = true
	return nil
}

// Remove deletes one managed package directory.
func (s *Store) Remove(
	ctx context.Context,
	id skilldomain.InstallationID,
) error {
	if _, err := skilldomain.ParseInstallationID(id.String()); err != nil {
		return fmt.Errorf("remove skill package: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.ensureRoot(); err != nil {
		return err
	}
	target := filepath.Join(s.root, id.String())
	info, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("remove skill package: inspect: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("remove skill package: package root must be a real directory")
	}
	if err := makeTreeWritable(target); err != nil {
		return fmt.Errorf("remove skill package: make writable: %w", err)
	}
	if err := os.RemoveAll(target); err != nil {
		return fmt.Errorf("remove skill package: %w", err)
	}
	return nil
}

func (s *Store) ensureRoot() error {
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return fmt.Errorf("skill package store: create root: %w", err)
	}
	info, err := os.Lstat(s.root)
	if err != nil {
		return fmt.Errorf("skill package store: inspect root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("skill package store: root must be a real directory")
	}
	//nolint:gosec // Directory access requires owner execute permission; no group/other bits are granted.
	if err := os.Chmod(s.root, 0o700); err != nil {
		return fmt.Errorf("skill package store: secure root: %w", err)
	}
	return nil
}

func copyResources(
	ctx context.Context,
	source string,
	destination string,
	resources []skilldomain.Resource,
) error {
	sourceRoot, err := os.OpenRoot(source)
	if err != nil {
		return fmt.Errorf("install local skill package: open source root: %w", err)
	}
	defer func() { _ = sourceRoot.Close() }()
	destinationRoot, err := os.OpenRoot(destination)
	if err != nil {
		return fmt.Errorf("install local skill package: open staging root: %w", err)
	}
	defer func() { _ = destinationRoot.Close() }()

	for _, resource := range resources {
		if err := ctx.Err(); err != nil {
			return err
		}
		relative := filepath.FromSlash(resource.Path)
		parent := filepath.Dir(relative)
		if parent != "." {
			if err := destinationRoot.MkdirAll(parent, 0o700); err != nil {
				return fmt.Errorf(
					"install local skill package: create resource parent %q: %w",
					resource.Path,
					err,
				)
			}
		}
		sourceFile, err := sourceRoot.Open(relative)
		if err != nil {
			return fmt.Errorf(
				"install local skill package: open source resource %q: %w",
				resource.Path,
				err,
			)
		}
		destinationFile, err := destinationRoot.OpenFile(
			relative,
			os.O_WRONLY|os.O_CREATE|os.O_EXCL,
			0o600,
		)
		if err != nil {
			_ = sourceFile.Close()
			return fmt.Errorf(
				"install local skill package: create resource %q: %w",
				resource.Path,
				err,
			)
		}
		written, copyErr := io.Copy(
			destinationFile,
			io.LimitReader(sourceFile, resource.Size+1),
		)
		sourceCloseErr := sourceFile.Close()
		destinationCloseErr := destinationFile.Close()
		if copyErr != nil {
			return fmt.Errorf("install local skill package: copy resource %q: %w", resource.Path, copyErr)
		}
		if sourceCloseErr != nil {
			return fmt.Errorf("install local skill package: close source resource %q: %w", resource.Path, sourceCloseErr)
		}
		if destinationCloseErr != nil {
			return fmt.Errorf("install local skill package: close resource %q: %w", resource.Path, destinationCloseErr)
		}
		if written != resource.Size {
			return fmt.Errorf("install local skill package: resource %q changed while copying", resource.Path)
		}
		mode := os.FileMode(0o400)
		if resource.Executable {
			mode = 0o500
		}
		if err := destinationRoot.Chmod(relative, mode); err != nil {
			return fmt.Errorf("install local skill package: secure resource %q: %w", resource.Path, err)
		}
	}
	return nil
}

func hardenTree(root string) error {
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer func() { _ = rootHandle.Close() }()

	return fs.WalkDir(os.DirFS(root), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("unexpected symlink %q", path)
		}
		if !entry.IsDir() {
			return nil
		}
		// Directory access requires owner execute permission; no group/other bits are granted.
		return rootHandle.Chmod(filepath.FromSlash(path), 0o500)
	})
}

func makeDeploymentWritable(
	root string,
	resources []skilldomain.Resource,
) error {
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer func() { _ = rootHandle.Close() }()

	for _, resource := range resources {
		mode := os.FileMode(0o600)
		if resource.Executable {
			mode = 0o700
		}
		if err := rootHandle.Chmod(filepath.FromSlash(resource.Path), mode); err != nil {
			return err
		}
	}
	return nil
}

func makeTreeWritable(root string) error {
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer func() { _ = rootHandle.Close() }()

	return fs.WalkDir(os.DirFS(root), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("unexpected symlink %q", path)
		}
		mode := os.FileMode(0o600)
		if entry.IsDir() {
			// Directory access requires owner execute permission; no group/other bits are granted.
			mode = 0o700
		}
		return rootHandle.Chmod(filepath.FromSlash(path), mode)
	})
}

func pathWithin(root, candidate string) (bool, error) {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false, err
	}
	return relative == "." ||
		(relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))), nil
}
