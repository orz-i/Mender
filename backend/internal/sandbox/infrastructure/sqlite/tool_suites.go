package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

const workspaceProviderPluginID = "io.mender.provider.workspace"

// GetWorkspaceSuite is a read-only association lookup. Absence is not
// permission to adopt/create a Provider while listing a Sandbox.
func (s *Store) GetWorkspaceSuite(ctx context.Context, id sandboxdomain.ID) (string, error) {
	if _, err := sandboxdomain.ParseID(id.String()); err != nil {
		return "", err
	}
	var providerID string
	err := s.db.QueryRowContext(ctx, "SELECT provider_id FROM sandbox_tool_suite_bindings WHERE sandbox_id=? AND suite_id='sandbox.workspace'", id.String()).Scan(&providerID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get Sandbox Tool Suite: %w", err)
	}
	return providerID, nil
}

// ClaimWorkspaceSuite serializes cross-Runtime claims at the SQLite boundary,
// including when one Provider already exists from the old manual workflow.
// Only exact immutable ConfigVersion=2 Workspace/Sandbox links may be owned.
func (s *Store) ClaimWorkspaceSuite(ctx context.Context, id sandboxdomain.ID, providerID string) error {
	if _, err := sandboxdomain.ParseID(id.String()); err != nil {
		return err
	}
	if providerID == "" {
		return sandboxdomain.ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var current string
	err = tx.QueryRowContext(ctx, "SELECT provider_id FROM sandbox_tool_suite_bindings WHERE sandbox_id=? AND suite_id='sandbox.workspace'", id.String()).Scan(&current)
	if err == nil {
		if current != providerID {
			return sandboxdomain.ErrConflict
		}
		return tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var raw, pluginID string
	var version int
	err = tx.QueryRowContext(ctx, "SELECT plugin_id,config_version,config_json FROM provider_configs WHERE provider_id=? AND plugin_installation_id IS NULL", providerID).Scan(&pluginID, &version, &raw)
	if err != nil {
		return fmt.Errorf("%w: exact Workspace provider missing: %w", sandboxdomain.ErrConflict, err)
	}
	var config struct{ SandboxID string }
	if pluginID != workspaceProviderPluginID || version != 2 ||
		json.Unmarshal([]byte(raw), &config) != nil || config.SandboxID != id.String() {
		return fmt.Errorf("%w: Provider does not target this Sandbox", sandboxdomain.ErrConflict)
	}
	var state string
	if err = tx.QueryRowContext(ctx, "SELECT state FROM sandbox_runs WHERE sandbox_id=?", id.String()).Scan(&state); err != nil {
		return fmt.Errorf("%w: Sandbox run unavailable: %w", sandboxdomain.ErrConflict, err)
	}
	if state == "destroying" || state == "destroyed" {
		return sandboxdomain.ErrConflict
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO sandbox_tool_suite_bindings(sandbox_id,suite_id,provider_id,created_at) VALUES(?,'sandbox.workspace',?,?)", id.String(), providerID, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("%w: concurrent Sandbox suite binding: %w", sandboxdomain.ErrConflict, err)
	}
	return tx.Commit()
}

// IsManagedSandboxSuiteProvider prevents generic Provider CRUD from editing
// either host-owned Workspace or Jobs facade. Explicit Suite retirement is
// separate from generic Provider deletion.
func (s *Store) IsManagedSandboxSuiteProvider(ctx context.Context, providerID string) (bool, error) {
	var v int
	err := s.db.QueryRowContext(ctx, "SELECT 1 FROM sandbox_tool_suite_bindings WHERE provider_id=? UNION ALL SELECT 1 FROM sandbox_job_suite_bindings WHERE provider_id=? LIMIT 1", providerID, providerID).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) ReleaseWorkspaceSuite(ctx context.Context, id sandboxdomain.ID, providerID string) error {
	if _, err := sandboxdomain.ParseID(id.String()); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, "DELETE FROM sandbox_tool_suite_bindings WHERE sandbox_id=? AND suite_id='sandbox.workspace' AND provider_id=?", id.String(), providerID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return sandboxdomain.ErrConflict
	}
	return nil
}
