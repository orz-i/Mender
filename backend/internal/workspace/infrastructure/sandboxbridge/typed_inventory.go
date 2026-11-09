package sandboxbridge

import (
	"context"
	"database/sql"
	"fmt"

	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governance "github.com/orz-i/Mender/backend/internal/governance/domain"
	profile "github.com/orz-i/Mender/backend/internal/profile/domain"
	sandboxapp "github.com/orz-i/Mender/backend/internal/sandbox/application"
	workspaceapp "github.com/orz-i/Mender/backend/internal/workspace/application"
	workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"
)

// TypedInventory is one read-only projection of the Stage E Workspace,
// Placement and Profile TargetScope authority. It never creates Workspaces,
// mounts a root, activates a Sandbox or grants any tool. The caller owns the
// v3 SQLite handle. The GuardedRouter and atomic typedlease recheck rights at
// actual acquisition; these reads are only for safe target selection.
type TypedInventory struct{ db *sql.DB }

func NewTypedInventory(ctx context.Context, db *sql.DB) (*TypedInventory, error) {
	if db == nil {
		return nil, workspaceapp.ErrWorkspaceUnavailable
	}
	for _, query := range []string{
		"SELECT workspace_id,owner_kind,owner_ref FROM workspaces LIMIT 0",
		"SELECT workspace_id,sandbox_id,backend_root FROM workspace_placements LIMIT 0",
		"SELECT workspace_id,profile_id FROM workspace_profile_grants LIMIT 0",
		"SELECT profile_id,tool_id,workspace_id,sandbox_id FROM profile_tool_target_scopes LIMIT 0",
	} {
		if _, err := db.ExecContext(ctx, query); err != nil {
			return nil, fmt.Errorf("typed workspace inventory: %w", err)
		}
	}
	return &TypedInventory{db: db}, nil
}

// serviceID accepts only the exact authenticated MCP Profile principal. A
// nominal ProfileID in arbitrary caller metadata is not a grant.
func (i *TypedInventory) callerScope(caller governance.Context) (profileID string, isOwner bool, err error) {
	if i == nil || i.db == nil || caller.Validate() != nil {
		return "", false, workspaceapp.ErrWorkspaceUnavailable
	}
	if id, ok := sandboxapp.ProfileServiceID(caller); ok {
		return id, false, nil
	}
	if caller.Scope().Kind() != governance.ScopeCatalog || caller.Principal().Kind() != governance.PrincipalLocalUser {
		return "", false, workspaceapp.ErrWorkspaceUnavailable
	}
	switch caller.Origin() {
	case governance.OriginCLI, governance.OriginDesktop, governance.OriginManagementHTTP:
		return "", true, nil
	default:
		return "", false, workspaceapp.ErrWorkspaceUnavailable
	}
}

func (i *TypedInventory) ListAccessibleWorkspaces(ctx context.Context, caller governance.Context) ([]workspace.Ref, error) {
	id, owner, err := i.callerScope(caller)
	if err != nil {
		return nil, err
	}
	var rows *sql.Rows
	if owner {
		rows, err = i.db.QueryContext(ctx, `SELECT workspace_id FROM workspaces
   WHERE owner_kind=? AND owner_ref=? ORDER BY workspace_id LIMIT 257`,
			string(caller.Principal().Kind()), caller.Principal().Reference())
	} else {
		rows, err = i.db.QueryContext(ctx, `SELECT w.workspace_id FROM workspaces w
   JOIN workspace_profile_grants g ON g.workspace_id=w.workspace_id
   WHERE g.profile_id=? ORDER BY w.workspace_id LIMIT 257`, id)
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []workspace.Ref{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		ref, err := workspace.ParseRef(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) > 256 {
		return nil, workspaceapp.ErrWorkspaceUnavailable
	}
	return out, nil
}

// No default Workspace table exists in the Stage E approved schema. Return
// none instead of inventing a host path, pin or global preference.
func (*TypedInventory) DefaultWorkspace(context.Context, governance.Context) (workspace.Ref, bool, error) {
	return "", false, nil
}

func (i *TypedInventory) MayUseWorkspace(ctx context.Context, caller governance.Context, ref workspace.Ref) (bool, error) {
	if err := ref.Validate(); err != nil {
		return false, err
	}
	id, owner, err := i.callerScope(caller)
	if err != nil {
		return false, err
	}
	var exists int
	if owner {
		err = i.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM workspaces
   WHERE workspace_id=? AND owner_kind=? AND owner_ref=?)`, ref.String(),
			string(caller.Principal().Kind()), caller.Principal().Reference()).Scan(&exists)
	} else {
		err = i.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM workspaces w
   JOIN workspace_profile_grants g ON g.workspace_id=w.workspace_id
   WHERE w.workspace_id=? AND g.profile_id=?)`, ref.String(), id).Scan(&exists)
	}
	return exists == 1, err
}

