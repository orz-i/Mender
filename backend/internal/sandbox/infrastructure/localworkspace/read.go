package localworkspace

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"
)

const (
	maxReadLines     = 2000
	maxReadFileBytes = 2 * 1024 * 1024
)

func (i *Instance) Read(
	ctx context.Context,
	input workspace.ReadRequest,
) (workspace.ReadResult, error) {
	if err := i.ready(ctx); err != nil {
		return workspace.ReadResult{}, err
	}
	if input.Offset == 0 {
		input.Offset = 1
	}
	if input.Limit == 0 {
		input.Limit = 200
	}
	if input.Offset < 1 {
		return workspace.ReadResult{}, toolFailure(fmt.Errorf("offset must be at least 1"))
	}
	if input.Limit < 1 || input.Limit > maxReadLines {
		return workspace.ReadResult{}, toolFailure(fmt.Errorf("limit must be between 1 and %d", maxReadLines))
	}

	relative, _, info, err := i.resolver.resolveExisting(input.Path, false)
	if err != nil {
		return workspace.ReadResult{}, toolFailure(err)
	}
	if err := regularFile(info); err != nil {
		return workspace.ReadResult{}, toolFailure(err)
	}
	if info.Size() > maxReadFileBytes {
		return workspace.ReadResult{}, toolFailure(fmt.Errorf("file exceeds %d byte read limit", maxReadFileBytes))
	}

	data, err := i.fsRoot.ReadFile(filepath.FromSlash(relative))
	if err != nil {
		return workspace.ReadResult{}, toolFailure(fmt.Errorf("read workspace file: %w", err))
	}
	if !utf8.Valid(data) {
		return workspace.ReadResult{}, toolFailure(fmt.Errorf("workspace.read supports UTF-8 text files only"))
	}

	lines := splitTextLines(string(data))
	startIndex := input.Offset - 1
	if startIndex >= len(lines) {
		return workspace.ReadResult{
			Path: relative,
		}, nil
	}
	endIndex := min(startIndex+input.Limit, len(lines))
	selected := lines[startIndex:endIndex]
	return workspace.ReadResult{
		Path:      relative,
		Content:   strings.Join(selected, "\n"),
		StartLine: startIndex + 1,
		EndLine:   endIndex,
		Truncated: endIndex < len(lines),
	}, nil
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
