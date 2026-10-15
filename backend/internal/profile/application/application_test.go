package application

import (
	"context"
	"errors"
	"testing"
	"time"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

func TestProfileApplicationCreateAddListRemove(t *testing.T) {
	t.Parallel()

	repository := newMemoryRepository()
	ids := &fixedIDs{ids: []profiledomain.ProfileID{
		mustProfileID(t, "prof_00000000000000000000000000000001"),
	}}
	tool := mustTool(t)

	create := NewCreateProfile(repository, ids)
	profile, err := create.Execute(context.Background(), CreateProfileRequest{
		Slug:        "default",
		DisplayName: "Default",
		Mode:        profiledomain.ExposureHybrid,
	})
	if err != nil {
		t.Fatalf("CreateProfile.Execute() error = %v", err)
	}

	add := NewAddTool(repository, fakeToolLookup{tool: tool}, nil)
	profile, err = add.Execute(context.Background(), AddToolRequest{
		ProfileSlug: profile.Slug(),
		ToolID:      tool.ID(),
		Direct:      true,
	})
	if err != nil {
		t.Fatalf("AddTool.Execute() error = %v", err)
	}
	if !profile.Contains(tool.ID()) || len(profile.DirectToolIDs()) != 1 {
		t.Fatalf("profile selections = %+v", profile.Bindings())
	}

	replace := NewReplaceTools(repository, fakeToolLookup{tool: tool}, nil)
	profile, err = replace.Execute(context.Background(), ReplaceToolsRequest{
		ProfileSlug: profile.Slug(),
		Tools: []ToolBindingInput{
			{ToolID: tool.ID(), Direct: false},
		},
	})
	if err != nil {
		t.Fatalf("ReplaceTools.Execute() error = %v", err)
	}
	if !profile.Contains(tool.ID()) {
		t.Fatalf("replaced profile selections = %+v", profile.Bindings())
	}

	list := NewListProfiles(repository)
	profiles, err := list.Execute(context.Background())
	if err != nil {
		t.Fatalf("ListProfiles.Execute() error = %v", err)
	}
	if len(profiles) != 1 || profiles[0].ID() != profile.ID() {
		t.Fatalf("ListProfiles = %+v", profiles)
	}

	remove := NewRemoveTool(repository)
	profile, err = remove.Execute(context.Background(), RemoveToolRequest{
		ProfileSlug: profile.Slug(),
		ToolID:      tool.ID(),
	})
	if err != nil {
		t.Fatalf("RemoveTool.Execute() error = %v", err)
	}
	if profile.Contains(tool.ID()) {
		t.Fatal("RemoveTool left Tool selected")
	}
}

type memoryRepository struct {
	bySlug map[profiledomain.Slug]profiledomain.Profile
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{bySlug: make(map[profiledomain.Slug]profiledomain.Profile)}
}

func (m *memoryRepository) CreateProfile(_ context.Context, profile profiledomain.Profile) error {
	if _, exists := m.bySlug[profile.Slug()]; exists {
		return ErrSlugConflict
	}
	m.bySlug[profile.Slug()] = profile
	return nil
}

func (m *memoryRepository) GetProfileByID(_ context.Context, id profiledomain.ProfileID) (profiledomain.Profile, error) {
	for _, profile := range m.bySlug {
		if profile.ID() == id {
			return profile, nil
		}
	}
	return profiledomain.Profile{}, ErrProfileNotFound
}

func (m *memoryRepository) GetProfileBySlug(_ context.Context, slug profiledomain.Slug) (profiledomain.Profile, error) {
	profile, exists := m.bySlug[slug]
	if !exists {
		return profiledomain.Profile{}, ErrProfileNotFound
	}
	return profile, nil
}

func (m *memoryRepository) ListProfiles(context.Context) ([]profiledomain.Profile, error) {
	profiles := make([]profiledomain.Profile, 0, len(m.bySlug))
	for _, profile := range m.bySlug {
		profiles = append(profiles, profile)
	}
	return profiles, nil
}

func (m *memoryRepository) SaveProfile(_ context.Context, profile profiledomain.Profile) error {
	if _, exists := m.bySlug[profile.Slug()]; !exists {
		return ErrProfileNotFound
	}
	m.bySlug[profile.Slug()] = profile
	return nil
}

type fixedIDs struct {
	ids []profiledomain.ProfileID
}

func (f *fixedIDs) NewProfileID() (profiledomain.ProfileID, error) {
	if len(f.ids) == 0 {
		return "", errors.New("no profile IDs remaining")
	}
	id := f.ids[0]
	f.ids = f.ids[1:]
	return id, nil
}

type fakeToolLookup struct {
	tool catalogdomain.Tool
}

func (f fakeToolLookup) GetTool(_ context.Context, id catalogdomain.ToolID) (catalogdomain.Tool, error) {
	if id != f.tool.ID() {
		return catalogdomain.Tool{}, catalogapplication.ErrToolNotFound
	}
	return f.tool, nil
}

func mustProfileID(t *testing.T, raw string) profiledomain.ProfileID {
	t.Helper()
	id, err := profiledomain.ParseProfileID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func mustTool(t *testing.T) catalogdomain.Tool {
	t.Helper()
	toolID, _ := catalogdomain.ParseToolID("tool_00000000000000000000000000000001")
	providerID, _ := catalogdomain.ParseProviderID("prov_00000000000000000000000000000001")
	schema, _ := catalogdomain.NewSchema([]byte(`{"type":"object"}`))
	tool, err := catalogdomain.NewTool(
		toolID,
		providerID,
		catalogdomain.ToolSpec{
			SourceKey:   "echo",
			Name:        "echo",
			InputSchema: schema,
		},
		time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	return tool
}
