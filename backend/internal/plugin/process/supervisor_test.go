package process

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	installationapp "github.com/orz-i/Mender/backend/internal/plugin/installation/application"
	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
)

type fakePackages struct {
	mu         sync.Mutex
	calls      int
	mismatched bool
	fail       error
}

func (p *fakePackages) Resolve(_ context.Context, item installationdomain.Installation) (installationapp.ResolvedPackage, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if p.fail != nil {
		return installationapp.ResolvedPackage{}, p.fail
	}
	manifest, _ := item.Manifest()
	if p.mismatched {
		manifest.Extensions[0].Kind = platformplugin.TunnelTransport
	}
	return installationapp.ResolvedPackage{Manifest: manifest}, nil
}
func (p *fakePackages) Count() int { p.mu.Lock(); defer p.mu.Unlock(); return p.calls }

type fakeClient struct {
	exited atomic.Bool
	closed atomic.Int32
}

func (c *fakeClient) Exited() bool { return c.exited.Load() }
func (c *fakeClient) Close() error { c.closed.Add(1); c.exited.Store(true); return nil }

type fakeDialer struct {
	mu      sync.Mutex
	started int
	client  *fakeClient
	wait    chan struct{}
}

func (d *fakeDialer) Connect(ctx context.Context, _ installationdomain.Installation, _ installationapp.ResolvedPackage) (Client, error) {
	d.mu.Lock()
	d.started++
	c := &fakeClient{}
	d.client = c
	d.mu.Unlock()
	if d.wait != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-d.wait:
		}
	}
	return c, nil
}
func (d *fakeDialer) Count() int { d.mu.Lock(); defer d.mu.Unlock(); return d.started }

