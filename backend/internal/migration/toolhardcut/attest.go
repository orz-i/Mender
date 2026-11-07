package toolhardcut

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// attestLocalSandboxRoot is a side-effect-free, host-side attestation. A
// configured Sandbox is not proof of access on another machine. Only an
// existing canonical local_workspace directory with a valid persistent owner
// is eligible. Stage F still rechecks all permissions and backend generation
// at each Invocation and does not trust this snapshot as a runtime grant.
func attestLocalSandboxRoot(ctx context.Context, tx *sql.Tx, sandboxID string) (bool, error) {
	var backend, root, ownerKind, ownerRef, state string
	err := tx.QueryRowContext(ctx, `SELECT s.backend,s.local_root,s.owner_kind,s.owner_ref,r.state
  FROM sandboxes s JOIN sandbox_runs r ON r.sandbox_id=s.id WHERE s.id=?`,
		sandboxID).Scan(&backend, &root, &ownerKind, &ownerRef, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if backend != "local_workspace" || (state != "ready" && state != "stopped") ||
		ownerKind == "" || strings.TrimSpace(ownerRef) == "" {
		return false, nil
	}
	if root == "" || !filepath.IsAbs(root) || filepath.Clean(root) != root ||
		strings.TrimSpace(root) != root {
		return false, nil
	}
	canonical, err := filepath.EvalSymlinks(root)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if canonical != root {
		return false, nil
	}
	info, err := os.Stat(root)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return info.IsDir(), nil
}
