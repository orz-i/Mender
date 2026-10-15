package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	runtimegovernancedomain "github.com/orz-i/Mender/backend/internal/runtimegovernance/domain"
)

func TestTunnelClientIsolationLauncherResolvesHostOwnedExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink-based executable resolution assertion targets Unix")
	}
	ctx := context.Background()
	bin := t.TempDir()
	executable := filepath.Join(bin, "tunnel-client")
	if err := os.Symlink("/usr/bin/true", executable); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	governance, err := NewRuntimeGovernance(ctx, filepath.Join(t.TempDir(), "mender.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = governance.Close() }()
	if _, err := governance.UpdateIsolation(ctx, runtimegovernancedomain.IsolationSpec{
		WorkspaceShell:   runtimegovernancedomain.IsolationHost,
		ExternalProvider: runtimegovernancedomain.IsolationHost,
		TunnelClient:     runtimegovernancedomain.IsolationHost,
	}); err != nil {
		t.Fatal(err)
	}

	launcher := tunnelClientIsolationLauncher{governance: governance}
	environment := []string{"CONTROL_PLANE_API_KEY=runtime-secret"}
	command, err := launcher.Command(ctx, []string{"run"}, environment)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(executable)
	if err != nil {
		t.Fatal(err)
	}
	if command.Path != canonical {
		t.Fatalf("command path = %q, want %q", command.Path, canonical)
	}
	if command.Dir != filepath.Dir(canonical) {
		t.Fatalf("command dir = %q, want %q", command.Dir, filepath.Dir(canonical))
	}
	if !slices.Equal(command.Args[1:], []string{"run"}) {
		t.Fatalf("command args = %#v", command.Args)
	}
	if !slices.Equal(command.Env, environment) {
		t.Fatalf("command environment = %#v", command.Env)
	}
}

func TestTunnelClientIsolationLauncherFailsWhenExecutableMissing(t *testing.T) {
	ctx := context.Background()
	t.Setenv("PATH", t.TempDir())
	governance, err := NewRuntimeGovernance(ctx, filepath.Join(t.TempDir(), "mender.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = governance.Close() }()

	launcher := tunnelClientIsolationLauncher{governance: governance}
	if _, err := launcher.Command(ctx, nil, []string{"CONTROL_PLANE_API_KEY=runtime-secret"}); err == nil {
		t.Fatal("Command() error = nil for missing tunnel-client")
	}
}

// Native Provider children must be wrapped by the platform's verified
// restriction when a fresh Runtime elects restricted_host. Do not silently
// execute the underlying binary with full host authority.
func TestFreshExternalProviderUsesVerifiedRestrictedHostLauncher(t *testing.T) {
	ctx := context.Background()
	governance, err := NewRuntimeGovernance(ctx, filepath.Join(t.TempDir(), "mender.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = governance.Close() }()
	if !governance.IsolationBackend().RestrictedHostAvailable {
		t.Skip("no verified restricted host backend")
	}
	isolation, err := governance.GetIsolation.Execute(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if isolation.ExternalProvider() != runtimegovernancedomain.IsolationRestrictedHost {
		t.Fatalf("fresh native Provider isolation = %s", isolation.ExternalProvider())
	}
	root := t.TempDir()
	cmd, err := (externalProviderIsolationLauncher{governance: governance}).Command(ctx, "/bin/true", root)
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Path == "/bin/true" {
		t.Fatal("restricted external Provider bypassed the isolation wrapper")
	}
	if len(cmd.Args) == 0 || cmd.Args[0] == "/bin/true" {
		t.Fatalf("unexpected unrestricted command: %q", cmd.Args)
	}
}
