package bootstrap

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"

	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

// managedProfileMCPHTTPHandler synchronizes stateless MCP requests to the
// durable Profile revision. A new immutable SDK projection is published only
// after all of its Tool descriptors and Provider admissions are ready.
//
// An in-flight request owns its original handler, but Invocation always
// enforces current Profile binding membership before executing any Tool.
// This boundary does not manage an Agent loop or restart an MCP/Tunnel server.
type managedProfileMCPHTTPHandler struct {
	id       profiledomain.ProfileID
	slug     profiledomain.Slug
	revision func(context.Context, profiledomain.ProfileID) (uint64, error)
	load     func(context.Context, profiledomain.ProfileID) (profiledomain.Profile, error)
	build    func(context.Context, profiledomain.Profile) (http.Handler, error)
	mu       sync.Mutex
	current  atomic.Pointer[profileMCPHTTPSnapshot]
}

type profileMCPHTTPSnapshot struct {
	revision uint64
	handler  http.Handler
}

func newManagedProfileMCPHTTPHandler(profile profiledomain.Profile, initial http.Handler,
	revision func(context.Context, profiledomain.ProfileID) (uint64, error),
	load func(context.Context, profiledomain.ProfileID) (profiledomain.Profile, error),
	build func(context.Context, profiledomain.Profile) (http.Handler, error),
) (*managedProfileMCPHTTPHandler, error) {
	if initial == nil || revision == nil || load == nil || build == nil {
		return nil, errors.New("managed MCP Profile refresh requires all dependencies")
	}
	handler := &managedProfileMCPHTTPHandler{id: profile.ID(), slug: profile.Slug(), revision: revision, load: load, build: build}
	handler.current.Store(&profileMCPHTTPSnapshot{revision: profile.Revision(), handler: initial})
	return handler, nil
}

func (h *managedProfileMCPHTTPHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	snapshot, err := h.snapshotForRequest(req.Context())
	if err != nil {
		// A failed revision query or refresh must never expose the old Profile
		// (even via tools/list), nor leak Tool IDs or provider diagnostics.
		http.Error(w, "MCP Profile unavailable", http.StatusServiceUnavailable)
		return
	}
	snapshot.handler.ServeHTTP(w, req)
}

func (h *managedProfileMCPHTTPHandler) snapshotForRequest(ctx context.Context) (*profileMCPHTTPSnapshot, error) {
	revision, err := h.revision(ctx, h.id)
	if err != nil {
		return nil, err
	}
	current := h.current.Load()
	if current != nil && current.revision == revision {
		return current, nil
	}

	// A request that saw an older revision cannot publish it after another
	// request has already observed and published a newer one.
	h.mu.Lock()
	defer h.mu.Unlock()
	profile, err := h.load(ctx, h.id)
	if err != nil {
		return nil, err
	}
	if profile.ID() != h.id || profile.Slug() != h.slug {
		return nil, errors.New("managed MCP Profile identity changed")
	}
	// Profile storage restores the aggregate and its binding rows with
	// separate queries. If a concurrent writer committed between them, do
	// not publish mismatched metadata under an earlier revision.
	confirmed, err := h.revision(ctx, h.id)
	if err != nil {
		return nil, err
	}
	if confirmed != profile.Revision() {
		return nil, errors.New("managed MCP Profile changed during aggregate load")
	}
	current = h.current.Load()
	if current != nil {
		if profile.Revision() < current.revision {
			return nil, errors.New("managed MCP Profile revision regressed")
		}
		if profile.Revision() == current.revision {
			return current, nil
		}
	}
	refreshed, err := h.build(ctx, profile)
	if err != nil {
		return nil, err
	}
	if refreshed == nil {
		return nil, errors.New("managed MCP Profile produced no handler")
	}
	// A change while activating providers/building the MCP view must be
	// retried on the next request, never published as a false current state.
	confirmed, err = h.revision(ctx, h.id)
	if err != nil {
		return nil, err
	}
	if confirmed != profile.Revision() {
		return nil, errors.New("managed MCP Profile changed during rebuild")
	}
	published := &profileMCPHTTPSnapshot{revision: profile.Revision(), handler: refreshed}
	h.current.Store(published)
	return published, nil
}
