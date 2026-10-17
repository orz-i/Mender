package acp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	runtimedomain "github.com/orz-i/Mender/backend/internal/runtimegovernance/domain"
	processisolation "github.com/orz-i/Mender/backend/internal/runtimegovernance/infrastructure/processisolation"
)

// opencodeACPReviewConfig is a host-selected, task-scoped OpenCode v2
// inline overlay. It is deliberately fixed and NEVER derived from model Tool
// arguments. It makes routine native file and shell operations *ask* for a
// cooperative ACP owner decision. This DOES NOT confine a host-mode process;
// project-defined Agents/plugins or runtime-user authority can still bypass
// protocol-level review, so the UI must not advertise strong enforcement.
//
// This uses the V2 "permissions" ordered rules, not V1 "permission/bash/task".
const opencodeACPReviewConfig = `{"permissions":[
 {"action":"read","resource":"*","effect":"ask"},
 {"action":"edit","resource":"*","effect":"ask"},
 {"action":"shell","resource":"*","effect":"ask"},
 {"action":"external_directory","resource":"*","effect":"deny"},
 {"action":"subagent","resource":"*","effect":"deny"},
 {"action":"execute","resource":"*","effect":"deny"}
]}`

type LocalFactory struct{ launcher *processisolation.Launcher }

func NewLocalFactory(launcher *processisolation.Launcher) (*LocalFactory, error) {
	if launcher == nil {
		return nil, errors.New("agent process isolation launcher is required")
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
	case agentdomain.BackendOpenCode:
		return trustedBinary("opencode")
	case agentdomain.BackendCodex:
		if _, err := trustedBinary("codex"); err != nil {
			return "", fmt.Errorf("%w: Codex CLI is missing: %w", agentdomain.ErrUnavailable, err)
		}
		return trustedBinary("codex-acp")
	default:
		return "", agentdomain.ErrUnavailable
	}
}

func trustedBinary(name string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("%w: resolve local CLI home: %w", agentdomain.ErrUnavailable, err)
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
	return f.CommandInWorkspace(ctx, a, a.Spec().WorkspaceRoot)
}

// CommandInWorkspace only changes the cwd to a previously admitted canonical
// local root. Its caller owns the workspace selection and Profile authorization.
func (f *LocalFactory) CommandInWorkspace(ctx context.Context, a agentdomain.Agent, workspaceRoot string) (*exec.Cmd, error) {
	return f.commandInWorkspace(ctx, a, workspaceRoot, true)
}

// CommandWithConfig rejects task Tool supply to any unrestricted host Agent,
// independent of Grok/OpenCode/Codex backend names. The process is confined
// before receiving any Task HTTP MCP connection secret.
func (f *LocalFactory) CommandWithConfig(ctx context.Context, a agentdomain.Agent,
	cfg agentapplication.RunConfig) (*exec.Cmd, error) {
	if err := cfg.ToolSupply.Validate(); err != nil {
		return nil, err
	}
	if cfg.ToolSupply.IsZero() {
		return f.CommandInWorkspace(ctx, a, cfg.WorkspaceRoot)
	}
	if a.Spec().Isolation != "restricted_host" {
		return nil, fmt.Errorf("%w: governed Tool supply requires restricted_host", agentdomain.ErrUnavailable)
	}
	return f.commandInWorkspace(ctx, a, cfg.WorkspaceRoot, false)
}

func (f *LocalFactory) commandInWorkspace(ctx context.Context, a agentdomain.Agent, workspaceRoot string, rootWritable bool) (*exec.Cmd, error) {
	spec := a.Spec()
	executable, err := f.Availability(spec.Backend)
	if err != nil {
		return nil, err
	}
	root, err := filepath.EvalSymlinks(workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("agent workspace unavailable: %w", err)
	}
	if filepath.Clean(root) != workspaceRoot {
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
	case agentdomain.BackendOpenCode:
		arguments = []string{"acp"}
		// Owner-mediated ACP review is always attempted for native actions.
		// Strong filesystem boundaries require separately verified isolation.
		environment = append(environment, "OPENCODE_CONFIG_CONTENT="+opencodeACPReviewConfig)
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
		PermittedRoot: root, RootWritable: rootWritable, Environment: environment,
	})
}
