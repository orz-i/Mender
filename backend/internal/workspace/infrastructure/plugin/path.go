package plugin

import (
	"fmt"
	"path/filepath"
	"strings"
)

const maxWorkspacePathBytes = 4096

func normalizeRelativePath(raw string, allowRoot bool) (string, error) {
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
	hostPath := filepath.FromSlash(raw)
	if filepath.IsAbs(hostPath) || filepath.VolumeName(hostPath) != "" {
		return "", fmt.Errorf("path must be workspace-relative")
	}
	cleaned := filepath.Clean(hostPath)
	if cleaned == "." {
		if allowRoot {
			return ".", nil
		}
		return "", fmt.Errorf("path must identify a workspace entry")
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path must not escape the workspace")
	}
	return filepath.ToSlash(cleaned), nil
}
