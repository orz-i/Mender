package toolhardcut

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/orz-i/Mender/backend/internal/catalog/infrastructure/sourcecatalog"
	"github.com/orz-i/Mender/backend/internal/toolsuite/infrastructure/curated"
)

// StagedCopy is a new, non-running SQLite database produced from a consistent
// backup. Source remains untouched. It is NOT a production cutover: Stage F
// must replace old runtime consumers, and an operator must separately
// authorize a quiesced swap after all dry-run gates pass.
type StagedCopy struct {
	CandidatePath        string `json:"candidatePath"`
	PreUpgradeBackupPath string `json:"preUpgradeBackupPath"`
	BeforeDigest         string `json:"beforeDigest"`
	AfterFileSHA256      string `json:"afterFileSha256"`
}

// StageCopy never modifies sourcePath; it requires two explicit, nonexistent
// output files (destination and destination+".preupgrade"). The online backup
// captures WAL content and the manifest is re-assessed in the same SQLite
// transaction immediately before any DDL. Any conflicting/uncertain resource
// blocks, without fallback or implicit grant.
func StageCopy(ctx context.Context, sourcePath, destinationPath, expectedManifestDigest string) (StagedCopy, error) {
	if expectedManifestDigest == "" {
		return StagedCopy{}, errors.New("hard cut: explicit reviewed manifest digest required")
	}
	report, err := AssessPath(ctx, sourcePath)
	if err != nil {
		return StagedCopy{}, err
	}
	if report.ManifestDigest != expectedManifestDigest {
		return StagedCopy{}, errors.New("hard cut: assessment snapshot changed")
	}
	if report.Status != "ready" {
		return StagedCopy{}, fmt.Errorf("hard cut: assessment %s with %d blockers and/or review decisions",
			report.Status, report.Counts.UnresolvedBlockers)
	}
	dest, err := filepath.Abs(destinationPath)
	if err != nil {
		return StagedCopy{}, err
	}
	src, err := filepath.Abs(sourcePath)
	if err != nil {
		return StagedCopy{}, err
	}
	backup := dest + ".preupgrade"
	if dest == src || src == backup || dest == backup || strings.TrimSpace(destinationPath) == "" {
		return StagedCopy{}, errors.New("hard cut: source and staged paths must be distinct")
	}
	for _, base := range []string{dest, backup} {
		for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
			path := base + suffix
			if _, err := os.Lstat(path); err == nil {
				return StagedCopy{}, fmt.Errorf("hard cut: candidate/backup file or journal already exists")
			} else if !os.IsNotExist(err) {
				return StagedCopy{}, err
			}
		}
	}
	before, err := BackupSnapshot(ctx, src, backup)
	if err != nil {
		return StagedCopy{}, err
	}
	if before.ManifestDigest != report.ManifestDigest {
		return StagedCopy{}, errors.New("hard cut: source changed during backup")
	}
	staged, err := BackupSnapshot(ctx, backup, dest)
	if err != nil {
		return StagedCopy{}, err
	}
	if staged.ManifestDigest != report.ManifestDigest {
		return StagedCopy{}, errors.New("hard cut: staged copy differs from reviewed backup")
	}
	// A failure leaves the untouched .preupgrade backup for investigation,
	// but removes any partial upgraded candidate. Both remain off the live path.
	completed := false
	defer func() {
		if !completed {
			for _, suffix := range []string{"-wal", "-shm", "-journal", ""} {
				_ = os.Remove(dest + suffix)
			}
		}
	}()
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(dest)}
	q := u.Query()
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return StagedCopy{}, err
	}
	db.SetMaxOpenConns(1)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		_ = db.Close()
		return StagedCopy{}, err
	}
	defer func() { _ = tx.Rollback(); _ = db.Close() }()
	current, err := assessTx(ctx, tx)
	if err != nil {
		return StagedCopy{}, err
	}
	if current.ManifestDigest != expectedManifestDigest || current.Status != "ready" {
		return StagedCopy{}, errors.New("hard cut: candidate changed at transactional admission")
	}
	if err := applyTypedSchema(ctx, tx, current); err != nil {
		return StagedCopy{}, err
	}
	if err := checkConverted(ctx, tx, current); err != nil {
		return StagedCopy{}, err
	}
	if err := tx.Commit(); err != nil {
		return StagedCopy{}, err
	}
	var mode string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode=DELETE").Scan(&mode); err != nil || mode != "delete" {
		return StagedCopy{}, fmt.Errorf("hard cut: candidate journal checkpoint failed: %w", err)
	}
	if err := db.Close(); err != nil {
		return StagedCopy{}, err
	}
	fileHash, err := stagedFileHash(dest)
	if err != nil {
		return StagedCopy{}, err
	}
	artifact := StagedCopy{
		CandidatePath: dest, PreUpgradeBackupPath: backup, BeforeDigest: report.ManifestDigest,
		AfterFileSHA256: fileHash,
	}
	// Reopen the committed new-Schema database with an independent read-only
	// connection; a committed upgrade is not accepted on in-transaction evidence
	// alone. On failure, discard only the candidate and retain its backup.
	if err := VerifyStagedCopy(ctx, artifact); err != nil {
		return StagedCopy{}, fmt.Errorf("verify committed hard-cut candidate: %w", err)
	}
	completed = true
	return artifact, nil
}

