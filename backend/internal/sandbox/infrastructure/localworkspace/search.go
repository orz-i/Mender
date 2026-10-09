package localworkspace

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
)

const (
	defaultSearchResults    = 100
	maxSearchResults        = 500
	maxSearchVisitedFiles   = 10000
	maxSearchFileBytes      = 1024 * 1024
	maxSearchAggregateBytes = 32 * 1024 * 1024
	maxSearchSnippetBytes   = 2048
	maxSearchQueryBytes     = 4096
	maxSearchPatternBytes   = 1024
)

type searchInput struct {
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

type searchOutput struct {
	Mode      string        `json:"mode"`
	Matches   []searchMatch `json:"matches"`
	Truncated bool          `json:"truncated"`
}

func (i *Instance) executeSearch(
	ctx context.Context,
	arguments invocationdomain.JSONValue,
) (invocationdomain.Outcome, error) {
	var input searchInput
	if err := decodeStrict(arguments.Bytes(), &input, "decode workspace.search arguments"); err != nil {
		return toolFailure(err)
	}
	if input.MaxResults == 0 {
		input.MaxResults = defaultSearchResults
	}
	if input.MaxResults < 1 || input.MaxResults > maxSearchResults {
		return toolFailure(fmt.Errorf("maxResults must be between 1 and %d", maxSearchResults))
	}
	if len(input.Query) > maxSearchQueryBytes {
		return toolFailure(fmt.Errorf("query must be at most %d bytes", maxSearchQueryBytes))
	}
	if len(input.Pattern) > maxSearchPatternBytes {
		return toolFailure(fmt.Errorf("pattern must be at most %d bytes", maxSearchPatternBytes))
	}
	if input.Path == "" {
		input.Path = "."
	}

	searchRelative, _, info, err := i.resolver.resolveExisting(input.Path, true)
	if err != nil {
		return toolFailure(err)
	}
	if !info.IsDir() {
		return toolFailure(fmt.Errorf("search path must be a directory"))
	}

	var pattern *regexp.Regexp
	if input.Pattern != "" {
		pattern, err = compileWorkspaceGlob(input.Pattern)
		if err != nil {
			return toolFailure(err)
		}
	}

	output := searchOutput{Mode: input.Mode, Matches: make([]searchMatch, 0)}
	switch input.Mode {
	case "files":
		if input.Pattern == "" {
			return toolFailure(fmt.Errorf("files search requires pattern"))
		}
	case "content":
		if input.Query == "" {
			return toolFailure(fmt.Errorf("content search requires query"))
		}
	default:
		return toolFailure(fmt.Errorf("unsupported search mode %q", input.Mode))
	}

	contentMatcher, err := compileContentMatcher(input)
	if err != nil {
		return toolFailure(err)
	}

	visitedFiles := 0
	aggregateBytes := int64(0)
	walkErr := fs.WalkDir(i.fsRoot.FS(), searchRelative, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if path == searchRelative {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return fs.SkipDir
			}
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		visitedFiles++
		if visitedFiles > maxSearchVisitedFiles {
			output.Truncated = true
			return fs.SkipAll
		}

		workspaceRelative := filepath.ToSlash(path)
		if pattern != nil && !pattern.MatchString(workspaceRelative) {
			return nil
		}

		if input.Mode == "files" {
			output.Matches = append(output.Matches, searchMatch{Path: workspaceRelative})
			if len(output.Matches) >= input.MaxResults {
				output.Truncated = true
				return fs.SkipAll
			}
			return nil
		}

		if info.Size() > maxSearchFileBytes {
			return nil
		}
		if aggregateBytes+info.Size() > maxSearchAggregateBytes {
			output.Truncated = true
			return fs.SkipAll
		}
		aggregateBytes += info.Size()
		data, err := i.fsRoot.ReadFile(filepath.FromSlash(workspaceRelative))
		if err != nil {
			return err
		}
		if !utf8.Valid(data) {
			return nil
		}
		for lineIndex, line := range splitTextLines(string(data)) {
			if !contentMatcher(line) {
				continue
			}
			output.Matches = append(output.Matches, searchMatch{
				Path: workspaceRelative,
				Line: lineIndex + 1,
				Text: truncateUTF8(line, maxSearchSnippetBytes),
			})
			if len(output.Matches) >= input.MaxResults {
				output.Truncated = true
				return fs.SkipAll
			}
		}
		return nil
	})
	if walkErr != nil {
		return toolFailure(fmt.Errorf("search workspace %q: %w", searchRelative, walkErr))
	}
	return completeJSON(output)
}

func compileContentMatcher(input searchInput) (func(string) bool, error) {
	if input.Mode != "content" {
		return func(string) bool { return false }, nil
	}
	if input.Regex {
		expression := input.Query
		if !input.CaseSensitive {
			expression = "(?i)" + expression
		}
		compiled, err := regexp.Compile(expression)
		if err != nil {
			return nil, fmt.Errorf("invalid search regex: %w", err)
		}
		return compiled.MatchString, nil
	}
	query := input.Query
	if input.CaseSensitive {
		return func(line string) bool { return strings.Contains(line, query) }, nil
	}
	query = strings.ToLower(query)
	return func(line string) bool {
		return strings.Contains(strings.ToLower(line), query)
	}, nil
}

func compileWorkspaceGlob(pattern string) (*regexp.Regexp, error) {
	pattern = filepath.ToSlash(strings.TrimSpace(pattern))
	if pattern == "" {
		return nil, fmt.Errorf("pattern must not be empty")
	}
	if strings.ContainsRune(pattern, 0) {
		return nil, fmt.Errorf("pattern must not contain NUL")
	}

	var builder strings.Builder
	builder.WriteByte('^')
	for index := 0; index < len(pattern); {
		switch pattern[index] {
		case '*':
			if index+1 < len(pattern) && pattern[index+1] == '*' {
				if index+2 < len(pattern) && pattern[index+2] == '/' {
					builder.WriteString("(?:.*/)?")
					index += 3
					continue
				}
				builder.WriteString(".*")
				index += 2
			} else {
				builder.WriteString("[^/]*")
				index++
			}
		case '?':
			builder.WriteString("[^/]")
			index++
		default:
			builder.WriteString(regexp.QuoteMeta(string(pattern[index])))
			index++
		}
	}
	builder.WriteByte('$')
	compiled, err := regexp.Compile(builder.String())
	if err != nil {
		return nil, fmt.Errorf("invalid search pattern: %w", err)
	}
	return compiled, nil
}

func truncateUTF8(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	end := maxBytes
	for end > 0 && !utf8.ValidString(value[:end]) {
		end--
	}
	return value[:end]
}
