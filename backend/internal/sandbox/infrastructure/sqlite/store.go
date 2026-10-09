package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

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
	_, err := s.db.ExecContext(ctx, `INSERT INTO sandboxes(id,display_name,backend,isolation,local_root,owner_kind,owner_ref,created_at)
 VALUES(?,?,?,?,?,?,?,?)`, spec.ID.String(), spec.Name, string(spec.Backend), string(spec.Isolation), spec.LocalRoot, spec.OwnerKind, spec.OwnerRef, time.Now().UTC().Format(time.RFC3339Nano))
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
	item, err := scanSandbox(s.db.QueryRowContext(ctx, `SELECT id, display_name, backend, isolation, local_root, owner_kind, owner_ref FROM sandboxes WHERE id = ?`, id.String()))
	if errors.Is(err, sql.ErrNoRows) {
		return sandboxdomain.Sandbox{}, sandboxdomain.ErrNotFound
	}
	if err != nil {
		return sandboxdomain.Sandbox{}, fmt.Errorf("get sandbox: %w", err)
	}
	return item, nil
}
func (s *Store) List(ctx context.Context) ([]sandboxdomain.Sandbox, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,display_name,backend,isolation,local_root,owner_kind,owner_ref FROM sandboxes ORDER BY display_name,id`)
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
	var rawID, backend, isolation string
	var spec sandboxdomain.Spec
	if err := row.Scan(&rawID, &spec.Name, &backend, &isolation, &spec.LocalRoot, &spec.OwnerKind, &spec.OwnerRef); err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	spec.ID = sandboxdomain.ID(rawID)
	spec.Backend = sandboxdomain.BackendKind(backend)
	spec.Isolation = sandboxdomain.IsolationClass(isolation)
	return sandboxdomain.NewSandbox(spec)
}
func uniqueError(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "unique constraint failed")
}
