package sandboxbridge

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	sandboxapp "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandbox "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	workspaceapp "github.com/orz-i/Mender/backend/internal/workspace/application"
)

// LocalRootAttestor verifies an owner-registered local Workspace placement
// against the exact Sandbox root. It only performs filesystem observations:
// no opening a Sandbox backend, process launch, mount, credential resolution,
// or automatic directory creation. The backend still enforces actual paths.
type LocalRootAttestor struct{}

func (LocalRootAttestor) AttestRoot(
	ctx context.Context, candidate workspaceapp.PlacementCandidate, placement sandboxapp.Placement,
) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if placement.Sandbox.Backend() != sandbox.BackendLocalWorkspace {
		// Docker, SSH, Daytona and external backends need separate trusted
		// target-side placement proof. Never equate identical path strings.
		return false, nil
	}
	expected := placement.Sandbox.Spec().LocalRoot
	if !filepath.IsAbs(candidate.TargetRoot) || filepath.Clean(candidate.TargetRoot) != candidate.TargetRoot ||
		candidate.TargetRoot != expected {
		return false, nil
	}
	canonical, err := filepath.EvalSymlinks(expected)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if canonical != expected {
		// Require an already canonical root; no symlink-based reinterpretation
		// of the Workspace authority. Filesystem races still require the
		// native Sandbox backend's path confinement on each operation.
		return false, nil
	}
	info, err := os.Stat(canonical)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return info.IsDir(), nil
}

var _ RootAttestor = LocalRootAttestor{}
