package dockerengine

import (
	"context"
	"fmt"
	"path"
	"strings"
	"unicode/utf8"

	workspacedomain "github.com/orz-i/Mender/backend/internal/workspace/domain"
)

const maxDockerPathBytes = 4096
const maxDockerReadBytes = 2 * 1024 * 1024
const maxDockerEditBytes = 4 * 1024 * 1024
const maxDockerSearchBytes = 32 * 1024 * 1024
const maxDockerCommandBytes = 64 * 1024
const maxDockerOutputBytes = 64 * 1024

func relativePath(raw string, allowRoot bool) (string, error) {
	if raw == "" && allowRoot {
		return ".", nil
	}
	if raw == "" || raw != strings.TrimSpace(raw) || len(raw) > maxDockerPathBytes || strings.ContainsAny(raw, "\x00\\") || strings.HasPrefix(raw, "/") {
		return "", fmt.Errorf("docker workspace path must be a bounded relative POSIX path")
	}
	clean := path.Clean(raw)
	if clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return "", fmt.Errorf("docker workspace path escapes root")
	}
	if clean == "." && !allowRoot {
		return "", fmt.Errorf("docker workspace path must identify a file")
	}
	return clean, nil
}
func containerPath(relative string) string {
	if relative == "." {
		return "/workspace"
	}
	return path.Join("/workspace", relative)
}
func dockerFailure(err error) error {
	if err == nil {
		err = fmt.Errorf("docker Workspace operation failed")
	}
	msg := err.Error()
	if utf8.RuneCountInString(msg) > 1000 {
		msg = string([]rune(msg)[:1000])
	}
	return &workspacedomain.ToolError{Message: msg}
}
func sanitizedFailure(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return dockerFailure(fmt.Errorf("docker Workspace operation unavailable"))
}
func splitDockerLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return []string{""}
	}
	return strings.Split(s, "\n")
}
