package managed

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	mcpdomain "github.com/orz-i/Mender/backend/internal/mcp/domain"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	tunnelapp "github.com/orz-i/Mender/backend/internal/tunnel/application"
	tunneldomain "github.com/orz-i/Mender/backend/internal/tunnel/domain"
)

func TestDeliveryRevisionRefreshKeepsListenerAndCatalogNoopStable(t *testing.T) {
	initial := freshProjectionProfile(t)
	var delivery atomic.Uint64
	var catalog atomic.Int64
	var unavailable atomic.Bool
	var builds atomic.Int64
	snapshot := func() Snapshot {
		config, err := mcpdomain.NewConfig(initial.ID(), mcpdomain.ModeDirect, "", nil)
		if err != nil {
			panic(err)
		}
		return Snapshot{Profile: initial.Profile, Config: config.WithRevision(delivery.Load())}
	}
	principal, err := gov.NewPrincipal(gov.PrincipalLocalUser, "os:managed-test")
	if err != nil {
		t.Fatal(err)
	}
	control, err := gov.NewContext(gov.OriginCLI, principal, gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	manager, err := New(Dependencies{
		LoadBySlug: func(context.Context, profiledomain.Slug) (Snapshot, error) { return snapshot(), nil },
		LoadByID:   func(context.Context, profiledomain.ProfileID) (Snapshot, error) { return snapshot(), nil },
		Revisions: func(context.Context, profiledomain.ProfileID) (Versions, error) {
			if unavailable.Load() {
				return Versions{}, errors.New("secret delivery storage failure")
			}
			return Versions{Delivery: delivery.Load()}, nil
		},
		BuildHTTP: func(_ context.Context, current Snapshot, authority gov.Context) (http.Handler, error) {
			if authority != control {
				return nil, errors.New("control authority changed")
			}
			builds.Add(1)
			return projectionText(fmt.Sprint("delivery-", current.Config.Revision())), nil
		},
		DescribeTool: func(context.Context, catalogdomain.ToolID) (catalogdomain.Tool, error) {
			return catalogdomain.Tool{}, errors.New("unexpected Tool lookup")
		},
		CatalogRevision: func(context.Context) (int64, error) { return catalog.Load(), nil },
		StartTunnel: func(context.Context, tunneldomain.TunnelID, string) (tunnelapp.Process, tunneldomain.Tunnel, error) {
			return nil, tunneldomain.Tunnel{}, errors.New("unexpected Tunnel startup")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	instance, err := manager.Start(context.Background(), initial.Slug(), "127.0.0.1:0", control)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: &http.Transport{}, Timeout: 3 * time.Second}
	t.Cleanup(client.CloseIdleConnections)
	request := func(wantStatus int, wantBody string) {
		t.Helper()
		response, err := client.Get(instance.Endpoint)
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(response.Body)
		closeErr := response.Body.Close()
		if readErr != nil || closeErr != nil || response.StatusCode != wantStatus || string(body) != wantBody {
			t.Fatalf("live response=%d %q read=%v close=%v; want %d %q", response.StatusCode, body, readErr, closeErr, wantStatus, wantBody)
		}
	}
	request(http.StatusOK, "delivery-0")
	delivery.Store(1)
	request(http.StatusOK, "delivery-1")
	catalog.Store(1)
	request(http.StatusOK, "delivery-1")
	if builds.Load() != 2 {
		t.Fatalf("build count=%d; want initial + one delivery refresh", builds.Load())
	}
	if list := manager.List(); len(list) != 1 || list[0] != instance {
		t.Fatalf("request refresh restarted/replaced listener: %+v", list)
	}
	unavailable.Store(true)
	request(http.StatusServiceUnavailable, "MCP Profile unavailable\n")
	unavailable.Store(false)
	request(http.StatusOK, "delivery-1")
	if err := manager.Stop(context.Background(), instance.ServerID); err != nil {
		t.Fatal(err)
	}
	if len(manager.List()) != 0 {
		t.Fatal("stopped listener remains registered")
	}
}

func TestDeliveryRefreshFencesConcurrentCommitAndRollback(t *testing.T) {
	for _, stage := range []string{"load", "build"} {
		t.Run(stage, func(t *testing.T) {
			initial := freshProjectionProfile(t)
			var revision atomic.Uint64
			var changed atomic.Bool
			var builds atomic.Int64
			handler, err := newManagedProfileMCPHTTPHandler(initial, projectionText("delivery-0"),
				func(context.Context, profiledomain.ProfileID) (Versions, error) {
					return Versions{Delivery: revision.Load()}, nil
				},
				func(context.Context, profiledomain.ProfileID) (Snapshot, error) {
					loaded := initial
					loaded.Config = loaded.Config.WithRevision(revision.Load())
					if stage == "load" && changed.CompareAndSwap(false, true) {
						revision.Add(1)
					}
					return loaded, nil
				},
				func(_ context.Context, snapshot Snapshot) (http.Handler, error) {
					builds.Add(1)
					if stage == "build" && changed.CompareAndSwap(false, true) {
						revision.Add(1)
					}
					return projectionText(fmt.Sprint("delivery-", snapshot.Config.Revision())), nil
				}, managedMCPToolCatalog{
					revision:        func(context.Context) (int64, error) { return 0, nil },
					fingerprint:     func(context.Context, Snapshot) (string, error) { return "same-tools", nil },
					initialRevision: 0, initialFingerprint: "same-tools",
				})
			if err != nil {
				t.Fatal(err)
			}
			revision.Store(1)
			if status, body := responseBody(t, handler); status != http.StatusServiceUnavailable || body == "delivery-0" || handler.current.Load().deliveryRevision != 0 {
				t.Fatalf("concurrent delivery commit published a stale projection: %d %q", status, body)
			}
			var requests sync.WaitGroup
			for i := 0; i < 20; i++ {
				requests.Add(1)
				go func() {
					defer requests.Done()
					if status, body := responseBody(t, handler); status != http.StatusOK || body != "delivery-2" {
						t.Errorf("retry=%d %q", status, body)
					}
				}()
			}
			requests.Wait()
			wantBuilds := int64(1)
			if stage == "build" {
				wantBuilds++
			}
			if builds.Load() != wantBuilds || handler.current.Load().deliveryRevision != 2 {
				t.Fatal("delivery revision did not publish exactly once")
			}
			revision.Store(1)
			if status, _ := responseBody(t, handler); status != http.StatusServiceUnavailable {
				t.Fatal("delivery rollback served stale metadata")
			}
		})
	}
}

func TestSnapshotReadAndBuildConfirmationRejectMixedRevisions(t *testing.T) {
	snapshot := freshProjectionProfile(t)
	current := Versions{Delivery: 1}
	manager := &Manager{deps: Dependencies{
		LoadByID:  func(context.Context, profiledomain.ProfileID) (Snapshot, error) { return snapshot, nil },
		Revisions: func(context.Context, profiledomain.ProfileID) (Versions, error) { return current, nil },
	}}
	if _, err := manager.LoadSnapshot(context.Background(), snapshot.ID()); err == nil {
		t.Fatal("separate Profile/Delivery reads returned a mixed snapshot")
	}
	snapshot.Config = snapshot.Config.WithRevision(1)
	loaded, err := manager.LoadSnapshot(context.Background(), snapshot.ID())
	if err != nil {
		t.Fatal(err)
	}
	current.Delivery = 2
	if err := manager.ConfirmSnapshot(context.Background(), loaded); err == nil {
		t.Fatal("delivery changed during a non-HTTP projection build without rejection")
	}
}
