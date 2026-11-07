package application

import (
	"context"
	"errors"
	"fmt"

	governance "github.com/orz-i/Mender/backend/internal/governance/domain"
	workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"
)

var (
	ErrWorkspaceUnavailable = errors.New("authorized Workspace unavailable")
	ErrWorkspaceAmbiguous   = errors.New("authorized Workspace selection ambiguous")
)

// WorkspaceInventory is an inert host-owned view of current approved
// Workspaces and the explicit default for one authenticated caller.
// The source must filter by owner/Principal/Profile grants before returning
// any ID. Names or directory similarity are never used as authorities.
type WorkspaceInventory interface {
	ListAccessibleWorkspaces(context.Context, governance.Context) ([]workspace.Ref, error)
	DefaultWorkspace(context.Context, governance.Context) (workspace.Ref, bool, error)
}

type WorkspaceSelection struct{ inventory WorkspaceInventory }

func NewWorkspaceSelection(inventory WorkspaceInventory) (WorkspaceSelection, error) {
	if inventory == nil {
		return WorkspaceSelection{}, fmt.Errorf("%w: inventory required", ErrWorkspaceUnavailable)
	}
	return WorkspaceSelection{inventory: inventory}, nil
}

// Resolve chooses the exact requested and visible Workspace, an approved
// default, or the only authorized Workspace. A missing/revoked default is
// rejected, never silently replaced by a different Workspace. Calling this
// method never probes filesystems, starts Agents or launches Sandbox backends.
func (s WorkspaceSelection) Resolve(
	ctx context.Context, caller governance.Context, requested workspace.Ref,
) (workspace.Ref, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if s.inventory == nil || caller.Validate() != nil {
		return "", ErrWorkspaceUnavailable
	}
	if requested != "" {
		if err := requested.Validate(); err != nil {
			return "", fmt.Errorf("%w: invalid requested Workspace", ErrWorkspaceUnavailable)
		}
	}
	available, err := s.inventory.ListAccessibleWorkspaces(ctx, caller)
	if err != nil {
		return "", err
	}
	if len(available) > 256 {
		return "", fmt.Errorf("%w: unbounded Workspace inventory", ErrWorkspaceUnavailable)
	}
	seen := make(map[workspace.Ref]bool, len(available))
	for _, ref := range available {
		if ref.Validate() != nil || seen[ref] {
			return "", fmt.Errorf("%w: conflicting Workspace inventory", ErrWorkspaceUnavailable)
		}
		seen[ref] = true
	}
	if requested != "" {
		if !seen[requested] {
			return "", ErrWorkspaceUnavailable
		}
		return requested, nil
	}
	preferred, hasDefault, err := s.inventory.DefaultWorkspace(ctx, caller)
	if err != nil {
		return "", err
	}
	if hasDefault {
		if preferred.Validate() != nil || !seen[preferred] {
			return "", fmt.Errorf("%w: default Workspace was revoked or renamed", ErrWorkspaceUnavailable)
		}
		return preferred, nil
	}
	if preferred != "" {
		// A broken read projection cannot be treated as "no preference".
		return "", ErrWorkspaceUnavailable
	}
	switch len(available) {
	case 0:
		return "", ErrWorkspaceUnavailable
	case 1:
		return available[0], nil
	default:
		// Multiple eligible Workspaces never select by list order.
		return "", ErrWorkspaceAmbiguous
	}
}
