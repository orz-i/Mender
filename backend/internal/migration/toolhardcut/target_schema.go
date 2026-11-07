package toolhardcut

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/orz-i/Mender/backend/internal/toolsuite/infrastructure/curated"
)

func installHardcutReferenceGuards(ctx context.Context, tx *sql.Tx, sourceSchema []string) error {
	// SourceCatalog's trusted Provider SQL owner guards are indexes 9..11;
	// fail if the vetted schema layout changes instead of silently omitting.
	if len(sourceSchema) != 12 {
		return errors.New("hard cut: unreviewed SourceCatalog DDL inventory")
	}
	for _, stmt := range []string{
		`CREATE TRIGGER tools_agent_owner_insert BEFORE INSERT ON tools
   WHEN NEW.source_kind='agent' AND
    (NOT EXISTS(SELECT 1 FROM agents WHERE agent_id=NEW.source_id)
     OR NEW.source_key NOT IN ('agent.delegate','agent.status','agent.result','agent.cancel','agent.resume'))
   BEGIN SELECT RAISE(ABORT,'Tool Agent owner/operation unavailable'); END`,
		`CREATE TRIGGER tools_agent_owner_update BEFORE UPDATE OF source_kind,source_id,source_key ON tools
   WHEN NEW.source_kind='agent' AND
    (NOT EXISTS(SELECT 1 FROM agents WHERE agent_id=NEW.source_id)
     OR NEW.source_key NOT IN ('agent.delegate','agent.status','agent.result','agent.cancel','agent.resume'))
   BEGIN SELECT RAISE(ABORT,'Tool Agent owner/operation unavailable'); END`,
		`CREATE TRIGGER tools_agent_owner_delete BEFORE DELETE ON agents
   WHEN EXISTS(SELECT 1 FROM tools WHERE source_kind='agent' AND source_id=OLD.agent_id)
   BEGIN SELECT RAISE(ABORT,'Agent has canonical Tool references'); END`,
		`CREATE TABLE native_tool_contracts(
    tool_id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL CHECK(source_id IN('workspace','jobs')),
    source_key TEXT NOT NULL UNIQUE,
    input_schema BLOB NOT NULL,
    output_schema BLOB NOT NULL,
    name TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    tags_json TEXT NOT NULL
   )`,
	} {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	registered, err := curated.PlannedNativeCatalogTools(time.Now().UTC())
	if err != nil {
		return err
	}
	for _, tool := range registered {
		output := tool.OutputSchema()
		if output == nil {
			return errors.New("hard cut: incomplete reviewed native Tool")
		}
		tags, err := json.Marshal(tool.Tags())
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO native_tool_contracts
   (tool_id,source_id,source_key,input_schema,output_schema,name,title,description,tags_json)
   VALUES(?,?,?,?,?,?,?,?,?)`,
			tool.ID().String(), tool.SourceRef().ID(), tool.SourceKey(),
			tool.InputSchema().Bytes(), output.Bytes(), tool.Name(), tool.Title(), tool.Description(), string(tags)); err != nil {
			return err
		}
	}
	for _, stmt := range []string{
		`CREATE TRIGGER native_tool_contract_immutable_update BEFORE UPDATE ON native_tool_contracts
   BEGIN SELECT RAISE(ABORT,'reviewed native Tool contract immutable'); END`,
		`CREATE TRIGGER native_tool_contract_immutable_delete BEFORE DELETE ON native_tool_contracts
   BEGIN SELECT RAISE(ABORT,'reviewed native Tool contract immutable'); END`,
		`CREATE TRIGGER tools_native_contract_insert BEFORE INSERT ON tools
   WHEN NEW.source_kind='native' AND NOT EXISTS(SELECT 1 FROM native_tool_contracts n
    WHERE n.tool_id=NEW.id AND n.source_id=NEW.source_id AND n.source_key=NEW.source_key
      AND n.input_schema=NEW.input_schema AND n.output_schema=NEW.output_schema
      AND n.name=NEW.name AND n.title=NEW.title AND n.description=NEW.description
      AND n.tags_json=NEW.tags_json)
   BEGIN SELECT RAISE(ABORT,'unreviewed native Tool identity or Schema'); END`,
		`CREATE TRIGGER tools_native_contract_update BEFORE UPDATE ON tools
   WHEN NEW.source_kind='native' AND NOT EXISTS(SELECT 1 FROM native_tool_contracts n
    WHERE n.tool_id=NEW.id AND n.source_id=NEW.source_id AND n.source_key=NEW.source_key
      AND n.input_schema=NEW.input_schema AND n.output_schema=NEW.output_schema
      AND n.name=NEW.name AND n.title=NEW.title AND n.description=NEW.description
      AND n.tags_json=NEW.tags_json)
   BEGIN SELECT RAISE(ABORT,'unreviewed native Tool identity or Schema'); END`,
		`CREATE TRIGGER profile_tool_reference_insert_guard BEFORE INSERT ON profile_tools
   WHEN NOT EXISTS(SELECT 1 FROM tools WHERE id=NEW.tool_id)
   BEGIN SELECT RAISE(ABORT,'Profile Tool reference unavailable'); END`,
		`CREATE TRIGGER profile_tool_reference_update_guard BEFORE UPDATE OF tool_id ON profile_tools
   WHEN NOT EXISTS(SELECT 1 FROM tools WHERE id=NEW.tool_id)
   BEGIN SELECT RAISE(ABORT,'Profile Tool reference unavailable'); END`,
		`CREATE TRIGGER policy_reference_insert_guard BEFORE INSERT ON policy_rules
   WHEN NOT json_valid(NEW.match_json)
      OR EXISTS(SELECT 1 FROM json_each(CASE WHEN json_valid(NEW.match_json) THEN NEW.match_json ELSE '{}' END,'$.providerIds') ref
        WHERE ref.type!='null' AND NOT EXISTS(SELECT 1 FROM providers WHERE id=ref.value))
      OR EXISTS(SELECT 1 FROM json_each(CASE WHEN json_valid(NEW.match_json) THEN NEW.match_json ELSE '{}' END,'$.toolIds') ref
        WHERE ref.type!='null' AND NOT EXISTS(SELECT 1 FROM tools WHERE id=ref.value))
   BEGIN SELECT RAISE(ABORT,'Policy source/Tool reference unavailable'); END`,
		`CREATE TRIGGER policy_reference_update_guard BEFORE UPDATE OF match_json ON policy_rules
   WHEN NOT json_valid(NEW.match_json)
      OR EXISTS(SELECT 1 FROM json_each(CASE WHEN json_valid(NEW.match_json) THEN NEW.match_json ELSE '{}' END,'$.providerIds') ref
        WHERE ref.type!='null' AND NOT EXISTS(SELECT 1 FROM providers WHERE id=ref.value))
      OR EXISTS(SELECT 1 FROM json_each(CASE WHEN json_valid(NEW.match_json) THEN NEW.match_json ELSE '{}' END,'$.toolIds') ref
        WHERE ref.type!='null' AND NOT EXISTS(SELECT 1 FROM tools WHERE id=ref.value))
   BEGIN SELECT RAISE(ABORT,'Policy source/Tool reference unavailable'); END`,
		`CREATE TRIGGER provider_reference_delete_guard BEFORE DELETE ON providers
   WHEN EXISTS(SELECT 1 FROM provider_configs WHERE provider_id=OLD.id)
     OR EXISTS(SELECT 1 FROM policy_rules r, json_each(
       CASE WHEN json_valid(r.match_json) THEN r.match_json ELSE '{}' END,'$.providerIds') ref
       WHERE ref.value=OLD.id)
   BEGIN SELECT RAISE(ABORT,'Provider has configured/Policy references'); END`,
		`CREATE TRIGGER tools_reference_delete_guard BEFORE DELETE ON tools
   WHEN OLD.source_kind='native'
      OR EXISTS(SELECT 1 FROM profile_tools WHERE tool_id=OLD.id)
      OR EXISTS(SELECT 1 FROM policy_rules r, json_each(
       CASE WHEN json_valid(r.match_json) THEN r.match_json ELSE '{}' END,'$.toolIds') ref
       WHERE ref.value=OLD.id)
   BEGIN SELECT RAISE(ABORT,'Tool is native or referenced by Profile/Policy'); END`,
	} {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("hard cut reference guard: %w", err)
		}
	}
	return nil
}

func installHardcutTargetSchema(ctx context.Context, tx *sql.Tx) error {
	for _, stmt := range []string{
		`CREATE TABLE workspaces (
   workspace_id TEXT PRIMARY KEY CHECK(length(workspace_id)=36 AND substr(workspace_id,1,4)='wrk_'),
   owner_kind TEXT NOT NULL,
   owner_ref TEXT NOT NULL,
   display_name TEXT NOT NULL,
   verified_root TEXT NOT NULL DEFAULT '',
   created_at TEXT NOT NULL
  )`,
		`CREATE TABLE workspace_placements (
   workspace_id TEXT NOT NULL REFERENCES workspaces(workspace_id) ON DELETE RESTRICT,
   sandbox_id TEXT NOT NULL REFERENCES sandboxes(id) ON DELETE RESTRICT,
   backend_root TEXT NOT NULL,
   verified_revision INTEGER NOT NULL CHECK(verified_revision>0),
   PRIMARY KEY(workspace_id,sandbox_id)
  )`,
		`CREATE TABLE profile_tool_target_scopes (
   profile_id TEXT NOT NULL,
   tool_id TEXT NOT NULL,
   workspace_id TEXT NOT NULL DEFAULT '',
   sandbox_id TEXT NOT NULL REFERENCES sandboxes(id) ON DELETE RESTRICT,
   PRIMARY KEY(profile_id,tool_id,workspace_id,sandbox_id),
   FOREIGN KEY(profile_id,tool_id) REFERENCES profile_tools(profile_id,tool_id) ON DELETE CASCADE
  )`,
		`CREATE TRIGGER profile_tool_target_insert_guard BEFORE INSERT ON profile_tool_target_scopes
   WHEN NOT EXISTS(
    SELECT 1 FROM tools t WHERE t.id=NEW.tool_id AND t.source_kind='native'
    AND ((t.source_id='workspace' AND NEW.workspace_id<>'' AND EXISTS(
       SELECT 1 FROM workspace_placements p
       WHERE p.workspace_id=NEW.workspace_id AND p.sandbox_id=NEW.sandbox_id))
     OR (t.source_id='jobs' AND NEW.workspace_id=''))
   )
   BEGIN SELECT RAISE(ABORT,'unreviewed Tool target scope'); END`,
		`CREATE TRIGGER profile_tool_target_update_guard BEFORE UPDATE ON profile_tool_target_scopes
   BEGIN SELECT RAISE(ABORT,'Tool target scope immutable; replace explicitly'); END`,
		`CREATE TRIGGER workspace_authority_immutable_update
   BEFORE UPDATE OF owner_kind,owner_ref,verified_root ON workspaces
   BEGIN SELECT RAISE(ABORT,'Workspace authority/root cannot be rebound in place'); END`,
		`CREATE TRIGGER workspace_placement_immutable_update BEFORE UPDATE ON workspace_placements
   BEGIN SELECT RAISE(ABORT,'Workspace placement immutable; owner must remove and re-attest'); END`,
		`CREATE TRIGGER workspace_placement_in_use_guard BEFORE DELETE ON workspace_placements
   WHEN EXISTS(SELECT 1 FROM profile_tool_target_scopes s
       WHERE s.workspace_id=OLD.workspace_id AND s.sandbox_id=OLD.sandbox_id)
   BEGIN SELECT RAISE(ABORT,'Workspace placement has Profile target scopes'); END`,
	} {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("hard cut target schema: %w", err)
		}
	}
	return nil
}

func checkConverted(ctx context.Context, tx *sql.Tx, report Report) error {
	var count int
	for _, check := range []struct {
		query  string
		expect int
	}{
		{"SELECT COUNT(*) FROM tools", report.Counts.ToolsAfter},
		{"SELECT COUNT(*) FROM tools_fts", report.Counts.ToolsAfter},
		{"SELECT COUNT(*) FROM native_tool_contracts", 9},
		{"SELECT COUNT(*) FROM hardcut_legacy_tool_identities", report.Counts.ToolsBefore},
		{"SELECT COUNT(*) FROM profile_tools WHERE tool_id NOT IN (SELECT id FROM tools)", 0},
		{"SELECT COUNT(*) FROM provider_configs WHERE plugin_id IN ('io.mender.provider.agent-delegation','io.mender.provider.workspace','io.mender.provider.sandbox-jobs')", 0},
	} {
		if err := tx.QueryRowContext(ctx, check.query).Scan(&count); err != nil {
			return err
		}
		if count != check.expect {
			return fmt.Errorf("hard cut row invariant %s: got %d want %d", check.query, count, check.expect)
		}
	}
	for _, table := range []string{"agent_tool_facades", "sandbox_tool_suite_bindings",
		"sandbox_job_suite_bindings", "workspace_tool_targets"} {
		present, err := tableExists(ctx, tx, table)
		if err != nil {
			return err
		}
		if present {
			return fmt.Errorf("hard cut: legacy table %s remains", table)
		}
	}
	rows, err := tx.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	if rows.Next() {
		_ = rows.Close()
		return errors.New("hard cut: foreign key violation")
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	var integrity string
	if err := tx.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil {
		return err
	}
	if integrity != "ok" {
		return fmt.Errorf("hard cut: SQLite integrity_check: %s", integrity)
	}
	return nil
}
