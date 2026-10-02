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

func (HostShellLauncher) Command(
	ctx context.Context,
	_ string,
	workingDirectory string,
	command string,
	environment []string,
) (*exec.Cmd, error) {
	//nolint:gosec // workspace.shell intentionally executes the caller-supplied command through a shell.
	cmd := exec.CommandContext(ctx, "/bin/sh", "-lc", command)
	cmd.Dir = workingDirectory
	cmd.Env = append([]string(nil), environment...)
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
	return cmd, nil
}
