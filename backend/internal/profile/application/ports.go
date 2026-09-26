package application

import (
	"context"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

// Repository owns persistent Profile aggregates.
type Repository interface {
	CreateProfile(context.Context, profiledomain.Profile) error
	GetProfileBySlug(context.Context, profiledomain.Slug) (profiledomain.Profile, error)
	ListProfiles(context.Context) ([]profiledomain.Profile, error)
	SaveProfile(context.Context, profiledomain.Profile) error
}

// ProfileIDGenerator allocates opaque Profile identities.
type ProfileIDGenerator interface {
	NewProfileID() (profiledomain.ProfileID, error)
}

// ToolLookup validates cross-context Tool references through Catalog's boundary.
type ToolLookup interface {
	GetTool(context.Context, catalogdomain.ToolID) (catalogdomain.Tool, error)
}
