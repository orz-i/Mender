package dockerengine

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	mobyclient "github.com/moby/moby/client"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

const maxDockerShellTimeout = 120 * time.Second

type shellArguments struct {
	Command   string `json:"command"`
	Cwd       string `json:"cwd,omitempty"`
	TimeoutMS int64  `json:"timeoutMs,omitempty"`
}
type shellResult struct {
	ExitCode  int    `json:"exitCode"`
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	TimedOut  bool   `json:"timedOut"`
	Truncated bool   `json:"truncated"`
}

type cappedWriter struct {
	content   []byte
	limit     int
	truncated bool
}

func (w *cappedWriter) Write(input []byte) (int, error) {
	n := len(input)
	space := w.limit - len(w.content)
	if space > 0 {
		w.content = append(w.content, input[:min(n, space)]...)
	}
	if n > space {
		w.truncated = true
	}
	return n, nil
}

// runExec executes a reviewed argv vector as the fixed unprivileged image user.
// Public shell deliberately uses /bin/sh -c; file mutations use *fixed* argv
// without injecting Tool text into command templates. Any ambiguous operation
// retains the durable external execution lease for operator reconciliation.
func (s *workspaceSession) runExec(ctx context.Context, cwd string, argv []string) (shellResult, error) {
	return s.runExecStreamBounded(ctx, cwd, argv, nil, maxDockerOutputBytes)
}

// runExecStream is the single bounded, non-root Docker stdin transport.
// It is required because Docker Engine archive PUT rejects ReadonlyRootfs.
func (s *workspaceSession) runExecStream(ctx context.Context, cwd string, argv []string, input []byte) (shellResult, error) {
	return s.runExecStreamBounded(ctx, cwd, argv, input, maxDockerOutputBytes)
}
func (s *workspaceSession) runExecOutput(ctx context.Context, cwd string, argv []string, maxStdout int) (shellResult, error) {
	return s.runExecStreamBounded(ctx, cwd, argv, nil, maxStdout)
}
func (s *workspaceSession) runExecStreamBounded(ctx context.Context, cwd string, argv []string, input []byte, maxStdout int) (shellResult, error) {
	if maxStdout < 1 || maxStdout > maxDockerSearchBytes+1 {
		return shellResult{}, fmt.Errorf("docker exec output budget invalid")
	}
	if err := ctx.Err(); err != nil {
		return shellResult{}, err
	}
	if len(argv) == 0 || len(argv) > 32 {
		return shellResult{}, sandboxdomain.ErrCapabilityUnavailable
	}
	config := mobyclient.ExecCreateOptions{
		User:       fmt.Sprintf("%d:%d", s.template.UserID, s.template.GroupID),
		Privileged: false, TTY: false, AttachStdin: input != nil, AttachStdout: true, AttachStderr: true,
		WorkingDir: cwd, Cmd: argv,
		Env: []string{"HOME=/workspace", "PATH=/usr/bin:/bin", "TMPDIR=/tmp"},
	}
	created, err := s.api.ExecCreate(ctx, s.containerID, config)
	if err != nil {
		s.uncertain.Store(true)
		return shellResult{}, safeDaemonError(ctx, err)
	}
	if created.ID == "" {
		s.uncertain.Store(true)
		return shellResult{}, sandboxdomain.ErrBackendUnavailable
	}
	attached, err := s.api.ExecAttach(ctx, created.ID, mobyclient.ExecAttachOptions{TTY: false})
	if err != nil {
		s.uncertain.Store(true)
		return shellResult{}, safeDaemonError(ctx, err)
	}
	defer attached.Close()
	if attached.Reader == nil || attached.Conn == nil {
		s.uncertain.Store(true)
		return shellResult{}, sandboxdomain.ErrBackendUnavailable
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			attached.Close()
		case <-done:
		}
	}()
	stdout := &cappedWriter{limit: maxStdout}
	stderr := &cappedWriter{limit: maxDockerOutputBytes}
	var writeResult chan error
	if input != nil {
		writeResult = make(chan error, 1)
		go func() {
			_, writeErr := io.Copy(attached.Conn, bytes.NewReader(input))
			closeErr := attached.CloseWrite()
			if writeErr == nil {
				writeErr = closeErr
			}
			writeResult <- writeErr
		}()
	}
	_, err = stdcopy.StdCopy(stdout, stderr, attached.Reader)
	if writeResult != nil {
		writeErr := <-writeResult
		if writeErr != nil {
			s.uncertain.Store(true)
			return shellResult{}, safeDaemonError(ctx, writeErr)
		}
	}
	if err != nil {
		s.uncertain.Store(true)
		return shellResult{}, safeDaemonError(ctx, err)
	}
	if err := ctx.Err(); err != nil {
		s.uncertain.Store(true)
		return shellResult{}, err
	}
	inspection, err := s.api.ExecInspect(ctx, created.ID, mobyclient.ExecInspectOptions{})
	if err != nil {
		s.uncertain.Store(true)
		return shellResult{}, safeDaemonError(ctx, err)
	}
	if inspection.ContainerID != s.containerID || inspection.Running || inspection.ID != created.ID {
		s.uncertain.Store(true)
		return shellResult{}, sandboxdomain.ErrConflict
	}
	return shellResult{ExitCode: inspection.ExitCode, Stdout: string(stdout.content), Stderr: string(stderr.content), Truncated: stdout.truncated || stderr.truncated}, nil
}

func (s *workspaceSession) shell(ctx context.Context, args invocationdomain.JSONValue) (invocationdomain.Outcome, error) {
	var input shellArguments
	if err := decodeDockerArgs(args.Bytes(), &input); err != nil {
		return dockerFailure(err)
	}
	if strings.TrimSpace(input.Command) == "" || len(input.Command) > maxDockerCommandBytes {
		return dockerFailure(fmt.Errorf("shell command must be nonempty and at most %d bytes", maxDockerCommandBytes))
	}
	timeout := 30 * time.Second
	if input.TimeoutMS != 0 {
		if input.TimeoutMS < 1 || input.TimeoutMS > maxDockerShellTimeout.Milliseconds() {
			return dockerFailure(fmt.Errorf("shell timeout exceeds allowed bounds"))
		}
		timeout = time.Duration(input.TimeoutMS) * time.Millisecond
	}
	cwd := input.Cwd
	if cwd == "" {
		cwd = "."
	}
	relative, err := relativePath(cwd, true)
	if err != nil {
		return dockerFailure(err)
	}
	if err := s.verifyPath(ctx, relative, true, false); err != nil {
		return dockerFailure(err)
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	// Command input is intentionally processed by /bin/sh ONLY inside the
	// admitted Docker container, not through an OS host shell.
	output, err := s.runExec(runCtx, containerPath(relative), []string{"/bin/sh", "-c", input.Command})
	if err != nil {
		if runCtx.Err() != nil {
			s.uncertain.Store(true)
			return invocationdomain.Outcome{}, fmt.Errorf("docker shell result uncertain after timeout/cancellation: %w", runCtx.Err())
		}
		return invocationdomain.Outcome{}, err
	}
	return dockerComplete(output)
}
func (s *workspaceSession) runFixed(ctx context.Context, argv []string) (int, error) {
	result, err := s.runExec(ctx, workspacePath, argv)
	if err != nil {
		return -1, err
	}
	return result.ExitCode, nil
}
