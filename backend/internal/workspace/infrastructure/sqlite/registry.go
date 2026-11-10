package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	profile "github.com/orz-i/Mender/backend/internal/profile/domain"
	sandbox "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	app "github.com/orz-i/Mender/backend/internal/workspace/application"
	workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"
)

// RegistryStore operates only on the already attested v3 connection. It does
// not migrate schemas, create Providers, activate Sandboxes, or grant Tools.
type RegistryStore struct{ db *sql.DB }

func NewRegistryStore(ctx context.Context, db *sql.DB) (*RegistryStore, error) {
	if db == nil {
		return nil, app.ErrRegistryInvalid
	}
	for _, q := range []string{"SELECT workspace_id,owner_kind,owner_ref FROM workspaces LIMIT 0", "SELECT workspace_id,sandbox_id FROM workspace_placements LIMIT 0", "SELECT workspace_id,profile_id FROM workspace_profile_grants LIMIT 0", "SELECT profile_id,tool_id,workspace_id,sandbox_id FROM profile_tool_target_scopes LIMIT 0"} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			return nil, fmt.Errorf("workspace registry schema: %w", err)
		}
	}
	return &RegistryStore{db: db}, nil
}
func failed(err error) error {
	if err == nil {
		return nil
	}
	t := strings.ToLower(err.Error())
	if strings.Contains(t, "constraint failed") || strings.Contains(t, "constraint") || strings.Contains(t, "unique") || strings.Contains(t, "immutable") || strings.Contains(t, "in use") || strings.Contains(t, "unreviewed") {
		return fmt.Errorf("%w: %w", app.ErrRegistryConflict, err)
	}
	return err
}
func checkRef(id workspace.Ref) error {
	if id.Validate() != nil {
		return app.ErrRegistryInvalid
	}
	return nil
}
func checkProfile(id string) error {
	if _, err := profile.ParseProfileID(id); err != nil {
		return app.ErrRegistryInvalid
	}
	return nil
}
func (s *RegistryStore) owner(ctx context.Context, tx *sql.Tx, kind, ref string, id workspace.Ref) (app.Record, error) {
	if checkRef(id) != nil {
		return app.Record{}, app.ErrRegistryInvalid
	}
	var item app.Record
	item.ID = id
	var actualKind, actualRef string
	err := tx.QueryRowContext(ctx, `SELECT owner_kind,owner_ref,display_name,verified_root,created_at FROM workspaces WHERE workspace_id=?`, id.String()).Scan(&actualKind, &actualRef, &item.DisplayName, &item.VerifiedRoot, &item.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return app.Record{}, app.ErrRegistryMissing
	}
	if err != nil {
		return app.Record{}, err
	}
	if actualKind != kind || actualRef != ref {
		return app.Record{}, app.ErrRegistryDenied
	}
	return item, nil
}
func (s *RegistryStore) Register(ctx context.Context, kind, owner string, item app.Record) error {
	if checkRef(item.ID) != nil || item.VerifiedRoot == "" || owner == "" || kind == "" {
		return app.ErrRegistryInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// Take a SQLite write reservation before checking root reuse to avoid races.
	if _, err = tx.ExecContext(ctx, `UPDATE workspaces SET display_name=display_name WHERE owner_kind=? AND owner_ref=?`, kind, owner); err != nil {
		return err
	}
	var exists int
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM workspaces WHERE owner_kind=? AND owner_ref=? AND verified_root=?)`, kind, owner, item.VerifiedRoot).Scan(&exists)
	if err != nil {
		return err
	}
	if exists != 0 {
		return app.ErrRegistryConflict
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO workspaces(workspace_id,owner_kind,owner_ref,display_name,verified_root,created_at) VALUES(?,?,?,?,?,?)`, item.ID.String(), kind, owner, item.DisplayName, item.VerifiedRoot, item.CreatedAt)
	if err != nil {
		return failed(err)
	}
	return tx.Commit()
}
func (s *RegistryStore) List(ctx context.Context, kind, owner string) ([]app.Record, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT workspace_id,display_name,verified_root,created_at FROM workspaces WHERE owner_kind=? AND owner_ref=? ORDER BY workspace_id LIMIT 257`, kind, owner)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []app.Record{}
	for rows.Next() {
		var raw string
		var r app.Record
		if err = rows.Scan(&raw, &r.DisplayName, &r.VerifiedRoot, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.ID, err = workspace.ParseRef(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(out) > 256 {
		return nil, app.ErrRegistryConflict
	}
	return out, nil
}
func (s *RegistryStore) Get(ctx context.Context, k, o string, id workspace.Ref) (app.Record, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return app.Record{}, err
	}
	defer func() { _ = tx.Rollback() }()
	item, err := s.owner(ctx, tx, k, o, id)
	if err != nil {
		return app.Record{}, err
	}
	return item, tx.Commit()
}
func (s *RegistryStore) Delete(ctx context.Context, k, o string, id workspace.Ref) error {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer func() { _ = tx.Rollback() }()
	if _, e = s.owner(ctx, tx, k, o, id); e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `DELETE FROM workspaces WHERE workspace_id=?`, id.String())
	if e != nil {
		return failed(e)
	}
	return tx.Commit()
}
func (s *RegistryStore) Link(ctx context.Context, k, o string, id workspace.Ref, sandboxID string, root string) error {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer func() { _ = tx.Rollback() }()
	item, e := s.owner(ctx, tx, k, o, id)
	if e != nil {
		return e
	}
	if item.VerifiedRoot != root {
		return app.ErrRegistryConflict
	}
	// Recheck the exact owner and local backend under the same SQLite writer.
	var count int
	e = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sandboxes WHERE id=? AND owner_kind=? AND owner_ref=? AND backend='local_workspace' AND local_root=?`, sandboxID, k, o, root).Scan(&count)
	if e != nil {
		return e
	}
	if count != 1 {
		return app.ErrRegistryConflict
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO workspace_placements(workspace_id,sandbox_id,backend_root,verified_revision) VALUES(?,?,?,1)`, id.String(), sandboxID, root)
	if e != nil {
		return failed(e)
	}
	return tx.Commit()
}
func (s *RegistryStore) Unlink(ctx context.Context, k, o string, id workspace.Ref, sandboxID string) error {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer func() { _ = tx.Rollback() }()
	if _, e = s.owner(ctx, tx, k, o, id); e != nil {
		return e
	}
	// Detaching an execution placement must also retire the exact Profile
	// Tool TargetScopes that referred to it. Otherwise re-attaching the same
	// Sandbox could silently restore a previously approved capability.
	if _, e = tx.ExecContext(ctx, `DELETE FROM profile_tool_target_scopes WHERE workspace_id=? AND sandbox_id=?`, id.String(), sandboxID); e != nil {
		return e
	}
	res, e := tx.ExecContext(ctx, `DELETE FROM workspace_placements WHERE workspace_id=? AND sandbox_id=?`, id.String(), sandboxID)
	if e != nil {
		return failed(e)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return app.ErrRegistryMissing
	}
	return tx.Commit()
}
func (s *RegistryStore) Placements(ctx context.Context, k, o string, id workspace.Ref) ([]app.PlacementRecord, error) {
	if _, e := s.Get(ctx, k, o, id); e != nil {
		return nil, e
	}
	rows, e := s.db.QueryContext(ctx, `SELECT sandbox_id,backend_root FROM workspace_placements WHERE workspace_id=? ORDER BY sandbox_id LIMIT 257`, id.String())
	if e != nil {
		return nil, e
	}
	defer func() { _ = rows.Close() }()
	out := []app.PlacementRecord{}
	for rows.Next() {
		var raw, root string
		if e = rows.Scan(&raw, &root); e != nil {
			return nil, e
		}
		if _, er := sandbox.ParseID(raw); er != nil {
			return nil, er
		}
		out = append(out, app.PlacementRecord{WorkspaceID: id, SandboxID: raw, BackendRoot: root})
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	if len(out) > 256 {
		return nil, app.ErrRegistryConflict
	}
	return out, nil
}
func (s *RegistryStore) Grants(ctx context.Context, k, o string, id workspace.Ref) ([]string, error) {
	if _, e := s.Get(ctx, k, o, id); e != nil {
		return nil, e
	}
	rows, e := s.db.QueryContext(ctx, `SELECT profile_id FROM workspace_profile_grants WHERE workspace_id=? ORDER BY profile_id LIMIT 257`, id.String())
	if e != nil {
		return nil, e
	}
	defer func() { _ = rows.Close() }()
	out := []string{}
	for rows.Next() {
		var v string
		if e = rows.Scan(&v); e != nil {
			return nil, e
		}
		if e = checkProfile(v); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	if len(out) > 256 {
		return nil, app.ErrRegistryConflict
	}
	return out, nil
}
func (s *RegistryStore) Grant(ctx context.Context, k, o string, id workspace.Ref, profileID string) error {
	if e := checkProfile(profileID); e != nil {
		return e
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer func() { _ = tx.Rollback() }()
	if _, e = s.owner(ctx, tx, k, o, id); e != nil {
		return e
	}
	var count int
	e = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM profiles WHERE id=?`, profileID).Scan(&count)
	if e != nil {
		return e
	}
	if count != 1 {
		return app.ErrRegistryMissing
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO workspace_profile_grants(workspace_id,profile_id,granted_at) VALUES(?,?,?)`, id.String(), profileID, time.Now().UTC().Format(time.RFC3339Nano))
	if e != nil {
		return failed(e)
	}
	return tx.Commit()
}
func (s *RegistryStore) Revoke(ctx context.Context, k, o string, id workspace.Ref, profileID string) error {
	if e := checkProfile(profileID); e != nil {
		return e
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer func() { _ = tx.Rollback() }()
	if _, e = s.owner(ctx, tx, k, o, id); e != nil {
		return e
	}
	// Prevent regrant from reactivating an old TargetScope without fresh review.
	if _, e = tx.ExecContext(ctx, `DELETE FROM profile_tool_target_scopes WHERE workspace_id=? AND profile_id=?`, id.String(), profileID); e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `DELETE FROM workspace_profile_grants WHERE workspace_id=? AND profile_id=?`, id.String(), profileID)
	if e != nil {
		return e
	}
	return tx.Commit()
}
func (s *RegistryStore) validateTarget(ctx context.Context, tx *sql.Tx, k, o string, t app.TargetRecord) error {
	if e := checkProfile(t.ProfileID); e != nil {
		return e
	}
	if _, e := catalog.ParseToolID(t.ToolID.String()); e != nil {
		return app.ErrRegistryInvalid
	}
	if _, e := sandbox.ParseID(t.SandboxID); e != nil {
		return app.ErrRegistryInvalid
	}
	var kind, source string
	e := tx.QueryRowContext(ctx, `SELECT t.source_kind,t.source_id FROM tools t JOIN profile_tools b ON b.tool_id=t.id WHERE b.profile_id=? AND t.id=?`, t.ProfileID, t.ToolID.String()).Scan(&kind, &source)
	if errors.Is(e, sql.ErrNoRows) {
		return app.ErrRegistryConflict
	}
	if e != nil {
		return e
	}
	if kind != "native" || (source != "workspace" && source != "jobs") {
		return app.ErrRegistryInvalid
	}
	if source == "jobs" && t.WorkspaceID != "" || source == "workspace" && t.WorkspaceID == "" {
		return app.ErrRegistryInvalid
	}
	if t.WorkspaceID != "" {
		ref, e := workspace.ParseRef(t.WorkspaceID)
		if e != nil {
			return app.ErrRegistryInvalid
		}
		if _, e = s.owner(ctx, tx, k, o, ref); e != nil {
			return e
		}
	}
	var count int
	e = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sandboxes WHERE id=? AND owner_kind=? AND owner_ref=?`, t.SandboxID, k, o).Scan(&count)
	if e != nil {
		return e
	}
	if count != 1 {
		return app.ErrRegistryDenied
	}
	return nil
}
func (s *RegistryStore) PutTarget(ctx context.Context, k, o string, t app.TargetRecord) error {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer func() { _ = tx.Rollback() }()
	if e = s.validateTarget(ctx, tx, k, o, t); e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO profile_tool_target_scopes(profile_id,tool_id,workspace_id,sandbox_id) VALUES(?,?,?,?)`, t.ProfileID, t.ToolID.String(), t.WorkspaceID, t.SandboxID)
	if e != nil {
		return failed(e)
	}
	return tx.Commit()
}
func (s *RegistryStore) DeleteTarget(ctx context.Context, k, o string, t app.TargetRecord) error {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer func() { _ = tx.Rollback() }()
	if e = s.validateTarget(ctx, tx, k, o, t); e != nil {
		return e
	}
	res, e := tx.ExecContext(ctx, `DELETE FROM profile_tool_target_scopes WHERE profile_id=? AND tool_id=? AND workspace_id=? AND sandbox_id=?`, t.ProfileID, t.ToolID.String(), t.WorkspaceID, t.SandboxID)
	if e != nil {
		return e
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return app.ErrRegistryMissing
	}
	return tx.Commit()
}
func (s *RegistryStore) Targets(ctx context.Context, k, o, profileID string) ([]app.TargetRecord, error) {
	if e := checkProfile(profileID); e != nil {
		return nil, e
	}
	rows, e := s.db.QueryContext(ctx, `SELECT s.tool_id,s.workspace_id,s.sandbox_id FROM profile_tool_target_scopes s JOIN sandboxes sb ON sb.id=s.sandbox_id WHERE s.profile_id=? AND sb.owner_kind=? AND sb.owner_ref=? ORDER BY s.tool_id,s.workspace_id,s.sandbox_id LIMIT 257`, profileID, k, o)
	if e != nil {
		return nil, e
	}
	defer func() { _ = rows.Close() }()
	out := []app.TargetRecord{}
	for rows.Next() {
		var toolID, workspaceID, sandboxID string
		if e = rows.Scan(&toolID, &workspaceID, &sandboxID); e != nil {
			return nil, e
		}
		ti, er := catalog.ParseToolID(toolID)
		if er != nil {
			return nil, er
		}
		if _, er := sandbox.ParseID(sandboxID); er != nil {
			return nil, er
		}
		out = append(out, app.TargetRecord{ProfileID: profileID, ToolID: ti, WorkspaceID: workspaceID, SandboxID: sandboxID})
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	if len(out) > 256 {
		return nil, app.ErrRegistryConflict
	}
	return out, nil
}

var _ app.RegistryStore = (*RegistryStore)(nil)
