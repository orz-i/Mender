package localworkspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"
)

const (
	maxShellTimeout      = 2 * time.Minute
	maxShellCommandBytes = 64 * 1024
	maxShellOutputBytes  = 64 * 1024
)

type ShellLauncher interface {
	// CheckIsolation must fail if the selected isolation class cannot actually
	// be applied. This is checked before *any* structured filesystem access.
	CheckIsolation(sandboxdomain.IsolationClass) error
	Command(
		context.Context,
		sandboxdomain.IsolationClass,
		string,
		string,
		string,
		[]string,
	) (*exec.Cmd, error)
}

func (i *Instance) Shell(
	ctx context.Context,
	input workspace.ShellRequest,
) (workspace.ShellResult, error) {
	if err := i.ready(ctx); err != nil {
		return workspace.ShellResult{}, err
	}
	if input.Cwd == "" {
		input.Cwd = "."
	}
	if input.TimeoutMS == 0 {
		input.TimeoutMS = 30000
	}
	if strings.TrimSpace(input.Command) == "" {
		return workspace.ShellResult{}, toolFailure(fmt.Errorf("command must not be empty"))
	}
	if len(input.Command) > maxShellCommandBytes {
		return workspace.ShellResult{}, toolFailure(fmt.Errorf("command exceeds %d byte limit", maxShellCommandBytes))
	}

	if input.TimeoutMS < 1 || input.TimeoutMS > maxShellTimeout.Milliseconds() {
		return workspace.ShellResult{}, toolFailure(fmt.Errorf("timeoutMs must be between 1 and %d", maxShellTimeout.Milliseconds()))
	}
	timeout := time.Duration(input.TimeoutMS) * time.Millisecond
	cwd := input.Cwd
	_, directory, info, err := i.resolver.resolveExisting(cwd, true)
	if err != nil {
		return workspace.ShellResult{}, toolFailure(err)
	}
	if !info.IsDir() {
		return workspace.ShellResult{}, toolFailure(fmt.Errorf("cwd must be a directory"))
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	stdout := newBoundedCapture(maxShellOutputBytes)
	stderr := newBoundedCapture(maxShellOutputBytes)
	if i.shellLauncher == nil {
		return workspace.ShellResult{}, fmt.Errorf("execute workspace shell: launcher is unavailable")
	}
	command, err := i.shellLauncher.Command(
		runCtx,
		i.mode,
		i.root,
		directory,
		input.Command,
		shellEnvironment(),
	)
	if err != nil {
		return workspace.ShellResult{}, fmt.Errorf("execute workspace shell: construct command: %w", err)
	}
	command.Stdout = stdout
	command.Stderr = stderr

	err = command.Run()
	parentErr := ctx.Err()
	runErr := runCtx.Err()
	if parentErr != nil {
		return workspace.ShellResult{}, parentErr
	}

	timedOut := errors.Is(runErr, context.DeadlineExceeded)
	exitCode := -1
	if command.ProcessState != nil {
		exitCode = command.ProcessState.ExitCode()
	}
	if err != nil && !timedOut {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return workspace.ShellResult{}, fmt.Errorf("execute workspace shell: %w", err)
		}
	}

	return workspace.ShellResult{
		ExitCode:  exitCode,
		Stdout:    string(stdout.Bytes()),
		Stderr:    string(stderr.Bytes()),
		TimedOut:  timedOut,
		Truncated: stdout.Truncated() || stderr.Truncated(),
	}, nil
}

var shellEnvironmentKeys = []string{
	"PATH",
	"HOME",
	"USER",
	"LOGNAME",
	"SHELL",
	"TMPDIR",
	"TMP",
	"TEMP",
	"LANG",
	"LC_ALL",
	"LC_CTYPE",
	"SystemRoot",
	"WINDIR",
	"COMSPEC",
	"PATHEXT",
}

func shellEnvironment() []string {
	result := make([]string, 0, len(shellEnvironmentKeys))
	for _, key := range shellEnvironmentKeys {
		if value, ok := os.LookupEnv(key); ok {
			result = append(result, key+"="+value)
		}
	}
	if _, ok := os.LookupEnv("PATH"); !ok {
		result = append(result, "PATH=/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin")
	}
	return result
}

type boundedCapture struct {
	mu        sync.Mutex
	limit     int
	data      []byte
	truncated bool
}

func newBoundedCapture(limit int) *boundedCapture {
	if limit < 1 {
		limit = 1
	}
	return &boundedCapture{limit: limit}
}

func (w *boundedCapture) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	remaining := w.limit - len(w.data)
	if remaining > 0 {
		take := min(remaining, len(p))
		w.data = append(w.data, p[:take]...)
	}
	if len(p) > remaining {
		w.truncated = true
	}
	return len(p), nil
}

func (w *boundedCapture) Bytes() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]byte(nil), w.data...)
}

func (w *boundedCapture) Truncated() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.truncated
}
