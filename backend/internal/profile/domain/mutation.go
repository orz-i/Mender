package domain

import (
	"fmt"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
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
	return NewProfileWithBindingsAndTunnel(
		p.id,
		p.slug,
		p.displayName,
		p.mode,
		p.tunnelID,
		bindings,
	)
}

// ReplaceBindings returns the same Profile identity with an exact ToolBinding set.
func (p Profile) ReplaceBindings(bindings []ToolBinding) (Profile, error) {
	return NewProfileWithBindingsAndTunnel(
		p.id,
		p.slug,
		p.displayName,
		p.mode,
		p.tunnelID,
		bindings,
	)
}

// SetTunnel returns the same Profile with one Tunnel binding.
func (p Profile) SetTunnel(tunnelID tunneldomain.TunnelID) (Profile, error) {
	return NewProfileWithBindingsAndTunnel(
		p.id,
		p.slug,
		p.displayName,
		p.mode,
		tunnelID,
		p.bindings,
	)
}

// ClearTunnel returns the same Profile with no Tunnel binding.
func (p Profile) ClearTunnel() (Profile, error) {
	return NewProfileWithBindingsAndTunnel(
		p.id,
		p.slug,
		p.displayName,
		p.mode,
		"",
		p.bindings,
	)
}

// AddTool is the legacy membership mutation retained while application adapters migrate.
// Deprecated: use AddBinding.
func (p Profile) AddTool(toolID catalogdomain.ToolID, direct bool) (Profile, error) {
	binding, err := DefaultToolBinding(toolID, direct)
	if err != nil {
		return Profile{}, err
	}
	return p.AddBinding(binding)
}

// ReplaceTools is the legacy membership mutation retained while application adapters migrate.
// Deprecated: use ReplaceBindings.
func (p Profile) ReplaceTools(selections []ToolSelection) (Profile, error) {
	bindings := make([]ToolBinding, 0, len(selections))
	for _, selection := range selections {
		binding, err := DefaultToolBinding(selection.ToolID, selection.Direct)
		if err != nil {
			return Profile{}, err
		}
		bindings = append(bindings, binding)
	}
	return p.ReplaceBindings(bindings)
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
	return NewProfileWithBindingsAndTunnel(
		p.id,
		p.slug,
		p.displayName,
		p.mode,
		p.tunnelID,
		bindings,
	)
}
