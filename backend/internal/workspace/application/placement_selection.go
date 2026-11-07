package application

import (
	"context"
	"errors"
	"fmt"

	workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"
)

var (
	ErrPlacementUnavailable = errors.New("workspace execution placement unavailable")
	ErrPlacementAmbiguous   = errors.New("workspace execution placement ambiguous")
)

// PlacementInspector is the consumer-owned, inert capability/authority
// evaluation boundary. Returning false skips an ineligible candidate.
// Unknown/error conditions must return an error, not false, so selection
// cannot silently shift execution to a different backend.
type PlacementInspector interface {
	InspectPlacement(context.Context, workspace.Operation, PlacementCandidate) (bool, error)
}

// Select uses an explicit, approved target or a *unique* eligible target.
// A preferred target is an owner/application decision, never a model-provided
// SandboxID. Neither Select nor its ports may start a backend or issue grants.
// A changed or revoked placement must be rechecked before execution.
func (q PlacementQuery) Select(
	ctx context.Context,
	ref workspace.Ref,
	operation workspace.Operation,
	pinnedSandboxID string,
	inspector PlacementInspector,
) (PlacementCandidate, error) {
	if err := ctx.Err(); err != nil {
		return PlacementCandidate{}, err
	}
	if ref.Validate() != nil {
		return PlacementCandidate{}, fmt.Errorf("%w: invalid Workspace", ErrPlacementUnavailable)
	}
	switch operation {
	case workspace.Read, workspace.Search, workspace.Edit, workspace.Shell:
	default:
		return PlacementCandidate{}, fmt.Errorf("%w: unsupported Workspace operation", ErrPlacementUnavailable)
	}
	if pinnedSandboxID != "" && !validSandboxReference(pinnedSandboxID) {
		return PlacementCandidate{}, fmt.Errorf("%w: invalid pinned Sandbox", ErrPlacementUnavailable)
	}
	if inspector == nil {
		return PlacementCandidate{}, fmt.Errorf("%w: missing candidate inspector", ErrPlacementUnavailable)
	}
	candidates, err := q.List(ctx, ref)
	if err != nil {
		return PlacementCandidate{}, err
	}
	var selected PlacementCandidate
	found := false
	for _, candidate := range candidates {
		// A pinned but missing/denied/unavailable target fails, never falls
		// back to another Sandbox (including Host).
		if pinnedSandboxID != "" && candidate.SandboxID != pinnedSandboxID {
			continue
		}
		eligible, err := inspector.InspectPlacement(ctx, operation, candidate)
		if err != nil {
			return PlacementCandidate{}, fmt.Errorf("inspect Workspace placement %s: %w", candidate.SandboxID, err)
		}
		if !eligible {
			continue
		}
		if found {
			return PlacementCandidate{}, ErrPlacementAmbiguous
		}
		selected, found = candidate, true
	}
	if !found {
		return PlacementCandidate{}, ErrPlacementUnavailable
	}
	return selected, nil
}
