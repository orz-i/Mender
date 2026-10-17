package domain

import (
	"encoding/hex"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// ToolPlacement is an explicit owner-managed choice of existing Mender
// Workspace Tools for one registered Agent working directory. It is not a
// Sandbox requirement for normal Agent tasks or a model-facing permission.
type ToolPlacement struct {
	AgentID       ID        `json:"agentId"`
	WorkspaceKey  string    `json:"workspaceKey"`
	WorkspaceRoot string    `json:"workspaceRoot"`
	SandboxID     string    `json:"sandboxId"`
	ToolIDs       []string  `json:"toolIds"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

func (p ToolPlacement) Validate() error {
	if _, err := ParseID(string(p.AgentID)); err != nil {
		return err
	}
	if p.WorkspaceKey != "" {
		if err := ValidateWorkspaceKey(p.WorkspaceKey); err != nil {
			return err
		}
	}
	if !filepath.IsAbs(p.WorkspaceRoot) || filepath.Clean(p.WorkspaceRoot) != p.WorkspaceRoot {
		return fmt.Errorf("%w: governed placement root", ErrInvalid)
	}
	if !isOpaquePlacementID(p.SandboxID, "sbx_") {
		return fmt.Errorf("%w: governed placement SandboxID", ErrInvalid)
	}
	if len(p.ToolIDs) == 0 || len(p.ToolIDs) > 3 {
		return fmt.Errorf("%w: governed placement ToolIDs", ErrInvalid)
	}
	for i, id := range p.ToolIDs {
		if !isOpaquePlacementID(id, "tool_") || (i > 0 && id <= p.ToolIDs[i-1]) {
			return fmt.Errorf("%w: duplicate or unnormalized ToolIDs", ErrInvalid)
		}
	}
	if p.UpdatedAt.IsZero() {
		return fmt.Errorf("%w: placement timestamp", ErrInvalid)
	}
	return nil
}
func isOpaquePlacementID(value, prefix string) bool {
	if len(value) != len(prefix)+32 || !strings.HasPrefix(value, prefix) {
		return false
	}
	hexPart := strings.TrimPrefix(value, prefix)
	if strings.ToLower(hexPart) != hexPart {
		return false
	}
	_, err := hex.DecodeString(hexPart)
	return err == nil
}
func SameToolPlacement(left, right ToolPlacement) bool {
	return left.AgentID == right.AgentID && left.WorkspaceKey == right.WorkspaceKey &&
		left.WorkspaceRoot == right.WorkspaceRoot && left.SandboxID == right.SandboxID &&
		slices.Equal(left.ToolIDs, right.ToolIDs) && left.UpdatedAt.Equal(right.UpdatedAt)
}
