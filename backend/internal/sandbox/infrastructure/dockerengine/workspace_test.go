package dockerengine

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	mobyclient "github.com/moby/moby/client"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	workspaceapplication "github.com/orz-i/Mender/backend/internal/workspace/application"
	workspacedomain "github.com/orz-i/Mender/backend/internal/workspace/domain"
)

func TestDockerTypedReadRetainsEmptyFileAndPathSafety(t *testing.T) {
	t.Parallel()
	api, session := workspaceFixture(func(options mobyclient.ExecCreateOptions) workspaceExecAnswer {
		return workspaceExecAnswer{}
	})
	result, err := session.Read(context.Background(), workspaceapplication.NormalizeRead(workspacedomain.ReadRequest{Path: "empty.txt"}))
	if err != nil || result.Path != "empty.txt" || result.StartLine != 0 || result.EndLine != 0 || result.Content != "" {
		t.Fatalf("empty file changed: %+v, %v", result, err)
	}
	if len(api.options) != 3 {
		t.Fatalf("root/component/read checks=%d, want 3", len(api.options))
	}
	for _, bad := range []string{"../escape", "/etc/passwd", "bad\\path"} {
		_, err := session.Read(context.Background(), workspaceapplication.NormalizeRead(workspacedomain.ReadRequest{Path: bad}))
		var tool *workspacedomain.ToolError
		if !errors.As(err, &tool) {
			t.Fatalf("unsafe path %q: %v", bad, err)
		}
	}
	if len(api.options) != 3 {
		t.Fatal("lexically unsafe paths reached the Engine")
	}
	api.respond = func(mobyclient.ExecCreateOptions) workspaceExecAnswer { return workspaceExecAnswer{exitCode: 1} }
	_, err = session.Read(context.Background(), workspaceapplication.NormalizeRead(workspacedomain.ReadRequest{Path: "linked/file"}))
	var tool *workspacedomain.ToolError
	if !errors.As(err, &tool) || len(api.options) != 4 {
		t.Fatalf("failed no-symlink root check did not stop read: %v, calls=%d", err, len(api.options))
	}
}

func TestDockerTypedSearchRetainsOptionalEmptyText(t *testing.T) {
	t.Parallel()
	_, session := workspaceFixture(func(options mobyclient.ExecCreateOptions) workspaceExecAnswer {
		if strings.Contains(options.Cmd[2], "find ") {
			return workspaceExecAnswer{stdout: "/workspace/empty-line.txt\x00/workspace/.git/hidden\x00"}
		}
		if strings.Contains(options.Cmd[2], "cat --") {
			return workspaceExecAnswer{stdout: "\n"}
		}
		return workspaceExecAnswer{}
	})
	result, err := session.Search(context.Background(), workspaceapplication.NormalizeSearch(workspacedomain.SearchRequest{
		Mode: "content", Query: "^$", Regex: true,
	}))
	if err != nil || len(result.Matches) != 1 || result.Matches[0].Line != 1 || result.Matches[0].Text != nil {
		t.Fatalf("empty text presence or .git exclusion changed: %+v, %v", result, err)
	}
}

