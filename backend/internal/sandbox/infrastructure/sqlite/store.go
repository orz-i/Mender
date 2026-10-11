package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"

	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	_ "modernc.org/sqlite"
)

// Store owns only Sandbox tables; it does not reach into Provider persistence.
type Store struct{ db *sql.DB }

func Open(ctx context.Context, path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("open sandbox sqlite: missing path")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("open sandbox sqlite: %w", err)
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(absolute)}
	query := u.Query()
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "busy_timeout(5000)")
	u.RawQuery = query.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, fmt.Errorf("open sandbox sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open sandbox sqlite ping: %w", err)
	}
	if err := backupPreSandboxUpgrade(ctx, db, absolute); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open sandbox sqlite: pre-binding backup: %w", err)
	}
	if err := migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}
func (s *Store) Create(ctx context.Context, item sandboxdomain.Sandbox) error {
	if err := item.Validate(); err != nil {
		return err
	}
	spec := item.Spec()
	_, err := s.db.ExecContext(ctx, `INSERT INTO sandboxes(id,display_name,backend,backend_plugin_id,backend_extension_id,backend_installation_id,isolation,local_root,owner_kind,owner_ref,created_at)
 VALUES(?,?,?,?,?,?,?,?,?,?,?)`, spec.ID.String(), spec.Name, string(spec.Backend), spec.Binding.PluginID.String(), spec.Binding.ExtensionID.String(), spec.Binding.InstallationID.String(), string(spec.Isolation), spec.LocalRoot, spec.OwnerKind, spec.OwnerRef, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		if uniqueError(err) {
			return sandboxdomain.ErrConflict
		}
		return fmt.Errorf("save sandbox: %w", err)
	}
	return nil
}
func (s *Store) Get(ctx context.Context, id sandboxdomain.ID) (sandboxdomain.Sandbox, error) {
	if _, err := sandboxdomain.ParseID(id.String()); err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	item, err := scanSandbox(s.db.QueryRowContext(ctx, `SELECT id, display_name, backend, backend_plugin_id, backend_extension_id, backend_installation_id, isolation, local_root, owner_kind, owner_ref FROM sandboxes WHERE id = ?`, id.String()))
	if errors.Is(err, sql.ErrNoRows) {
		return sandboxdomain.Sandbox{}, sandboxdomain.ErrNotFound
	}
	if err != nil {
		return sandboxdomain.Sandbox{}, fmt.Errorf("get sandbox: %w", err)
	}
	return item, nil
}
func (s *Store) List(ctx context.Context) ([]sandboxdomain.Sandbox, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,display_name,backend,backend_plugin_id,backend_extension_id,backend_installation_id,isolation,local_root,owner_kind,owner_ref FROM sandboxes ORDER BY display_name,id`)
	if err != nil {
		return nil, fmt.Errorf("list sandboxes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	items := make([]sandboxdomain.Sandbox, 0)
	for rows.Next() {
		item, scanErr := scanSandbox(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list sandboxes: %w", scanErr)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list sandboxes: %w", err)
	}
	return items, nil
}
func (s *Store) CreateRun(ctx context.Context, run sandboxdomain.Run) error {
	if _, err := sandboxdomain.NewRun(run.SandboxID, run.Generation, run.State, run.ExternalID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO sandbox_runs(sandbox_id,generation,state,external_id,updated_at) VALUES(?,?,?,?,?)`, run.SandboxID.String(), run.Generation, string(run.State), run.ExternalID, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		if uniqueError(err) {
			return sandboxdomain.ErrConflict
		}
		return fmt.Errorf("create sandbox run: %w", err)
	}
	return nil
}
func (s *Store) GetRun(ctx context.Context, id sandboxdomain.ID) (sandboxdomain.Run, error) {
	if _, err := sandboxdomain.ParseID(id.String()); err != nil {
		return sandboxdomain.Run{}, err
	}
	var rawID, state, externalID string
	var generation uint64
	err := s.db.QueryRowContext(ctx, `SELECT sandbox_id,generation,state,external_id FROM sandbox_runs WHERE sandbox_id = ?`, id.String()).Scan(&rawID, &generation, &state, &externalID)
	if errors.Is(err, sql.ErrNoRows) {
		return sandboxdomain.Run{}, sandboxdomain.ErrNotFound
	}
	if err != nil {
		return sandboxdomain.Run{}, fmt.Errorf("get sandbox run: %w", err)
	}
	return sandboxdomain.NewRun(sandboxdomain.ID(rawID), generation, sandboxdomain.State(state), externalID)
}

