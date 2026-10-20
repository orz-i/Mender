package bootstrap

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"

	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

type changingCatalogFixture struct {
	revision          atomic.Int64
	fingerprint       atomic.Value
	unavailable       atomic.Bool
	fingerprintFailed atomic.Bool
	changeDuringBuild atomic.Bool
	builds            atomic.Int64
}

func newCatalogChangeHandler(t *testing.T) (*managedProfileMCPHTTPHandler, *changingCatalogFixture) {
	t.Helper()
	p := freshProjectionProfile(t)
	state := &changingCatalogFixture{}
	state.fingerprint.Store("v0")
	handler, err := newManagedProfileMCPHTTPHandler(p, projectionText("v0"),
		func(context.Context, profiledomain.ProfileID) (uint64, error) { return 0, nil },
		func(context.Context, profiledomain.ProfileID) (profiledomain.Profile, error) { return p, nil },
		func(context.Context, profiledomain.Profile) (http.Handler, error) {
			state.builds.Add(1)
			if state.changeDuringBuild.CompareAndSwap(true, false) {
				state.revision.Add(1)
			}
			return projectionText(state.fingerprint.Load().(string)), nil
		},
		managedMCPToolCatalog{
			revision: func(context.Context) (int64, error) {
				if state.unavailable.Load() {
					return 0, errors.New("secret Catalog storage failure")
				}
				return state.revision.Load(), nil
			},
			fingerprint: func(context.Context, profiledomain.Profile) (string, error) {
				if state.fingerprintFailed.Load() {
					return "", errors.New("secret fingerprint error")
				}
				return state.fingerprint.Load().(string), nil
			},
			initialRevision: 0, initialFingerprint: "v0",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return handler, state
}
func TestManagedProfileCatalogHotRefreshNoopAndConcurrentPublish(t *testing.T) {
	h, state := newCatalogChangeHandler(t)
	// Unrelated Provider writes and identical Tool snapshots change the
	// global catalog revision, but must never reactivate this Profile.
	for _, version := range []int64{1, 2, 3} {
		state.revision.Store(version)
		if code, body := responseBody(t, h); code != 200 || body != "v0" {
			t.Fatalf("unrelated Catalog change yielded %d %q", code, body)
		}
	}
	if state.builds.Load() != 0 || h.current.Load().catalogRevision != 3 {
		t.Fatalf("no-op change rebuilt adapter: %+v builds=%d", h.current.Load(), state.builds.Load())
	}
	// The changed Profile-bound Tool digest must publish exactly once even
	// when many requests arrive with the same catalog revision.
	state.fingerprint.Store("v1")
	state.revision.Store(4)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if code, body := responseBody(t, h); code != 200 || body != "v1" {
				t.Errorf("concurrent hot refresh failed: %d %q", code, body)
			}
		}()
	}
	wg.Wait()
	if got := state.builds.Load(); got != 1 {
		t.Fatalf("redundant or missing MCP Adapter rebuild: %d", got)
	}
	if code, body := responseBody(t, h); code != 200 || body != "v1" {
		t.Fatalf("cached new projection=%d %q", code, body)
	}
}
func TestManagedProfileCatalogHotRefreshFailsClosedAndRetries(t *testing.T) {
	h, state := newCatalogChangeHandler(t)
	state.unavailable.Store(true)
	if code, body := responseBody(t, h); code != 503 || body == "v0" || body == "secret Catalog storage failure" {
		t.Fatalf("catalog read failure leaked stale projection: %d %q", code, body)
	}
	state.unavailable.Store(false)
	state.fingerprintFailed.Store(true)
	state.revision.Store(1)
	if code, body := responseBody(t, h); code != 503 || body == "v0" || body == "secret fingerprint error" {
		t.Fatalf("digest read failure leaked stale projection: %d %q", code, body)
	}
	state.fingerprintFailed.Store(false)
	state.fingerprint.Store("v1")
	state.changeDuringBuild.Store(true)
	if code, body := responseBody(t, h); code != 503 || body == "v1" || body == "v0" {
		t.Fatalf("mid-build Catalog commit published stale Adapter: %d %q", code, body)
	}
	snap := h.current.Load()
	if snap.catalogRevision != 0 || snap.fingerprint != "v0" {
		t.Fatalf("failed refresh published stale snapshot: %+v", snap)
	}
	if code, body := responseBody(t, h); code != 200 || body != "v1" {
		t.Fatalf("retry after racing Catalog commit: %d %q", code, body)
	}
	if snap = h.current.Load(); snap.catalogRevision != 2 {
		t.Fatalf("expected latest catalog revision 2, got %+v", snap)
	}
	// Existing catalog/profile versions must never go backwards. Fail closed.
	state.revision.Store(1)
	if code, body := responseBody(t, h); code != 503 || body == "v1" {
		t.Fatalf("revision rollback accepted: %d %q", code, body)
	}
}
func TestManagedProfileCatalogMetadataChangeDuringLoadNeverPublishes(t *testing.T) {
	p := freshProjectionProfile(t)
	var rev atomic.Int64
	fingerprint := "unchanged"
	var builds atomic.Int64
	h, err := newManagedProfileMCPHTTPHandler(p, projectionText("v0"),
		func(context.Context, profiledomain.ProfileID) (uint64, error) { return 0, nil },
		func(context.Context, profiledomain.ProfileID) (profiledomain.Profile, error) { return p, nil },
		func(context.Context, profiledomain.Profile) (http.Handler, error) {
			builds.Add(1)
			return projectionText("v1"), nil
		},
		managedMCPToolCatalog{
			revision: func(context.Context) (int64, error) { return rev.Load(), nil },
			fingerprint: func(context.Context, profiledomain.Profile) (string, error) {
				rev.Add(1) // Writer commits while loading Tool metadata.
				return fingerprint, nil
			}, initialRevision: 0, initialFingerprint: "v0",
		})
	if err != nil {
		t.Fatal(err)
	}
	rev.Store(1)
	if code, body := responseBody(t, h); code != 503 || body == "v0" || body == "v1" {
		t.Fatalf("mid-fingerprint mutation leaked projection %d %q", code, body)
	}
	if builds.Load() != 0 {
		t.Fatalf("rebuild should not start with a torn Tool snapshot")
	}
	if h.current.Load().catalogRevision != 0 {
		t.Fatal("published a torn Catalog version")
	}
}
