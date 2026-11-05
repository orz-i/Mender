package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// migrateDeliveryV7 is the one-time shared-schema handoff from Profile v6 to
// MCP delivery v1. Both version records and every data/DDL change commit in the
// caller's transaction. DROP COLUMN keeps table identities and foreign-context
// triggers intact, unlike renaming/dropping the referenced Profile tables.
func migrateDeliveryV7(ctx context.Context, tx *sql.Tx) error {
	statements := []string{
		`CREATE TABLE mcp_delivery_schema_migrations(version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`,
		`CREATE TABLE mcp_delivery_configs(
			profile_id TEXT PRIMARY KEY REFERENCES profiles(id) ON DELETE CASCADE,
			mode TEXT NOT NULL CHECK(mode IN ('direct','dynamic','hybrid')),
			tunnel_id TEXT NOT NULL DEFAULT '',
			revision INTEGER NOT NULL DEFAULT 0 CHECK(revision >= 0)
		)`,
		`CREATE INDEX idx_mcp_delivery_tunnel ON mcp_delivery_configs(tunnel_id) WHERE tunnel_id <> ''`,
		`CREATE TABLE mcp_delivery_direct_tools(
			profile_id TEXT NOT NULL REFERENCES mcp_delivery_configs(profile_id) ON DELETE CASCADE,
			tool_id TEXT NOT NULL,
			PRIMARY KEY(profile_id,tool_id),
			FOREIGN KEY(profile_id,tool_id) REFERENCES profile_tools(profile_id,tool_id) ON DELETE CASCADE
		)`,
		`CREATE INDEX idx_mcp_delivery_direct_tool ON mcp_delivery_direct_tools(tool_id)`,
		`INSERT INTO mcp_delivery_configs(profile_id,mode,tunnel_id,revision)
		 SELECT id,exposure_mode,tunnel_id,0 FROM profiles`,
		// Old Profile restoration normalizes direct mode to all members and
		// dynamic mode to none, regardless of historical per-binding flags.
		// Only hybrid mode therefore has a stored explicit direct subset.
		`INSERT INTO mcp_delivery_direct_tools(profile_id,tool_id)
		 SELECT b.profile_id,b.tool_id FROM profile_tools b JOIN profiles p ON p.id=b.profile_id
		 WHERE p.exposure_mode='hybrid' AND b.direct=1`,
		`ALTER TABLE profile_tools DROP COLUMN direct`,
		`ALTER TABLE profiles DROP COLUMN exposure_mode`,
		`ALTER TABLE profiles DROP COLUMN tunnel_id`,
		// A selection removal prunes its direct exposure through the composite
		// FK. Bump the independent delivery revision before that cascade. Saves
		// of unchanged Profile members must not delete/reinsert these rows.
		`CREATE TRIGGER mcp_delivery_member_removal_revision BEFORE DELETE ON profile_tools
		 WHEN EXISTS(SELECT 1 FROM mcp_delivery_direct_tools d
			WHERE d.profile_id=OLD.profile_id AND d.tool_id=OLD.tool_id)
		 BEGIN UPDATE mcp_delivery_configs SET revision=revision+1 WHERE profile_id=OLD.profile_id; END`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migrate Profile delivery ownership: %w", err)
		}
	}
	// Existing Tunnel relations must be guarded when the handoff is published,
	// before any new MCP store is opened. Standalone Profile databases need not
	// contain Tunnel schema; the MCP store also installs these guards when that
	// schema is introduced later.
	var hasTunnels bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type='table' AND name='tunnels')").Scan(&hasTunnels); err != nil {
		return err
	}
	if hasTunnels {
		for _, statement := range []string{
			`CREATE TRIGGER mcp_delivery_tunnel_insert_guard BEFORE INSERT ON mcp_delivery_configs
			 WHEN NEW.tunnel_id<>'' AND NOT EXISTS(SELECT 1 FROM tunnels WHERE id=NEW.tunnel_id)
			 BEGIN SELECT RAISE(ABORT,'MCP delivery Tunnel reference unavailable'); END`,
			`CREATE TRIGGER mcp_delivery_tunnel_update_guard BEFORE UPDATE OF tunnel_id ON mcp_delivery_configs
			 WHEN NEW.tunnel_id<>'' AND NOT EXISTS(SELECT 1 FROM tunnels WHERE id=NEW.tunnel_id)
			 BEGIN SELECT RAISE(ABORT,'MCP delivery Tunnel reference unavailable'); END`,
			`CREATE TRIGGER mcp_delivery_tunnel_delete_guard BEFORE DELETE ON tunnels
			 WHEN EXISTS(SELECT 1 FROM mcp_delivery_configs WHERE tunnel_id=OLD.id)
			 BEGIN SELECT RAISE(ABORT,'MCP delivery Tunnel in use'); END`,
		} {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("migrate MCP delivery Tunnel guards: %w", err)
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO mcp_delivery_schema_migrations(version,applied_at) VALUES(1,?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("record MCP delivery schema: %w", err)
	}
	return nil
}
