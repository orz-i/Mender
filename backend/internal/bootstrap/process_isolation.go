package bootstrap

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"

	runtimegovernancedomain "github.com/orz-i/Mender/backend/internal/runtimegovernance/domain"
	runtimegovernance "github.com/orz-i/Mender/backend/internal/runtimegovernance/infrastructure/processisolation"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

type workspaceShellIsolationLauncher struct {
	governance *RuntimeGovernance
}

func (a workspaceShellIsolationLauncher) Command(
	ctx context.Context,
	root string,
	workingDirectory string,
	command string,
	environment []string,
) (*exec.Cmd, error) {
	if a.governance == nil || a.governance.processIsolation == nil {
		return nil, fmt.Errorf("workspace shell isolation launcher is unavailable")
	}
	isolation, err := a.governance.GetIsolation.Execute(ctx)
	if err != nil {
		return nil, fmt.Errorf("workspace shell isolation: load settings: %w", err)
	}
	executable, arguments, err := workspaceShellProcess(command)
	if err != nil {
		return nil, err
	}
	return a.governance.processIsolation.Command(
		ctx,
		isolation.WorkspaceShell(),
		runtimegovernance.CommandSpec{
			Executable:       executable,
			Arguments:        arguments,
			WorkingDirectory: workingDirectory,
			PermittedRoot:    root,
			RootWritable:     true,
			Environment:      environment,
		},
	)
}

type externalProviderIsolationLauncher struct {
	governance *RuntimeGovernance
}

func (a externalProviderIsolationLauncher) Command(
	ctx context.Context,
	executable string,
	workingDirectory string,
) (*exec.Cmd, error) {
	if a.governance == nil || a.governance.processIsolation == nil {
		return nil, fmt.Errorf("external Provider isolation launcher is unavailable")
	}
	isolation, err := a.governance.GetIsolation.Execute(ctx)
	if err != nil {
		return nil, fmt.Errorf("external Provider isolation: load settings: %w", err)
	}
	return a.governance.processIsolation.Command(
		ctx,
		isolation.ExternalProvider(),
		runtimegovernance.CommandSpec{
			Executable:       executable,
			WorkingDirectory: workingDirectory,
			PermittedRoot:    workingDirectory,
			RootWritable:     false,
		},
	)
}

type tunnelClientIsolationLauncher struct {
	governance *RuntimeGovernance
}

func (a tunnelClientIsolationLauncher) Command(
	ctx context.Context,
	arguments []string,
	environment []string,
) (*exec.Cmd, error) {
	if a.governance == nil || a.governance.processIsolation == nil {
		return nil, fmt.Errorf("tunnel client isolation launcher is unavailable")
	}
	resolved, err := exec.LookPath("tunnel-client")
	if err != nil {
		return nil, fmt.Errorf("resolve tunnel-client executable: %w", err)
	}
	absolute, err := filepath.Abs(resolved)
	if err != nil {
		return nil, fmt.Errorf("resolve tunnel-client absolute path: %w", err)
	}
	executable, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, fmt.Errorf("resolve tunnel-client canonical path: %w", err)
	}
	root := filepath.Dir(executable)
	isolation, err := a.governance.GetIsolation.Execute(ctx)
	if err != nil {
		return nil, fmt.Errorf("tunnel client isolation: load settings: %w", err)
	}
	return a.governance.processIsolation.Command(
		ctx,
		isolation.TunnelClient(),
		runtimegovernance.CommandSpec{
			Executable:       executable,
			Arguments:        arguments,
			WorkingDirectory: root,
			PermittedRoot:    root,
			RootWritable:     false,
			Environment:      environment,
		},
	)
}

// sandboxWorkspaceShellLauncher uses the persisted Sandbox isolation selection;
// it never reads the mutable Runtime-wide default and never downgrades a
// requested restricted_host to host when the sandbox backend is unavailable.
type sandboxWorkspaceShellLauncher struct{ governance *RuntimeGovernance }

func (a sandboxWorkspaceShellLauncher) Command(ctx context.Context, mode sandboxdomain.IsolationClass, root, workingDirectory, command string, environment []string) (*exec.Cmd, error) {
	if a.governance == nil || a.governance.processIsolation == nil {
		return nil, fmt.Errorf("sandbox workspace shell isolation launcher unavailable")
	}
	var isolation runtimegovernancedomain.IsolationMode
	switch mode {
	case sandboxdomain.IsolationHost:
		isolation = runtimegovernancedomain.IsolationHost
	case sandboxdomain.IsolationRestrictedHost:
		isolation = runtimegovernancedomain.IsolationRestrictedHost
	default:
		return nil, fmt.Errorf("sandbox workspace shell: invalid isolation %q", mode)
	}
	executable, arguments, err := workspaceShellProcess(command)
	if err != nil {
		return nil, err
	}
	return a.governance.processIsolation.Command(ctx, isolation, runtimegovernance.CommandSpec{Executable: executable, Arguments: arguments, WorkingDirectory: workingDirectory, PermittedRoot: root, RootWritable: true, Environment: environment})
}
