// Package sourcecatalog contains the proposed SourceRef-based Catalog storage
// contract for ADR-0064. It is NOT opened by bootstrap.Runtime in Stage D.
// Stage E must install its table layout and ownership guards inside a single
// cross-context, forward-only migration before any production use.
package sourcecatalog

// FreshSchema contains the proposed canonical Catalog table/index/FTS layout
// for a NEW isolated database. These statements MUST NOT be run as migrations
// against a legacy Mender database: that database also owns Profile, Policy,
// Agent, Sandbox and Audit references which must be migrated together.
func FreshSchema() []string {
	return []string{
		`CREATE TABLE providers (
			id TEXT PRIMARY KEY,
			namespace TEXT NOT NULL UNIQUE,
			display_name TEXT NOT NULL,
			description TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE tools (
			id TEXT PRIMARY KEY,
			source_kind TEXT NOT NULL CHECK(source_kind IN ('provider','agent','native')),
			source_id TEXT NOT NULL CHECK(length(source_id)>0),
			source_key TEXT NOT NULL CHECK(length(source_key)>0),
			name TEXT NOT NULL,
			title TEXT NOT NULL,
			description TEXT NOT NULL,
			input_schema BLOB NOT NULL,
			output_schema BLOB,
			tags_json TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(source_kind,source_id,source_key)
		)`,
		`CREATE INDEX tools_source_ref_idx ON tools(source_kind,source_id,source_key,id)`,
		`CREATE VIRTUAL TABLE tools_fts USING fts5(
			tool_id UNINDEXED,
			source_kind UNINDEXED,
			source_id UNINDEXED,
			name,
			title,
			description,
			tags
		)`,
		`CREATE TABLE catalog_tool_revision (
			id INTEGER PRIMARY KEY CHECK(id=1),
			revision INTEGER NOT NULL CHECK(revision>=0)
		)`,
		`INSERT INTO catalog_tool_revision(id,revision) VALUES (1,0)`,
		`CREATE TRIGGER catalog_tool_insert_revision AFTER INSERT ON tools
		  BEGIN UPDATE catalog_tool_revision SET revision=revision+1 WHERE id=1; END`,
		`CREATE TRIGGER catalog_tool_update_revision AFTER UPDATE ON tools
		  BEGIN UPDATE catalog_tool_revision SET revision=revision+1 WHERE id=1; END`,
		`CREATE TRIGGER catalog_tool_delete_revision AFTER DELETE ON tools
		  BEGIN UPDATE catalog_tool_revision SET revision=revision+1 WHERE id=1; END`,
		// Real Provider source identities are persisted cross-table, not only
		// checked by the application. Agent owner/FK triggers are installed
		// by Stage E when the Agent bounded-context tables already exist.
		`CREATE TRIGGER tools_provider_owner_insert BEFORE INSERT ON tools
		  WHEN NEW.source_kind='provider' AND NOT EXISTS(SELECT 1 FROM providers WHERE id=NEW.source_id)
		  BEGIN SELECT RAISE(ABORT,'Tool Provider owner unavailable'); END`,
		`CREATE TRIGGER tools_provider_owner_update BEFORE UPDATE OF source_kind,source_id ON tools
		  WHEN NEW.source_kind='provider' AND NOT EXISTS(SELECT 1 FROM providers WHERE id=NEW.source_id)
		  BEGIN SELECT RAISE(ABORT,'Tool Provider owner unavailable'); END`,
		`CREATE TRIGGER tools_provider_owner_delete BEFORE DELETE ON providers
		  WHEN EXISTS(SELECT 1 FROM tools WHERE source_kind='provider' AND source_id=OLD.id)
		  BEGIN SELECT RAISE(ABORT,'Provider has owned Tool references'); END`,
	}
}
