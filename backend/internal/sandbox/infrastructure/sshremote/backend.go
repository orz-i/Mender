package sshremote

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	sandboxapp "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	workspaceapp "github.com/orz-i/Mender/backend/internal/workspace/application"
	workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"
	"golang.org/x/crypto/ssh"
)

type TemplateLookup interface {
	GetSSHRemoteTemplate(context.Context, sandboxdomain.ID) (Template, error)
}
type Backend struct {
	Dial      Dialer
	Templates TemplateLookup
}

func (*Backend) Kind() sandboxdomain.BackendKind { return sandboxdomain.BackendSSHRemote }
func (*Backend) Capabilities() sandboxdomain.Capabilities {
	return sandboxdomain.Capabilities{Read: true, Search: true, Edit: true, Shell: true}
}
func (b *Backend) Open(ctx context.Context, item sandboxdomain.Sandbox, run sandboxdomain.Run) (sandboxapp.Session, error) {
	if b == nil || b.Templates == nil || b.Dial.Credentials == nil || item.Backend() != sandboxdomain.BackendSSHRemote ||
		item.Isolation() != sandboxdomain.IsolationRemote || run.State != sandboxdomain.StateReady || run.SandboxID != item.ID() ||
		run.ExternalID != "ssh_remote:"+item.ID().String() {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	t, err := b.Templates.GetSSHRemoteTemplate(ctx, item.ID())
	if err != nil {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	conn, err := b.Dial.Connect(ctx, t)
	if err != nil {
		return nil, err
	}
	return &Session{conn: conn, root: t.Root, sandboxID: item.ID(), remoteUser: t.User}, nil
}

type Session struct {
	conn       *Connection
	root       string
	sandboxID  sandboxdomain.ID
	remoteUser string
	uncertain  atomic.Bool
}

func (s *Session) Close() error {
	if s == nil {
		return nil
	}
	if s.conn != nil {
		_ = s.conn.Close()
	}
	if s.uncertain.Load() {
		return fmt.Errorf("SSH workspace outcome uncertain; durable lease retained: %w", sandboxdomain.ErrConflict)
	}
	return nil
}

// beginWorkspace bounds context-free SFTP requests by closing the exact
// authenticated transport on cancellation. Uncertainty remains sticky through
// Close so the Sandbox router retains its durable execution lease.
func (s *Session) beginWorkspace(ctx context.Context) (func(), error) {
	if s == nil || s.conn == nil || s.conn.SFTP == nil {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	stopCancel := context.AfterFunc(ctx, func() {
		s.uncertain.Store(true)
		_ = s.conn.Close()
	})
	return func() { stopCancel() }, nil
}

func failure(message string) error { return &workspace.ToolError{Message: message} }

func relative(raw string, allowRoot bool) (string, error) {
	if len(raw) > 1024 || path.IsAbs(raw) || strings.ContainsAny(raw, "\x00\r\n") {
		return "", errors.New("SSH workspace path must be relative and bounded")
	}
	if raw == "" {
		raw = "."
	}
	// Do not silently normalize an explicit traversal segment away. This
	// is a path-safety contract, not remote process isolation.
	for _, segment := range strings.Split(raw, "/") {
		if segment == ".." {
			return "", errors.New("SSH workspace path contains parent traversal")
		}
	}
	clean := path.Clean(raw)
	if clean == ".." || strings.HasPrefix(clean, "../") || (clean == "." && !allowRoot) {
		return "", errors.New("SSH workspace path escapes or names root")
	}
	return clean, nil
}
func (s *Session) checked(raw string, allowRoot, mayCreate bool) (string, fs.FileInfo, error) {
	rel, err := relative(raw, allowRoot)
	if err != nil {
		return "", nil, err
	}
	current := s.root
	if rel == "." {
		info, err := s.conn.SFTP.Lstat(current)
		return current, info, err
	}
	parts := strings.Split(rel, "/")
	for index, part := range parts {
		current = path.Join(current, part)
		info, e := s.conn.SFTP.Lstat(current)
		last := index == len(parts)-1
		if errors.Is(e, os.ErrNotExist) && last && mayCreate {
			return current, nil, nil
		}
		if e != nil {
			return "", nil, errors.New("remote workspace path unavailable")
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return "", nil, errors.New("remote workspace symlink refused")
		}
		if !last && !info.IsDir() {
			return "", nil, errors.New("remote workspace parent is not a directory")
		}
		if last {
			return current, info, nil
		}
	}
	return "", nil, errors.New("invalid workspace path")
}

func (s *Session) Read(ctx context.Context, in workspace.ReadRequest) (workspace.ReadResult, error) {
	stopCancel, err := s.beginWorkspace(ctx)
	if err != nil {
		return workspace.ReadResult{}, err
	}
	defer stopCancel()
	if in.Offset < 1 || in.Limit < 1 || in.Limit > 2000 {
		return workspace.ReadResult{}, failure("read offset/limit out of bounds")
	}
	remote, info, err := s.checked(in.Path, false, false)
	if err != nil || !info.Mode().IsRegular() {
		return workspace.ReadResult{}, failure("remote file not available")
	}
	if info.Size() > 2<<20 {
		return workspace.ReadResult{}, failure("file exceeds SSH read limit")
	}
	f, err := s.conn.SFTP.Open(remote)
	if err != nil {
		return workspace.ReadResult{}, failure("remote file not readable")
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, 2<<20+1))
	if err != nil {
		s.uncertain.Store(true)
		return workspace.ReadResult{}, sandboxdomain.ErrBackendUnavailable
	}
	if len(data) > 2<<20 || !utf8.Valid(data) {
		return workspace.ReadResult{}, failure("remote file too large or not UTF-8")
	}
	text := strings.TrimSuffix(string(data), "\n")
	lines := []string{}
	if text != "" {
		lines = strings.Split(text, "\n")
	}
	start := min(in.Offset-1, len(lines))
	end := min(start+in.Limit, len(lines))
	content := ""
	if end > start {
		content = strings.Join(lines[start:end], "\n")
	}
	return workspace.ReadResult{Path: in.Path, Content: content, StartLine: start + 1, EndLine: end, Truncated: end < len(lines)}, nil
}

func (s *Session) Edit(ctx context.Context, in workspace.EditRequest) (workspace.EditResult, error) {
	stopCancel, err := s.beginWorkspace(ctx)
	if err != nil {
		return workspace.EditResult{}, err
	}
	defer stopCancel()
	if len(in.Content) > 4<<20 || !utf8.ValidString(in.Content) || len(in.Edits) > 64 {
		return workspace.EditResult{}, failure("edit exceeds limits")
	}
	mayCreate := in.Operation == "create"
	remote, info, err := s.checked(in.Path, false, mayCreate)
	if err != nil {
		return workspace.EditResult{}, failure("remote edit path unavailable")
	}
	var original []byte
	before := int64(0)
	replacements := 0
	switch in.Operation {
	case "create":
		if info != nil || len(in.Edits) != 0 {
			return workspace.EditResult{}, failure("create requires absent file and no replacements")
		}
	case "delete", "replace":
		if info == nil || !info.Mode().IsRegular() || info.Size() > 4<<20 {
			return workspace.EditResult{}, failure("edit requires regular bounded file")
		}
		before = info.Size()
		if in.Operation == "delete" {
			if in.Content != "" || len(in.Edits) != 0 {
				return workspace.EditResult{}, failure("delete must not specify content")
			}
			if err := s.conn.SFTP.Remove(remote); err != nil {
				s.uncertain.Store(true)
				return workspace.EditResult{}, sandboxdomain.ErrBackendUnavailable
			}
			return workspace.EditResult{Path: in.Path, Operation: "delete", BytesBefore: before}, nil
		}
		if in.Content != "" || len(in.Edits) == 0 {
			return workspace.EditResult{}, failure("replace requires replacement list")
		}
		f, e := s.conn.SFTP.Open(remote)
		if e != nil {
			return workspace.EditResult{}, failure("file cannot be opened")
		}
		original, e = io.ReadAll(io.LimitReader(f, 4<<20+1))
		_ = f.Close()
		if e != nil {
			s.uncertain.Store(true)
			return workspace.EditResult{}, sandboxdomain.ErrBackendUnavailable
		}
		if len(original) > 4<<20 || !utf8.Valid(original) {
			return workspace.EditResult{}, failure("remote file too large or not UTF-8")
		}
		working := string(original)
		for _, ch := range in.Edits {
			expected := ch.ExpectedReplacements
			if ch.OldText == "" || expected < 1 || expected > 1000 || strings.Count(working, ch.OldText) != expected {
				return workspace.EditResult{}, failure("replacement text or expected count mismatch")
			}
			working = strings.ReplaceAll(working, ch.OldText, ch.NewText)
			replacements += expected
			if len(working) > 4<<20 {
				return workspace.EditResult{}, failure("result exceeds edit limit")
			}
		}
		in.Content = working
	default:
		return workspace.EditResult{}, failure("unsupported SSH edit operation")
	}
	if in.Operation == "create" {
		// SSH_FXF_EXCL must be checked atomically by the trusted SFTP server.
		// A separate Lstat-then-Rename would overwrite a file created between
		// those calls, even if both calls reported success independently.
		file, err := s.conn.SFTP.OpenFile(remote, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
		if err != nil {
			return workspace.EditResult{}, failure("remote target already exists or exclusive create unavailable")
		}
		written, writeErr := io.Copy(file, strings.NewReader(in.Content))
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil || written != int64(len(in.Content)) {
			s.uncertain.Store(true)
			return workspace.EditResult{}, sandboxdomain.ErrBackendUnavailable
		}
		return workspace.EditResult{Path: in.Path, Operation: "create", BytesAfter: written}, nil
	}
	// Replace uses a new exclusive temporary file and POSIX atomic rename
	// on SFTP servers that support it. An uncertain result retains the lease.
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return workspace.EditResult{}, err
	}
	temporary := path.Join(path.Dir(remote), ".mender-"+hex.EncodeToString(nonce[:]))
	file, err := s.conn.SFTP.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		return workspace.EditResult{}, failure("remote temporary file unavailable")
	}
	written, writeErr := io.Copy(file, strings.NewReader(in.Content))
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil || written != int64(len(in.Content)) {
		s.uncertain.Store(true)
		return workspace.EditResult{}, sandboxdomain.ErrBackendUnavailable
	}
	if err := s.conn.SFTP.PosixRename(temporary, remote); err != nil {
		s.uncertain.Store(true)
		return workspace.EditResult{}, sandboxdomain.ErrBackendUnavailable
	}
	return workspace.EditResult{Path: in.Path, Operation: "replace", Replacements: replacements, BytesBefore: before, BytesAfter: written}, nil
}

func (s *Session) Search(ctx context.Context, in workspace.SearchRequest) (workspace.SearchResult, error) {
	stopCancel, err := s.beginWorkspace(ctx)
	if err != nil {
		return workspace.SearchResult{}, err
	}
	defer stopCancel()
	if in.MaxResults < 1 || in.MaxResults > 500 || len(in.Query) > 4096 || len(in.Pattern) > 1024 {
		return workspace.SearchResult{}, failure("search limits exceeded")
	}
	if in.Mode != "files" && in.Mode != "content" {
		return workspace.SearchResult{}, failure("unsupported search mode")
	}
	if (in.Mode == "files" && in.Pattern == "") || (in.Mode == "content" && in.Query == "") {
		return workspace.SearchResult{}, failure("missing search query or pattern")
	}
	directory, info, e := s.checked(in.Path, true, false)
	if e != nil || !info.IsDir() {
		return workspace.SearchResult{}, failure("search root unavailable")
	}
	var re *regexp.Regexp
	if in.Regex {
		expression := in.Query
		if !in.CaseSensitive {
			expression = "(?i)" + expression
		}
		var err error
		re, err = regexp.Compile(expression)
		if err != nil {
			return workspace.SearchResult{}, failure("invalid search expression")
		}
	}
	matches := make([]workspace.SearchMatch, 0)
	files := 0
	visitedEntries := 0
	discoveredDirs := 1
	aggregate := int64(0)
	truncated := false
	exhausted := false
	type node struct {
		dir   string
		depth int
	}
	queue := []node{{directory, 0}}
	for len(queue) > 0 {
		if ctx.Err() != nil {
			return workspace.SearchResult{}, ctx.Err()
		}
		curr := queue[0]
		queue = queue[1:]
		if curr.depth > 16 {
			truncated = true
			continue
		}
		entries, err := s.conn.SFTP.ReadDir(curr.dir)
		if err != nil {
			return workspace.SearchResult{}, failure("remote search listing failed")
		}
		for _, entry := range entries {
			if ctx.Err() != nil {
				return workspace.SearchResult{}, ctx.Err()
			}
			visitedEntries++
			if visitedEntries > 10000 {
				truncated, exhausted = true, true
				break
			}
			if entry.Mode()&fs.ModeSymlink != 0 || entry.Name() == ".git" {
				continue
			}
			full := path.Join(curr.dir, entry.Name())
			rel := strings.TrimPrefix(full, s.root+"/")
			if entry.IsDir() {
				discoveredDirs++
				if discoveredDirs > 1024 {
					truncated, exhausted = true, true
					break
				}
				queue = append(queue, node{full, curr.depth + 1})
				continue
			}
			if !entry.Mode().IsRegular() {
				continue
			}
			files++
			if files > 10000 {
				truncated = true
				break
			}
			if in.Pattern != "" {
				matched, err := path.Match(in.Pattern, rel)
				if err != nil {
					return workspace.SearchResult{}, failure("invalid search path pattern")
				}
				if !matched {
					continue
				}
			}
			if in.Mode == "files" {
				matches = append(matches, workspace.SearchMatch{Path: rel})
			} else {
				if entry.Size() > 1<<20 || aggregate+entry.Size() > 32<<20 {
					truncated = true
					continue
				}
				aggregate += entry.Size()
				f, e := s.conn.SFTP.Open(full)
				if e != nil {
					continue
				}
				data, e := io.ReadAll(io.LimitReader(f, 1<<20+1))
				_ = f.Close()
				if e != nil || len(data) > 1<<20 || !utf8.Valid(data) {
					continue
				}
				text := strings.TrimSuffix(string(data), "\n")
				for idx, line := range strings.Split(text, "\n") {
					found := false
					if in.Regex {
						found = re.MatchString(line)
					} else if in.CaseSensitive {
						found = strings.Contains(line, in.Query)
					} else {
						found = strings.Contains(strings.ToLower(line), strings.ToLower(in.Query))
					}
					if found {
						if len(line) > 2048 {
							line = line[:2048]
						}
						matches = append(matches, workspace.SearchMatch{Path: rel, Line: idx + 1, Text: workspace.OptionalText(line)})
					}
					if len(matches) >= in.MaxResults {
						truncated = true
						break
					}
				}
			}
			if len(matches) >= in.MaxResults {
				truncated = true
				break
			}
		}
		if exhausted || (truncated && (files > 10000 || len(matches) >= in.MaxResults)) {
			break
		}
	}
	return workspace.SearchResult{Mode: in.Mode, Matches: matches, Truncated: truncated}, nil
}

type limitedOutput struct {
	data      bytes.Buffer
	limit     int
	truncated bool
}

func (b *limitedOutput) Write(raw []byte) (int, error) {
	n := len(raw)
	space := b.limit - b.data.Len()
	if space > 0 {
		_, _ = b.data.Write(raw[:min(n, space)])
	}
	if n > space {
		b.truncated = true
	}
	return n, nil
}
func quoted(raw string) string { return "'" + strings.ReplaceAll(raw, "'", "'\\''") + "'" }

// Shell executes as the selected SSH account, outside Mender's local host.
// The workspace root is only a convenience for shell cwd, NOT a jail.
func (s *Session) Shell(ctx context.Context, in workspace.ShellRequest) (workspace.ShellResult, error) {
	stopCancel, err := s.beginWorkspace(ctx)
	if err != nil {
		return workspace.ShellResult{}, err
	}
	defer stopCancel()
	if strings.TrimSpace(in.Command) == "" || len(in.Command) > 64*1024 {
		return workspace.ShellResult{}, failure("shell command missing or too large")
	}
	if in.TimeoutMS < 1 || in.TimeoutMS > 120000 {
		return workspace.ShellResult{}, failure("shell timeout exceeds 2 minutes")
	}
	timeout := time.Duration(in.TimeoutMS) * time.Millisecond
	cwd := in.Cwd
	directory, info, err := s.checked(cwd, true, false)
	if err != nil || !info.IsDir() {
		return workspace.ShellResult{}, failure("remote shell cwd unavailable")
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	session, err := s.conn.SSH.NewSession()
	if err != nil {
		return workspace.ShellResult{}, sandboxdomain.ErrBackendUnavailable
	}
	defer func() { _ = session.Close() }()
	stdout := &limitedOutput{limit: 64 * 1024}
	stderr := &limitedOutput{limit: 64 * 1024}
	session.Stdout = stdout
	session.Stderr = stderr
	done := make(chan error, 1)
	// command and cwd shell quoted separately; untrusted command is explicitly
	// executed as the reviewed remote login, not as a local host subprocess.
	remoteCommand := "cd " + quoted(directory) + " && exec sh -c " + quoted(in.Command)
	go func() { done <- session.Run(remoteCommand) }()
	var result error
	select {
	case result = <-done:
	case <-runCtx.Done():
		_ = session.Close()
		_ = s.conn.SSH.Close()
		s.uncertain.Store(true)
		return workspace.ShellResult{}, runCtx.Err()
	}
	exitCode := 0
	if result != nil {
		var exit *ssh.ExitError
		if errors.As(result, &exit) {
			exitCode = exit.ExitStatus()
		} else {
			s.uncertain.Store(true)
			return workspace.ShellResult{}, sandboxdomain.ErrBackendUnavailable
		}
	}
	return workspace.ShellResult{ExitCode: exitCode, Stdout: stdout.data.String(), Stderr: stderr.data.String(), TimedOut: false, Truncated: stdout.truncated || stderr.truncated}, nil
}

var _ sandboxapp.Backend = (*Backend)(nil)
var _ workspaceapp.Session = (*Session)(nil)