// TransitionRun is an atomic compare-and-swap. A stale holder cannot update
// a newer run generation or a state it no longer owns.
func (s *Store) TransitionRun(ctx context.Context, expected sandboxdomain.Run, to sandboxdomain.State, externalID string) (sandboxdomain.Run, error) {
	if _, err := sandboxdomain.NewRun(expected.SandboxID, expected.Generation, expected.State, expected.ExternalID); err != nil {
		return sandboxdomain.Run{}, err
	}
	next, err := expected.Transition(to, externalID)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE sandbox_runs SET generation = ?, state = ?, external_id = ?, updated_at = ? WHERE sandbox_id = ? AND generation = ? AND state = ? AND external_id = ?`, next.Generation, string(next.State), next.ExternalID, time.Now().UTC().Format(time.RFC3339Nano), expected.SandboxID.String(), expected.Generation, string(expected.State), expected.ExternalID)
	if err != nil {
		return sandboxdomain.Run{}, fmt.Errorf("transition sandbox run: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return sandboxdomain.Run{}, fmt.Errorf("transition sandbox run: rows: %w", err)
	}
	if count != 1 {
		return sandboxdomain.Run{}, sandboxdomain.ErrConflict
	}
	return next, nil
}

type scanner interface{ Scan(...any) error }

func scanSandbox(row scanner) (sandboxdomain.Sandbox, error) {
	var rawID, backend, pluginID, extensionID, installationID, isolation string
	var spec sandboxdomain.Spec
	if err := row.Scan(&rawID, &spec.Name, &backend, &pluginID, &extensionID, &installationID, &isolation, &spec.LocalRoot, &spec.OwnerKind, &spec.OwnerRef); err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	spec.ID = sandboxdomain.ID(rawID)
	spec.Backend = sandboxdomain.BackendKind(backend)
	spec.Binding = sandboxdomain.BackendBinding{PluginID: platformplugin.ID(pluginID), ExtensionID: platformplugin.ExtensionID(extensionID), InstallationID: platformplugin.InstallationID(installationID)}
	spec.Isolation = sandboxdomain.IsolationClass(isolation)
	return sandboxdomain.NewSandbox(spec)
}
func uniqueError(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "unique constraint failed")
}

// CreateReady atomically persists an explicitly admitted local Sandbox and its
// initial run. No partially configured resource can become visible.
func (s *Store) CreateReady(ctx context.Context, item sandboxdomain.Sandbox, run sandboxdomain.Run) error {
	if err := item.Validate(); err != nil {
		return err
	}
	if _, err := sandboxdomain.NewRun(run.SandboxID, run.Generation, run.State, run.ExternalID); err != nil {
		return err
	}
	if item.ID() != run.SandboxID || run.State != sandboxdomain.StateReady || run.Generation != 1 {
		return fmt.Errorf("create ready sandbox: invalid run binding")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("create ready sandbox: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	spec := item.Spec()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = tx.ExecContext(ctx, `INSERT INTO sandboxes(id,display_name,backend,backend_plugin_id,backend_extension_id,backend_installation_id,isolation,local_root,owner_kind,owner_ref,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, spec.ID.String(), spec.Name, string(spec.Backend), spec.Binding.PluginID.String(), spec.Binding.ExtensionID.String(), spec.Binding.InstallationID.String(), string(spec.Isolation), spec.LocalRoot, spec.OwnerKind, spec.OwnerRef, now)
	if err != nil {
		if uniqueError(err) {
			return sandboxdomain.ErrConflict
		}
		return fmt.Errorf("create ready sandbox: record resource: %w", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO sandbox_runs(sandbox_id,generation,state,external_id,updated_at) VALUES(?,?,?,?,?)`, run.SandboxID.String(), run.Generation, string(run.State), run.ExternalID, now)
	if err != nil {
		return fmt.Errorf("create ready sandbox: record run: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("create ready sandbox: commit: %w", err)
	}
	return nil
}

// backupPreSandboxUpgrade creates a WAL-consistent snapshot before the
// forward-only v8/v9 -> v10 schema hard cut. An already-current database is
// never snapshotted again, and an unsupported future schema never downgraded.
func backupPreSandboxUpgrade(ctx context.Context, db *sql.DB, path string) error {
	var present int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='sandbox_schema_migrations'").Scan(&present); err != nil {
		return err
	}
	if present == 0 {
		return nil
	}
	var version sql.NullInt64
	if err := db.QueryRowContext(ctx, "SELECT MAX(version) FROM sandbox_schema_migrations").Scan(&version); err != nil {
		return err
	}
	if !version.Valid || version.Int64 >= latestSchemaVersion {
		return nil
	}
	prefix := ".mender-sandbox-v8-backup-*.db"
	if version.Int64 >= 10 {
		prefix = ".mender-sandbox-v10-backup-*.db"
	} else if version.Int64 >= 9 {
		prefix = ".mender-sandbox-v9-backup-*.db"
	}
	file, err := os.CreateTemp(filepath.Dir(path), prefix)
	if err != nil {
		return err
	}
	destination := file.Name()
	if err = file.Close(); err != nil {
		return err
	}
	if err = os.Remove(destination); err != nil {
		return err
	}
	if _, err = db.ExecContext(ctx, "VACUUM INTO ?", destination); err != nil {
		_ = os.Remove(destination)
		return fmt.Errorf("sandbox SQLite consistent backup: %w", err)
	}
	return nil
}
