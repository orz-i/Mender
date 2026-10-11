package process

import (
	"context"
	"errors"
	"os/exec"
	"slices"
	"testing"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	tunneldomain "github.com/orz-i/Mender/backend/internal/tunnel/domain"
)

func TestRunnerUsesLauncherWithBoundedEnvironment(t *testing.T) {
	t.Setenv("MENDER_UNRELATED_SECRET", "must-not-inherit")
	ctx := context.Background()
	launcher := &captureLauncher{}
	runner := NewRunner(launcher)
	tunnel := testTunnel(t)

	process, err := runner.Start(ctx, tunnel, "http://127.0.0.1:1234/mcp", []byte("runtime-secret"))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(launcher.environment, []string{"CONTROL_PLANE_API_KEY=runtime-secret"}) {
		t.Fatalf("launcher environment = %#v", launcher.environment)
	}
	if slices.Contains(launcher.environment, "MENDER_UNRELATED_SECRET=must-not-inherit") {
		t.Fatal("runner inherited unrelated host environment")
	}
	wantArguments := []string{
		"run",
		"--control-plane.tunnel-id=tunnel_external_test",
		"--control-plane.api-key=env:CONTROL_PLANE_API_KEY",
		"--mcp.server-url=http://127.0.0.1:1234/mcp",
		"--health.listen-addr=127.0.0.1:0",
	}
	if !slices.Equal(launcher.arguments, wantArguments) {
		t.Fatalf("launcher arguments = %#v", launcher.arguments)
	}
	child, ok := process.(*childProcess)
	if !ok {
		t.Fatalf("process type = %T", process)
	}
	if child.command.Env != nil {
		t.Fatalf("parent command retained secret environment: %#v", child.command.Env)
	}
	if err := <-process.Done(); err != nil {
		t.Fatalf("child exit = %v", err)
	}
}

func TestRunnerFailsWhenLauncherFails(t *testing.T) {
	runner := NewRunner(&captureLauncher{err: errors.New("launcher denied")})
	if _, err := runner.Start(
		context.Background(),
		testTunnel(t),
		"http://127.0.0.1:1234/mcp",
		[]byte("runtime-secret"),
	); err == nil {
		t.Fatal("Start() error = nil")
	}
}

type captureLauncher struct {
	arguments   []string
	environment []string
	err         error
}

func (l *captureLauncher) Command(
	ctx context.Context,
	arguments []string,
	environment []string,
) (*exec.Cmd, error) {
	l.arguments = append([]string(nil), arguments...)
	l.environment = append([]string(nil), environment...)
	if l.err != nil {
		return nil, l.err
	}
	command := exec.CommandContext(ctx, "go", "version") // #nosec G204 -- fixed test command.
	command.Env = append([]string(nil), environment...)
	return command, nil
}

func testTunnel(t *testing.T) tunneldomain.Tunnel {
	t.Helper()
	id, err := tunneldomain.ParseTunnelID("tun_00000000000000000000000000000001")
	if err != nil {
		t.Fatal(err)
	}
	ref, err := credentialdomain.NewReference("openai", "tunnel-runtime")
	if err != nil {
		t.Fatal(err)
	}
	tunnel, err := tunneldomain.NewTunnel(
		id,
		"OpenAI Test",
		tunneldomain.ProviderOpenAI,
		tunneldomain.OpenAIBinding(),
		"tunnel_external_test",
		ref,
	)
	if err != nil {
		t.Fatal(err)
	}
	return tunnel
}
