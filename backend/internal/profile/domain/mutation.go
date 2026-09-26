package domain

import (
	"fmt"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
)

// AddTool returns a new Profile with one additional Tool membership.
func (p Profile) AddTool(toolID catalogdomain.ToolID, direct bool) (Profile, error) {
	if p.Contains(toolID) {
		return Profile{}, fmt.Errorf("profile tool: ToolID %s already selected", toolID)
	}

	selections := p.Selections()
	selections = append(selections, ToolSelection{ToolID: toolID, Direct: direct})
	return NewProfile(p.id, p.slug, p.displayName, p.mode, selections)
}

// RemoveTool returns a new Profile without one Tool membership.
func (p Profile) RemoveTool(toolID catalogdomain.ToolID) (Profile, error) {
	if !p.Contains(toolID) {
		return Profile{}, fmt.Errorf("profile tool: ToolID %s is not selected", toolID)
	}

	selections := make([]ToolSelection, 0, len(p.selections)-1)
	for _, selection := range p.selections {
		if selection.ToolID != toolID {
			selections = append(selections, selection)
		}
	}
	return NewProfile(p.id, p.slug, p.displayName, p.mode, selections)
}
