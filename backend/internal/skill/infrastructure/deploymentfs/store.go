package deploymentfs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	skillapplication "github.com/orz-i/Mender/backend/internal/skill/application"
	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
	"github.com/orz-i/Mender/backend/internal/skill/infrastructure/agentskill"
)

// Store owns host-specific Skill deployment paths and drift observation.
type Store struct{}

// New constructs a deployment filesystem adapter.
func New() *Store {
	return &Store{}
}

// ResolveDestination validates one explicit project/user root, prepares only
// real directory parents beneath it, and rejects silent overwrite.
func (*Store) ResolveDestination(
	target skilldomain.DeploymentTarget,
	root string,
	name string,
) (string, error) {
	if err := target.Validate(); err != nil {
		return "", err
	}
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "." || !filepath.IsAbs(root) {
		return "", fmt.Errorf("deployment root must be an absolute path")
	}
	info, err := os.Lstat(root)
	if err != nil {
		return "", fmt.Errorf("deployment root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", fmt.Errorf("deployment root must be a real directory")
	}

	relativeParent, err := targetParent(target)
	if err != nil {
		return "", err
	}
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return "", fmt.Errorf("open deployment root: %w", err)
	}
	defer func() { _ = rootHandle.Close() }()

	if err := ensureRealDirectories(rootHandle, relativeParent); err != nil {
		return "", err
	}
	relativeDestination := filepath.Join(relativeParent, name)
	if info, err := rootHandle.Lstat(relativeDestination); err == nil {
		return "", fmt.Errorf(
			"%w: %s",
			skillapplication.ErrDeploymentTargetExists,
			filepath.Join(root, relativeDestination),
		)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect deployment target: %w", err)
	} else if info != nil {
		return "", fmt.Errorf("inspect deployment target: unexpected file info")
	}
	return filepath.Join(root, relativeDestination), nil
}

// Observe reports whether one deployment directory exists and, when valid,
// its current Agent Skill digest. Invalid/tampered content is represented by an
// empty digest so callers classify it as drift rather than hiding the record.
func (*Store) Observe(
	ctx context.Context,
	destination string,
) (bool, string, error) {
	if err := ctx.Err(); err != nil {
		return false, "", err
	}
	destination = filepath.Clean(strings.TrimSpace(destination))
	if destination == "." || !filepath.IsAbs(destination) {
		return false, "", fmt.Errorf("observe deployment: destination must be absolute")
	}
	info, err := os.Lstat(destination)
	if errors.Is(err, os.ErrNotExist) {
		return false, "", nil
	}
	if err != nil {
		return false, "", fmt.Errorf("observe deployment: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return true, "", nil
	}
	snapshot, err := agentskill.LoadDirectory(destination)
	if err != nil {
		return true, "", nil
	}
	return true, snapshot.Digest, nil
}

// Remove deletes only the exact recorded directory beneath its explicit root.
func (*Store) Remove(
	ctx context.Context,
	root string,
	destination string,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	root = filepath.Clean(strings.TrimSpace(root))
	destination = filepath.Clean(strings.TrimSpace(destination))
	if root == "." || destination == "." ||
		!filepath.IsAbs(root) || !filepath.IsAbs(destination) {
		return fmt.Errorf("remove deployment: root and destination must be absolute")
	}
	relative, err := filepath.Rel(root, destination)
	if err != nil {
		return fmt.Errorf("remove deployment: compare root: %w", err)
	}
	if relative == "." ||
		relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("remove deployment: destination must remain below root")
	}

	rootInfo, err := os.Lstat(root)
	if err != nil {
		return fmt.Errorf("remove deployment: root: %w", err)
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return fmt.Errorf("remove deployment: root must be a real directory")
	}
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return fmt.Errorf("remove deployment: open root: %w", err)
	}
	defer func() { _ = rootHandle.Close() }()

	info, err := rootHandle.Lstat(relative)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("remove deployment: inspect target: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("remove deployment: target must be a real directory")
	}
	if err := rootHandle.RemoveAll(relative); err != nil {
		return fmt.Errorf("remove deployment: %w", err)
	}
	return nil
}

func targetParent(target skilldomain.DeploymentTarget) (string, error) {
	switch target {
	case skilldomain.DeploymentAgentsProject:
		return filepath.Join(".agents", "skills"), nil
	case skilldomain.DeploymentGitHubProject:
		return filepath.Join(".github", "skills"), nil
	case skilldomain.DeploymentClaudeProject:
		return filepath.Join(".claude", "skills"), nil
	case skilldomain.DeploymentAgentsUser:
		return filepath.Join(".agents", "skills"), nil
	case skilldomain.DeploymentCopilotUser:
		return filepath.Join(".copilot", "skills"), nil
	default:
		return "", fmt.Errorf("unsupported deployment target %q", target)
	}
}

func ensureRealDirectories(root *os.Root, relative string) error {
	current := ""
	for _, segment := range strings.Split(filepath.ToSlash(relative), "/") {
		if segment == "" || segment == "." {
			continue
		}
		current = filepath.Join(current, segment)
		info, err := root.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			//nolint:gosec // Deployment parent dirs are user-private and require owner execute.
			if err := root.Mkdir(current, 0o700); err != nil {
				return fmt.Errorf("create deployment parent %q: %w", current, err)
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect deployment parent %q: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("deployment parent %q must be a real directory", current)
		}
	}
	return nil
}
