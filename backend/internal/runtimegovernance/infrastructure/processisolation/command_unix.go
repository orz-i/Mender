//go:build !windows

package processisolation

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func configureCommand(cmd *exec.Cmd) {
	// A separate PGID still inherits the parent's controlling terminal.
	// In interactive Wails development, a background Agent reading that TTY
	// can be job-control stopped while its Task worker keeps heartbeating.
	// Setsid drops the inherited TTY and also creates a PGID equal to PID,
	// preserving the existing process-group-wide cancellation below.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
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
}