func installation(t *testing.T, n byte, kind platformplugin.Kind) installationdomain.Installation {
	t.Helper()
	id, _ := installationdomain.ParseInstallationID(fmt.Sprintf("pinst_%032x", n))
	pluginID, _ := platformplugin.ParseID("dev.example.driver")
	manifest := []byte(fmt.Sprintf(`{"manifestVersion":2,"pluginId":"dev.example.driver","version":"1.0.0","displayName":"Driver","entrypoints":{"linux/amd64":"bin/driver"},"extensions":[{"id":"main","kind":%q,"protocolVersion":1}]}`, kind))
	item, err := installationdomain.NewInstallation(id, pluginID, "1.0.0", installationdomain.SourceLocal, "/tmp/driver", "sha256:"+strings.Repeat(fmt.Sprintf("%x", n), 64), manifest, installationdomain.StatusEnabled, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	return item
}

func TestCrossKindProcessOwnershipAndNoSideEffectLookup(t *testing.T) {
	t.Parallel()
	pkgs := &fakePackages{}
	host, err := NewSupervisor(pkgs)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []platformplugin.Kind{platformplugin.ProviderTools, platformplugin.SandboxBackend, platformplugin.TunnelTransport} {
		if err := host.Register(kind, &fakeDialer{}); err != nil {
			t.Fatal(err)
		}
	}
	if err := host.Register(platformplugin.ProviderTools, &fakeDialer{}); err == nil {
		t.Fatal("duplicate registration accepted")
	}
	for index, kind := range []platformplugin.Kind{platformplugin.ProviderTools, platformplugin.SandboxBackend, platformplugin.TunnelTransport} {
		item := installation(t, byte(index+1), kind)
		if host.Running(item.ID()) {
			t.Fatal("catalog lookup started process")
		}
		first, err := host.Ensure(context.Background(), item)
		if err != nil {
			t.Fatal(err)
		}
		second, err := host.Ensure(context.Background(), item)
		if err != nil || first != second {
			t.Fatalf("ensure did not reuse client %v", err)
		}
		first.(*fakeClient).exited.Store(true)
		third, err := host.Ensure(context.Background(), item)
		if err != nil || third == first {
			t.Fatalf("crashed process did not restart %v", err)
		}
		if err := host.CloseInstallation(item.ID()); err != nil {
			t.Fatal(err)
		}
		if host.Running(item.ID()) {
			t.Fatal("closed process still reported as live")
		}
	}
	if pkgs.Count() != 6 {
		t.Fatalf("package verified %d times, want 6", pkgs.Count())
	}
	if err := host.Register(platformplugin.TunnelTransport, &fakeDialer{}); err == nil {
		t.Fatal("registration after startup accepted")
	}
	if err := host.CloseAll(); err != nil {
		t.Fatal(err)
	}
	if _, err := host.Ensure(context.Background(), installation(t, 4, platformplugin.ProviderTools)); err == nil {
		t.Fatal("closed host accepted Ensure")
	}
}

func TestHostRejectsMissingKindTamperedPackageAndDisabledInstall(t *testing.T) {
	t.Parallel()
	pkgs := &fakePackages{}
	host, _ := NewSupervisor(pkgs)
	sandbox := installation(t, 1, platformplugin.SandboxBackend)
	if _, err := host.Ensure(context.Background(), sandbox); err == nil || !strings.Contains(err.Error(), "unsupported extension kind") {
		t.Fatalf("missing kind: %v", err)
	}
	if pkgs.Count() != 0 {
		t.Fatal("unsupported kind resolved package")
	}
	dialer := &fakeDialer{}
	if err := host.Register(platformplugin.SandboxBackend, dialer); err != nil {
		t.Fatal(err)
	}
	disabled, _ := sandbox.WithStatus(installationdomain.StatusDisabled)
	if _, err := host.Ensure(context.Background(), disabled); err == nil {
		t.Fatal("disabled package started")
	}
	pkgs.mismatched = true
	if _, err := host.Ensure(context.Background(), sandbox); err == nil || !strings.Contains(err.Error(), "identity/extension mismatch") {
		t.Fatalf("tampered manifest accepted: %v", err)
	}
	if dialer.Count() != 0 {
		t.Fatal("dialer called for tampered package")
	}
	pkgs.mismatched = false
	if _, err := host.Ensure(context.Background(), sandbox); err != nil {
		t.Fatal(err)
	}
	changed := installation(t, 1, platformplugin.SandboxBackend)
	// Different digest for the same immutable ID must never reuse the client.
	changed, err := installationdomain.NewInstallation(changed.ID(), changed.PluginID(), changed.PackageVersion(), changed.SourceKind(), changed.SourceReference(), "sha256:"+strings.Repeat("f", 64), changed.ManifestJSON(), changed.Status(), changed.InstalledAt())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := host.Ensure(context.Background(), changed); err == nil || !strings.Contains(err.Error(), "digest changed") {
		t.Fatalf("changed digest accepted: %v", err)
	}
}

func TestConcurrentEnsureAndCloseAllFence(t *testing.T) {
	t.Parallel()
	pkgs := &fakePackages{}
	host, _ := NewSupervisor(pkgs)
	dialer := &fakeDialer{wait: make(chan struct{})}
	if err := host.Register(platformplugin.ProviderTools, dialer); err != nil {
		t.Fatal(err)
	}
	item := installation(t, 5, platformplugin.ProviderTools)
	var wg sync.WaitGroup
	results := make(chan Client, 12)
	errs := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); c, err := host.Ensure(context.Background(), item); results <- c; errs <- err }()
	}
	deadline := time.Now().Add(2 * time.Second)
	for dialer.Count() != 1 {
		if time.Now().After(deadline) {
			t.Fatal("dialer never started")
		}
		time.Sleep(time.Millisecond)
	}
	close(dialer.wait)
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var first Client
	for c := range results {
		if first == nil {
			first = c
		}
		if c != first {
			t.Fatal("concurrent ensure created multiple clients")
		}
	}
	if dialer.Count() != 1 {
		t.Fatalf("dialer starts = %d", dialer.Count())
	}
	if err := host.CloseAll(); err != nil {
		t.Fatal(err)
	}
	if first.(*fakeClient).closed.Load() != 1 {
		t.Fatal("process not closed exactly once")
	}
	if err := host.CloseAll(); err != nil {
		t.Fatal(err)
	}
}

func TestHostFailsClosedOnPackageVerificationError(t *testing.T) {
	t.Parallel()
	pkgs := &fakePackages{fail: errors.New("digest mismatch")}
	host, _ := NewSupervisor(pkgs)
	dialer := &fakeDialer{}
	if err := host.Register(platformplugin.ProviderTools, dialer); err != nil {
		t.Fatal(err)
	}
	if _, err := host.Ensure(context.Background(), installation(t, 7, platformplugin.ProviderTools)); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("verification not propagated: %v", err)
	}
	if dialer.Count() != 0 {
		t.Fatal("dialer called after verification failure")
	}
}
