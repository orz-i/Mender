package toolhardcut

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
)

// These tables are not migrated, reinterpreted or deleted by the hard cut.
// Recheck their bytes across independent read-only SQLite connections after
// the migration commits; a valid FK or ToolID alone cannot prove no grant,
// Policy, Task/Session or SSH job evidence was lost or broadened.
var preservedHardcutTables = [...]string{
	"profiles", "profile_tools", "profile_skills", "policy_settings",
	"policy_rules", "mcp_delivery_configs", "mcp_delivery_direct_tools",
	"agent_grants", "agent_grant_events", "agent_admissions",
	"agent_tasks", "agent_native_sessions", "agent_execution_attempts",
	"agent_execution_processes", "agent_execution_events", "agent_task_continuations",
	"agent_task_approvals", "agent_approval_policies", "agent_approval_policy_events",
	"agent_workspaces", "agent_workspace_grants", "agent_tool_placements",
	"sandboxes", "sandbox_runs", "sandbox_profile_grants", "sandbox_profile_grant_events",
	"sandbox_admissions", "sandbox_execution_leases", "sandbox_lifecycle_events",
	"sandbox_docker_templates", "sandbox_daytona_templates", "sandbox_ssh_remote_templates",
	"sandbox_external_operations", "sandbox_external_configs",
	"sandbox_ssh_job_obligations", "sandbox_ssh_job_recovery_events",
	"sandbox_ssh_job_admission_controls", "sandbox_ssh_job_admission_events",
	"invocation_audit", "provider_activation_audit",
}

// verifyPreservedEvidence is intentionally not a generic migration framework.
// The fixed tables are copied bit-for-bit by Stage E; v3 target/owner
// identity is validated separately by VerifyStagedCopy.
func verifyPreservedEvidence(ctx context.Context, candidate *sql.Tx, backupPath string) error {
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(backupPath)}
	q := u.Query()
	q.Set("mode", "ro")
	q.Add("_pragma", "query_only(1)")
	u.RawQuery = q.Encode()
	backup, err := sql.Open("sqlite", u.String())
	if err != nil {
		return err
	}
	defer func() { _ = backup.Close() }()
	backup.SetMaxOpenConns(1)
	tx, err := backup.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, table := range preservedHardcutTables {
		before, err := fingerprintPreservedTable(ctx, tx, table)
		if err != nil {
			return fmt.Errorf("hard cut verify: backup table %s: %w", table, err)
		}
		after, err := fingerprintPreservedTable(ctx, candidate, table)
		if err != nil {
			return fmt.Errorf("hard cut verify: candidate table %s: %w", table, err)
		}
		if before != after {
			return fmt.Errorf("hard cut verify: immutable Profile/Policy/Task/Sandbox evidence changed in %s", table)
		}
	}
	return nil
}

func fingerprintPreservedTable(ctx context.Context, tx *sql.Tx, table string) ([sha256.Size]byte, error) {
	// #nosec G202 -- table names only come from the fixed declaration above.
	rows, err := tx.QueryContext(ctx, "SELECT * FROM "+table+" ORDER BY rowid")
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	defer func() { _ = rows.Close() }()
	columns, err := rows.Columns()
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	h := sha256.New()
	encoder := json.NewEncoder(h)
	if err := encoder.Encode(columns); err != nil {
		return [sha256.Size]byte{}, err
	}
	for rows.Next() {
		values := make([]any, len(columns))
		dest := make([]any, len(values))
		for i := range values {
			dest[i] = &values[i]
		}
		if err := rows.Scan(dest...); err != nil {
			return [sha256.Size]byte{}, err
		}
		if err := encoder.Encode(values); err != nil {
			return [sha256.Size]byte{}, err
		}
	}
	if err := rows.Err(); err != nil {
		return [sha256.Size]byte{}, err
	}
	var digest [sha256.Size]byte
	copy(digest[:], h.Sum(nil))
	return digest, nil
}