func TestDockerTypedMutationRetainsFixedArgvDigestAndConflict(t *testing.T) {
	t.Parallel()
	for _, conflict := range []bool{false, true} {
		t.Run(fmt.Sprintf("conflict_%v", conflict), func(t *testing.T) {
			content := "new $(not-host-code)"
			commitSeen, cleanupSeen, hashSeen := false, false, false
			api, session := workspaceFixture(func(options mobyclient.ExecCreateOptions) workspaceExecAnswer {
				command := options.Cmd[2]
				if strings.Contains(command, "cat --") {
					return workspaceExecAnswer{stdout: "old"}
				}
				if strings.Contains(command, "mv -f") {
					commitSeen = true
					expected := sha256.Sum256([]byte("old"))
					if len(options.Cmd) != 7 || options.Cmd[4] != "/workspace/draft.txt" || options.Cmd[6] != hex.EncodeToString(expected[:]) {
						t.Fatalf("replace lost original digest/argv fence: %+v", options.Cmd)
					}
					if conflict {
						return workspaceExecAnswer{exitCode: 1}
					}
				}
				if strings.Contains(command, "sha256sum") && !strings.Contains(command, "mv -f") {
					hashSeen = true
					expected := sha256.Sum256([]byte(content))
					if options.Cmd[len(options.Cmd)-1] != hex.EncodeToString(expected[:]) {
						t.Fatal("staged content was not verified before commit")
					}
				}
				cleanupSeen = cleanupSeen || strings.Contains(command, "rm -f")
				return workspaceExecAnswer{}
			})
			result, err := session.Edit(context.Background(), workspaceapplication.NormalizeEdit(workspacedomain.EditRequest{
				Path: "draft.txt", Operation: "replace", Edits: []workspacedomain.TextEdit{{OldText: "old", NewText: content}},
			}))
			var tool *workspacedomain.ToolError
			if conflict && !errors.As(err, &tool) || !conflict && (err != nil || result.Replacements != 1 || result.BytesAfter != int64(len(content))) {
				t.Fatalf("replace = %+v, %v", result, err)
			}
			if !commitSeen || !cleanupSeen || !hashSeen || api.stdin() != content {
				t.Fatalf("mutation transport changed: commit=%v cleanup=%v hash=%v stdin=%q", commitSeen, cleanupSeen, hashSeen, api.stdin())
			}
			for _, options := range api.options {
				if strings.Contains(strings.Join(options.Cmd, "\x00"), content) {
					t.Fatal("file content was interpolated into shell argv")
				}
			}
			if err := session.Close(); err != nil {
				t.Fatalf("known conflict/success retained a lease: %v", err)
			}
		})
	}
}

func TestDockerTypedShellRetainsOutputBoundsAndUncertainLease(t *testing.T) {
	t.Parallel()
	for _, uncertain := range []bool{false, true} {
		t.Run(fmt.Sprintf("uncertain_%v", uncertain), func(t *testing.T) {
			api, session := workspaceFixture(func(options mobyclient.ExecCreateOptions) workspaceExecAnswer {
				if len(options.Cmd) == 3 {
					return workspaceExecAnswer{stdout: strings.Repeat("x", maxDockerOutputBytes+1), stderr: "diagnostic", exitCode: 9, waitForCancellation: uncertain}
				}
				return workspaceExecAnswer{}
			})
			request := workspaceapplication.NormalizeShell(workspacedomain.ShellRequest{Command: "printf user-command"})
			if uncertain {
				request.TimeoutMS = 1 // Docker's existing minimum remains accepted.
			}
			result, err := session.Shell(context.Background(), request)
			if uncertain {
				var tool *workspacedomain.ToolError
				if !errors.Is(err, context.DeadlineExceeded) || errors.As(err, &tool) || !errors.Is(session.Close(), sandboxdomain.ErrConflict) {
					t.Fatalf("uncertainty was downgraded: result=%+v err=%v close=%v", result, err, session.Close())
				}
			} else if err != nil || !result.Truncated || len(result.Stdout) != maxDockerOutputBytes || result.Stderr != "diagnostic" || result.ExitCode != 9 || result.TimedOut {
				t.Fatalf("shell result changed: %+v, %v", result, err)
			}
			for _, options := range api.options {
				if options.User != "1000:1000" || options.Privileged || options.TTY || options.WorkingDir != "/workspace" ||
					strings.Join(options.Env, "|") != "HOME=/workspace|PATH=/usr/bin:/bin|TMPDIR=/tmp" {
					t.Fatalf("exec security context changed: %+v", options)
				}
			}
		})
	}
}

type workspaceExecAnswer struct {
	stdout, stderr      string
	exitCode            int
	waitForCancellation bool
}
type workspaceExecution struct {
	answer workspaceExecAnswer
	conn   *workspaceConnection
}
type workspaceAPI struct {
	containerID string
	options     []mobyclient.ExecCreateOptions
	executions  map[string]*workspaceExecution
	respond     func(mobyclient.ExecCreateOptions) workspaceExecAnswer
}