func (i *TypedInventory) ListWorkspacePlacements(ctx context.Context, ref workspace.Ref) ([]workspaceapp.PlacementCandidate, error) {
	if i == nil || i.db == nil || ref.Validate() != nil {
		return nil, workspaceapp.ErrWorkspaceUnavailable
	}
	rows, err := i.db.QueryContext(ctx, `SELECT sandbox_id,backend_root FROM workspace_placements
  WHERE workspace_id=? ORDER BY sandbox_id LIMIT 257`, ref.String())
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []workspaceapp.PlacementCandidate{}
	for rows.Next() {
		var sandboxID, root string
		if err := rows.Scan(&sandboxID, &root); err != nil {
			return nil, err
		}
		out = append(out, workspaceapp.PlacementCandidate{Workspace: ref, SandboxID: sandboxID, TargetRoot: root})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) > 256 {
		return nil, workspaceapp.ErrPlacementUnavailable
	}
	return out, nil
}

func (i *TypedInventory) BindingTargetScope(ctx context.Context, profileID string, toolID catalog.ToolID) (profile.TargetScope, bool, error) {
	if i == nil || i.db == nil {
		return profile.TargetScope{}, false, workspaceapp.ErrWorkspaceUnavailable
	}
	if _, err := profile.ParseProfileID(profileID); err != nil {
		return profile.TargetScope{}, false, nil
	}
	if _, err := catalog.ParseToolID(toolID.String()); err != nil {
		return profile.TargetScope{}, false, nil
	}
	rows, err := i.db.QueryContext(ctx, `SELECT s.workspace_id,s.sandbox_id
  FROM profile_tool_target_scopes s JOIN profile_tools b ON b.profile_id=s.profile_id AND b.tool_id=s.tool_id
  JOIN tools t ON t.id=b.tool_id
  WHERE s.profile_id=? AND s.tool_id=? AND t.source_kind='native' AND t.source_id='workspace'
  ORDER BY s.workspace_id,s.sandbox_id LIMIT 257`, profileID, toolID.String())
	if err != nil {
		return profile.TargetScope{}, false, err
	}
	defer func() { _ = rows.Close() }()
	targets := []profile.ExecutionTarget{}
	for rows.Next() {
		var w, s string
		if err := rows.Scan(&w, &s); err != nil {
			return profile.TargetScope{}, false, err
		}
		target, err := profile.NewExecutionTarget(w, s)
		if err != nil || w == "" {
			return profile.TargetScope{}, false, workspaceapp.ErrWorkspaceUnavailable
		}
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return profile.TargetScope{}, false, err
	}
	if len(targets) == 0 {
		return profile.TargetScope{}, false, nil
	}
	scope, err := profile.NewTargetScope(targets)
	if err != nil {
		return profile.TargetScope{}, false, err
	}
	return scope, true, nil
}

var _ workspaceapp.WorkspaceInventory = (*TypedInventory)(nil)
var _ workspaceapp.PlacementReader = (*TypedInventory)(nil)
var _ WorkspaceAuthority = (*TypedInventory)(nil)
var _ BindingScopeLookup = (*TypedInventory)(nil)
