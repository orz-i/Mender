package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	v3schema "github.com/orz-i/Mender/backend/internal/schema/v3"
	tunnelapplication "github.com/orz-i/Mender/backend/internal/tunnel/application"
	tunneldomain "github.com/orz-i/Mender/backend/internal/tunnel/domain"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

// Open is the only Tunnel Store constructor. It verifies the pinned v3
// schema and never upgrades, backs up, imports or rebinds legacy Tunnels.
func Open(ctx context.Context, path string) (*Store, error) {
	db, err := v3schema.OpenContextStore(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("open Tunnel sqlite: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Create(ctx context.Context, tunnel tunneldomain.Tunnel) error {
	query := `INSERT INTO tunnels(id,display_name,provider,external_tunnel_id,
        runtime_credential_ref,plugin_id,extension_id,installation_id)
        VALUES (?,?,?,?,?,?,?,?)`
	args := []any{tunnel.ID().String(), tunnel.DisplayName(), string(tunnel.Provider()),
		tunnel.ExternalTunnelID(), tunnel.RuntimeCredentialRef().String(),
		tunnel.Binding().PluginID.String(), tunnel.Binding().ExtensionID.String(),
		tunnel.Binding().InstallationID.String()}
	if tunnel.Provider() == tunneldomain.ProviderExternal {
		// Select the exact enabled and verified package in the same SQLite
		// statement. No stale preflight can authorize a disabled extension.
		query = `INSERT INTO tunnels(id,display_name,provider,external_tunnel_id,
            runtime_credential_ref,plugin_id,extension_id,installation_id)
            SELECT ?,?,?,?,?,?,?,? WHERE EXISTS(
                SELECT 1 FROM plugin_installations pi
                WHERE pi.installation_id=? AND pi.plugin_id=? AND pi.status='enabled'
                  AND pi.verified_descriptor_json IS NOT NULL
                  AND json_extract(pi.manifest_json,'$.extensions[0].kind')='tunnel.transport'
            )`
		args = append(args, tunnel.Binding().InstallationID.String(), tunnel.Binding().PluginID.String())
	}
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		lower := strings.ToLower(err.Error())
		if strings.Contains(lower, "unique") || strings.Contains(lower, "primary key") {
			return tunnelapplication.ErrTunnelConflict
		}
		return fmt.Errorf("create tunnel: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("create tunnel: affected rows: %w", err)
	}
	if count != 1 {
		return fmt.Errorf("create tunnel: extension installation unavailable")
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
			runtime_credential_ref,
            plugin_id,extension_id,installation_id
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
			runtime_credential_ref,
            plugin_id,extension_id,installation_id
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
		if strings.Contains(err.Error(), "MCP delivery Tunnel in use") {
			return fmt.Errorf("%w: %w", tunnelapplication.ErrTunnelInUse, err)
		}
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
	var rawID, displayName, provider, externalID, rawCredentialRef, pluginID, extensionID, installationID string
	if err := scanner.Scan(
		&rawID,
		&displayName,
		&provider,
		&externalID,
		&rawCredentialRef,
		&pluginID, &extensionID, &installationID,
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
		tunneldomain.Binding{PluginID: platformplugin.ID(pluginID), ExtensionID: platformplugin.ExtensionID(extensionID),
			InstallationID: platformplugin.InstallationID(installationID)},
		externalID,
		ref,
	)
}

// All Tunnel resources hold immutable installation references, including
// unbound or stopped Tunnels. Every reference blocks disable/removal.
func (s *Store) ReferencedInstallation(ctx context.Context, installationID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tunnels WHERE provider='external' AND installation_id=?", installationID).Scan(&count)
	return count, err
}
