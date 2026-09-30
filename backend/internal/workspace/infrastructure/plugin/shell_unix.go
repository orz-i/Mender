//go:build !windows

package plugin

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func newHostShellCommand(ctx context.Context, command string) *exec.Cmd {
	//nolint:gosec // workspace.shell intentionally executes the caller-supplied command through a shell.
	cmd := exec.CommandContext(ctx, "/bin/sh", "-lc", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = 2 * time.Second
	return cmd
}
