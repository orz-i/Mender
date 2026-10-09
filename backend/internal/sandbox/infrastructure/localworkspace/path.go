package localworkspace

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const maxWorkspacePathBytes = 4096

type workspacePathResolver struct {
	root string
}

func newWorkspacePathResolver(root string) (workspacePathResolver, error) {
	root = filepath.Clean(root)
	if !filepath.IsAbs(root) {
		return workspacePathResolver{}, fmt.Errorf("workspace root must be absolute")
	}
	info, err := os.Stat(root)
	if err != nil {
		return workspacePathResolver{}, fmt.Errorf("workspace root: %w", err)
	}
	if !info.IsDir() {
		return workspacePathResolver{}, fmt.Errorf("workspace root must be a directory")
	}
	return workspacePathResolver{root: root}, nil
}

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

func (r workspacePathResolver) resolveExisting(
	raw string,
	allowRoot bool,
) (string, string, fs.FileInfo, error) {
	relative, err := normalizeRelativePath(raw, allowRoot)
	if err != nil {
		return "", "", nil, err
	}
	path, info, err := r.walk(relative, false)
	if err != nil {
		return "", "", nil, err
	}
	return relative, path, info, nil
}

func (r workspacePathResolver) resolveCreate(raw string) (string, string, error) {
	relative, err := normalizeRelativePath(raw, false)
	if err != nil {
		return "", "", err
	}
	path, info, err := r.walk(relative, true)
	if err != nil {
		return "", "", err
	}
	if info != nil {
		return "", "", fmt.Errorf("path already exists")
	}
	return relative, path, nil
}

func (r workspacePathResolver) walk(
	relative string,
	allowMissingFinal bool,
) (string, fs.FileInfo, error) {
	if relative == "." {
		info, err := os.Lstat(r.root)
		return r.root, info, err
	}

	parts := strings.Split(filepath.FromSlash(relative), string(filepath.Separator))
	current := r.root
	for index, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		final := index == len(parts)-1
		if errors.Is(err, os.ErrNotExist) && final && allowMissingFinal {
			if err := ensureWithinRoot(r.root, current); err != nil {
				return "", nil, err
			}
			return current, nil, nil
		}
		if err != nil {
			return "", nil, fmt.Errorf("inspect workspace path %q: %w", relative, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", nil, fmt.Errorf("workspace path %q contains a symlink", relative)
		}
		if !final && !info.IsDir() {
			return "", nil, fmt.Errorf("workspace path %q contains a non-directory parent", relative)
		}
	}
	if err := ensureWithinRoot(r.root, current); err != nil {
		return "", nil, err
	}
	info, err := os.Lstat(current)
	if err != nil {
		return "", nil, fmt.Errorf("inspect workspace path %q: %w", relative, err)
	}
	return current, info, nil
}

func ensureWithinRoot(root, candidate string) error {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return fmt.Errorf("check workspace containment: %w", err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) ||
		filepath.IsAbs(relative) {
		return fmt.Errorf("path escapes the workspace")
	}
	return nil
}

func regularFile(info fs.FileInfo) error {
	if info == nil {
		return fmt.Errorf("file does not exist")
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("path must be a regular file")
	}
	return nil
}
