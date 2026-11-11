package domain

import (
	"fmt"
	"path"
	"strings"
)

const maxWorkspacePathBytes = 4096

// NormalizeRelativePath is the shared Workspace path-confinement rule for
// argument-derived Policy facts and typed backend operations. It never grants
// filesystem access; the selected backend must enforce confinement again.
// The canonical Workspace path protocol strictly uses forward slashes, forbids
// Windows drive prefixes, colons (NTFS Alternate Data Streams), backslashes,
// and control characters regardless of host platform.
func NormalizeRelativePath(raw string, allowRoot bool) (string, error) {
	if raw == "" && allowRoot {
		return ".", nil
	}
	if raw != strings.TrimSpace(raw) {
		return "", fmt.Errorf("path must not contain surrounding whitespace")
	}
	if raw == "" {
		return "", fmt.Errorf("path must not be empty")
	}
	if len(raw) > maxWorkspacePathBytes {
		return "", fmt.Errorf("path must be at most %d bytes", maxWorkspacePathBytes)
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] < 32 {
			return "", fmt.Errorf("path must not contain control characters")
		}
	}
	if strings.Contains(raw, `\`) {
		return "", fmt.Errorf("path must use forward slashes only")
	}
	if strings.Contains(raw, ":") {
		return "", fmt.Errorf("path must not contain colons")
	}
	if strings.HasPrefix(raw, "/") {
		return "", fmt.Errorf("path must be workspace-relative")
	}
	cleaned := path.Clean(raw)
	if cleaned == "." {
		if allowRoot {
			return ".", nil
		}
		return "", fmt.Errorf("path must identify a workspace entry")
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("path must not escape the workspace")
	}
	return cleaned, nil
}
