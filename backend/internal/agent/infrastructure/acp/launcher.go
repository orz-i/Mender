package acp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	runtimedomain "github.com/orz-i/Mender/backend/internal/runtimegovernance/domain"
	processisolation "github.com/orz-i/Mender/backend/internal/runtimegovernance/infrastructure/processisolation"
)

type LocalFactory struct{ launcher *processisolation.Launcher }

func NewLocalFactory(launcher *processisolation.Launcher) (*LocalFactory, error) {
	if launcher == nil {
		return nil, errors.New("Agent process isolation launcher is required")
	}
	return &LocalFactory{launcher: launcher}, nil
}

// Availability is an inert filesystem-only read. No CLI starts, no account
// login is attempted, and no npm/npx auto-download may occur.
func (f *LocalFactory) Availability(backend agentdomain.Backend) (string, error) {
	if f == nil || f.launcher == nil {
		return "", agentdomain.ErrUnavailable
	}
	switch backend {
	case agentdomain.BackendGrok:
		return trustedBinary("grok")
	case agentdomain.BackendCodex:
		if _, err := trustedBinary("codex"); err != nil {
			return "", fmt.Errorf("%w: Codex CLI is missing: %v", agentdomain.ErrUnavailable, err)
		}
		return trustedBinary("codex-acp")
	default:
		return "", agentdomain.ErrUnavailable
	}
}

func trustedBinary(name string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("%w: resolve local CLI home: %v", agentdomain.ErrUnavailable, err)
	}
	// The executable is selected by this host-owned allowlist, *not* from
	// inherited PATH or user-/model-supplied command-line arguments.
	for _, dir := range []string{"/opt/homebrew/bin", "/usr/local/bin", filepath.Join(home, ".local", "bin"),
		filepath.Join(home, ".npm-global", "bin"), "/usr/bin"} {
		candidate := filepath.Join(dir, name)
		canonical, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			continue
		}
		absolute, err := filepath.Abs(canonical)
		if err != nil {
			continue
		}
		info, err := os.Stat(absolute)
		if err == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0 {
			return absolute, nil
		}
	}
	return "", fmt.Errorf("%w: %s not found in approved executable locations", agentdomain.ErrUnavailable, name)
}

// Command returns a governed process plan, not a runnable shell string.
// Agent-owned flags, cwd, environment and executable are never Tool arguments.
func (f *LocalFactory) Command(ctx context.Context, a agentdomain.Agent) (*exec.Cmd, error) {
	spec := a.Spec()
	executable, err := f.Availability(spec.Backend)
	if err != nil {
		return nil, err
	}
	root, err := filepath.EvalSymlinks(spec.WorkspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("Agent workspace unavailable: %w", err)
	}
	if filepath.Clean(root) != spec.WorkspaceRoot {
		return nil, fmt.Errorf("%w: Agent workspace root changed", agentdomain.ErrUnavailable)
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, agentdomain.ErrUnavailable
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	isolation := runtimedomain.IsolationMode(spec.Isolation)
	arguments := []string{}
	environment := []string{"PATH=/usr/bin:/bin:/opt/homebrew/bin", "HOME=" + home,
		"TMPDIR=" + os.TempDir(), "NO_BROWSER=1", "CI=1"}
	switch spec.Backend {
	case agentdomain.BackendGrok:
		arguments = []string{"agent", "stdio"}
	case agentdomain.BackendCodex:
		codex, err := trustedBinary("codex")
		if err != nil {
			return nil, err
		}
		environment = append(environment, "CODEX_PATH="+codex)
	default:
		return nil, agentdomain.ErrUnavailable
	}
	return f.launcher.Command(ctx, isolation, processisolation.CommandSpec{
		Executable: executable, Arguments: arguments, WorkingDirectory: root,
		PermittedRoot: root, RootWritable: true, Environment: environment,
	})
}
