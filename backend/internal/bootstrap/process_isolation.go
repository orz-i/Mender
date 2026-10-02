package bootstrap

import (
	"context"
	"fmt"
	"os/exec"

	runtimegovernance "github.com/orz-i/Mender/backend/internal/runtimegovernance/infrastructure/processisolation"
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