func workspaceFixture(respond func(mobyclient.ExecCreateOptions) workspaceExecAnswer) (*workspaceAPI, *workspaceSession) {
	api := &workspaceAPI{containerID: strings.Repeat("a", 64), executions: make(map[string]*workspaceExecution), respond: respond}
	return api, &workspaceSession{api: api, containerID: api.containerID, template: Template{UserID: 1000, GroupID: 1000}}
}

func (*workspaceAPI) ContainerInspect(context.Context, string, mobyclient.ContainerInspectOptions) (mobyclient.ContainerInspectResult, error) {
	return mobyclient.ContainerInspectResult{}, errors.New("Workspace operation must not inspect another container")
}
func (a *workspaceAPI) ExecCreate(_ context.Context, containerID string, options mobyclient.ExecCreateOptions) (mobyclient.ExecCreateResult, error) {
	if containerID != a.containerID {
		return mobyclient.ExecCreateResult{}, sandboxdomain.ErrConflict
	}
	a.options = append(a.options, options)
	id := fmt.Sprintf("exec-%d", len(a.options))
	a.executions[id] = &workspaceExecution{answer: a.respond(options)}
	return mobyclient.ExecCreateResult{ID: id}, nil
}
func (a *workspaceAPI) ExecAttach(ctx context.Context, id string, _ mobyclient.ExecAttachOptions) (mobyclient.ExecAttachResult, error) {
	execution := a.executions[id]
	if execution.answer.waitForCancellation {
		<-ctx.Done()
		return mobyclient.ExecAttachResult{}, ctx.Err()
	}
	var framed []byte
	for i, content := range []string{execution.answer.stdout, execution.answer.stderr} {
		// Docker non-TTY stream framing: channel byte, 3 reserved, BE length.
		header := []byte{byte(i + 1), 0, 0, 0, 0, 0, 0, 0}
		binary.BigEndian.PutUint32(header[4:], uint32(len(content))) // #nosec G115 -- test fixtures are bounded below 2^32 bytes.
		framed = append(framed, header...)
		framed = append(framed, content...)
	}
	execution.conn = &workspaceConnection{}
	return mobyclient.ExecAttachResult{HijackedResponse: mobyclient.HijackedResponse{
		Conn: execution.conn, Reader: bufio.NewReader(bytes.NewReader(framed)),
	}}, nil
}
func (a *workspaceAPI) ExecInspect(_ context.Context, id string, _ mobyclient.ExecInspectOptions) (mobyclient.ExecInspectResult, error) {
	return mobyclient.ExecInspectResult{ID: id, ContainerID: a.containerID, ExitCode: a.executions[id].answer.exitCode}, nil
}
func (a *workspaceAPI) stdin() string {
	var content strings.Builder
	for _, execution := range a.executions {
		if execution.conn != nil {
			execution.conn.mu.Lock()
			content.Write(execution.conn.input.Bytes())
			execution.conn.mu.Unlock()
		}
	}
	return content.String()
}

// The fake only supplies memory buffers, never an OS socket or shell.
type workspaceConnection struct {
	mu    sync.Mutex
	input bytes.Buffer
}

func (*workspaceConnection) Read([]byte) (int, error) { return 0, io.EOF }
func (c *workspaceConnection) Write(data []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.input.Write(data)
}
func (*workspaceConnection) Close() error                     { return nil }
func (*workspaceConnection) LocalAddr() net.Addr              { return &net.IPAddr{} }
func (*workspaceConnection) RemoteAddr() net.Addr             { return &net.IPAddr{} }
func (*workspaceConnection) SetDeadline(time.Time) error      { return nil }
func (*workspaceConnection) SetReadDeadline(time.Time) error  { return nil }
func (*workspaceConnection) SetWriteDeadline(time.Time) error { return nil }
