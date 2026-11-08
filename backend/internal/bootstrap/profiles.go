package bootstrap

import (
	"context"
	"fmt"

	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	profileidentity "github.com/orz-i/Mender/backend/internal/profile/infrastructure/identity"
	profilesqlite "github.com/orz-i/Mender/backend/internal/profile/infrastructure/sqlite"
	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
)

// Profiles composes persistent Profile use cases.
type Profiles struct {
	Create        profileapplication.CreateProfile
	Get           profileapplication.GetProfile
	GetByID       profileapplication.GetProfileByID
	GetRevision   profileapplication.GetProfileRevision
	List          profileapplication.ListProfiles
	AddTool       profileapplication.AddTool
	ReplaceTools  profileapplication.ReplaceTools
	ReplaceSkills profileapplication.ReplaceSkills
	RemoveTool    profileapplication.RemoveTool

	store *profilesqlite.Store
}

func NewProfiles(
	ctx context.Context,
	databasePath string,
	tools profileapplication.ToolLookup,
	credentials profileapplication.CredentialLookup,
) (*Profiles, error) {
	store, err := profilesqlite.Open(ctx, databasePath)
	if err != nil {
		return nil, fmt.Errorf("compose profiles: %w", err)
	}
	return composeProfiles(store, tools, credentials), nil
}

// NewTypedProfiles keeps the same Profile application services, using the
// already-migrated v3 Store. It does not create grants or run old migrations.
func NewTypedProfiles(ctx context.Context, databasePath string, tools profileapplication.ToolLookup,
	credentials profileapplication.CredentialLookup) (*Profiles, error) {
	store, err := profilesqlite.OpenTyped(ctx, databasePath)
	if err != nil {
		return nil, fmt.Errorf("compose typed profiles: %w", err)
	}
	return composeProfiles(store, tools, credentials), nil
}

func composeProfiles(store *profilesqlite.Store, tools profileapplication.ToolLookup,
	credentials profileapplication.CredentialLookup) *Profiles {
	ids := profileidentity.NewGenerator()

	return &Profiles{
		Create:       profileapplication.NewCreateProfile(store, ids),
		Get:          profileapplication.NewGetProfile(store),
		GetByID:      profileapplication.NewGetProfileByID(store),
		GetRevision:  profileapplication.NewGetProfileRevision(store),
		List:         profileapplication.NewListProfiles(store),
		AddTool:      profileapplication.NewAddTool(store, tools, credentials),
		ReplaceTools: profileapplication.NewReplaceTools(store, tools, credentials),
		RemoveTool:   profileapplication.NewRemoveTool(store),
		store:        store,
	}
}

// Bootstrap bridge: Profile validates exact Skill state without reading content
// or owning the Skill Library's trust, files, or deployments.
type profileSkillStatus struct{ skills *Skills }

func (lookup profileSkillStatus) GetSkillState(ctx context.Context, id skilldomain.InstallationID) (bool, bool, error) {
	return lookup.skills.SkillState(ctx, id)
}

func (p *Profiles) Close() error {
	return p.store.Close()
}
