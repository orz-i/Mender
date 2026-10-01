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

type Runner struct{}

func (Runner) Start(
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

	command := exec.Command(
		tunnel.Executable(),
		"run",
		"--control-plane.tunnel-id="+tunnel.ExternalTunnelID(),
		"--control-plane.api-key=env:CONTROL_PLANE_API_KEY",
		"--mcp.server-url="+endpoint,
		"--health.listen-addr=127.0.0.1:0",
	)
	command.Env = append(os.Environ(), "CONTROL_PLANE_API_KEY="+string(apiKey))
	command.Stdout = io.Discard
	command.Stderr = io.Discard

	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("run tunnel client: start: %w", err)
	}

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
