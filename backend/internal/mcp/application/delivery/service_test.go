package delivery

import (
	"context"
	"errors"
	"testing"

	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	mcp "github.com/orz-i/Mender/backend/internal/mcp/domain"
	profile "github.com/orz-i/Mender/backend/internal/profile/domain"
	tunnel "github.com/orz-i/Mender/backend/internal/tunnel/domain"
)

type deliveryRepository struct {
	puts             int
	err              error
	profileRevision  uint64
	expectedRevision *uint64
}

func (*deliveryRepository) GetConfig(context.Context, profile.ProfileID) (mcp.Config, error) {
	return mcp.Config{}, ErrNotFound
}
func (*deliveryRepository) GetConfigRevision(context.Context, profile.ProfileID) (uint64, error) {
	return 0, ErrNotFound
}
func (r *deliveryRepository) PutConfig(_ context.Context, cfg mcp.Config, revision uint64, expected *uint64) (mcp.Config, error) {
	r.puts++
	r.profileRevision = revision
	r.expectedRevision = expected
	return cfg, r.err
}

type profileLookupFunc func(context.Context, profile.ProfileID) (profile.Profile, error)

func (f profileLookupFunc) Execute(ctx context.Context, id profile.ProfileID) (profile.Profile, error) {
	return f(ctx, id)
}

type tunnelLookupFunc func(context.Context, tunnel.TunnelID) (tunnel.Tunnel, error)

func (f tunnelLookupFunc) Execute(ctx context.Context, id tunnel.TunnelID) (tunnel.Tunnel, error) {
	return f(ctx, id)
}

func selectedProfile(t *testing.T) profile.Profile {
	t.Helper()
	binding, err := profile.NewToolBinding(profile.ToolBindingSpec{ToolID: catalog.ToolID("tool_00000000000000000000000000000001")})
	if err != nil {
		t.Fatal(err)
	}
	p, err := profile.NewProfileWithBindings(profile.ProfileID("prof_00000000000000000000000000000001"), profile.Slug("selected"), "Selected", []profile.ToolBinding{binding})
	if err != nil {
		t.Fatal(err)
	}
	return p.WithRevision(7)
}

func TestConfigureValidatesSelectionAndForwardsBothCommitFences(t *testing.T) {
	p := selectedProfile(t)
	for _, test := range []struct {
		name     string
		mode     mcp.Mode
		revision uint64
		ids      []catalog.ToolID
		want     error
	}{
		{"valid", mcp.ModeHybrid, 7, p.ToolIDs(), nil},
		{"invalid mode", "unknown", 7, nil, ErrInvalidConfiguration},
		{"stale selection", mcp.ModeHybrid, 6, p.ToolIDs(), ErrProfileRevisionConflict},
		{"outside selection", mcp.ModeHybrid, 7, []catalog.ToolID{"tool_00000000000000000000000000000099"}, ErrToolOutsideProfile},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := &deliveryRepository{}
			uc := NewConfigure(repo, profileLookupFunc(func(context.Context, profile.ProfileID) (profile.Profile, error) { return p, nil }), nil)
			expected := uint64(0)
			_, err := uc.Execute(context.Background(), ConfigureRequest{ProfileID: p.ID(), Mode: test.mode, DirectToolIDs: test.ids, ExpectedProfileRevision: test.revision, ExpectedRevision: &expected})
			if !errors.Is(err, test.want) {
				t.Fatalf("error=%v want %v", err, test.want)
			}
			if test.want != nil && repo.puts != 0 {
				t.Fatal("invalid/stale selection reached commit")
			}
			if test.want == nil && (repo.puts != 1 || repo.profileRevision != 7 || repo.expectedRevision != &expected) {
				t.Fatal("commit authority lost independent fences")
			}
		})
	}
}

func TestConfigureRechecksTunnelAndPropagatesConcurrentCommitConflict(t *testing.T) {
	p := selectedProfile(t)
	missing := errors.New("Tunnel removed")
	for _, lookupErr := range []error{missing, nil} {
		repo := &deliveryRepository{err: ErrProfileRevisionConflict}
		uc := NewConfigure(repo, profileLookupFunc(func(context.Context, profile.ProfileID) (profile.Profile, error) { return p, nil }),
			tunnelLookupFunc(func(context.Context, tunnel.TunnelID) (tunnel.Tunnel, error) { return tunnel.Tunnel{}, lookupErr }))
		_, err := uc.Execute(context.Background(), ConfigureRequest{ProfileID: p.ID(), Mode: mcp.ModeDynamic, TunnelID: tunnel.TunnelID("tun_00000000000000000000000000000001"), ExpectedProfileRevision: 7})
		if lookupErr != nil {
			if !errors.Is(err, missing) || repo.puts != 0 {
				t.Fatalf("missing Tunnel reached commit: %v", err)
			}
		} else if !errors.Is(err, ErrProfileRevisionConflict) || repo.puts != 1 {
			t.Fatalf("commit-time selection race was hidden: %v", err)
		}
	}
}

func TestMissingDeliveryReadsNeverCreateConfiguration(t *testing.T) {
	repo := &deliveryRepository{}
	id := profile.ProfileID("prof_00000000000000000000000000000001")
	if _, err := NewGet(repo).Execute(context.Background(), id); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := NewGetRevision(repo).Execute(context.Background(), id); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if repo.puts != 0 {
		t.Fatal("read repaired missing delivery")
	}
}
