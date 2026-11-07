package toolhardcut

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// VerifyStagedCopy reopens a completed candidate in read-only mode.
// This verifies an offline upgrade, NOT authorization to promote it to live.
func VerifyStagedCopy(ctx context.Context, artifact StagedCopy) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	candidate, err := filepath.Abs(artifact.CandidatePath)
	if err != nil {
		return err
	}
	backup, err := filepath.Abs(artifact.PreUpgradeBackupPath)
	if err != nil {
		return err
	}
	if candidate == backup || strings.TrimSpace(artifact.CandidatePath) == "" ||
		strings.TrimSpace(artifact.PreUpgradeBackupPath) == "" ||
		!strings.HasPrefix(artifact.BeforeDigest, "sha256:") ||
		!strings.HasPrefix(artifact.AfterFileSHA256, "sha256:") {
		return errors.New("hard cut verify: candidate, backup and reviewed digests required")
	}
	for _, path := range []string{candidate, backup} {
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("hard cut verify: inspect file: %w", err)
		}
		if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
			return errors.New("hard cut verify: candidate/backup must be regular private files")
		}
		for _, suffix := range []string{"-wal", "-shm", "-journal"} {
			_, err = os.Lstat(path + suffix)
			if err == nil {
				return errors.New("hard cut verify: SQLite journal/WAL sidecar present")
			}
			if !os.IsNotExist(err) {
				return err
			}
		}
	}
	hash, err := stagedFileHash(candidate)
	if err != nil {
		return err
	}
	if hash != artifact.AfterFileSHA256 {
		return errors.New("hard cut verify: candidate digest mismatch")
	}
	legacy, err := AssessPath(ctx, backup)
	if err != nil {
		return err
	}
	if legacy.Status != "ready" || legacy.ManifestDigest != artifact.BeforeDigest {
		return errors.New("hard cut verify: backup differs from reviewed ready manifest")
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(candidate)}
	q := u.Query()
	q.Set("mode", "ro")
	q.Add("_pragma", "query_only(1)")
	q.Add("_pragma", "foreign_keys(1)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var version int
	var recordedDigest string
	if err := tx.QueryRowContext(ctx, "SELECT version,before_manifest_digest FROM tool_hardcut_schema_migrations").Scan(&version, &recordedDigest); err != nil {
		return fmt.Errorf("hard cut verify: missing completed migration: %w", err)
	}
	if version != 1 || recordedDigest != artifact.BeforeDigest {
		return errors.New("hard cut verify: upgraded marker does not match the reviewed manifest")
	}
	for _, v := range []struct {
		table   string
		version int
	}{
		{"catalog_schema_migrations", 3}, {"profile_schema_migrations", 8},
		{"policy_schema_migrations", 4}, {"agent_schema_migrations", 20},
		{"sandbox_schema_migrations", 20}, {"provider_config_schema_migrations", 5},
		{"mcp_delivery_schema_migrations", 2},
	} {
		var actual int
		// #nosec G202 -- reviewed, constant migration table names only.
		if err := tx.QueryRowContext(ctx, "SELECT MAX(version) FROM "+v.table).Scan(&actual); err != nil {
			return err
		}
		if actual != v.version {
			return fmt.Errorf("hard cut verify: %s version %d, want %d", v.table, actual, v.version)
		}
	}
	for _, table := range []string{
		"agent_tool_facades", "sandbox_tool_suite_bindings",
		"sandbox_job_suite_bindings", "workspace_tool_targets",
	} {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&n); err != nil {
			return err
		}
		if n != 0 {
			return fmt.Errorf("hard cut verify: retired facade table %s remains", table)
		}
	}
	var tools, fts, native int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM tools").Scan(&tools); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM tools_fts").Scan(&fts); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM native_tool_contracts").Scan(&native); err != nil {
		return err
	}
	if tools != fts || native != 9 || tools != legacy.Counts.ToolsAfter {
		return errors.New("hard cut verify: Catalog, FTS, native contract or reviewed Tool count mismatch")
	}
	var legacyIDs int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM hardcut_legacy_tool_identities").Scan(&legacyIDs); err != nil {
		return err
	}
	if legacyIDs != legacy.Counts.ToolsBefore {
		return errors.New("hard cut verify: lost or invented an old-to-new ToolID mapping")
	}
	for _, mapping := range legacy.ToolMappings {
		var oldProvider, newID, kind, id, operation string
		if err := tx.QueryRowContext(ctx, `SELECT old_provider_id,new_tool_id,new_source_kind,new_source_id,operation
			FROM hardcut_legacy_tool_identities WHERE old_tool_id=?`, mapping.OldToolID).
			Scan(&oldProvider, &newID, &kind, &id, &operation); err != nil {
			return fmt.Errorf("hard cut verify: missing reviewed ToolID lineage: %w", err)
		}
		newKind, newSourceID, ok := strings.Cut(mapping.NewSource, ":")
		if !ok || oldProvider != strings.TrimPrefix(mapping.OldSource, "provider:") ||
			newID != mapping.NewToolID || kind != newKind || id != newSourceID || operation != mapping.Operation {
			return errors.New("hard cut verify: ToolID lineage differs from assessed source")
		}
		var actualKind, actualID, actualKey string
		if err := tx.QueryRowContext(ctx, "SELECT source_kind,source_id,source_key FROM tools WHERE id=?", newID).
			Scan(&actualKind, &actualID, &actualKey); err != nil {
			return fmt.Errorf("hard cut verify: approved canonical ToolID absent: %w", err)
		}
		if actualKind != kind || actualID != id || actualKey != operation {
			return errors.New("hard cut verify: canonical ToolID/source-key was reassigned")
		}
	}
	for _, check := range []string{
		`SELECT COUNT(*) FROM tools t WHERE
		 (t.source_kind='provider' AND NOT EXISTS(SELECT 1 FROM providers p WHERE p.id=t.source_id))
		 OR (t.source_kind='agent' AND NOT EXISTS(SELECT 1 FROM agents a WHERE a.agent_id=t.source_id))
		 OR (t.source_kind='native' AND NOT EXISTS(
		  SELECT 1 FROM native_tool_contracts n WHERE n.tool_id=t.id AND n.source_id=t.source_id
		   AND n.source_key=t.source_key AND n.input_schema=t.input_schema
		   AND n.output_schema=t.output_schema AND n.name=t.name AND n.title=t.title
		   AND n.description=t.description AND n.tags_json=t.tags_json))`,
		`SELECT COUNT(*) FROM native_tool_contracts n WHERE NOT EXISTS(
		  SELECT 1 FROM tools t WHERE t.id=n.tool_id AND t.source_kind='native'
		   AND t.source_id=n.source_id AND t.source_key=n.source_key)`,
		`SELECT COUNT(*) FROM tools t WHERE NOT EXISTS(
		  SELECT 1 FROM tools_fts f WHERE f.tool_id=t.id AND f.source_kind=t.source_kind
		   AND f.source_id=t.source_id AND f.name=t.name AND f.title=t.title
		   AND f.description=t.description)`,
		`SELECT COUNT(*) FROM profile_tools p WHERE NOT EXISTS(SELECT 1 FROM tools t WHERE t.id=p.tool_id)`,
		`SELECT COUNT(*) FROM provider_configs WHERE plugin_id IN
		  ('io.mender.provider.agent-delegation','io.mender.provider.workspace','io.mender.provider.sandbox-jobs')`,
	} {
		var invalid int
		if err := tx.QueryRowContext(ctx, check).Scan(&invalid); err != nil {
			return err
		}
		if invalid != 0 {
			return errors.New("hard cut verify: invalid source, Profile binding or retired Provider")
		}
	}
	rows, err := tx.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	if rows.Next() {
		_ = rows.Close()
		return errors.New("hard cut verify: foreign key violation")
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
		return fmt.Errorf("hard cut verify: SQLite integrity: %s", integrity)
	}
	// The F4 migration may remap Catalog provenance, but it must not rewrite
	// existing Profile grants, Policies, Agent Task evidence or SSH obligations.
	if err := verifyPreservedEvidence(ctx, tx, backup); err != nil {
		return err
	}
	if err := tx.Rollback(); err != nil {
		return err
	}
	after, err := stagedFileHash(candidate)
	if err != nil {
		return err
	}
	if after != artifact.AfterFileSHA256 {
		return errors.New("hard cut verify: candidate modified during read-only verification")
	}
	return nil
}

func stagedFileHash(path string) (string, error) {
	// #nosec G304 -- only verified 0600 regular candidate file.
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, copyErr := io.Copy(h, f)
	closeErr := f.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}
