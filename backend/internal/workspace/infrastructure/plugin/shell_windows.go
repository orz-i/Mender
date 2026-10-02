//go:build windows

package plugin

import (
	"context"
	"os/exec"
	"time"
)

func (HostShellLauncher) Command(
	ctx context.Context,
	_ string,
	workingDirectory string,
	command string,
	environment []string,
) (*exec.Cmd, error) {
	//nolint:gosec // workspace.shell intentionally executes the caller-supplied command through cmd.exe.
	cmd := exec.CommandContext(ctx, "cmd.exe", "/d", "/s", "/c", command)
	cmd.Dir = workingDirectory
	cmd.Env = append([]string(nil), environment...)
	cmd.WaitDelay = 2 * time.Second
	return cmd, nil
}
