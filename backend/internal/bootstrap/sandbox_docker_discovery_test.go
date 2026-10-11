package bootstrap

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	sandboxdocker "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/dockerengine"
)

func TestDockerMonitorContinuesAfterInitialConnectionUntilCancelled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var count atomic.Int32
	done := make(chan struct{})
	go func() {
		runDockerMonitor(ctx, 5*time.Millisecond, func(context.Context) {
			if count.Add(1) >= 4 {
				cancel()
			}
		})
		close(done)
	}()
	select {
	case <-done:
		if count.Load() != 4 {
			t.Fatalf("monitor stopped prematurely or ignored cancellation: %d", count.Load())
		}
	case <-time.After(time.Second):
		t.Fatal("Docker monitor did not complete")
	}
}

func TestDockerDiscoveryCancelledOnShutdown(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan struct{})
	done := make(chan struct{})
	var count atomic.Int32
	go func() {
		runDockerMonitor(ctx, 5*time.Millisecond, func(context.Context) {
			count.Add(1)
			select {
			case <-first:
			default:
				close(first)
			}
		})
		close(done)
	}()
	select {
	case <-first:
	case <-time.After(time.Second):
		t.Fatal("initial Docker probe never happened")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Docker discovery worker leaked after cancellation")
	}
	calls := count.Load()
	time.Sleep(15 * time.Millisecond)
	if count.Load() != calls {
		t.Fatal("Docker discovery continued after shutdown")
	}
}

func TestDockerAutoDiscoverySingleWorkerAndRuntimeShutdown(t *testing.T) {
	t.Setenv(DockerSocketEnvironment, "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "mender.db")})
	if err != nil {
		t.Fatal(err)
	}
	socket := "unix://" + filepath.Join(t.TempDir(), "nonexistent", "docker.sock")
	if err := r.Sandboxes.startDockerAutoDiscovery(ctx, []string{socket}, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	first := r.Sandboxes.dockerAutoDone
	if first == nil {
		t.Fatal("worker not started")
	}
	if err := r.Sandboxes.startDockerAutoDiscovery(ctx, []string{socket}, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if first != r.Sandboxes.dockerAutoDone {
		t.Fatal("duplicate Docker discovery worker")
	}
	if r.Sandboxes.DockerAvailable() {
		t.Fatal("unavailable test daemon connected")
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-first:
	default:
		t.Fatal("worker still running after runtime close")
	}
	if err := r.Sandboxes.startDockerAutoDiscovery(ctx, []string{socket}, time.Millisecond); err == nil {
		t.Fatal("closed Runtime accepted new Docker discovery worker")
	}
}

func TestDockerHealthFailClosedAndSameIdentityRecovery(t *testing.T) {
	t.Parallel()
	controller := &sandboxdocker.Provisioner{}
	s := &Sandboxes{
		docker: controller, dockerTemplates: &sandboxdocker.TemplateStore{},
		dockerReady: true,
	}
	if !s.DockerAvailable() || s.DockerStatus() != "ready" {
		t.Fatal("ready Engine not reported")
	}
	s.updateDockerHealth(controller, errors.New("socket disconnected"))
	if s.DockerAvailable() || s.DockerStatus() != "Docker Engine disconnected or insecure; retrying" {
		t.Fatalf("unhealthy Engine still exposed: %q", s.DockerStatus())
	}
	s.updateDockerHealth(controller, errors.Join(sandboxdocker.ErrEngineIdentityChanged, errors.New("untrusted daemon")))
	if s.DockerAvailable() || s.DockerStatus() != "Docker Engine identity changed; restart Mender to review" {
		t.Fatalf("Engine drift accepted: %q", s.DockerStatus())
	}
	s.updateDockerHealth(controller, nil)
	if !s.DockerAvailable() || s.DockerStatus() != "ready" {
		t.Fatalf("same verified Engine did not recover: %q", s.DockerStatus())
	}
	s.updateDockerHealth(&sandboxdocker.Provisioner{}, errors.New("stale controller"))
	if !s.DockerAvailable() {
		t.Fatal("stale Controller overwrote healthy Engine")
	}
	s.dockerClosed = true
	s.updateDockerHealth(controller, nil)
	if s.DockerAvailable() {
		t.Fatal("closed Runtime still exposes Docker")
	}
}

func TestDockerDiscoveryStopsOnCancelledContextBeforeInitialProbe(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	count := 0
	runDockerMonitor(ctx, time.Millisecond, func(context.Context) { count++ })
	if count != 0 {
		t.Fatal("probe executed after shutdown")
	}
}
