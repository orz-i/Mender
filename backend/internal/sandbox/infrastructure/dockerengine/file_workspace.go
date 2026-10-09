package dockerengine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"

	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
)

type readArguments struct {
	Path   string `json:"path"`
	Offset int    `json:"offset,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}
type readResult struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
	Truncated bool   `json:"truncated"`
}
type editArguments struct {
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
type editResult struct {
	Path         string `json:"path"`
	Operation    string `json:"operation"`
	Replacements int    `json:"replacements"`
	BytesBefore  int64  `json:"bytesBefore"`
	BytesAfter   int64  `json:"bytesAfter"`
}
type searchArguments struct {
	Mode          string `json:"mode"`
	Path          string `json:"path,omitempty"`
	Query         string `json:"query,omitempty"`
	Pattern       string `json:"pattern,omitempty"`
	Regex         bool   `json:"regex,omitempty"`
	CaseSensitive bool   `json:"caseSensitive,omitempty"`
	MaxResults    int    `json:"maxResults,omitempty"`
}
type searchMatch struct {
	Path string `json:"path"`
	Line int    `json:"line,omitempty"`
	Text string `json:"text,omitempty"`
}
type searchResult struct {
	Mode      string        `json:"mode"`
	Matches   []searchMatch `json:"matches"`
	Truncated bool          `json:"truncated"`
}

func (s *workspaceSession) read(ctx context.Context, args invocationdomain.JSONValue) (invocationdomain.Outcome, error) {
	var input readArguments
	if err := decodeDockerArgs(args.Bytes(), &input); err != nil {
		return dockerFailure(err)
	}
	rel, err := relativePath(input.Path, false)
	if err != nil {
		return dockerFailure(err)
	}
	if input.Offset == 0 {
		input.Offset = 1
	}
	if input.Limit == 0 {
		input.Limit = 200
	}
	if input.Offset < 1 || input.Limit < 1 || input.Limit > 2000 {
		return dockerFailure(fmt.Errorf("read offset or limit outside bounds"))
	}
	data, err := s.readFile(ctx, rel, maxDockerReadBytes)
	if err != nil {
		return sanitizedFailure(ctx, err)
	}
	if !utf8.Valid(data) {
		return dockerFailure(fmt.Errorf("docker Workspace supports UTF-8 text only"))
	}
	lines := splitDockerLines(string(data))
	start := input.Offset - 1
	if start >= len(lines) {
		return dockerComplete(readResult{Path: rel})
	}
	end := min(start+input.Limit, len(lines))
	return dockerComplete(readResult{Path: rel, Content: strings.Join(lines[start:end], "\n"), StartLine: start + 1, EndLine: end, Truncated: end < len(lines)})
}
func (s *workspaceSession) edit(ctx context.Context, args invocationdomain.JSONValue) (invocationdomain.Outcome, error) {
	var input editArguments
	if err := decodeDockerArgs(args.Bytes(), &input); err != nil {
		return dockerFailure(err)
	}
	rel, err := relativePath(input.Path, false)
	if err != nil {
		return dockerFailure(err)
	}
	if len(input.Content) > maxDockerEditBytes || len(input.Edits) > 64 {
		return dockerFailure(fmt.Errorf("edit payload exceeds bounds"))
	}
	parent := path.Dir(rel)
	if err := s.verifyPath(ctx, parent, true, false); err != nil {
		return sanitizedFailure(ctx, err)
	}
	dest := containerPath(rel)
	switch input.Operation {
	case "create":
		if len(input.Edits) != 0 || !utf8.ValidString(input.Content) {
			return dockerFailure(fmt.Errorf("invalid create content"))
		}
		// An existing file or symlink is rejected; the fixed `ln` below then
		// performs an atomic fail-if-exists operation at the final component.
		if err := s.verifyPath(ctx, rel, false, true); err != nil {
			return sanitizedFailure(ctx, err)
		}
		stage, err := s.stageFile(ctx, parent, []byte(input.Content))
		if err != nil {
			return invocationdomain.Outcome{}, err
		}
		code, err := s.runFixed(ctx, []string{"/bin/sh", "-c", "test ! -e \"$2\" && test ! -L \"$2\" && ln \"$1\" \"$2\"", "mender", stage, dest})
		s.cleanupStage(ctx, stage)
		if err != nil {
			return invocationdomain.Outcome{}, err
		}
		if code != 0 {
			return dockerFailure(fmt.Errorf("create target already exists or is not writable"))
		}
		return dockerComplete(editResult{Path: rel, Operation: "create", BytesAfter: int64(len(input.Content))})
	case "replace":
		if input.Content != "" || len(input.Edits) == 0 {
			return dockerFailure(fmt.Errorf("replace requires edits and no content"))
		}
		before, err := s.readFile(ctx, rel, maxDockerEditBytes)
		if err != nil {
			return sanitizedFailure(ctx, err)
		}
		if !utf8.Valid(before) {
			return dockerFailure(fmt.Errorf("edit requires UTF-8 file"))
		}
		working := string(before)
		count := 0
		for _, edit := range input.Edits {
			wanted := edit.ExpectedReplacements
			if wanted == 0 {
				wanted = 1
			}
			if edit.OldText == "" || wanted < 1 || wanted > 1000 || strings.Count(working, edit.OldText) != wanted {
				return dockerFailure(fmt.Errorf("edit replacement count mismatch"))
			}
			working = strings.ReplaceAll(working, edit.OldText, edit.NewText)
			count += wanted
			if len(working) > maxDockerEditBytes || !utf8.ValidString(working) {
				return dockerFailure(fmt.Errorf("edited file exceeds UTF-8 or size bounds"))
			}
		}
		stage, err := s.stageFile(ctx, parent, []byte(working))
		if err != nil {
			return invocationdomain.Outcome{}, err
		}
		oldHash := sha256.Sum256(before)
		// The script is constant and all target names/hash are argv, never shell
		// interpolated. sha256 check guards ordinary competing edits.
		fixed := "test -f \"$1\" && test ! -L \"$1\" && test \"$(sha256sum \"$1\" | cut -d' ' -f1)\" = \"$3\" && mv -f -- \"$2\" \"$1\""
		code, err := s.runFixed(ctx, []string{"/bin/sh", "-c", fixed, "mender", dest, stage, hex.EncodeToString(oldHash[:])})
		s.cleanupStage(ctx, stage)
		if err != nil {
			return invocationdomain.Outcome{}, err
		}
		if code != 0 {
			return dockerFailure(fmt.Errorf("edit conflict: file changed before atomic replace"))
		}
		return dockerComplete(editResult{Path: rel, Operation: "replace", Replacements: count, BytesBefore: int64(len(before)), BytesAfter: int64(len(working))})
	case "delete":
		if input.Content != "" || len(input.Edits) != 0 {
			return dockerFailure(fmt.Errorf("delete cannot include edits or content"))
		}
		data, err := s.readFile(ctx, rel, maxDockerEditBytes)
		if err != nil {
			return sanitizedFailure(ctx, err)
		}
		code, err := s.runFixed(ctx, []string{"/bin/sh", "-c", "test -f \"$1\" && test ! -L \"$1\" && rm -- \"$1\"", "mender", dest})
		if err != nil {
			return invocationdomain.Outcome{}, err
		}
		if code != 0 {
			return dockerFailure(fmt.Errorf("delete conflict or permission denied"))
		}
		return dockerComplete(editResult{Path: rel, Operation: "delete", BytesBefore: int64(len(data))})
	default:
		return dockerFailure(fmt.Errorf("unknown edit operation"))
	}
}
func (s *workspaceSession) search(ctx context.Context, args invocationdomain.JSONValue) (invocationdomain.Outcome, error) {
	var input searchArguments
	if err := decodeDockerArgs(args.Bytes(), &input); err != nil {
		return dockerFailure(err)
	}
	if input.Path == "" {
		input.Path = "."
	}
	rel, err := relativePath(input.Path, true)
	if err != nil {
		return dockerFailure(err)
	}
	if input.MaxResults == 0 {
		input.MaxResults = 100
	}
	if input.MaxResults < 1 || input.MaxResults > 500 || len(input.Query) > 4096 || len(input.Pattern) > 1024 {
		return dockerFailure(fmt.Errorf("search limits exceeded"))
	}
	if input.Mode == "files" && input.Pattern == "" {
		return dockerFailure(fmt.Errorf("files search requires pattern"))
	}
	if input.Mode == "content" && input.Query == "" {
		return dockerFailure(fmt.Errorf("content search requires query"))
	}
	if input.Mode != "files" && input.Mode != "content" {
		return dockerFailure(fmt.Errorf("unknown search mode"))
	}
	var pattern *regexp.Regexp
	if input.Pattern != "" {
		pattern, err = compileDockerGlob(input.Pattern)
		if err != nil {
			return dockerFailure(err)
		}
	}
	matchContent := func(string) bool { return false }
	if input.Mode == "content" {
		if input.Regex {
			expression := input.Query
			if !input.CaseSensitive {
				expression = "(?i)" + expression
			}
			compiled, err := regexp.Compile(expression)
			if err != nil {
				return dockerFailure(fmt.Errorf("invalid content regex"))
			}
			matchContent = compiled.MatchString
		} else if input.CaseSensitive {
			matchContent = func(line string) bool { return strings.Contains(line, input.Query) }
		} else {
			query := strings.ToLower(input.Query)
			matchContent = func(line string) bool { return strings.Contains(strings.ToLower(line), query) }
		}
	}
	entries, truncated, err := s.listFilePaths(ctx, rel)
	if err != nil {
		return sanitizedFailure(ctx, err)
	}
	result := searchResult{Mode: input.Mode, Matches: make([]searchMatch, 0), Truncated: truncated}
	for _, filename := range entries {
		if pattern != nil && !pattern.MatchString(filename) {
			continue
		}
		if input.Mode == "files" {
			result.Matches = append(result.Matches, searchMatch{Path: filename})
		}
		if input.Mode == "content" {
			data, readErr := s.readFile(ctx, filename, 1024*1024)
			if readErr != nil || !utf8.Valid(data) {
				continue
			}
			for index, line := range splitDockerLines(string(data)) {
				if matchContent(line) {
					if len(line) > 2048 {
						line = string([]rune(line)[:min(len([]rune(line)), 512)])
					}
					result.Matches = append(result.Matches, searchMatch{Path: filename, Line: index + 1, Text: line})
				}
				if len(result.Matches) >= input.MaxResults {
					break
				}
			}
		}
		if len(result.Matches) >= input.MaxResults {
			result.Truncated = true
			break
		}
	}
	return dockerComplete(result)
}
func compileDockerGlob(glob string) (*regexp.Regexp, error) {
	if glob == "" || strings.ContainsRune(glob, '\x00') || strings.HasPrefix(glob, "/") || strings.Contains(glob, "../") {
		return nil, fmt.Errorf("invalid workspace glob")
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
