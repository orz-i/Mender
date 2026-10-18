package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

func freshProjectionProfile(t *testing.T) profiledomain.Profile {
	t.Helper()
	id, _ := profiledomain.ParseProfileID("prof_00000000000000000000000000000001")
	slug, _ := profiledomain.NewSlug("live-projection")
	p, err := profiledomain.NewProfileWithBindings(id, slug, "Live Projection", profiledomain.ExposureDynamic, nil)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func projectionText(value string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = fmt.Fprint(w, value) })
}

func responseBody(t *testing.T, h http.Handler) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mcp", nil))
	return rec.Code, rec.Body.String()
}

func TestManagedProfileHotRefreshFailsClosedAndRetries(t *testing.T) {
	p := freshProjectionProfile(t)
	var rev atomic.Uint64
	var unavailable atomic.Bool
	var rebuilding atomic.Bool
	var builds atomic.Int64
	h, err := newManagedProfileMCPHTTPHandler(p, projectionText("v0"),
		func(context.Context, profiledomain.ProfileID) (uint64, error) {
			if unavailable.Load() {
				return 0, errors.New("secret profile storage error")
			}
			return rev.Load(), nil
		},
		func(context.Context, profiledomain.ProfileID) (profiledomain.Profile, error) {
			return p.WithRevision(rev.Load()), nil
		},
		func(_ context.Context, latest profiledomain.Profile) (http.Handler, error) {
			builds.Add(1)
			if rebuilding.Load() {
				return nil, errors.New("secret tool metadata error")
			}
			return projectionText(fmt.Sprint("v", latest.Revision())), nil
		})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if status, body := responseBody(t, h); status != 200 || body != "v0" {
			t.Fatalf("cached response=%d %q", status, body)
		}
	}
	if builds.Load() != 0 {
		t.Fatalf("unnecessary rebuild=%d", builds.Load())
	}
	rev.Store(1)
	rebuilding.Store(true)
	if status, body := responseBody(t, h); status != 503 || body == "v0" || body == "secret tool metadata error" {
		t.Fatalf("stale projection returned: %d %q", status, body)
	}
	rebuilding.Store(false)
	if status, body := responseBody(t, h); status != 200 || body != "v1" {
		t.Fatalf("retry=%d %q", status, body)
	}
	unavailable.Store(true)
	if status, body := responseBody(t, h); status != 503 || body == "v1" || body == "secret profile storage error" {
		t.Fatalf("read failure leaked stale: %d %q", status, body)
	}
	unavailable.Store(false)
	if status, body := responseBody(t, h); status != 200 || body != "v1" {
		t.Fatalf("restore=%d %q", status, body)
	}
}

func TestManagedProfileHotRefreshPublishesMonotonicallyAcrossConcurrentRequests(t *testing.T) {
	p := freshProjectionProfile(t)
	var currentRevision atomic.Uint64
	var publishes atomic.Int64
	h, err := newManagedProfileMCPHTTPHandler(p, projectionText("v0"),
		func(context.Context, profiledomain.ProfileID) (uint64, error) { return currentRevision.Load(), nil },
		func(context.Context, profiledomain.ProfileID) (profiledomain.Profile, error) {
			return p.WithRevision(currentRevision.Load()), nil
		},
		func(_ context.Context, latest profiledomain.Profile) (http.Handler, error) {
			publishes.Add(1)
			return projectionText(fmt.Sprint("v", latest.Revision())), nil
		})
	if err != nil {
		t.Fatal(err)
	}
	for _, wanted := range []uint64{1, 2, 3, 4} {
		currentRevision.Store(wanted)
		var wg sync.WaitGroup
		for i := 0; i < 28; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				status, _ := responseBody(t, h)
				if status != 200 {
					t.Errorf("unexpected status=%d", status)
				}
			}()
		}
		wg.Wait()
		snapshot := h.current.Load()
		if snapshot == nil || snapshot.revision != wanted {
			t.Fatalf("published revision=%v expected=%d", snapshot, wanted)
		}
	}
	// Exactly one new SDK projection per revision, not per concurrent call.
	if got := publishes.Load(); got != 4 {
		t.Fatalf("rebuild count=%d", got)
	}
	if status, body := responseBody(t, h); status != 200 || body != "v4" {
		t.Fatalf("final=%d %q", status, body)
	}
}

func TestManagedProfileRefreshRejectsIdentityChangeAndRollback(t *testing.T) {
	p := freshProjectionProfile(t)
	otherID, _ := profiledomain.ParseProfileID("prof_00000000000000000000000000000002")
	slug, _ := profiledomain.NewSlug("replaced")
	different, err := profiledomain.NewProfileWithBindings(otherID, slug, "Different", profiledomain.ExposureDynamic, nil)
	if err != nil {
		t.Fatal(err)
	}
	h, err := newManagedProfileMCPHTTPHandler(p.WithRevision(2), projectionText("v2"),
		func(context.Context, profiledomain.ProfileID) (uint64, error) { return 3, nil },
		func(context.Context, profiledomain.ProfileID) (profiledomain.Profile, error) {
			return different.WithRevision(3), nil
		},
		func(context.Context, profiledomain.Profile) (http.Handler, error) {
			return projectionText("wrong"), nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if status, body := responseBody(t, h); status != 503 || body == "wrong" || body == "v2" {
		t.Fatalf("identity change was accepted: %d %q", status, body)
	}

	h.load = func(context.Context, profiledomain.ProfileID) (profiledomain.Profile, error) {
		return p.WithRevision(1), nil
	}
	if status, _ := responseBody(t, h); status != 503 {
		t.Fatalf("revision rollback status=%d", status)
	}
}

func TestManagedProfileRefreshRejectsConcurrentCommitAcrossLoadAndBuild(t *testing.T) {
	p := freshProjectionProfile(t)
	var revision atomic.Uint64
	revision.Store(1)
	var updateAtBuild atomic.Bool
	h, err := newManagedProfileMCPHTTPHandler(p, projectionText("v0"),
		func(context.Context, profiledomain.ProfileID) (uint64, error) { return revision.Load(), nil },
		func(context.Context, profiledomain.ProfileID) (profiledomain.Profile, error) {
			return p.WithRevision(revision.Load()), nil
		},
		func(_ context.Context, latest profiledomain.Profile) (http.Handler, error) {
			if updateAtBuild.Load() {
				revision.Add(1)
				updateAtBuild.Store(false)
			}
			return projectionText(fmt.Sprint("v", latest.Revision())), nil
		})
	if err != nil {
		t.Fatal(err)
	}
	updateAtBuild.Store(true)
	if status, body := responseBody(t, h); status != 503 || body == "v1" || body == "v0" {
		t.Fatalf("stale mid-build revision published: %d %q", status, body)
	}
	if snap := h.current.Load(); snap.revision != 0 {
		t.Fatalf("published stale revision %d", snap.revision)
	}
	if status, body := responseBody(t, h); status != 200 || body != "v2" {
		t.Fatalf("retry after concurrent commit: %d %q", status, body)
	}
}
