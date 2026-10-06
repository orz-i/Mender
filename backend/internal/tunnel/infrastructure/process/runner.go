package process

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

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

func (r Runner) Start(
	ctx context.Context,
	tunnel tunneldomain.Tunnel,
	endpoint string,
	apiKey []byte,
) (tunnelapplication.Process, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("run tunnel client: MCP endpoint must not be empty")
	}
	if len(apiKey) == 0 {
		return nil, fmt.Errorf("run tunnel client: runtime API key must not be empty")
	}
	if r.launcher == nil {
		return nil, fmt.Errorf("run tunnel client: command launcher is unavailable")
	}

	command, err := r.launcher.Command(
		ctx,
		[]string{
			"run",
			"--control-plane.tunnel-id=" + tunnel.ExternalTunnelID(),
			"--control-plane.api-key=env:" + controlPlaneAPIKeyEnvironment,
			"--mcp.server-url=" + endpoint,
			"--health.listen-addr=127.0.0.1:0",
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

	if err := command.Start(); err != nil {
		command.Env = nil
		return nil, fmt.Errorf("run tunnel client: start: %w", err)
	}
	// The environment has already been copied into the child by Start. Do not
	// retain the API key on the parent-owned exec.Cmd for the process lifetime.
	command.Env = nil

	child := &childProcess{
		command: command,
		done:    make(chan error, 1),
	}
	go child.wait()
	return child, nil
}

type childProcess struct {
	command *exec.Cmd
	done    chan error

	stopOnce sync.Once
}

func (p *childProcess) wait() {
	err := p.command.Wait()
	p.done <- err
	close(p.done)
}

func (p *childProcess) Done() <-chan error {
	return p.done
}

func (p *childProcess) Stop(ctx context.Context) error {
	var killErr error
	p.stopOnce.Do(func() {
		if p.command.Process == nil {
			return
		}
		killErr = p.command.Process.Kill()
		if errors.Is(killErr, os.ErrProcessDone) {
			killErr = nil
		}
	})
	if killErr != nil {
		return fmt.Errorf("stop tunnel client: %w", killErr)
	}

	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("stop tunnel client: %w", ctx.Err())
	}
}

var _ tunnelapplication.ProcessRunner = Runner{}
