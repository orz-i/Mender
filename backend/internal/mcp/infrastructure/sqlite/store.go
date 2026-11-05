package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	delivery "github.com/orz-i/Mender/backend/internal/mcp/application/delivery"
	mcp "github.com/orz-i/Mender/backend/internal/mcp/domain"
	profileapp "github.com/orz-i/Mender/backend/internal/profile/application"
	profile "github.com/orz-i/Mender/backend/internal/profile/domain"
	tunnel "github.com/orz-i/Mender/backend/internal/tunnel/domain"
	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

// Open never repairs or partly migrates delivery ownership. Profile Open owns
// the atomic legacy handoff to Profile 7 + MCP delivery 1 before composition.
func Open(ctx context.Context, path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("open MCP delivery: empty database path")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	slashed := filepath.ToSlash(absolute)
	if !strings.HasPrefix(slashed, "/") {
		slashed = "/" + slashed
	}
	u := url.URL{Scheme: "file", Path: slashed}
	query := u.Query()
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "busy_timeout(5000)")
	u.RawQuery = query.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	store := &Store{db: db}
	if err := store.checkSchema(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := ensureTunnelGuards(ctx, tx); err != nil {
		_ = tx.Rollback()
		_ = db.Close()
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) checkSchema(ctx context.Context) error {
	for _, schema := range []struct {
		table   string
		version int
	}{{"profile_schema_migrations", 7}, {"mcp_delivery_schema_migrations", 1}} {
		var version int
		if err := s.db.QueryRowContext(ctx, "SELECT COALESCE(MAX(version),0) FROM "+schema.table).Scan(&version); err != nil {
			return fmt.Errorf("open MCP delivery: requires atomic Profile 7/MCP delivery 1 migration: %w", err)
		}
		if version > schema.version {
			return fmt.Errorf("%s version %d is newer than supported version %d", schema.table, version, schema.version)
		}
		if version != schema.version {
			return fmt.Errorf("open MCP delivery: %s must be version %d, got %d", schema.table, schema.version, version)
		}
	}
	return nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) GetConfigRevision(ctx context.Context, id profile.ProfileID) (uint64, error) {
	var revision uint64
	err := s.db.QueryRowContext(ctx, "SELECT revision FROM mcp_delivery_configs WHERE profile_id=?", id.String()).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, delivery.ErrNotFound
	}
	return revision, err
}

func (s *Store) GetConfig(ctx context.Context, id profile.ProfileID) (mcp.Config, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return mcp.Config{}, err
	}
	defer func() { _ = tx.Rollback() }()
	config, err := readConfig(ctx, tx, id)
	if err != nil {
		return mcp.Config{}, err
	}
	if err := tx.Commit(); err != nil {
		return mcp.Config{}, err
	}
	return config, nil
}

func readConfig(ctx context.Context, tx *sql.Tx, id profile.ProfileID) (mcp.Config, error) {
	var mode, tunnelID string
	var revision uint64
	if err := tx.QueryRowContext(ctx, "SELECT mode,tunnel_id,revision FROM mcp_delivery_configs WHERE profile_id=?", id.String()).Scan(&mode, &tunnelID, &revision); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mcp.Config{}, delivery.ErrNotFound
		}
		return mcp.Config{}, err
	}
	rows, err := tx.QueryContext(ctx, "SELECT tool_id FROM mcp_delivery_direct_tools WHERE profile_id=? ORDER BY tool_id", id.String())
	if err != nil {
		return mcp.Config{}, err
	}
	var direct []catalog.ToolID
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			_ = rows.Close()
			return mcp.Config{}, err
		}
		tool, err := catalog.ParseToolID(raw)
		if err != nil {
			_ = rows.Close()
			return mcp.Config{}, err
		}
		direct = append(direct, tool)
	}
	rowsErr, closeErr := rows.Err(), rows.Close()
	if rowsErr != nil {
		return mcp.Config{}, rowsErr
	}
	if closeErr != nil {
		return mcp.Config{}, closeErr
	}
	config, err := mcp.NewConfig(id, mcp.Mode(mode), tunnel.TunnelID(tunnelID), direct)
	return config.WithRevision(revision), err
}

