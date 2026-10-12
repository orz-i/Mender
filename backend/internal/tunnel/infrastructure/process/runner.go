package process

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"sync"
	"time"

	tunnelapplication "github.com/orz-i/Mender/backend/internal/tunnel/application"
	tunneldomain "github.com/orz-i/Mender/backend/internal/tunnel/domain"
)

const controlPlaneAPIKeyEnvironment = "CONTROL_PLANE_API_KEY"

type CommandLauncher interface {
	Command(context.Context, []string, []string) (*exec.Cmd, error)
}

type Runner struct {
	launcher CommandLauncher
}

func NewRunner(launcher CommandLauncher) Runner {
	return Runner{launcher: launcher}
}

func (r Runner) Preflight(_ context.Context, tunnel tunneldomain.Tunnel) error {
	if r.launcher == nil {
		return fmt.Errorf("openai Tunnel launcher unavailable")
	}
	return tunnel.Binding().ValidateProvider(tunneldomain.ProviderOpenAI)
}

// reserveNativeHealthAddress chooses an OS-assigned LOOPBACK port so the host
// can observe the exact daemon's health without reading private child logs.
// A health URL file is deliberately not used: Linux restricted_host has its
// own /tmp mount and a child-written URL file may not be visible to the host.
func reserveNativeHealthAddress() (net.Listener, string, error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, "", fmt.Errorf("reserve native Tunnel health address: %w", err)
	}
	return listener, listener.Addr().String(), nil
}

func (r Runner) Start(
	ctx context.Context,
	tunnel tunneldomain.Tunnel,
	endpoint string,
	apiKey []byte,
) (tunnelapplication.Process, error) {
	if err := r.Preflight(ctx, tunnel); err != nil {
		return nil, err
	}
	if err := tunnelapplication.ValidateManagedMCPEndpoint(endpoint); err != nil {
		return nil, err
	}
	if len(apiKey) == 0 {
		return nil, fmt.Errorf("run tunnel client: runtime API key must not be empty")
	}
	if r.launcher == nil {
		return nil, fmt.Errorf("run tunnel client: command launcher is unavailable")
	}
	reservation, healthAddress, err := reserveNativeHealthAddress()
	if err != nil {
		return nil, err
	}
	defer func() { _ = reservation.Close() }()

	command, err := r.launcher.Command(
		ctx,
		[]string{
			"run",
			"--control-plane.tunnel-id=" + tunnel.ExternalTunnelID(),
			"--control-plane.api-key=env:" + controlPlaneAPIKeyEnvironment,
			"--mcp.server-url=" + endpoint,
			"--health.listen-addr=" + healthAddress,
		},
		[]string{controlPlaneAPIKeyEnvironment + "=" + string(apiKey)},
	)
	if err != nil {
		return nil, fmt.Errorf("run tunnel client: construct command: %w", err)
	}
	if command == nil {
		return nil, fmt.Errorf("run tunnel client: command launcher returned nil command")
	}
	command.Stdout = io.Discard
	command.Stderr = io.Discard

	// Release the reserved port immediately before exec. A rare bind race or
	// native listen failure leads to a missing health report / child exit,
	// NEVER a fabricated ready status.
	if err := reservation.Close(); err != nil {
		command.Env = nil
		return nil, fmt.Errorf("run tunnel client: release health port: %w", err)
	}
	if err := command.Start(); err != nil {
		command.Env = nil
		return nil, fmt.Errorf("run tunnel client: start: %w", err)
	}
	// The environment was copied into the child by Start. Do not retain the
	// API key on the parent-owned exec.Cmd for the process lifetime.
	command.Env = nil

	child := &childProcess{
		command:       command,
		done:          make(chan error, 1),
		exited:        make(chan struct{}),
		health:        "starting",
		healthAddress: healthAddress,
		startedAt:     time.Now(),
	}
	go child.wait()
	go child.watchHealth()
	return child, nil
}

type childProcess struct {
	command *exec.Cmd
	done    chan error
	exited  chan struct{}

	stopOnce      sync.Once
	healthMu      sync.RWMutex
	health        string
	healthAddress string
	startedAt     time.Time
	exitedFlag    bool
}

func (p *childProcess) wait() {
	err := p.command.Wait()
	p.healthMu.Lock()
	p.exitedFlag = true
	p.health = "exited"
	p.healthMu.Unlock()
	close(p.exited)
	p.done <- err
	close(p.done)
}

func (p *childProcess) Done() <-chan error { return p.done }

// TunnelHealth is a cached, side-effect-free read. Background health checks
// alone interact with the loopback child; management reads never poll.
func (p *childProcess) TunnelHealth() string {
	p.healthMu.RLock()
	defer p.healthMu.RUnlock()
	return p.health
}
func (p *childProcess) setHealth(status string) {
	p.healthMu.Lock()
	if !p.exitedFlag {
		p.health = status
	}
	p.healthMu.Unlock()
}

func (p *childProcess) Stop(ctx context.Context) error {
	var killErr error
	p.stopOnce.Do(func() {
		if p.command.Process == nil {
			return
		}
		// The Runtime isolation launcher installs an owned process-group
		// cancellation function. Use it rather than killing only the leader.
		if p.command.Cancel != nil {
			killErr = p.command.Cancel()
		} else {
			killErr = p.command.Process.Kill()
		}
		if errors.Is(killErr, os.ErrProcessDone) {
			killErr = nil
		}
	})
	if killErr != nil {
		return fmt.Errorf("stop tunnel client: %w", killErr)
	}

	select {
	case <-p.exited:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("stop tunnel client: %w", ctx.Err())
	}
}

var _ tunnelapplication.ProcessRunner = Runner{}
var _ tunnelapplication.HealthReporter = (*childProcess)(nil)
