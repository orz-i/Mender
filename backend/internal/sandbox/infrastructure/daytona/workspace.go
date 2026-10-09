package daytona

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

const (
	maxRemotePathBytes = 4096
	maxReadBytes       = 2 * 1024 * 1024
	maxEditBytes       = 4 * 1024 * 1024
	maxSearchBytes     = 16 * 1024 * 1024
	maxShellBytes      = 64 * 1024
	maxOutputBytes     = 64 * 1024
)

var safeFilePath = regexp.MustCompile(`^[a-zA-Z0-9._/-]+$`)

type readArgs struct {
	Path   string `json:"path"`
	Offset int    `json:"offset,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}
type editArgs struct {
	Path      string     `json:"path"`
	Operation string     `json:"operation"`
	Content   string     `json:"content,omitempty"`
	Edits     []textEdit `json:"edits,omitempty"`
}
type textEdit struct {
	OldText              string `json:"oldText"`
	NewText              string `json:"newText"`
	ExpectedReplacements int    `json:"expectedReplacements,omitempty"`
}
type searchArgs struct {
	Mode          string `json:"mode"`
	Path          string `json:"path,omitempty"`
	Query         string `json:"query,omitempty"`
	Pattern       string `json:"pattern,omitempty"`
	Regex         bool   `json:"regex,omitempty"`
	CaseSensitive bool   `json:"caseSensitive,omitempty"`
	MaxResults    int    `json:"maxResults,omitempty"`
}
type shellArgs struct {
	Command   string `json:"command"`
	Cwd       string `json:"cwd,omitempty"`
	TimeoutMS int64  `json:"timeoutMs,omitempty"`
}

func strictJSON(raw []byte, target any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return fmt.Errorf("daytona Workspace: invalid arguments: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("daytona Workspace: trailing data")
	}
	return nil
}
func remotePath(raw string, rootOK bool) (string, error) {
	if raw == "" && rootOK {
		return ".", nil
	}
	if raw == "" || raw != strings.TrimSpace(raw) || len(raw) > maxRemotePathBytes || !safeFilePath.MatchString(raw) || strings.HasPrefix(raw, "/") || strings.Contains(raw, "//") {
		return "", fmt.Errorf("daytona Workspace requires a bounded safe relative POSIX path")
	}
	for _, part := range strings.Split(raw, "/") {
		if part == ".." || part == "" {
			return "", fmt.Errorf("daytona Workspace path escapes root")
		}
	}
	cleaned := path.Clean(raw)
	if !rootOK && cleaned == "." {
		return "", fmt.Errorf("daytona Workspace requires a file path")
	}
	return cleaned, nil
}
func remoteQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
func (s *session) absolute(relative string) string {
	if relative == "." {
		return s.root
	}
	return path.Join(s.root, relative)
}
func (s *session) checkPath(ctx context.Context, relative string, wantDirectory, missingLast bool) error {
	// The service-provided root is not a Tool mount override.
	if err := validateRemoteRoot(s.root); err != nil {
		return err
	}
	if err := s.checkOne(ctx, s.root, true, false); err != nil {
		return err
	}
	if relative == "." {
		if !wantDirectory {
			return fmt.Errorf("daytona Workspace root is not a file")
		}
		return nil
	}
	parts := strings.Split(relative, "/")
	for index := range parts {
		full := s.absolute(strings.Join(parts[:index+1], "/"))
		final := index == len(parts)-1
		if err := s.checkOne(ctx, full, !final || wantDirectory, final && missingLast); err != nil {
			return err
		}
	}
	return nil
}
func (s *session) checkOne(ctx context.Context, absolute string, dir, missing bool) error {
	check := "test ! -L " + remoteQuote(absolute)
	switch {
	case missing:
		check += " && test ! -e " + remoteQuote(absolute)
	case dir:
		check += " && test -d " + remoteQuote(absolute)
	default:
		check += " && test -f " + remoteQuote(absolute)
	}
	result, err := s.executeFixed(ctx, check, 8*time.Second)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("daytona Workspace path is missing, linked or invalid")
	}
	return nil
}
func (s *session) readBytes(ctx context.Context, relative string, limit int) ([]byte, error) {
	if limit < 1 || limit > maxEditBytes {
		return nil, sandboxdomain.ErrCapabilityUnavailable
	}
	if err := s.checkPath(ctx, relative, false, false); err != nil {
		return nil, err
	}
	remotePath := s.absolute(relative)
	metadata, err := s.remote.FileInfo(ctx, remotePath)
	if err != nil {
		return nil, err
	}
	if metadata.IsDirectory || metadata.Size < 0 || metadata.Size > int64(limit) {
		return nil, fmt.Errorf("daytona Workspace file exceeds approved size")
	}
	reader, err := s.remote.ReadStream(ctx, remotePath)
	if err != nil {
		return nil, err
	}
	if reader == nil {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	defer func() { _ = reader.Close() }()
	content, err := io.ReadAll(io.LimitReader(reader, int64(limit)+1))
	if err != nil {
		return nil, safeRemoteError(ctx, err)
	}
	if len(content) > limit {
		return nil, fmt.Errorf("daytona Workspace returned oversized file")
	}
	return content, nil
}
func completed(value any) (invocationdomain.Outcome, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return invocationdomain.Outcome{}, err
	}
	encoded, err := invocationdomain.NewJSONValue(raw)
	if err != nil {
		return invocationdomain.Outcome{}, err
	}
	return invocationdomain.Complete(encoded)
}
func rejected(err error) (invocationdomain.Outcome, error) {
	if err == nil {
		err = fmt.Errorf("daytona Workspace rejected operation")
	}
	text := err.Error()
	if len(text) > 1024 {
		text = text[:1024]
	}
	return invocationdomain.Failed(invocationdomain.FailureSourceTool, invocationdomain.FailureToolError, text)
}
func (s *session) read(ctx context.Context, args invocationdomain.JSONValue) (invocationdomain.Outcome, error) {
	var a readArgs
	if err := strictJSON(args.Bytes(), &a); err != nil {
		return rejected(err)
	}
	rel, err := remotePath(a.Path, false)
	if err != nil {
		return rejected(err)
	}
	if a.Offset == 0 {
		a.Offset = 1
	}
	if a.Limit == 0 {
		a.Limit = 200
	}
	if a.Offset < 1 || a.Limit < 1 || a.Limit > 2000 {
		return rejected(fmt.Errorf("read line offset/limit out of bounds"))
	}
	content, err := s.readBytes(ctx, rel, maxReadBytes)
	if err != nil {
		return rejected(err)
	}
	if !utf8.Valid(content) {
		return rejected(fmt.Errorf("UTF-8 text required"))
	}
	lines := strings.Split(strings.TrimSuffix(string(content), "\n"), "\n")
	start := a.Offset - 1
	if start >= len(lines) {
		return completed(map[string]any{"path": rel, "content": "", "startLine": 0, "endLine": 0, "truncated": false})
	}
	end := min(start+a.Limit, len(lines))
	return completed(map[string]any{"path": rel, "content": strings.Join(lines[start:end], "\n"), "startLine": start + 1, "endLine": end, "truncated": end < len(lines)})
}
func (s *session) shell(ctx context.Context, args invocationdomain.JSONValue) (invocationdomain.Outcome, error) {
	var a shellArgs
	if err := strictJSON(args.Bytes(), &a); err != nil {
		return rejected(err)
	}
	if strings.TrimSpace(a.Command) == "" || len(a.Command) > maxShellBytes {
		return rejected(fmt.Errorf("daytona shell must be nonempty and bounded"))
	}
	if a.Cwd == "" {
		a.Cwd = "."
	}
	cwd, err := remotePath(a.Cwd, true)
	if err != nil {
		return rejected(err)
	}
	if err := s.checkPath(ctx, cwd, true, false); err != nil {
		return rejected(err)
	}
	duration := 30 * time.Second
	if a.TimeoutMS != 0 {
		if a.TimeoutMS < 100 || a.TimeoutMS > 120000 {
			return rejected(fmt.Errorf("daytona shell timeout exceeds bounds"))
		}
		duration = time.Duration(a.TimeoutMS) * time.Millisecond
	}
	runCtx, cancel := context.WithTimeout(ctx, duration+2*time.Second)
	defer cancel()
	output, err := s.remote.Exec(runCtx, a.Command, s.absolute(cwd), duration)
	if err != nil {
		s.uncertain.Store(true) // The remote process may still be running.
		return invocationdomain.Outcome{}, safeRemoteError(runCtx, err)
	}
	if len(output.Output) > maxOutputBytes {
		return rejected(fmt.Errorf("daytona shell output exceeded approved bounds"))
	}
	return completed(map[string]any{"exitCode": output.ExitCode, "stdout": output.Output, "stderr": "", "truncated": false, "timedOut": false})
}
func (s *session) stage(ctx context.Context, relative string, data []byte) (string, error) {
	if len(data) > maxEditBytes {
		return "", fmt.Errorf("daytona staged content exceeds bounds")
	}
	var randomness [12]byte
	if _, err := rand.Read(randomness[:]); err != nil {
		return "", err
	}
	staged := path.Join(path.Dir(s.absolute(relative)), ".mender-stage-"+hex.EncodeToString(randomness[:]))
	if err := s.remote.UploadStream(ctx, bytes.NewReader(data), staged); err != nil {
		s.uncertain.Store(true)
		return "", err
	}
	// Verify remote byte-for-byte digest before committing the user-visible path.
	sum := sha256.Sum256(data)
	check := "test -f " + remoteQuote(staged) + " && test ! -L " + remoteQuote(staged) + " && test \"$(sha256sum " + remoteQuote(staged) + " | cut -d' ' -f1)\" = " + remoteQuote(hex.EncodeToString(sum[:]))
	answer, err := s.executeFixed(ctx, check, 15*time.Second)
	if err != nil || answer.ExitCode != 0 {
		s.uncertain.Store(true)
		return "", fmt.Errorf("daytona staged file integrity unavailable")
	}
	return staged, nil
}
func (s *session) cleanupStage(ctx context.Context, staged string) {
	if staged == "" {
		return
	}
	answer, err := s.executeFixed(ctx, "rm -f -- "+remoteQuote(staged), 10*time.Second)
	if err != nil || answer.ExitCode != 0 {
		s.uncertain.Store(true)
	}
}
func (s *session) edit(ctx context.Context, args invocationdomain.JSONValue) (invocationdomain.Outcome, error) {
	var a editArgs
	if err := strictJSON(args.Bytes(), &a); err != nil {
		return rejected(err)
	}
	relative, err := remotePath(a.Path, false)
	if err != nil {
		return rejected(err)
	}
	if len(a.Content) > maxEditBytes || len(a.Edits) > 64 {
		return rejected(fmt.Errorf("edit size/count exceeds limits"))
	}
	parent := path.Dir(relative)
	if err := s.checkPath(ctx, parent, true, false); err != nil {
		return rejected(err)
	}
	dest := s.absolute(relative)
	switch a.Operation {
	case "create":
		if len(a.Edits) > 0 || !utf8.ValidString(a.Content) {
			return rejected(fmt.Errorf("create requires UTF-8 content"))
		}
		if err := s.checkPath(ctx, relative, false, true); err != nil {
			return rejected(err)
		}
		staged, err := s.stage(ctx, relative, []byte(a.Content))
		if err != nil {
			return invocationdomain.Outcome{}, err
		}
		result, err := s.executeFixed(ctx, "test ! -e "+remoteQuote(dest)+" && test ! -L "+remoteQuote(dest)+" && ln "+remoteQuote(staged)+" "+remoteQuote(dest), 15*time.Second)
		s.cleanupStage(ctx, staged)
		if err != nil {
			return invocationdomain.Outcome{}, err
		}
		if result.ExitCode != 0 {
			return rejected(fmt.Errorf("create destination already exists"))
		}
		return completed(map[string]any{"path": relative, "operation": "create", "bytesBefore": 0, "bytesAfter": len(a.Content), "replacements": 0})
	case "replace":
		if a.Content != "" || len(a.Edits) == 0 {
			return rejected(fmt.Errorf("replace requires edits"))
		}
		before, err := s.readBytes(ctx, relative, maxEditBytes)
		if err != nil {
			return rejected(err)
		}
		if !utf8.Valid(before) {
			return rejected(fmt.Errorf("replace requires UTF-8 file"))
		}
		modified := string(before)
		replacements := 0
		for _, entry := range a.Edits {
			wanted := entry.ExpectedReplacements
			if wanted == 0 {
				wanted = 1
			}
			if entry.OldText == "" || wanted < 1 || wanted > 1000 || strings.Count(modified, entry.OldText) != wanted {
				return rejected(fmt.Errorf("replace count mismatch"))
			}
			modified = strings.ReplaceAll(modified, entry.OldText, entry.NewText)
			if len(modified) > maxEditBytes || !utf8.ValidString(modified) {
				return rejected(fmt.Errorf("replace would exceed file limits"))
			}
			replacements += wanted
		}
		staged, err := s.stage(ctx, relative, []byte(modified))
		if err != nil {
			return invocationdomain.Outcome{}, err
		}
		oldHash := sha256.Sum256(before)
		command := "test -f " + remoteQuote(dest) + " && test ! -L " + remoteQuote(dest) + " && test \"$(sha256sum " + remoteQuote(dest) + " | cut -d' ' -f1)\" = " + remoteQuote(hex.EncodeToString(oldHash[:])) + " && mv -f -- " + remoteQuote(staged) + " " + remoteQuote(dest)
		out, err := s.executeFixed(ctx, command, 20*time.Second)
		s.cleanupStage(ctx, staged)
		if err != nil {
			return invocationdomain.Outcome{}, err
		}
		if out.ExitCode != 0 {
			return rejected(fmt.Errorf("file content changed before replace"))
		}
		return completed(map[string]any{"path": relative, "operation": "replace", "replacements": replacements, "bytesBefore": len(before), "bytesAfter": len(modified)})
	case "delete":
		if len(a.Edits) != 0 || a.Content != "" {
			return rejected(fmt.Errorf("delete cannot include content"))
		}
		original, err := s.readBytes(ctx, relative, maxEditBytes)
		if err != nil {
			return rejected(err)
		}
		output, err := s.executeFixed(ctx, "test -f "+remoteQuote(dest)+" && test ! -L "+remoteQuote(dest)+" && rm -- "+remoteQuote(dest), 15*time.Second)
		if err != nil {
			return invocationdomain.Outcome{}, err
		}
		if output.ExitCode != 0 {
			return rejected(fmt.Errorf("delete failed or path changed"))
		}
		return completed(map[string]any{"path": relative, "operation": "delete", "replacements": 0, "bytesBefore": len(original), "bytesAfter": 0})
	default:
		return rejected(fmt.Errorf("unsupported edit operation"))
	}
}
func (s *session) search(ctx context.Context, args invocationdomain.JSONValue) (invocationdomain.Outcome, error) {
	var a searchArgs
	if err := strictJSON(args.Bytes(), &a); err != nil {
		return rejected(err)
	}
	if a.Path == "" {
		a.Path = "."
	}
	folder, err := remotePath(a.Path, true)
	if err != nil {
		return rejected(err)
	}
	if a.MaxResults == 0 {
		a.MaxResults = 100
	}
	if a.MaxResults < 1 || a.MaxResults > 500 || len(a.Pattern) > 1024 || len(a.Query) > 4096 {
		return rejected(fmt.Errorf("search bounds exceeded"))
	}
	if a.Mode != "files" && a.Mode != "content" {
		return rejected(fmt.Errorf("unsupported search mode"))
	}
	if a.Mode == "files" && a.Pattern == "" || a.Mode == "content" && a.Query == "" {
		return rejected(fmt.Errorf("search pattern or query missing"))
	}
	if err := s.checkPath(ctx, folder, true, false); err != nil {
		return rejected(err)
	}
	files, err := s.remote.ListFiles(ctx, s.absolute(folder), 4)
	if err != nil {
		return rejected(err)
	}
	if len(files) > maxRemoteCalls {
		return rejected(fmt.Errorf("remote listing exceeds limit"))
	}
	var glob *regexp.Regexp
	if a.Pattern != "" {
		glob, err = remoteGlob(a.Pattern)
		if err != nil {
			return rejected(err)
		}
	}
	var matcher func(string) bool
	if a.Mode == "content" {
		if a.Regex {
			expr := a.Query
			if !a.CaseSensitive {
				expr = "(?i)" + expr
			}
			rx, err := regexp.Compile(expr)
			if err != nil {
				return rejected(err)
			}
			matcher = rx.MatchString
		}
		if !a.Regex && a.CaseSensitive {
			matcher = func(line string) bool { return strings.Contains(line, a.Query) }
		}
		if !a.Regex && !a.CaseSensitive {
			lower := strings.ToLower(a.Query)
			matcher = func(line string) bool { return strings.Contains(strings.ToLower(line), lower) }
		}
	}
	result := make([]map[string]any, 0)
	truncated := false
	total := 0
	for _, file := range files {
		if file.IsDirectory || !strings.HasPrefix(file.Path, s.root+"/") {
			continue
		}
		rel, err := remotePath(strings.TrimPrefix(file.Path, s.root+"/"), false)
		if err != nil {
			continue
		}
		if strings.HasPrefix(rel, ".git/") || strings.Contains(rel, "/.git/") {
			continue
		}
		if glob != nil && !glob.MatchString(rel) {
			continue
		}
		if a.Mode == "files" {
			result = append(result, map[string]any{"path": rel})
		}
		if a.Mode == "content" {
			if file.Size < 0 || file.Size > 1024*1024 || total+int(file.Size) > maxSearchBytes {
				truncated = true
				break
			}
			content, err := s.readBytes(ctx, rel, 1024*1024)
			if err != nil || !utf8.Valid(content) {
				continue
			}
			total += len(content)
			for number, line := range strings.Split(string(content), "\n") {
				if matcher(line) {
					if len(line) > 2048 {
						line = line[:2048]
					}
					result = append(result, map[string]any{"path": rel, "line": number + 1, "text": line})
				}
				if len(result) >= a.MaxResults {
					break
				}
			}
		}
		if len(result) >= a.MaxResults {
			truncated = true
			break
		}
	}
	return completed(map[string]any{"mode": a.Mode, "matches": result, "truncated": truncated})
}
func remoteGlob(glob string) (*regexp.Regexp, error) {
	if glob == "" || len(glob) > 1024 || strings.HasPrefix(glob, "/") || strings.Contains(glob, "../") {
		return nil, fmt.Errorf("invalid search glob")
	}
	var b strings.Builder
	b.WriteByte('^')
	for i := 0; i < len(glob); i++ {
		switch glob[i] {
		case '*':
			if i+1 < len(glob) && glob[i+1] == '*' {
				b.WriteString(".*")
				i++
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(glob[i])))
		}
	}
	b.WriteByte('$')
	return regexp.Compile(b.String())
}
