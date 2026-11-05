package application

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

// LocalWorkspaceVerifier supplies only local filesystem mechanisms. ResolvePath
// resolves symbolic links; the type queries follow links like os.Stat. The
// application retains approved-root equality and lexical containment rules.
// This port does not grant sandboxing or typed Workspace execution authority.
type LocalWorkspaceVerifier interface {
	ResolvePath(context.Context, string) (string, error)
	IsDirectory(context.Context, string) (bool, error)
	IsRegularFile(context.Context, string) (bool, error)
}

type localWorkspaceRules struct{ workspace LocalWorkspaceVerifier }

// canonicalLocalWorkspace is an explicit owner-selected directory, not a
// Sandbox resource or a path provided by an MCP task.
func (s *localWorkspaceRules) canonicalLocalWorkspace(ctx context.Context, root string) (string, error) {
	if !filepath.IsAbs(root) {
		return "", fmt.Errorf("%w: local workspace must be an absolute path", agentdomain.ErrInvalid)
	}
	canonical, err := s.workspace.ResolvePath(ctx, root)
	if err != nil {
		return "", fmt.Errorf("%w: workspace path unavailable: %w", agentdomain.ErrInvalid, err)
	}
	directory, err := s.workspace.IsDirectory(ctx, canonical)
	if err != nil {
		return "", fmt.Errorf("%w: workspace path unavailable: %w", agentdomain.ErrInvalid, err)
	}
	if !directory {
		return "", fmt.Errorf("%w: workspace is not a directory", agentdomain.ErrInvalid)
	}
	return filepath.Clean(canonical), nil
}

// validateRegisteredWorkspaceRoot rechecks the approved root before task
// admission. A replaced symlink cannot silently redirect an old Profile grant.
func (s *localWorkspaceRules) validateRegisteredWorkspaceRoot(ctx context.Context, root string) error {
	canonical, err := s.canonicalLocalWorkspace(ctx, root)
	if err != nil {
		return agentdomain.ErrUnavailable
	}
	if canonical != root {
		return fmt.Errorf("%w: approved workspace root changed", agentdomain.ErrUnavailable)
	}
	return nil
}

// revalidateReadPreview is a last-moment cooperative boundary check, not
// filesystem isolation. OpenCode still executes its own native read and
// could deviate from reported ACP metadata in host mode.
func (s *localWorkspaceRules) revalidateReadPreview(ctx context.Context, root, relative string) error {
	if relative == "" || filepath.IsAbs(relative) || filepath.Clean(relative) != relative ||
		relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) ||
		strings.ContainsAny(relative, "\r\n\x00") {
		return agentdomain.ErrForbidden
	}
	canonicalRoot, err := s.workspace.ResolvePath(ctx, root)
	if err != nil || canonicalRoot != root {
		return agentdomain.ErrForbidden
	}
	canonicalTarget, err := s.workspace.ResolvePath(ctx, filepath.Join(root, relative))
	if err != nil {
		return agentdomain.ErrForbidden
	}
	inside, err := filepath.Rel(root, canonicalTarget)
	if err != nil || inside == "." || inside == ".." ||
		strings.HasPrefix(inside, ".."+string(filepath.Separator)) ||
		filepath.IsAbs(inside) {
		return agentdomain.ErrForbidden
	}
	regular, err := s.workspace.IsRegularFile(ctx, canonicalTarget)
	if err != nil || !regular {
		return agentdomain.ErrForbidden
	}
	return nil
}
