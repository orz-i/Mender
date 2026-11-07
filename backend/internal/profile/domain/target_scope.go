package domain

import (
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
)

// ExecutionTarget is an exact, immutable Workspace/Sandbox pair admitted by a
// Profile ToolBinding. Workspace may be empty only for target-bound operations
// (e.g. SSH Jobs) without a logical Workspace. Syntax is not authorization.
type ExecutionTarget struct {
	workspaceRef string
	sandboxID    string
}

func NewExecutionTarget(workspaceRef, sandboxID string) (ExecutionTarget, error) {
	if workspaceRef != "" && !validExecutionTargetID(workspaceRef, "wrk_") {
		return ExecutionTarget{}, fmt.Errorf("tool target scope: invalid workspace reference")
	}
	if !validExecutionTargetID(sandboxID, "sbx_") {
		return ExecutionTarget{}, fmt.Errorf("tool target scope: invalid sandbox reference")
	}
	return ExecutionTarget{workspaceRef: workspaceRef, sandboxID: sandboxID}, nil
}

func (t ExecutionTarget) WorkspaceRef() string { return t.workspaceRef }
func (t ExecutionTarget) SandboxID() string    { return t.sandboxID }

func (t ExecutionTarget) Validate() error {
	_, err := NewExecutionTarget(t.workspaceRef, t.sandboxID)
	return err
}

// TargetScope is a restrictive, exact allow-set. A zero scope denies every
// target; there is no "*", auto-grant or implicit all-Sandboxes sentinel.
// This is a Stage B pure value object, deliberately NOT wired to the active
// Profile ToolBinding until persisted validation and Invocation enforcement
// are introduced together in the hard-cut release.
type TargetScope struct {
	allowed []ExecutionTarget
}

// NewTargetScope rejects empty, invalid or duplicate entries instead of
// silently turning an unknown legacy binding into broad target authority.
func NewTargetScope(targets []ExecutionTarget) (TargetScope, error) {
	if len(targets) == 0 || len(targets) > 256 {
		return TargetScope{}, fmt.Errorf("tool target scope: expected 1..256 exact targets")
	}
	out := slices.Clone(targets)
	for _, target := range out {
		if err := target.Validate(); err != nil {
			return TargetScope{}, err
		}
	}
	slices.SortFunc(out, compareTargets)
	for i := 1; i < len(out); i++ {
		if out[i] == out[i-1] {
			return TargetScope{}, fmt.Errorf("tool target scope: duplicate target")
		}
	}
	return TargetScope{allowed: out}, nil
}

func (s TargetScope) Targets() []ExecutionTarget { return slices.Clone(s.allowed) }

func (s TargetScope) Allows(target ExecutionTarget) bool {
	if target.Validate() != nil {
		return false
	}
	_, exists := slices.BinarySearchFunc(s.allowed, target, compareTargets)
	return exists
}

// IsSubsetOf answers whether every permission in this scope is covered by
// the other scope. It is a migration proof helper, not an authorization grant.
func (s TargetScope) IsSubsetOf(other TargetScope) bool {
	for _, target := range s.allowed {
		if !other.Allows(target) {
			return false
		}
	}
	return true
}

func (s TargetScope) Equal(other TargetScope) bool {
	return slices.Equal(s.allowed, other.allowed)
}

func compareTargets(a, b ExecutionTarget) int {
	if n := strings.Compare(a.workspaceRef, b.workspaceRef); n != 0 {
		return n
	}
	return strings.Compare(a.sandboxID, b.sandboxID)
}

func validExecutionTargetID(raw, prefix string) bool {
	if len(raw) != len(prefix)+32 || !strings.HasPrefix(raw, prefix) {
		return false
	}
	hexPart := strings.TrimPrefix(raw, prefix)
	if hexPart != strings.ToLower(hexPart) {
		return false
	}
	_, err := hex.DecodeString(hexPart)
	return err == nil
}
