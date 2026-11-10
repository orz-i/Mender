package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	credential "github.com/orz-i/Mender/backend/internal/credential/domain"
	delivery "github.com/orz-i/Mender/backend/internal/mcp/application/delivery"
	mcp "github.com/orz-i/Mender/backend/internal/mcp/domain"
	profile "github.com/orz-i/Mender/backend/internal/profile/domain"
	profilesqlite "github.com/orz-i/Mender/backend/internal/profile/infrastructure/sqlite"
	v3fixture "github.com/orz-i/Mender/backend/internal/testfixtures/v3"
	tunnelapp "github.com/orz-i/Mender/backend/internal/tunnel/application"
	tunnel "github.com/orz-i/Mender/backend/internal/tunnel/domain"
	tunnelsqlite "github.com/orz-i/Mender/backend/internal/tunnel/infrastructure/sqlite"
)

func deliveryFixture(t *testing.T) (*Store, *profilesqlite.Store, profile.Profile, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "delivery.db")
	v3fixture.Prepare(t, path)
	profiles, err := profilesqlite.OpenTyped(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = profiles.Close() })
	id, _ := profile.ParseProfileID("prof_00000000000000000000000000000001")
	slug, _ := profile.NewSlug("selected")
	first, _ := profile.NewToolBinding(profile.ToolBindingSpec{ToolID: catalog.ToolID("tool_882fb9c25db58744f34da684f7f7ab5f"), Alias: "one"})
	second, _ := profile.NewToolBinding(profile.ToolBindingSpec{ToolID: catalog.ToolID("tool_33a65e204e96265604efddd7536eabd9"), Alias: "two"})
	p, err := profile.NewProfileWithBindings(id, slug, "Selected", []profile.ToolBinding{first, second})
	if err != nil {
		t.Fatal(err)
	}
	if err := profiles.CreateProfile(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	p, err = profiles.GetProfileByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	s, err := OpenTyped(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, profiles, p, path
}

func configFor(t *testing.T, p profile.Profile, mode mcp.Mode, direct []catalog.ToolID) mcp.Config {
	t.Helper()
	c, err := mcp.NewConfig(p.ID(), mode, "", direct)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestMCPConfigurationIsExplicitAndHasIndependentRevision(t *testing.T) {
	t.Parallel()
	s, profiles, p, _ := deliveryFixture(t)
	ctx := context.Background()
	if _, err := s.GetConfig(ctx, p.ID()); !errors.Is(err, delivery.ErrNotFound) {
		t.Fatalf("new Profile unexpectedly has delivery: %v", err)
	}
	if _, err := s.GetConfigRevision(ctx, p.ID()); !errors.Is(err, delivery.ErrNotFound) {
		t.Fatal(err)
	}
	cfg, err := s.PutConfig(ctx, configFor(t, p, mcp.ModeHybrid, p.ToolIDs()[:1]), p.Revision(), nil)
	if err != nil || cfg.Revision() != 0 {
		t.Fatalf("initial config: %+v %v", cfg, err)
	}
	zero := uint64(0)
	changed, err := s.PutConfig(ctx, configFor(t, p, mcp.ModeDynamic, nil), p.Revision(), &zero)
	if err != nil || changed.Revision() != 1 {
		t.Fatalf("update: %+v %v", changed, err)
	}
	if revision, err := profiles.GetProfileRevision(ctx, p.ID()); err != nil || revision != p.Revision() {
		t.Fatalf("MCP change dirtied selection/Skill revision: %d %v", revision, err)
	}
	if _, err := s.PutConfig(ctx, cfg, p.Revision(), &zero); !errors.Is(err, delivery.ErrRevisionConflict) {
		t.Fatalf("stale config replaced new delivery: %v", err)
	}
	if _, err := s.PutConfig(ctx, cfg, p.Revision(), nil); !errors.Is(err, delivery.ErrRevisionConflict) {
		t.Fatalf("create-only overwrote delivery: %v", err)
	}
}

func TestMCPConfigurationCASAcrossConnectionsHasOneWinner(t *testing.T) {
	t.Parallel()
	s, _, p, path := deliveryFixture(t)
	other, err := OpenTyped(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.Close() }()
	if _, err := s.PutConfig(context.Background(), configFor(t, p, mcp.ModeHybrid, nil), p.Revision(), nil); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, store := range []*Store{s, other} {
		go func(store *Store) {
			<-start
			revision := uint64(0)
			_, err := store.PutConfig(context.Background(), configFor(t, p, mcp.ModeHybrid, p.ToolIDs()), p.Revision(), &revision)
			results <- err
		}(store)
	}
	close(start)
	won, stale := 0, 0
	for range 2 {
		err := <-results
		if err == nil {
			won++
		} else if errors.Is(err, delivery.ErrRevisionConflict) {
			stale++
		} else {
			t.Fatal(err)
		}
	}
	if won != 1 || stale != 1 {
		t.Fatalf("CAS winners/conflicts=%d/%d", won, stale)
	}
}

func TestProfileMutationPreservesOrPrunesDirectMembershipWithBothFences(t *testing.T) {
	t.Parallel()
	s, profiles, p, _ := deliveryFixture(t)
	ctx := context.Background()
	cfg, err := s.PutConfig(ctx, configFor(t, p, mcp.ModeHybrid, p.ToolIDs()), p.Revision(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := profiles.SaveProfile(ctx, p); err != nil {
		t.Fatal(err)
	}
	kept, err := s.GetConfig(ctx, p.ID())
	if err != nil || kept.Revision() != 0 || !slices.Equal(kept.DirectToolIDs(), cfg.DirectToolIDs()) {
		t.Fatalf("unchanged members erased MCP selection: %+v %v", kept, err)
	}
	zero := uint64(0)
	if _, err := s.PutConfig(ctx, cfg, p.Revision(), &zero); !errors.Is(err, delivery.ErrProfileRevisionConflict) {
		t.Fatalf("selection race escaped CAS: %v", err)
	}
	fresh, err := profiles.GetProfileByID(ctx, p.ID())
	if err != nil {
		t.Fatal(err)
	}
	removed := fresh.ToolIDs()[0]
	next, err := fresh.RemoveTool(removed)
	if err != nil {
		t.Fatal(err)
	}
	if err := profiles.SaveProfile(ctx, next); err != nil {
		t.Fatal(err)
	}
	pruned, err := s.GetConfig(ctx, p.ID())
	if err != nil || pruned.Revision() != 1 || len(pruned.DirectToolIDs()) != 1 || pruned.DirectToolIDs()[0] == removed {
		t.Fatalf("membership removal failed to prune/revision delivery: %+v %v", pruned, err)
	}
	current, err := profiles.GetProfileByID(ctx, p.ID())
	if err != nil {
		t.Fatal(err)
	}
	staleSelection := configFor(t, current, mcp.ModeHybrid, []catalog.ToolID{removed})
	before := pruned.Revision()
	if _, err := s.PutConfig(ctx, staleSelection, current.Revision(), &before); !errors.Is(err, delivery.ErrToolOutsideProfile) {
		t.Fatalf("outside member committed: %v", err)
	}
	after, err := s.GetConfig(ctx, p.ID())
	if err != nil || after.Revision() != before || !slices.Equal(after.DirectToolIDs(), pruned.DirectToolIDs()) {
		t.Fatalf("failed subset write was not atomic: %+v %v", after, err)
	}
}

func TestMCPConfigureAndProfileRevocationSerializeWithoutRestoringAccess(t *testing.T) {
	t.Parallel()
	s, _, p, path := deliveryFixture(t)
	other, err := profilesqlite.OpenTyped(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.Close() }()
	if _, err := s.PutConfig(context.Background(), configFor(t, p, mcp.ModeHybrid, nil), p.Revision(), nil); err != nil {
		t.Fatal(err)
	}
	removed := p.ToolIDs()[0]
	next, err := p.RemoveTool(removed)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	var configureErr, selectionErr error
	go func() {
		defer wg.Done()
		<-start
		revision := uint64(0)
		_, configureErr = s.PutConfig(context.Background(), configFor(t, p, mcp.ModeHybrid, []catalog.ToolID{removed}), p.Revision(), &revision)
	}()
	go func() { defer wg.Done(); <-start; selectionErr = other.SaveProfile(context.Background(), next) }()
	close(start)
	wg.Wait()
	if selectionErr != nil {
		t.Fatal(selectionErr)
	}
	if configureErr != nil && !errors.Is(configureErr, delivery.ErrProfileRevisionConflict) {
		t.Fatal(configureErr)
	}
	cfg, err := s.GetConfig(context.Background(), p.ID())
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(cfg.DirectToolIDs(), removed) {
		t.Fatal("concurrent configuration restored removed Tool exposure")
	}
}

func TestMCPConfigTunnelReferenceIsFencedAgainstConcurrentDeletion(t *testing.T) {
	t.Parallel()
	s, _, p, path := deliveryFixture(t)
	tunnels, err := tunnelsqlite.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tunnels.Close() }()
	id := tunnel.TunnelID("tun_00000000000000000000000000000001")
	ref, _ := credential.NewReference("openai", "runtime")
	item, err := tunnel.NewTunnel(id, "Reachability", tunnel.ProviderOpenAI, tunnel.OpenAIBinding(), "remote-id", ref)
	if err != nil {
		t.Fatal(err)
	}
	if err := tunnels.Create(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	base, err := s.PutConfig(context.Background(), configFor(t, p, mcp.ModeDynamic, nil), p.Revision(), nil)
	if err != nil {
		t.Fatal(err)
	}
	linked, err := mcp.NewConfig(p.ID(), mcp.ModeDynamic, id, nil)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	var putErr, deleteErr error
	go func() {
		defer wg.Done()
		<-start
		revision := base.Revision()
		_, putErr = s.PutConfig(context.Background(), linked, p.Revision(), &revision)
	}()
	go func() { defer wg.Done(); <-start; deleteErr = tunnels.Delete(context.Background(), id) }()
	close(start)
	wg.Wait()
	if putErr != nil && !errors.Is(putErr, delivery.ErrInvalidConfiguration) {
		t.Fatal(putErr)
	}
	if deleteErr != nil && !errors.Is(deleteErr, tunnelapp.ErrTunnelInUse) {
		t.Fatal(deleteErr)
	}
	if putErr == nil && deleteErr == nil {
		t.Fatal("configured a deleted Tunnel")
	}
	if putErr == nil {
		if err := tunnels.Delete(context.Background(), id); !errors.Is(err, tunnelapp.ErrTunnelInUse) {
			t.Fatalf("cross-connection delete guard missing: %v", err)
		}
	} else {
		current, err := s.GetConfig(context.Background(), p.ID())
		if err != nil {
			t.Fatal(err)
		}
		if _, bound := current.TunnelID(); bound || current.Revision() != base.Revision() {
			t.Fatal("failed Tunnel write partially committed")
		}
	}
}

func TestMCPStoreRejectsFutureSchemaWithoutChangingIt(t *testing.T) {
	t.Parallel()
	s, _, _, path := deliveryFixture(t)
	if _, err := s.db.Exec("INSERT INTO mcp_delivery_schema_migrations VALUES(999,'future')"); err != nil {
		t.Fatal(err)
	}
	if newer, err := OpenTyped(context.Background(), path); err == nil {
		_ = newer.Close()
		t.Fatal("future delivery schema accepted")
	} else if !strings.Contains(err.Error(), "expected version 2") {
		t.Fatal(err)
	}
	var version int
	if err := s.db.QueryRow("SELECT MAX(version) FROM mcp_delivery_schema_migrations").Scan(&version); err != nil || version != 999 {
		t.Fatalf("future schema changed: %d %v", version, err)
	}
}
