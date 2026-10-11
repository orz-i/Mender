package bootstrap

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestDockerDiscoveryRetriesUntilEngineReady(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var count atomic.Int32
	done := make(chan struct{})
	go func() {
		runDockerDiscovery(ctx, 5*time.Millisecond, func(context.Context) bool {
			return count.Add(1) >= 3
		})
		close(done)
	}()
	select {
	case <-done:
		if count.Load() != 3 {
			t.Fatalf("unexpected probes: %d", count.Load())
		}
	case <-ctx.Done():
		t.Fatal("Docker discovery failed to retry and stop")
	}
}

func TestDockerDiscoveryCancelledOnShutdown(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan struct{})
	done := make(chan struct{})
	var count atomic.Int32
	go func() {
		runDockerDiscovery(ctx, 5*time.Millisecond, func(context.Context) bool {
			count.Add(1)
			select {
			case <-first:
			default:
				close(first)
			}
			return false
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

func TestDockerDiscoveryStopsOnCancelledContextBeforeInitialProbe(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	count := 0
	runDockerDiscovery(ctx, time.Millisecond, func(context.Context) bool { count++; return true })
	if count != 0 {
		t.Fatal("probe executed after shutdown")
	}
}
