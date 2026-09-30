//go:build windows

package plugin

import (
	"context"
	"os/exec"
	"time"
)

func newHostShellCommand(ctx context.Context, command string) *exec.Cmd {
	//nolint:gosec // workspace.shell intentionally executes the caller-supplied command through cmd.exe.
	cmd := exec.CommandContext(ctx, "cmd.exe", "/d", "/s", "/c", command)
	cmd.WaitDelay = 2 * time.Second
	return cmd
}
