package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// installSandboxProviderBindingGuards blocks late Workspace/Jobs Providers
// and managed Tool Suite references to destroyed Sandbox runs. It is installed
// after the forward-only Workspace migration, across all Runtime processes.
func installSandboxProviderBindingGuards(ctx context.Context, path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("sandbox provider binding guards: %w", err)
	}
	slashed := filepath.ToSlash(absolute)
	if !strings.HasPrefix(slashed, "/") {
		slashed = "/" + slashed
	}
	uri := url.URL{Scheme: "file", Path: slashed}
	q := uri.Query()
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	uri.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return fmt.Errorf("sandbox provider binding guards: database: %w", err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sandbox provider binding guards: begin: %w", err)
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
		`CREATE TRIGGER IF NOT EXISTS sandbox_jobs_config_insert_guard
   BEFORE INSERT ON provider_configs
   WHEN NEW.plugin_id='io.mender.provider.sandbox-jobs' AND NEW.plugin_installation_id IS NULL AND NEW.config_version=1
   AND NOT EXISTS (
     SELECT 1 FROM sandboxes s JOIN sandbox_runs r ON r.sandbox_id=s.id
     WHERE s.id=CASE WHEN json_valid(NEW.config_json) THEN json_extract(NEW.config_json,'$.sandboxId') END
       AND s.backend='ssh_remote' AND s.backend_plugin_id='io.mender.sandbox.ssh-remote'
       AND s.backend_installation_id='' AND r.state NOT IN ('destroying','destroyed')
   )
   BEGIN SELECT RAISE(ABORT,'invalid SSH Jobs Sandbox reference'); END`,
		`CREATE TRIGGER IF NOT EXISTS sandbox_jobs_config_update_guard
   BEFORE UPDATE OF plugin_id,plugin_installation_id,config_version,config_json ON provider_configs
   WHEN NEW.plugin_id='io.mender.provider.sandbox-jobs' AND NEW.plugin_installation_id IS NULL AND NEW.config_version=1
   AND NOT EXISTS (
     SELECT 1 FROM sandboxes s JOIN sandbox_runs r ON r.sandbox_id=s.id
     WHERE s.id=CASE WHEN json_valid(NEW.config_json) THEN json_extract(NEW.config_json,'$.sandboxId') END
       AND s.backend='ssh_remote' AND s.backend_plugin_id='io.mender.sandbox.ssh-remote'
       AND s.backend_installation_id='' AND r.state NOT IN ('destroying','destroyed')
   )
   BEGIN SELECT RAISE(ABORT,'invalid SSH Jobs Sandbox reference'); END`,
		`CREATE TRIGGER IF NOT EXISTS sandbox_workspace_suite_insert_guard
   BEFORE INSERT ON sandbox_tool_suite_bindings
   WHEN NOT EXISTS (SELECT 1 FROM sandbox_runs WHERE sandbox_id=NEW.sandbox_id
     AND state NOT IN ('destroying','destroyed'))
   BEGIN SELECT RAISE(ABORT,'invalid Workspace Suite Sandbox reference'); END`,
		`CREATE TRIGGER IF NOT EXISTS sandbox_jobs_suite_insert_guard
   BEFORE INSERT ON sandbox_job_suite_bindings
   WHEN NOT EXISTS (SELECT 1 FROM sandbox_runs WHERE sandbox_id=NEW.sandbox_id
     AND state NOT IN ('destroying','destroyed'))
   BEGIN SELECT RAISE(ABORT,'invalid Jobs Suite Sandbox reference'); END`,
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("sandbox provider binding guards: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sandbox provider binding guards: commit: %w", err)
	}
	return nil
}
