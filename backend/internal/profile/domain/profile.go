package domain

import (
	"fmt"
	"slices"
	"strings"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
)

// ExposureMode controls how Profile Tools are projected through MCP.
type ExposureMode string

const (
	ExposureDirect  ExposureMode = "direct"
	ExposureDynamic ExposureMode = "dynamic"
	ExposureHybrid  ExposureMode = "hybrid"
)

// Profile is the immutable Tool exposure and governance boundary consumed by adapters.
type Profile struct {
	id          ProfileID
	slug        Slug
	displayName string
	mode        ExposureMode
	bindings    []ToolBinding
}

// NewProfile is the legacy membership constructor retained while adapters migrate.
// Deprecated: use NewProfileWithBindings.
func NewProfile(
	id ProfileID,
	slug Slug,
	displayName string,
	mode ExposureMode,
	selections []ToolSelection,
) (Profile, error) {
	bindings := make([]ToolBinding, 0, len(selections))
	for _, selection := range selections {
		binding, err := DefaultToolBinding(selection.ToolID, selection.Direct)
		if err != nil {
			return Profile{}, err
		}
		bindings = append(bindings, binding)
	}
	return NewProfileWithBindings(id, slug, displayName, mode, bindings)
}

// NewProfileWithBindings constructs a Profile and normalizes binding exposure by mode.
func NewProfileWithBindings(
	id ProfileID,
	slug Slug,
	displayName string,
	mode ExposureMode,
	bindings []ToolBinding,
) (Profile, error) {
	if _, err := ParseProfileID(id.String()); err != nil {
		return Profile{}, err
	}
	if _, err := NewSlug(slug.String()); err != nil {
		return Profile{}, err
	}

	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return Profile{}, fmt.Errorf("display_name: must not be empty")
	}
	if len(displayName) > 160 {
		return Profile{}, fmt.Errorf("display_name: is too long")
	}

	switch mode {
	case ExposureDirect, ExposureDynamic, ExposureHybrid:
	default:
		return Profile{}, fmt.Errorf("exposure_mode: unknown mode %q", mode)
	}

	normalized := make([]ToolBinding, 0, len(bindings))
	seen := make(map[catalogdomain.ToolID]struct{}, len(bindings))
	for _, binding := range bindings {
		rebuilt, err := NewToolBinding(ToolBindingSpec{
			ToolID:                binding.ToolID(),
			Direct:                binding.Direct(),
			Alias:                 binding.Alias(),
			TitleOverride:         binding.TitleOverride(),
			DescriptionOverride:   binding.DescriptionOverride(),
			PolicyConstraint:      binding.PolicyConstraint(),
			ConcurrencyConstraint: binding.ConcurrencyConstraint(),
			CredentialRefs:        binding.CredentialRefs(),
		})
		if err != nil {
			return Profile{}, err
		}
		if _, exists := seen[rebuilt.ToolID()]; exists {
			return Profile{}, fmt.Errorf(
				"tool binding: duplicate ToolID %s",
				rebuilt.ToolID(),
			)
		}
		seen[rebuilt.ToolID()] = struct{}{}

		switch mode {
		case ExposureDirect:
			rebuilt = rebuilt.withDirect(true)
		case ExposureDynamic:
			rebuilt = rebuilt.withDirect(false)
		case ExposureHybrid:
		}
		normalized = append(normalized, rebuilt)
	}

	return Profile{
		id:          id,
		slug:        slug,
		displayName: displayName,
		mode:        mode,
		bindings:    normalized,
	}, nil
}

func (p Profile) ID() ProfileID {
	return p.id
}

func (p Profile) Slug() Slug {
	return p.slug
}

func (p Profile) DisplayName() string {
	return p.displayName
}

func (p Profile) Mode() ExposureMode {
	return p.mode
}

func (p Profile) Bindings() []ToolBinding {
	return slices.Clone(p.bindings)
}

// Selections returns behavior-compatible legacy membership state.
// Deprecated: use Bindings.
func (p Profile) Selections() []ToolSelection {
	selections := make([]ToolSelection, 0, len(p.bindings))
	for _, binding := range p.bindings {
		selections = append(selections, ToolSelection{
			ToolID: binding.ToolID(),
			Direct: binding.Direct(),
		})
	}
	return selections
}

func (p Profile) Binding(toolID catalogdomain.ToolID) (ToolBinding, bool) {
	for _, binding := range p.bindings {
		if binding.ToolID() == toolID {
			return binding, true
		}
	}
	return ToolBinding{}, false
}

func (p Profile) Contains(toolID catalogdomain.ToolID) bool {
	_, ok := p.Binding(toolID)
	return ok
}

func (p Profile) ToolIDs() []catalogdomain.ToolID {
	ids := make([]catalogdomain.ToolID, 0, len(p.bindings))
	for _, binding := range p.bindings {
		ids = append(ids, binding.ToolID())
	}
	return ids
}

func (p Profile) DirectToolIDs() []catalogdomain.ToolID {
	ids := make([]catalogdomain.ToolID, 0, len(p.bindings))
	for _, binding := range p.bindings {
		if binding.Direct() {
			ids = append(ids, binding.ToolID())
		}
	}
	return ids
}
