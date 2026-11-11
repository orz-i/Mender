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

	workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"
)

const (
	maxSearchResults        = 500
	maxSearchVisitedFiles   = 10000
	maxSearchFileBytes      = 1024 * 1024
	maxSearchAggregateBytes = 32 * 1024 * 1024
	maxSearchSnippetBytes   = 2048
	maxSearchQueryBytes     = 4096
	maxSearchPatternBytes   = 1024
)

func (i *Instance) Search(
	ctx context.Context,
	input workspace.SearchRequest,
) (workspace.SearchResult, error) {
	if err := i.ready(ctx); err != nil {
		return workspace.SearchResult{}, err
	}
	if input.Path == "" {
		input.Path = "."
	}
	if input.MaxResults == 0 {
		input.MaxResults = 100
	}
	if input.MaxResults < 1 || input.MaxResults > maxSearchResults {
		return workspace.SearchResult{}, toolFailure(fmt.Errorf("maxResults must be between 1 and %d", maxSearchResults))
	}
	if len(input.Query) > maxSearchQueryBytes {
		return workspace.SearchResult{}, toolFailure(fmt.Errorf("query must be at most %d bytes", maxSearchQueryBytes))
	}
	if len(input.Pattern) > maxSearchPatternBytes {
		return workspace.SearchResult{}, toolFailure(fmt.Errorf("pattern must be at most %d bytes", maxSearchPatternBytes))
	}

	searchRelative, _, info, err := i.resolver.resolveExisting(input.Path, true)
	if err != nil {
		return workspace.SearchResult{}, toolFailure(err)
	}
	if !info.IsDir() {
		return workspace.SearchResult{}, toolFailure(fmt.Errorf("search path must be a directory"))
	}

	var pattern *regexp.Regexp
	if input.Pattern != "" {
		pattern, err = compileWorkspaceGlob(input.Pattern)
		if err != nil {
			return workspace.SearchResult{}, toolFailure(err)
		}
	}

	output := workspace.SearchResult{Mode: input.Mode, Matches: make([]workspace.SearchMatch, 0)}
	switch input.Mode {
	case "files":
		if input.Pattern == "" {
			return workspace.SearchResult{}, toolFailure(fmt.Errorf("files search requires pattern"))
		}
	case "content":
		if input.Query == "" {
			return workspace.SearchResult{}, toolFailure(fmt.Errorf("content search requires query"))
		}
	default:
		return workspace.SearchResult{}, toolFailure(fmt.Errorf("unsupported search mode %q", input.Mode))
	}

	contentMatcher, err := compileContentMatcher(input)
	if err != nil {
		return workspace.SearchResult{}, toolFailure(err)
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
			output.Matches = append(output.Matches, workspace.SearchMatch{Path: workspaceRelative})
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
			output.Matches = append(output.Matches, workspace.SearchMatch{
				Path: workspaceRelative,
				Line: lineIndex + 1,
				Text: workspace.OptionalText(truncateUTF8(line, maxSearchSnippetBytes)),
			})
			if len(output.Matches) >= input.MaxResults {
				output.Truncated = true
				return fs.SkipAll
			}
		}
		return nil
	})
	if walkErr != nil {
		return workspace.SearchResult{}, toolFailure(fmt.Errorf("search workspace %q: %w", searchRelative, walkErr))
	}
	return output, nil
}

func compileContentMatcher(input workspace.SearchRequest) (func(string) bool, error) {
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
