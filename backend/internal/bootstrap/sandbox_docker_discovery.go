package bootstrap

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sandboxdocker "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/dockerengine"
)

const dockerDiscoveryInterval = 10 * time.Second

// StartDockerAutoDiscovery starts a single bounded and cancellable host-owned
// worker. Desktop invokes this once per Runtime. No HTTP/Tool API calls it.
// An explicit MENDER_DOCKER_SOCKET always takes precedence over discovery.
func (s *Sandboxes) StartDockerAutoDiscovery(ctx context.Context) error {
	if s == nil || s.store == nil || s.backendPlugins == nil {
		return sandboxdomain.ErrBackendUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if os.Getenv(DockerSocketEnvironment) != "" {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("docker auto discovery: home unavailable: %w", err)
	}
	candidates := sandboxdocker.KnownLocalEndpoints(home, runtime.GOOS, os.Getuid())
	return s.startDockerAutoDiscovery(ctx, candidates, dockerDiscoveryInterval)
}

// startDockerAutoDiscovery is isolated for deterministic no-daemon lifecycle
// tests; production candidates are derived only by the Desktop host path.
func (s *Sandboxes) startDockerAutoDiscovery(ctx context.Context, candidates []string, interval time.Duration) error {
	if len(candidates) == 0 || interval <= 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.dockerMu.Lock()
	if s.dockerClosed {
		s.dockerMu.Unlock()
		return sandboxdomain.ErrBackendUnavailable
	}
	if s.dockerAutoDone != nil || s.docker != nil {
		s.dockerMu.Unlock()
		return nil
	}
	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	s.dockerAutoCancel = cancel
	s.dockerAutoDone = done
	s.dockerUnavailableReason = "Docker Engine not detected"
	s.dockerMu.Unlock()

	go func() {
		defer close(done)
		runDockerDiscovery(workerCtx, interval, func(ctx context.Context) bool {
			for _, raw := range candidates {
				if ctx.Err() != nil {
					return false
				}
				endpoint, err := sandboxdocker.ParseEngineEndpoint(raw)
				if err != nil || endpoint.PreflightSocket() != nil {
					continue
				}
				if err := s.ActivateDocker(ctx, raw); err == nil {
					return true
				}
			}
			return false
		})
	}()
	return nil
}

// runDockerDiscovery never starts multiple probes concurrently. It retries
// only while unavailable and stops immediately on success or cancellation.
func runDockerDiscovery(ctx context.Context, interval time.Duration, discover func(context.Context) bool) {
	if interval <= 0 || discover == nil {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil || discover(ctx) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