func applyTypedSchema(ctx context.Context, tx *sql.Tx, report Report) error {
	// Explicit retirement of known old trigger contracts; reject any
	// unknown direct dependency on old tools/provider_id before proceeding.
	retired := []string{
		"catalog_tool_insert_revision", "catalog_tool_update_revision", "catalog_tool_delete_revision",
		"profile_tool_reference_insert_guard", "profile_tool_reference_update_guard",
		"policy_reference_insert_guard", "policy_reference_update_guard",
		"provider_reference_delete_guard", "agent_facade_provider_config_delete_guard",
		"agent_facade_provider_config_update_guard", "agent_bound_provider_delete_guard",
		"sandbox_suite_config_delete_guard", "sandbox_suite_config_update_guard",
		"workspace_tool_target_suite_guard", "workspace_tool_target_sandbox_guard",
		"sandbox_workspace_suite_insert_guard", "sandbox_jobs_suite_insert_guard",
		"workspace_sandbox_v2_insert_guard", "workspace_sandbox_v2_update_guard",
		"sandbox_jobs_config_insert_guard", "sandbox_jobs_config_update_guard",
	}
	known := map[string]bool{}
	for _, name := range retired {
		known[name] = true
	}
	rows, err := tx.QueryContext(ctx, "SELECT name,sql FROM sqlite_master WHERE type='trigger'")
	if err != nil {
		return err
	}
	for rows.Next() {
		var name, body string
		if err := rows.Scan(&name, &body); err != nil {
			_ = rows.Close()
			return err
		}
		if strings.Contains(strings.ToLower(body), "t.provider_id") ||
			strings.Contains(strings.ToLower(body), "tools.provider_id") ||
			strings.Contains(strings.ToLower(body), "agent_tool_facades") ||
			strings.Contains(strings.ToLower(body), "sandbox_tool_suite_bindings") ||
			strings.Contains(strings.ToLower(body), "sandbox_job_suite_bindings") ||
			strings.Contains(strings.ToLower(body), "workspace_tool_targets") {
			if !known[name] {
				_ = rows.Close()
				return fmt.Errorf("hard cut: unreviewed legacy trigger %s", name)
			}
		}
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	for _, name := range retired {
		if _, err := tx.ExecContext(ctx, "DROP TRIGGER IF EXISTS "+name); err != nil {
			return fmt.Errorf("retire trigger %s: %w", name, err)
		}
	}
	// All providers with a facade source have already passed strict pin checks.
	// Preserve Agent ToolID unchanged and record exact provenance below.
	if _, err := tx.ExecContext(ctx, `CREATE TABLE hardcut_legacy_tool_identities(
  old_tool_id TEXT PRIMARY KEY,
  old_provider_id TEXT NOT NULL,
  new_tool_id TEXT NOT NULL,
  new_source_kind TEXT NOT NULL,
  new_source_id TEXT NOT NULL,
  operation TEXT NOT NULL,
  CHECK(new_source_kind IN ('provider','agent','native'))
 )`); err != nil {
		return err
	}
	for _, m := range report.ToolMappings {
		kind, id, ok := strings.Cut(m.NewSource, ":")
		if !ok {
			return errors.New("hard cut: invalid typed SourceRef mapping")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO hardcut_legacy_tool_identities
   (old_tool_id,old_provider_id,new_tool_id,new_source_kind,new_source_id,operation)
   VALUES(?,?,?,?,?,?)`, m.OldToolID, strings.TrimPrefix(m.OldSource, "provider:"), m.NewToolID, kind, id, m.Operation); err != nil {
			return err
		}
	}
	schema := sourcecatalog.FreshSchema()
	// Stage a new physical table without renaming the active one; SQLite's
	// foreign references and trigger bodies must NOT be blindly auto-rewritten.
	toolsV3 := strings.Replace(schema[1], "CREATE TABLE tools (", "CREATE TABLE tools_v3 (", 1)
	if toolsV3 == schema[1] {
		return errors.New("hard cut: source catalog table definition changed unexpectedly")
	}
	if _, err := tx.ExecContext(ctx, toolsV3); err != nil {
		return err
	}
	for _, m := range report.ToolMappings {
		kind, id, ok := strings.Cut(m.NewSource, ":")
		if !ok {
			return errors.New("hard cut: invalid source mapping")
		}
		if kind == "native" {
			// Old per-Sandbox ToolIDs are retired. Stage the single reviewed
			// native operation once below; retain exact old→new audit mapping.
			// An owner-approved non-equivalent Profile/Policy/Job target would
			// already have blocked the assessed manifest.
			if m.Status != "requires_review" || len(m.AllowedTargetRefs) == 0 {
				return errors.New("hard cut: native mapping lacks exact reviewed targets")
			}
			continue
		}
		// Exact read-only manifest mapping governs the converted identity; original
		// Schema, tags, timing, title, and ToolID are copied bit-for-bit.
		if _, err := tx.ExecContext(ctx, `INSERT INTO tools_v3
   (id,source_kind,source_id,source_key,name,title,description,input_schema,output_schema,tags_json,created_at,updated_at)
   SELECT id,?,?,source_key,name,title,description,input_schema,output_schema,tags_json,created_at,updated_at
   FROM tools WHERE id=? AND source_key=?`, kind, id, m.OldToolID, m.Operation); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, "DROP TABLE tools_fts"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DROP TABLE tools"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "ALTER TABLE tools_v3 RENAME TO tools"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, schema[2]); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, schema[3]); err != nil {
		return err
	}
	// Host-pinned native tools have one catalog identity per operation, not one
	// per Sandbox. They are not selected by any Profile until F completes grants.
	now := time.Now().UTC()
	native, err := curated.PlannedNativeCatalogTools(now)
	if err != nil {
		return err
	}
	for _, tool := range native {
		source := tool.SourceRef()
		output := tool.OutputSchema()
		if output == nil {
			return errors.New("hard cut: native output schema missing")
		}
		rawTags, err := json.Marshal(tool.Tags())
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO tools
   (id,source_kind,source_id,source_key,name,title,description,input_schema,output_schema,tags_json,created_at,updated_at)
   VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, tool.ID().String(), string(source.Kind()), source.ID(),
			tool.SourceKey(), tool.Name(), tool.Title(), tool.Description(), tool.InputSchema().Bytes(),
			output.Bytes(), string(rawTags), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	// Manually populated FTS matches the canonical Tool snapshot, with no
	// Provider-ID filtering or "private source as Provider" behavior.
	if _, err := tx.ExecContext(ctx, `INSERT INTO tools_fts
 (tool_id,source_kind,source_id,name,title,description,tags)
 SELECT id,source_kind,source_id,name,title,description,
   COALESCE((SELECT group_concat(j.value,' ') FROM json_each(tools.tags_json) j),'')
 FROM tools`); err != nil {
		return err
	}
	for _, stmt := range schema[6:] {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	// Retire only pinned, assessed facades. Nonlocal Workspace and durable Job
	// sources fail the assessor until trusted placement/recovery proofs exist.
	// No unverified resource may silently become a global native target.
	if _, err := tx.ExecContext(ctx, "DROP TABLE IF EXISTS workspace_tool_targets"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DROP TABLE IF EXISTS sandbox_tool_suite_bindings"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DROP TABLE IF EXISTS sandbox_job_suite_bindings"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DROP TABLE IF EXISTS agent_tool_facades"); err != nil {
		return err
	}
	for _, m := range report.ToolMappings {
		if strings.HasPrefix(m.NewSource, "provider:") {
			continue
		}
		providerID := strings.TrimPrefix(m.OldSource, "provider:")
		// Only exact verified pinned internal facades reach this branch.
		// Duplicate DELETEs are idempotent within the one transaction.
		if _, err := tx.ExecContext(ctx, "DELETE FROM provider_configs WHERE provider_id=?", providerID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM providers WHERE id=?", providerID); err != nil {
			return err
		}
	}
	if err := installHardcutReferenceGuards(ctx, tx, schema); err != nil {
		return err
	}
	if err := installHardcutTargetSchema(ctx, tx); err != nil {
		return err
	}
	if err := insertVerifiedWorkspaces(ctx, tx, report); err != nil {
		return err
	}
	for _, record := range []struct {
		table   string
		version int
	}{
		{"catalog_schema_migrations", 3},
		{"profile_schema_migrations", 8},
		{"policy_schema_migrations", 4},
		{"agent_schema_migrations", 20},
		{"sandbox_schema_migrations", 20},
		{"provider_config_schema_migrations", 5},
		{"mcp_delivery_schema_migrations", 2},
	} {
		// #nosec G202 -- record.table originates from the fixed context migration table list above.
		if _, err := tx.ExecContext(ctx, "INSERT INTO "+record.table+"(version,applied_at) VALUES (?,?)",
			record.version, now.Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, "UPDATE catalog_tool_revision SET revision=revision+1 WHERE id=1"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `CREATE TABLE tool_hardcut_schema_migrations(
  version INTEGER PRIMARY KEY CHECK(version=1), applied_at TEXT NOT NULL,
  before_manifest_digest TEXT NOT NULL
 )`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO tool_hardcut_schema_migrations VALUES(1,?,?)",
		now.Format(time.RFC3339Nano), report.ManifestDigest); err != nil {
		return err
	}
	return nil
}
