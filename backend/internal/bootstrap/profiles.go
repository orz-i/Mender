package bootstrap

import (
	"context"
	"fmt"

	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	profileidentity "github.com/orz-i/Mender/backend/internal/profile/infrastructure/identity"
	profilesqlite "github.com/orz-i/Mender/backend/internal/profile/infrastructure/sqlite"
)

// Profiles composes persistent Profile use cases.
type Profiles struct {
	Create     profileapplication.CreateProfile
	Get        profileapplication.GetProfile
	List       profileapplication.ListProfiles
	AddTool    profileapplication.AddTool
	RemoveTool profileapplication.RemoveTool

	store *profilesqlite.Store
}

func NewProfiles(
	ctx context.Context,
	databasePath string,
	tools profileapplication.ToolLookup,
) (*Profiles, error) {
	store, err := profilesqlite.Open(ctx, databasePath)
	if err != nil {
		return nil, fmt.Errorf("compose profiles: %w", err)
	}
	ids := profileidentity.NewGenerator()

	return &Profiles{
		Create:     profileapplication.NewCreateProfile(store, ids),
		Get:        profileapplication.NewGetProfile(store),
		List:       profileapplication.NewListProfiles(store),
		AddTool:    profileapplication.NewAddTool(store, tools),
		RemoveTool: profileapplication.NewRemoveTool(store),
		store:      store,
	}, nil
}

func (p *Profiles) Close() error {
	return p.store.Close()
}