func (s *Store) PutConfig(ctx context.Context, config mcp.Config, expectedProfileRevision uint64, expectedRevision *uint64) (mcp.Config, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return mcp.Config{}, err
	}
	defer func() { _ = tx.Rollback() }()
	tunnelID, _ := config.TunnelID()
	// This write is the commit-point gate, before membership/Tunnel reads.
	var result sql.Result
	if expectedRevision == nil {
		result, err = tx.ExecContext(ctx, `INSERT INTO mcp_delivery_configs(profile_id,mode,tunnel_id,revision)
		 SELECT id,?,?,0 FROM profiles WHERE id=? AND revision=? ON CONFLICT(profile_id) DO NOTHING`,
			string(config.Mode()), tunnelID.String(), config.ProfileID().String(), expectedProfileRevision)
	} else {
		result, err = tx.ExecContext(ctx, `UPDATE mcp_delivery_configs SET mode=?,tunnel_id=?,revision=revision+1
		 WHERE profile_id=? AND revision=? AND EXISTS(SELECT 1 FROM profiles WHERE id=? AND revision=?)`,
			string(config.Mode()), tunnelID.String(), config.ProfileID().String(), *expectedRevision, config.ProfileID().String(), expectedProfileRevision)
	}
	if err != nil {
		return mcp.Config{}, mapWriteError(err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return mcp.Config{}, err
	}
	if count == 0 {
		return mcp.Config{}, fenceError(ctx, tx, config.ProfileID(), expectedProfileRevision)
	}
	hasTunnels, err := ensureTunnelGuards(ctx, tx)
	if err != nil {
		return mcp.Config{}, err
	}
	if tunnelID != "" {
		if !hasTunnels {
			return mcp.Config{}, fmt.Errorf("%w: Tunnel store unavailable", delivery.ErrInvalidConfiguration)
		}
		var present bool
		if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM tunnels WHERE id=?)", tunnelID.String()).Scan(&present); err != nil {
			return mcp.Config{}, err
		}
		if !present {
			return mcp.Config{}, fmt.Errorf("%w: Tunnel reference unavailable", delivery.ErrInvalidConfiguration)
		}
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM mcp_delivery_direct_tools WHERE profile_id=?", config.ProfileID().String()); err != nil {
		return mcp.Config{}, err
	}
	for _, id := range config.DirectToolIDs() {
		var member bool
		if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM profile_tools WHERE profile_id=? AND tool_id=?)", config.ProfileID().String(), id.String()).Scan(&member); err != nil {
			return mcp.Config{}, err
		}
		if !member {
			return mcp.Config{}, fmt.Errorf("%w: %s", delivery.ErrToolOutsideProfile, id)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO mcp_delivery_direct_tools(profile_id,tool_id) VALUES(?,?)", config.ProfileID().String(), id.String()); err != nil {
			return mcp.Config{}, err
		}
	}
	updated, err := readConfig(ctx, tx, config.ProfileID())
	if err != nil {
		return mcp.Config{}, err
	}
	if err := tx.Commit(); err != nil {
		return mcp.Config{}, err
	}
	return updated, nil
}

func fenceError(ctx context.Context, tx *sql.Tx, id profile.ProfileID, expected uint64) error {
	var revision uint64
	if err := tx.QueryRowContext(ctx, "SELECT revision FROM profiles WHERE id=?", id.String()).Scan(&revision); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return profileapp.ErrProfileNotFound
		}
		return err
	}
	if revision != expected {
		return delivery.ErrProfileRevisionConflict
	}
	return delivery.ErrRevisionConflict
}

func mapWriteError(err error) error {
	if strings.Contains(err.Error(), "MCP delivery Tunnel reference unavailable") {
		return fmt.Errorf("%w: %w", delivery.ErrInvalidConfiguration, err)
	}
	return err
}

func ensureTunnelGuards(ctx context.Context, tx *sql.Tx) (bool, error) {
	var present bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type='table' AND name='tunnels')").Scan(&present); err != nil {
		return false, err
	}
	if !present {
		return false, nil
	}
	for _, statement := range []string{
		`CREATE TRIGGER IF NOT EXISTS mcp_delivery_tunnel_insert_guard BEFORE INSERT ON mcp_delivery_configs
		 WHEN NEW.tunnel_id<>'' AND NOT EXISTS(SELECT 1 FROM tunnels WHERE id=NEW.tunnel_id)
		 BEGIN SELECT RAISE(ABORT,'MCP delivery Tunnel reference unavailable'); END`,
		`CREATE TRIGGER IF NOT EXISTS mcp_delivery_tunnel_update_guard BEFORE UPDATE OF tunnel_id ON mcp_delivery_configs
		 WHEN NEW.tunnel_id<>'' AND NOT EXISTS(SELECT 1 FROM tunnels WHERE id=NEW.tunnel_id)
		 BEGIN SELECT RAISE(ABORT,'MCP delivery Tunnel reference unavailable'); END`,
		`CREATE TRIGGER IF NOT EXISTS mcp_delivery_tunnel_delete_guard BEFORE DELETE ON tunnels
		 WHEN EXISTS(SELECT 1 FROM mcp_delivery_configs WHERE tunnel_id=OLD.id)
		 BEGIN SELECT RAISE(ABORT,'MCP delivery Tunnel in use'); END`,
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return false, err
		}
	}
	return true, nil
}

var _ delivery.Repository = (*Store)(nil)
