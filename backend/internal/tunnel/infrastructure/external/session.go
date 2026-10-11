package external

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	pluginprocess "github.com/orz-i/Mender/backend/internal/plugin/process"
	tunnelapplication "github.com/orz-i/Mender/backend/internal/tunnel/application"
	tunnelv1 "github.com/orz-i/Mender/sdk/tunnelplugin/go/protocol/v1"
)

const (
	healthInterval    = 400 * time.Millisecond
	healthTimeout     = 2 * time.Second
	disconnectTimeout = 5 * time.Second
)

type Session struct {
	process        *Process
	service        tunnelv1.TunnelTransportServiceClient
	sessionID      string
	host           *pluginprocess.Supervisor
	installationID platformplugin.InstallationID

	mu        sync.Mutex
	health    string
	done      chan error
	doneOnce  sync.Once
	stopOnce  sync.Once
	stopError error
}

// newSession is called only after the host accepted the exact session
// identifier and a validated evidence-based initial phase.
func newSession(process *Process, id string, phase string,
	host *pluginprocess.Supervisor, installationID platformplugin.InstallationID) *Session {
	s := &Session{process: process, service: process.Service(), sessionID: id,
		host: host, installationID: installationID, health: phase, done: make(chan error, 1)}
	go s.watch()
	return s
}
func (s *Session) TunnelHealth() string {
	if s == nil {
		return "failed"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.health
}
func (s *Session) setHealth(value string) {
	s.mu.Lock()
	s.health = value
	s.mu.Unlock()
}
func (s *Session) Done() <-chan error {
	if s == nil {
		d := make(chan error)
		close(d)
		return d
	}
	return s.done
}
func (s *Session) finish(err error) {
	s.doneOnce.Do(func() {
		s.setHealth("exited")
		if err != nil {
			s.done <- err
		}
		close(s.done)
	})
}
func validatePhase(phase string) error {
	switch phase {
	case "starting", "ready", "degraded", "failed":
		return nil
	default:
		return fmt.Errorf("tunnel transport reported invalid health phase %q", phase)
	}
}
func (s *Session) watch() {
	ticker := time.NewTicker(healthInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			if s.process.Exited() {
				s.finish(fmt.Errorf("tunnel transport process exited"))
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), healthTimeout)
			status, err := s.service.Status(ctx, &tunnelv1.StatusRequest{SessionId: s.sessionID})
			cancel()
			if err != nil {
				s.setHealth("degraded")
				continue
			}
			if err := validatePhase(status.GetPhase()); err != nil {
				s.finish(err)
				return
			}
			s.setHealth(status.GetPhase())
			if status.GetPhase() == "failed" {
				s.finish(fmt.Errorf("tunnel transport reported failed"))
				return
			}
		}
	}
}

// Stop is per-Tunnel session, idempotent and bounded. If a native disconnect
// RPC fails, the host kills its exact plugin installation process so an
// orphaned child cannot continue exposing the old Profile endpoint.
func (s *Session) Stop(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.stopOnce.Do(func() {
		if s.process.Exited() {
			// A dead native transport no longer owns a live session. The
			// managed MCP listener must still be able to close cleanly.
			s.finish(nil)
			return
		}
		stopCtx := ctx
		cancel := func() {}
		if _, hasDeadline := ctx.Deadline(); !hasDeadline {
			stopCtx, cancel = context.WithTimeout(ctx, disconnectTimeout)
		}
		defer cancel()
		_, err := s.service.Disconnect(stopCtx, &tunnelv1.DisconnectRequest{SessionId: s.sessionID})
		if err != nil {
			closeErr := s.host.CloseInstallation(s.installationID)
			s.stopError = errors.Join(fmt.Errorf("tunnel transport disconnect: %w", err), closeErr)
		}
		s.finish(err)
	})
	return s.stopError
}

var _ tunnelapplication.Process = (*Session)(nil)
var _ tunnelapplication.HealthReporter = (*Session)(nil)
