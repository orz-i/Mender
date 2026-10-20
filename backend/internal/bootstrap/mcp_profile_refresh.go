package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"

	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

// managedProfileMCPHTTPHandler serves immutable Profile MCP projections.
// The Catalog revision is a cheap dirty bit; changes in unrelated Tool
// sources only trigger a bounded comparison of this Profile's bound metadata.
// A changed projection is published atomically after confirming both durable
// versions. No new Tunnel/listener, Profile grant or Agent loop is created.
type managedProfileMCPHTTPHandler struct {
	id       profiledomain.ProfileID
	slug     profiledomain.Slug
	revision func(context.Context, profiledomain.ProfileID) (uint64, error)
	load     func(context.Context, profiledomain.ProfileID) (profiledomain.Profile, error)
	build    func(context.Context, profiledomain.Profile) (http.Handler, error)
	catalog  managedMCPToolCatalog
	mu       sync.Mutex
	current  atomic.Pointer[profileMCPHTTPSnapshot]
}

// managedMCPToolCatalog uses the existing canonical catalog revision and
// a metadata-only fingerprint of the Profile's currently bound Tool IDs.
// initialRevision/initialFingerprint must be captured BEFORE the initial
// Adapter build and rechecked after it, so startup cannot claim newer Tool
// metadata than the Adapter actually captured.
type managedMCPToolCatalog struct {
	revision           func(context.Context) (int64, error)
	fingerprint        func(context.Context, profiledomain.Profile) (string, error)
	initialRevision    int64
	initialFingerprint string
}

type profileMCPHTTPSnapshot struct {
	revision        uint64
	catalogRevision int64
	fingerprint     string
	handler         http.Handler
}

func newManagedProfileMCPHTTPHandler(profile profiledomain.Profile, initial http.Handler,
	revision func(context.Context, profiledomain.ProfileID) (uint64, error),
	load func(context.Context, profiledomain.ProfileID) (profiledomain.Profile, error),
	build func(context.Context, profiledomain.Profile) (http.Handler, error),
	catalog managedMCPToolCatalog,
) (*managedProfileMCPHTTPHandler, error) {
	if initial == nil || revision == nil || load == nil || build == nil ||
		catalog.revision == nil || catalog.fingerprint == nil ||
		catalog.initialRevision < 0 || catalog.initialFingerprint == "" {
		return nil, errors.New("managed MCP Profile refresh requires all dependencies and an initial catalog snapshot")
	}
	h := &managedProfileMCPHTTPHandler{id: profile.ID(), slug: profile.Slug(),
		revision: revision, load: load, build: build, catalog: catalog}
	h.current.Store(&profileMCPHTTPSnapshot{revision: profile.Revision(),
		catalogRevision: catalog.initialRevision, fingerprint: catalog.initialFingerprint,
		handler: initial})
	return h, nil
}

func (h *managedProfileMCPHTTPHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	snap, err := h.snapshotForRequest(req.Context())
	if err != nil {
		// Any storage/fingerprint/rebuild error must fail closed instead of
		// revealing stale bound Tool schemas, IDs or Provider diagnostics.
		http.Error(w, "MCP Profile unavailable", http.StatusServiceUnavailable)
		return
	}
	snap.handler.ServeHTTP(w, req)
}

func (h *managedProfileMCPHTTPHandler) versions(ctx context.Context) (uint64, int64, error) {
	profileRev, err := h.revision(ctx, h.id)
	if err != nil {
		return 0, 0, err
	}
	catalogRev, err := h.catalog.revision(ctx)
	if err != nil {
		return 0, 0, err
	}
	if catalogRev < 0 {
		return 0, 0, errors.New("managed MCP catalog revision is negative")
	}
	return profileRev, catalogRev, nil
}

func (h *managedProfileMCPHTTPHandler) snapshotForRequest(ctx context.Context) (*profileMCPHTTPSnapshot, error) {
	profileRev, catalogRev, err := h.versions(ctx)
	if err != nil {
		return nil, err
	}
	cur := h.current.Load()
	if cur != nil && cur.revision == profileRev && cur.catalogRevision == catalogRev {
		return cur, nil
	}

	// On a changed version, exactly one request compares/rebuilds the view.
	// Request handlers that already acquired an old immutable snapshot may
	// finish; Invocation still checks current Profile membership at call time.
	h.mu.Lock()
	defer h.mu.Unlock()

	profileRev, catalogRev, err = h.versions(ctx)
	if err != nil {
		return nil, err
	}
	cur = h.current.Load()
	if cur != nil {
		if profileRev < cur.revision || catalogRev < cur.catalogRevision {
			return nil, errors.New("managed MCP Profile or Catalog revision regressed")
		}
		if cur.revision == profileRev && cur.catalogRevision == catalogRev {
			return cur, nil
		}
	}

	profile, err := h.load(ctx, h.id)
	if err != nil {
		return nil, err
	}
	if profile.ID() != h.id || profile.Slug() != h.slug {
		return nil, errors.New("managed MCP Profile identity changed")
	}
	if profile.Revision() != profileRev {
		return nil, errors.New("managed MCP Profile changed during aggregate load")
	}
	fingerprint, err := h.catalog.fingerprint(ctx, profile)
	if err != nil {
		return nil, fmt.Errorf("managed MCP Profile Tool metadata: %w", err)
	}
	// Profile bindings load in separate reads, and Catalog Tool definitions
	// are read per bound ToolID. Both revisions must remain stable across it.
	verifiedProfileRev, verifiedCatalogRev, err := h.versions(ctx)
	if err != nil {
		return nil, err
	}
	if verifiedProfileRev != profileRev || verifiedCatalogRev != catalogRev {
		return nil, errors.New("managed MCP Profile or Catalog changed during metadata load")
	}
	if cur != nil && cur.revision == profileRev && cur.fingerprint == fingerprint {
		// No bound Tool metadata changed. A no-op sync or unrelated Provider
		// write must not reactivate Providers or rebuild a live SDK projection.
		next := &profileMCPHTTPSnapshot{revision: profileRev,
			catalogRevision: catalogRev, fingerprint: fingerprint, handler: cur.handler}
		h.current.Store(next)
		return next, nil
	}
	fresh, err := h.build(ctx, profile)
	if err != nil {
		return nil, err
	}
	if fresh == nil {
		return nil, errors.New("managed MCP Profile produced no handler")
	}
	confirmedProfileRev, confirmedCatalogRev, err := h.versions(ctx)
	if err != nil {
		return nil, err
	}
	if confirmedProfileRev != profileRev || confirmedCatalogRev != catalogRev {
		return nil, errors.New("managed MCP Profile or Catalog changed during rebuild")
	}
	published := &profileMCPHTTPSnapshot{revision: profileRev,
		catalogRevision: catalogRev, fingerprint: fingerprint, handler: fresh}
	h.current.Store(published)
	return published, nil
}
