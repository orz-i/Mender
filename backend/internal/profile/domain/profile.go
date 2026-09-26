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

// ToolSelection adds one canonical Tool to a Profile.
//
// Direct is meaningful only in hybrid mode. Direct mode exports every selected
// Tool directly; dynamic mode exports none directly.
type ToolSelection struct {
	ToolID catalogdomain.ToolID
	Direct bool
}

// Profile is the immutable Tool exposure boundary consumed by adapters.
type Profile struct {
	id          ProfileID
	slug        Slug
	displayName string
	mode        ExposureMode
	selections  []ToolSelection
}

// NewProfile constructs a Profile and normalizes Direct flags according to mode.
func NewProfile(
	id ProfileID,
	slug Slug,
	displayName string,
	mode ExposureMode,
	selections []ToolSelection,
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

	normalized := make([]ToolSelection, 0, len(selections))
	seen := make(map[catalogdomain.ToolID]struct{}, len(selections))
	for _, selection := range selections {
		if _, err := catalogdomain.ParseToolID(selection.ToolID.String()); err != nil {
			return Profile{}, fmt.Errorf("tool selection: %w", err)
		}
		if _, exists := seen[selection.ToolID]; exists {
			return Profile{}, fmt.Errorf("tool selection: duplicate ToolID %s", selection.ToolID)
		}
		seen[selection.ToolID] = struct{}{}

		switch mode {
		case ExposureDirect:
			selection.Direct = true
		case ExposureDynamic:
			selection.Direct = false
		case ExposureHybrid:
		}
		normalized = append(normalized, selection)
	}

	return Profile{
		id:          id,
		slug:        slug,
		displayName: displayName,
		mode:        mode,
		selections:  normalized,
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

func (p Profile) Selections() []ToolSelection {
	return slices.Clone(p.selections)
}

func (p Profile) Contains(toolID catalogdomain.ToolID) bool {
	for _, selection := range p.selections {
		if selection.ToolID == toolID {
			return true
		}
	}
	return false
}

func (p Profile) ToolIDs() []catalogdomain.ToolID {
	ids := make([]catalogdomain.ToolID, 0, len(p.selections))
	for _, selection := range p.selections {
		ids = append(ids, selection.ToolID)
	}
	return ids
}

func (p Profile) DirectToolIDs() []catalogdomain.ToolID {
	ids := make([]catalogdomain.ToolID, 0, len(p.selections))
	for _, selection := range p.selections {
		if selection.Direct {
			ids = append(ids, selection.ToolID)
		}
	}
	return ids
}
