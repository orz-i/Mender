package localworkspace

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
)

const (
	defaultReadLimit = 200
	maxReadLines     = 2000
	maxReadFileBytes = 2 * 1024 * 1024
)

type readInput struct {
	Path   string `json:"path"`
	Offset int    `json:"offset,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

type readOutput struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
	Truncated bool   `json:"truncated"`
}

func (i *Instance) executeRead(
	ctx context.Context,
	arguments invocationdomain.JSONValue,
) (invocationdomain.Outcome, error) {
	select {
	case <-ctx.Done():
		return invocationdomain.Outcome{}, ctx.Err()
	default:
	}

	var input readInput
	if err := decodeStrict(arguments.Bytes(), &input, "decode workspace.read arguments"); err != nil {
		return toolFailure(err)
	}
	if input.Offset == 0 {
		input.Offset = 1
	}
	if input.Limit == 0 {
		input.Limit = defaultReadLimit
	}
	if input.Offset < 1 {
		return toolFailure(fmt.Errorf("offset must be at least 1"))
	}
	if input.Limit < 1 || input.Limit > maxReadLines {
		return toolFailure(fmt.Errorf("limit must be between 1 and %d", maxReadLines))
	}

	relative, _, info, err := i.resolver.resolveExisting(input.Path, false)
	if err != nil {
		return toolFailure(err)
	}
	if err := regularFile(info); err != nil {
		return toolFailure(err)
	}
	if info.Size() > maxReadFileBytes {
		return toolFailure(fmt.Errorf("file exceeds %d byte read limit", maxReadFileBytes))
	}

	data, err := i.fsRoot.ReadFile(filepath.FromSlash(relative))
	if err != nil {
		return toolFailure(fmt.Errorf("read workspace file: %w", err))
	}
	if !utf8.Valid(data) {
		return toolFailure(fmt.Errorf("workspace.read supports UTF-8 text files only"))
	}

	lines := splitTextLines(string(data))
	startIndex := input.Offset - 1
	if startIndex >= len(lines) {
		return completeJSON(readOutput{
			Path: relative,
		})
	}
	endIndex := min(startIndex+input.Limit, len(lines))
	selected := lines[startIndex:endIndex]
	return completeJSON(readOutput{
		Path:      relative,
		Content:   strings.Join(selected, "\n"),
		StartLine: startIndex + 1,
		EndLine:   endIndex,
		Truncated: endIndex < len(lines),
	})
}

func splitTextLines(value string) []string {
	if value == "" {
		return nil
	}
	value = strings.TrimSuffix(value, "\n")
	if value == "" {
		return []string{""}
	}
	return strings.Split(value, "\n")
}
