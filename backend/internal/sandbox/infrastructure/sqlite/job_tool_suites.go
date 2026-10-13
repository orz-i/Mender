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

const jobsProviderPluginID = "io.mender.provider.sandbox-jobs"

// GetJobSuite is a read-only association. A missing binding never creates or
// adopts a generic Provider, and catalogue reads never open SSH connections.
func (s *Store) GetJobSuite(ctx context.Context, id sandboxdomain.ID) (string, error) {
	if _, err := sandboxdomain.ParseID(id.String()); err != nil {
		return "", err
	}
	var provider string
	err := s.db.QueryRowContext(ctx, `SELECT provider_id FROM sandbox_job_suite_bindings WHERE sandbox_id=?`, id.String()).Scan(&provider)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get SSH Jobs suite: %w", err)
	}
	return provider, nil
}

// ClaimJobSuite binds only an exact, host-owned first-party facade to one
// SSH Sandbox. It never accepts arbitrary user-provided PluginID/ConfigVersion
// or a BackendKind substitution, even if a Provider config was manually forged.
func (s *Store) ClaimJobSuite(ctx context.Context, id sandboxdomain.ID, providerID string) error {
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
	var existing string
	err = tx.QueryRowContext(ctx, `SELECT provider_id FROM sandbox_job_suite_bindings WHERE sandbox_id=?`, id.String()).Scan(&existing)
	if err == nil {
		if existing != providerID {
			return sandboxdomain.ErrConflict
		}
		return tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var pluginID, raw, state, backend, backendID, loginUser string
	var configVersion int
	err = tx.QueryRowContext(ctx, `SELECT p.plugin_id,p.config_version,p.config_json,r.state,s.backend,s.backend_plugin_id,t.login_user
 FROM provider_configs p JOIN sandbox_runs r ON r.sandbox_id=?
 JOIN sandboxes s ON s.id=r.sandbox_id LEFT JOIN sandbox_ssh_remote_templates t ON t.sandbox_id=s.id
 WHERE p.provider_id=? AND p.plugin_installation_id IS NULL`, id.String(), providerID).
		Scan(&pluginID, &configVersion, &raw, &state, &backend, &backendID, &loginUser)
	if err != nil {
		return fmt.Errorf("%w: SSH Jobs provider missing: %w", sandboxdomain.ErrConflict, err)
	}
	var cfg struct {
		SandboxID string `json:"sandboxId"`
	}
	if pluginID != jobsProviderPluginID || configVersion != 1 || json.Unmarshal([]byte(raw), &cfg) != nil || cfg.SandboxID != id.String() ||
		backend != string(sandboxdomain.BackendSSHRemote) || backendID != "io.mender.sandbox.ssh-remote" || loginUser != "root" ||
		state == string(sandboxdomain.StateDestroyed) || state == string(sandboxdomain.StateDestroying) {
		return sandboxdomain.ErrConflict
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO sandbox_job_suite_bindings(sandbox_id,provider_id,created_at) VALUES(?,?,?)`, id.String(), providerID, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("%w: concurrent SSH Jobs Suite binding: %w", sandboxdomain.ErrConflict, err)
	}
	return tx.Commit()
}

// ReleaseJobSuite only removes the reviewed host association after Runtime
// confirms no Profile/Policy reference or remote active/uncertain Job.
// TaskIDs, terminal tombstones and lifecycle obligation remain backend-owned.
func (s *Store) ReleaseJobSuite(ctx context.Context, id sandboxdomain.ID, providerID string) error {
	if _, err := sandboxdomain.ParseID(id.String()); err != nil {
		return err
	}
	outcome, err := s.db.ExecContext(ctx, `DELETE FROM sandbox_job_suite_bindings WHERE sandbox_id=? AND provider_id=?`, id.String(), providerID)
	if err != nil {
		return err
	}
	n, err := outcome.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return sandboxdomain.ErrConflict
	}
	return nil
}
