package dockerengine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"
	"unicode/utf8"

	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
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
func decodeDockerArgs(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid Docker Workspace arguments: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("docker Workspace arguments have trailing data")
	}
	return nil
}
func dockerComplete(value any) (invocationdomain.Outcome, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return invocationdomain.Outcome{}, err
	}
	result, err := invocationdomain.NewJSONValue(raw)
	if err != nil {
		return invocationdomain.Outcome{}, err
	}
	return invocationdomain.Complete(result)
}
func dockerFailure(err error) (invocationdomain.Outcome, error) {
	if err == nil {
		err = fmt.Errorf("docker Workspace operation failed")
	}
	msg := err.Error()
	if utf8.RuneCountInString(msg) > 1000 {
		msg = string([]rune(msg)[:1000])
	}
	return invocationdomain.Failed(invocationdomain.FailureSourceTool, invocationdomain.FailureToolError, msg)
}
func sanitizedFailure(ctx context.Context, err error) (invocationdomain.Outcome, error) {
	if ctx.Err() != nil {
		return invocationdomain.Outcome{}, ctx.Err()
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
