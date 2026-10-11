package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"time"

	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sandboxdocker "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/dockerengine"
)

const dockerDiscoveryInterval = 10 * time.Second

// StartDockerAutoDiscovery starts one bounded native-Desktop Engine monitor.
// It first discovers trusted fixed local sockets, then continually verifies
// the *same* pinned Engine; it never probes another candidate after binding.
// An explicit host-configured MENDER_DOCKER_SOCKET takes precedence and is
// monitored in place (never replaced by a discovery candidate).
func (s *Sandboxes) StartDockerAutoDiscovery(ctx context.Context) error {
	if s == nil || s.store == nil || s.backendPlugins == nil {
		return sandboxdomain.ErrBackendUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if os.Getenv(DockerSocketEnvironment) != "" {
		return s.startDockerAutoDiscovery(ctx, nil, dockerDiscoveryInterval)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("docker auto discovery: home unavailable: %w", err)
	}
	candidates := sandboxdocker.KnownLocalEndpoints(home, runtime.GOOS, os.Getuid())
	return s.startDockerAutoDiscovery(ctx, candidates, dockerDiscoveryInterval)
}

// startDockerAutoDiscovery accepts a known, host-owned candidate set. The
// injection point is package-private for deterministic no-daemon tests.
func (s *Sandboxes) startDockerAutoDiscovery(ctx context.Context, candidates []string, interval time.Duration) error {
	if s == nil || s.store == nil || s.backendPlugins == nil {
		return sandboxdomain.ErrBackendUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.dockerMu.Lock()
	defer s.dockerMu.Unlock()
	if s.dockerClosed {
		return sandboxdomain.ErrBackendUnavailable
	}
	if s.dockerAutoDone != nil || interval <= 0 || (len(candidates) == 0 && s.docker == nil) {
		return nil
	}
	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	s.dockerAutoCancel = cancel
	s.dockerAutoDone = done
	if s.docker == nil {
		s.dockerUnavailableReason = "Docker Engine not detected"
	}
	go func() {
		defer close(done)
		runDockerMonitor(workerCtx, interval, func(ctx context.Context) {
			if s.refreshDockerHealth(ctx) {
				return // Never switch to a different Engine after registration.
			}
			for _, raw := range candidates {
				if ctx.Err() != nil {
					return
				}
				endpoint, err := sandboxdocker.ParseEngineEndpoint(raw)
				if err != nil || endpoint.PreflightSocket() != nil {
					continue
				}
				if err := s.ActivateDocker(ctx, raw); err == nil {
					return // Next tick monitors the newly pinned Engine.
				}
			}
		})
	}()
	return nil
}

// refreshDockerHealth checks only the already bound Controller. It never
// re-registers backends or changes the persisted Sandbox identity or Run.
func (s *Sandboxes) refreshDockerHealth(ctx context.Context) bool {
	s.dockerMu.RLock()
	controller := s.docker
	closed := s.dockerClosed
	s.dockerMu.RUnlock()
	if controller == nil || closed {
		return false
	}
	_, err := controller.Probe(ctx)
	s.updateDockerHealth(controller, err)
	return true
}

// updateDockerHealth cannot publish an old probe after shutdown or replace
// the status of a different Controller. All management reads remain inert.
func (s *Sandboxes) updateDockerHealth(controller *sandboxdocker.Provisioner, err error) {
	s.dockerMu.Lock()
	defer s.dockerMu.Unlock()
	if s.dockerClosed || controller == nil || s.docker != controller {
		return
	}
	s.dockerReady = err == nil
	switch {
	case err == nil:
		s.dockerUnavailableReason = ""
	case errors.Is(err, sandboxdocker.ErrEngineIdentityChanged):
		s.dockerUnavailableReason = "Docker Engine identity changed; restart Mender to review"
	default:
		s.dockerUnavailableReason = "Docker Engine disconnected or insecure; retrying"
	}
}

// runDockerMonitor uses a single serial, bounded cadence. On successful first
// discovery it remains alive to notice outages and same-ID daemon recovery.
// A read-only Marketplace/API lookup never invokes it or initiates probes.
func runDockerMonitor(ctx context.Context, interval time.Duration, step func(context.Context)) {
	if interval <= 0 || step == nil {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		step(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
