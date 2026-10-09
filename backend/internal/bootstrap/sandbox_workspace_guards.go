package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// installWorkspaceSandboxBindingGuards makes the persisted Provider->Sandbox
// link invariant hold across multiple Runtime processes and direct DB writers.
// The guards run after the forward-only v1 Workspace migration; they reject
// new v2 references to absent/destroyed execution resources atomically.
func installWorkspaceSandboxBindingGuards(ctx context.Context, path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("workspace binding guards: %w", err)
	}
	uri := url.URL{Scheme: "file", Path: filepath.ToSlash(absolute)}
	q := uri.Query()
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	uri.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return fmt.Errorf("workspace binding guards: database: %w", err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("workspace binding guards: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, statement := range []string{
		`CREATE TRIGGER IF NOT EXISTS workspace_sandbox_v2_insert_guard
   BEFORE INSERT ON provider_configs
   WHEN NEW.plugin_id='io.mender.provider.workspace' AND NEW.plugin_installation_id IS NULL AND NEW.config_version=2
   AND NOT EXISTS (
     SELECT 1 FROM sandboxes s JOIN sandbox_runs r ON r.sandbox_id=s.id
     WHERE s.id=CASE WHEN json_valid(NEW.config_json) THEN json_extract(NEW.config_json,'$.sandboxId') END
       AND r.state NOT IN ('destroying','destroyed')
   )
   BEGIN SELECT RAISE(ABORT,'invalid Workspace Sandbox reference'); END`,
		`CREATE TRIGGER IF NOT EXISTS workspace_sandbox_v2_update_guard
   BEFORE UPDATE OF plugin_id,plugin_installation_id,config_version,config_json ON provider_configs
   WHEN NEW.plugin_id='io.mender.provider.workspace' AND NEW.plugin_installation_id IS NULL AND NEW.config_version=2
   AND NOT EXISTS (
     SELECT 1 FROM sandboxes s JOIN sandbox_runs r ON r.sandbox_id=s.id
     WHERE s.id=CASE WHEN json_valid(NEW.config_json) THEN json_extract(NEW.config_json,'$.sandboxId') END
       AND r.state NOT IN ('destroying','destroyed')
   )
   BEGIN SELECT RAISE(ABORT,'invalid Workspace Sandbox reference'); END`,
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("workspace binding guards: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("workspace binding guards: commit: %w", err)
	}
	return nil
}
