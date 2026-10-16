package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// WorkspaceToolTarget is one owner-selected alias for an existing Sandbox.
// It never duplicates the canonical Provider or its ToolIDs.
type WorkspaceToolTarget struct {
	Key       string `json:"key"`
	SandboxID string `json:"sandboxId"`
}

// WorkspaceSuiteOwner resolves the one managed default Sandbox for a service.
// It does not infer ownership from arbitrary Provider config or display name.
func (s *Store) WorkspaceSuiteOwner(ctx context.Context, providerID string) (sandboxdomain.ID, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `SELECT sandbox_id FROM sandbox_tool_suite_bindings
 WHERE provider_id=? AND suite_id='sandbox.workspace'`, providerID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", sandboxdomain.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return sandboxdomain.ParseID(id)
}

// ResolveWorkspaceTarget requires an exact managed Workspace Tool Suite binding.
// No data mutation, backend activation, or permissions are inferred by lookup.
func (s *Store) ResolveWorkspaceTarget(ctx context.Context, providerID, key string) (sandboxdomain.ID, error) {
	if err := validateWorkspaceTargetKey(key); err != nil {
		return "", err
	}
	var sandboxID string
	err := s.db.QueryRowContext(ctx, `SELECT t.sandbox_id FROM workspace_tool_targets t
 JOIN sandbox_tool_suite_bindings b ON t.provider_id=b.provider_id AND b.suite_id='sandbox.workspace'
 WHERE t.provider_id=? AND t.workspace_key=?`, providerID, key).Scan(&sandboxID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", sandboxdomain.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return sandboxdomain.ParseID(sandboxID)
}

func validateWorkspaceTargetKey(key string) error {
	if len(key) < 1 || len(key) > 64 || key == "default" {
		return sandboxdomain.ErrConflict
	}
	for _, ch := range key {
		if (ch < 'a' || ch > 'z') && (ch < '0' || ch > '9') && ch != '-' && ch != '_' {
			return sandboxdomain.ErrConflict
		}
	}
	return nil
}

// BindWorkspaceTarget checks ownership and existence atomically with insertion,
// including cross-process changes of the primary Suite binding.
func (s *Store) BindWorkspaceTarget(ctx context.Context, providerID, key string, target sandboxdomain.ID) error {
	if err := validateWorkspaceTargetKey(key); err != nil {
		return err
	}
	if _, err := sandboxdomain.ParseID(target.String()); err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO workspace_tool_targets(provider_id,workspace_key,sandbox_id,created_at)
 SELECT b.provider_id,?,?,? FROM sandbox_tool_suite_bindings b
 JOIN sandboxes source ON source.id=b.sandbox_id
 JOIN sandboxes dest ON dest.id=?
 JOIN sandbox_runs run ON run.sandbox_id=dest.id
 WHERE b.provider_id=? AND b.suite_id='sandbox.workspace'
 AND source.owner_kind=dest.owner_kind AND source.owner_ref=dest.owner_ref
 AND source.id!=dest.id AND run.state='ready'`,
		key, target.String(), time.Now().UTC().Format(time.RFC3339Nano), target.String(), providerID)
	if err != nil {
		return fmt.Errorf("%w: workspace target binding: %w", sandboxdomain.ErrConflict, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return sandboxdomain.ErrConflict
	}
	return nil
}
func (s *Store) ListWorkspaceTargets(ctx context.Context, providerID string) ([]WorkspaceToolTarget, error) {
	// A non-owned/retired Provider has no targets, rather than leaking orphan entries.
	rows, err := s.db.QueryContext(ctx, `SELECT t.workspace_key,t.sandbox_id FROM workspace_tool_targets t
 JOIN sandbox_tool_suite_bindings b ON b.provider_id=t.provider_id AND b.suite_id='sandbox.workspace'
 WHERE t.provider_id=? ORDER BY t.workspace_key`, providerID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := []WorkspaceToolTarget{}
	for rows.Next() {
		var item WorkspaceToolTarget
		if err := rows.Scan(&item.Key, &item.SandboxID); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
func (s *Store) UnbindWorkspaceTarget(ctx context.Context, providerID, key string) error {
	if err := validateWorkspaceTargetKey(key); err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM workspace_tool_targets WHERE provider_id=? AND workspace_key=?`, providerID, key)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return sandboxdomain.ErrNotFound
	}
	return nil
}
