package application

import (
	"context"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"
)

// PlacementCandidate describes an owner-reviewed relation between one logical
// Workspace and one existing Sandbox. The root is backend-specific metadata;
// it is NOT evidence of a verified mount, sandbox availability, isolation or
// execution permission. The official GuardedRouter independently verifies
// backend capability, owner and grants at admission.
type PlacementCandidate struct {
	Workspace  workspace.Ref
	SandboxID  string
	TargetRoot string
}

// PlacementReader is the consumer-owned, side-effect-free application query
// port. Implementations MUST NOT activate a driver, inspect secrets, create a
// Sandbox or mount a Workspace while reading candidate metadata.
type PlacementReader interface {
	ListWorkspacePlacements(context.Context, workspace.Ref) ([]PlacementCandidate, error)
}

type PlacementQuery struct{ reader PlacementReader }

func NewPlacementQuery(reader PlacementReader) (PlacementQuery, error) {
	if reader == nil {
		return PlacementQuery{}, fmt.Errorf("workspace placement query: reader required")
	}
	return PlacementQuery{reader: reader}, nil
}

// List validates all returned identities before presenting candidates.
// This is NOT Sandbox admission: no candidate may execute without the
// official owner/grant/policy/generation checks and a Sandbox lease.
func (q PlacementQuery) List(ctx context.Context, ref workspace.Ref) ([]PlacementCandidate, error) {
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	if q.reader == nil {
		return nil, fmt.Errorf("workspace placement query: reader required")
	}
	entries, err := q.reader.ListWorkspacePlacements(ctx, ref)
	if err != nil {
		return nil, err
	}
	result := make([]PlacementCandidate, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, p := range entries {
		if p.Workspace != ref || !validSandboxReference(p.SandboxID) ||
			strings.TrimSpace(p.TargetRoot) == "" || strings.ContainsRune(p.TargetRoot, 0) ||
			p.TargetRoot != strings.TrimSpace(p.TargetRoot) {
			return nil, fmt.Errorf("workspace placement query: invalid candidate")
		}
		if _, exists := seen[p.SandboxID]; exists {
			return nil, fmt.Errorf("workspace placement query: duplicate target %s", p.SandboxID)
		}
		seen[p.SandboxID] = struct{}{}
		result = append(result, p)
	}
	slices.SortFunc(result, func(a, b PlacementCandidate) int {
		return strings.Compare(a.SandboxID, b.SandboxID)
	})
	return result, nil
}

// Only syntax is validated here to avoid a Workspace→Sandbox context import.
// The Sandbox application must check binding, owner, isolation, capabilities,
// backend root, generation, grant and lease at actual execution time.
func validSandboxReference(raw string) bool {
	if len(raw) != len("sbx_")+32 || !strings.HasPrefix(raw, "sbx_") {
		return false
	}
	part := strings.TrimPrefix(raw, "sbx_")
	if part != strings.ToLower(part) {
		return false
	}
	_, err := hex.DecodeString(part)
	return err == nil
}
