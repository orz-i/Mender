package domain

import (
	"fmt"
	"slices"
	"strings"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
)

// Profile is the immutable capability selection and governance boundary consumed by delivery adapters.
type Profile struct {
	id          ProfileID
	slug        Slug
	displayName string
	bindings    []ToolBinding
	skills      []skilldomain.InstallationID
	revision    uint64
}

// Revision is the persisted aggregate revision. It is not a client authority token.
func (p Profile) Revision() uint64 { return p.revision }

// WithRevision restores the persisted version after an authoritative repository read.
func (p Profile) WithRevision(revision uint64) Profile {
	p.revision = revision
	return p
}

// NewProfileWithBindings constructs a capability bundle without choosing a delivery.
func NewProfileWithBindings(id ProfileID, slug Slug, displayName string, bindings []ToolBinding) (Profile, error) {
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

	normalized := make([]ToolBinding, 0, len(bindings))
	seen := make(map[catalogdomain.ToolID]struct{}, len(bindings))
	for _, binding := range bindings {
		rebuilt, err := NewToolBinding(ToolBindingSpec{
			ToolID:                binding.ToolID(),
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

		normalized = append(normalized, rebuilt)
	}

	return Profile{
		id:          id,
		slug:        slug,
		displayName: displayName,
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

func (p Profile) Bindings() []ToolBinding {
	return slices.Clone(p.bindings)
}

// Skills are exact installed Skill versions selected for this Profile.
// They do not grant Tool execution or deploy files by themselves.
func (p Profile) Skills() []skilldomain.InstallationID {
	return slices.Clone(p.skills)
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
