package domain

import (
	"fmt"
	"slices"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
	tunneldomain "github.com/orz-i/Mender/backend/internal/tunnel/domain"
)

// AddBinding returns a new Profile with one additional ToolBinding.
func (p Profile) AddBinding(binding ToolBinding) (Profile, error) {
	if p.Contains(binding.ToolID()) {
		return Profile{}, fmt.Errorf(
			"profile tool: ToolID %s already selected",
			binding.ToolID(),
		)
	}
	bindings := p.Bindings()
	bindings = append(bindings, binding)
	return p.rebuild(bindings, p.tunnelID)
}

// ReplaceBindings returns the same Profile identity with an exact ToolBinding set.
func (p Profile) ReplaceBindings(bindings []ToolBinding) (Profile, error) {
	return p.rebuild(bindings, p.tunnelID)
}

// SetTunnel returns the same Profile with one Tunnel binding.
func (p Profile) SetTunnel(tunnelID tunneldomain.TunnelID) (Profile, error) {
	return p.rebuild(p.bindings, tunnelID)
}

// ClearTunnel returns the same Profile with no Tunnel binding.
func (p Profile) ClearTunnel() (Profile, error) {
	return p.rebuild(p.bindings, "")
}

// RemoveTool returns a new Profile without one ToolBinding.
func (p Profile) RemoveTool(toolID catalogdomain.ToolID) (Profile, error) {
	if !p.Contains(toolID) {
		return Profile{}, fmt.Errorf("profile tool: ToolID %s is not selected", toolID)
	}

	bindings := make([]ToolBinding, 0, len(p.bindings)-1)
	for _, binding := range p.bindings {
		if binding.ToolID() != toolID {
			bindings = append(bindings, binding)
		}
	}
	return p.rebuild(bindings, p.tunnelID)
}

// ReplaceSkills binds exact immutable Skill installations without deploying them.
func (p Profile) ReplaceSkills(ids []skilldomain.InstallationID) (Profile, error) {
	selected := slices.Clone(ids)
	slices.Sort(selected)
	for index, id := range selected {
		if _, err := skilldomain.ParseInstallationID(id.String()); err != nil {
			return Profile{}, fmt.Errorf("profile skill: %w", err)
		}
		if index > 0 && selected[index-1] == id {
			return Profile{}, fmt.Errorf("profile skill: duplicate installation %s", id)
		}
	}
	p.skills = selected
	return p, nil
}

// rebuild preserves the read aggregate revision for optimistic concurrency.
func (p Profile) rebuild(bindings []ToolBinding, tunnelID tunneldomain.TunnelID) (Profile, error) {
	next, err := NewProfileWithBindingsAndTunnel(p.id, p.slug, p.displayName, p.mode, tunnelID, bindings)
	if err != nil {
		return Profile{}, err
	}
	next.skills = slices.Clone(p.skills)
	return next.WithRevision(p.revision), nil
}
