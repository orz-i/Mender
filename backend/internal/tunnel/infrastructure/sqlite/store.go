package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	tunnelapplication "github.com/orz-i/Mender/backend/internal/tunnel/application"
	tunneldomain "github.com/orz-i/Mender/backend/internal/tunnel/domain"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(ctx context.Context, path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("open tunnel sqlite: path must not be empty")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("open tunnel sqlite: resolve path: %w", err)
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(absolute)}
	query := u.Query()
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "busy_timeout(5000)")
	u.RawQuery = query.Encode()

	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, fmt.Errorf("open tunnel sqlite: %w", err)
	}
	store := &Store{db: db}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open tunnel sqlite: ping: %w", err)
	}
	if err := migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open tunnel sqlite: migrate: %w", err)
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Create(ctx context.Context, tunnel tunneldomain.Tunnel) error {
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO tunnels(
			id,
			display_name,
			provider,
			external_tunnel_id,
			runtime_credential_ref
		) VALUES (?, ?, ?, ?, ?)`,
		tunnel.ID().String(),
		tunnel.DisplayName(),
		string(tunnel.Provider()),
		tunnel.ExternalTunnelID(),
		tunnel.RuntimeCredentialRef().String(),
	)
	if err != nil {
		lower := strings.ToLower(err.Error())
		if strings.Contains(lower, "unique") || strings.Contains(lower, "primary key") {
			return tunnelapplication.ErrTunnelConflict
		}
		return fmt.Errorf("create tunnel: %w", err)
	}
	return nil
}

func (s *Store) Get(
	ctx context.Context,
	id tunneldomain.TunnelID,
) (tunneldomain.Tunnel, error) {
	item, err := scanTunnel(s.db.QueryRowContext(
		ctx,
		`SELECT
			id,
			display_name,
			provider,
			external_tunnel_id,
			runtime_credential_ref
		FROM tunnels
		WHERE id = ?`,
		id.String(),
	))
	if errors.Is(err, sql.ErrNoRows) {
		return tunneldomain.Tunnel{}, tunnelapplication.ErrTunnelNotFound
	}
	if err != nil {
		return tunneldomain.Tunnel{}, fmt.Errorf("get tunnel: %w", err)
	}
	return item, nil
}

func (s *Store) List(ctx context.Context) ([]tunneldomain.Tunnel, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT
			id,
			display_name,
			provider,
			external_tunnel_id,
			runtime_credential_ref
		FROM tunnels
		ORDER BY display_name, id`,
	)
	if err != nil {
		return nil, fmt.Errorf("list tunnels: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]tunneldomain.Tunnel, 0)
	for rows.Next() {
		item, err := scanTunnel(rows)
		if err != nil {
			return nil, fmt.Errorf("list tunnels: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list tunnels: rows: %w", err)
	}
	return items, nil
}

func (s *Store) Delete(ctx context.Context, id tunneldomain.TunnelID) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM tunnels WHERE id = ?", id.String())
	if err != nil {
		return fmt.Errorf("delete tunnel: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete tunnel: rows affected: %w", err)
	}
	if count == 0 {
		return tunnelapplication.ErrTunnelNotFound
	}
	return nil
}

type rowScanner interface {
	Scan(...any) error
}

func scanTunnel(scanner rowScanner) (tunneldomain.Tunnel, error) {
	var rawID, displayName, provider, externalID, rawCredentialRef string
	if err := scanner.Scan(
		&rawID,
		&displayName,
		&provider,
		&externalID,
		&rawCredentialRef,
	); err != nil {
		return tunneldomain.Tunnel{}, err
	}
	id, err := tunneldomain.ParseTunnelID(rawID)
	if err != nil {
		return tunneldomain.Tunnel{}, fmt.Errorf("parse tunnel id: %w", err)
	}
	ref, err := credentialdomain.ParseReference(rawCredentialRef)
	if err != nil {
		return tunneldomain.Tunnel{}, fmt.Errorf("parse tunnel credential reference: %w", err)
	}
	return tunneldomain.NewTunnel(
		id,
		displayName,
		tunneldomain.Provider(provider),
		externalID,
		ref,
	)
}
