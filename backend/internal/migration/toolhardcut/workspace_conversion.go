package toolhardcut

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// insertVerifiedWorkspaces retains distinct Workspace identities for each
// approved legacy Provider alias and exact Sandbox placement. It creates no
// Profile grant; old per-facade ToolBinding scope is never inferred from a
// global Sandbox grant.
func insertVerifiedWorkspaces(ctx context.Context, tx *sql.Tx, report Report) error {
	for _, item := range report.WorkspacePlacements {
		// Agent workspaces require an explicit grant/CWD migration and cannot
		// enter this function without first clearing the dry-run blockers.
		if item.Verification != "verified_local_root" || item.SandboxID == "" {
			return fmt.Errorf("hard cut: unverified Workspace placement %s", item.PlannedWorkspaceID)
		}
		canonical := plannedWorkspaceID(item.Source, item.WorkspaceKey, item.SandboxID)
		if item.PlannedWorkspaceID != canonical {
			return fmt.Errorf("hard cut: Workspace identity changed in manifest")
		}
		stillVerified, err := attestLocalSandboxRoot(ctx, tx, item.SandboxID)
		if err != nil {
			return err
		}
		if !stillVerified {
			return fmt.Errorf("hard cut: Workspace root changed before commit")
		}
		var ownerKind, ownerRef, root string
		var generation uint64
		err = tx.QueryRowContext(ctx, `SELECT s.owner_kind,s.owner_ref,s.local_root,r.generation
    FROM sandboxes s JOIN sandbox_runs r ON r.sandbox_id=s.id WHERE s.id=?`,
			item.SandboxID).Scan(&ownerKind, &ownerRef, &root, &generation)
		if err != nil {
			return err
		}
		if generation == 0 {
			return fmt.Errorf("hard cut: missing Sandbox run generation")
		}
		stamp := time.Now().UTC().Format(time.RFC3339Nano)
		_, err = tx.ExecContext(ctx, `INSERT INTO workspaces
   (workspace_id,owner_kind,owner_ref,display_name,verified_root,created_at)
   VALUES(?,?,?,?,?,?)`, canonical, ownerKind, ownerRef, item.WorkspaceKey, root, stamp)
		if err != nil {
			return fmt.Errorf("hard cut: register verified Workspace: %w", err)
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO workspace_placements
   (workspace_id,sandbox_id,backend_root,verified_revision)
   VALUES(?,?,?,?)`, canonical, item.SandboxID, root, generation)
		if err != nil {
			return fmt.Errorf("hard cut: bind verified Workspace placement: %w", err)
		}
	}
	return nil
}
