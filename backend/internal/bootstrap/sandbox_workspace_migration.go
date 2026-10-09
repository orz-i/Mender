package bootstrap

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	workspaceplugin "github.com/orz-i/Mender/backend/internal/workspace/infrastructure/plugin"
	_ "modernc.org/sqlite"
)

const workspaceSandboxConfigVersion = 2

type legacyWorkspaceConfig struct {
	Root string `json:"root"`
}
type workspaceMigrationRow struct {
	providerID    string
	configVersion int
	rawConfig     string
}

// migrateWorkspaceProviderV1ToSandbox is a forward-only, one-database upgrade
// bridge. It runs in Runtime composition before provider activation, and uses a
// single SQLite transaction across the Provider Config and Sandbox tables.
// The v1 decoder exists only here: it must not be called from runtime Plugin.Open.
func migrateWorkspaceProviderV1ToSandbox(ctx context.Context, databasePath string, owner governancedomain.Principal, isolation sandboxdomain.IsolationClass) error {
	if err := owner.Validate(); err != nil {
		return fmt.Errorf("migrate Workspace: owner: %w", err)
	}
	if isolation != sandboxdomain.IsolationHost && isolation != sandboxdomain.IsolationRestrictedHost {
		return fmt.Errorf("migrate Workspace: invalid local isolation %q", isolation)
	}
	absolute, err := filepath.Abs(databasePath)
	if err != nil {
		return fmt.Errorf("migrate Workspace: database: %w", err)
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(absolute)}
	params := u.Query()
	params.Add("_pragma", "foreign_keys(1)")
	params.Add("_pragma", "busy_timeout(5000)")
	u.RawQuery = params.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return fmt.Errorf("migrate Workspace: connect: %w", err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("migrate Workspace: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `SELECT provider_id, config_version, config_json FROM provider_configs WHERE plugin_id = ? AND plugin_installation_id IS NULL ORDER BY provider_id`, workspaceplugin.ID.String())
	if err != nil {
		return fmt.Errorf("migrate Workspace: read legacy configs: %w", err)
	}
	var pending []workspaceMigrationRow
	for rows.Next() {
		var row workspaceMigrationRow
		if scanErr := rows.Scan(&row.providerID, &row.configVersion, &row.rawConfig); scanErr != nil {
			_ = rows.Close()
			return fmt.Errorf("migrate Workspace: scan: %w", scanErr)
		}
		pending = append(pending, row)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("migrate Workspace: iterate: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("migrate Workspace: close rows: %w", err)
	}
	for _, row := range pending {
		switch row.configVersion {
		case workspaceSandboxConfigVersion:
			// Already migrated; post-cut Workspace plugin validates the canonical v2 config.
			continue
		case 1:
		default:
			return fmt.Errorf("migrate Workspace Provider %s: unsupported ConfigVersion %d", row.providerID, row.configVersion)
		}
		providerID, err := catalogdomain.ParseProviderID(row.providerID)
		if err != nil {
			return fmt.Errorf("migrate Workspace: ProviderID: %w", err)
		}
		var previous legacyWorkspaceConfig
		decoder := json.NewDecoder(bytes.NewReader([]byte(row.rawConfig)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&previous); err != nil {
			return fmt.Errorf("migrate Workspace %s: legacy config: %w", providerID, err)
		}
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			return fmt.Errorf("migrate Workspace %s: trailing legacy config", providerID)
		}
		if previous.Root == "" || previous.Root != strings.TrimSpace(previous.Root) || len(previous.Root) > 4096 || !filepath.IsAbs(previous.Root) {
			return fmt.Errorf("migrate Workspace %s: root is not a canonical absolute path", providerID)
		}
		root, err := filepath.EvalSymlinks(filepath.Clean(previous.Root))
		if err != nil {
			return fmt.Errorf("migrate Workspace %s: resolve root: %w", providerID, err)
		}
		stat, err := os.Stat(root)
		if err != nil {
			return fmt.Errorf("migrate Workspace %s: inspect root: %w", providerID, err)
		}
		if !stat.IsDir() {
			return fmt.Errorf("migrate Workspace %s: root is not a directory", providerID)
		}
		digest := sha256.Sum256([]byte("mender/local-workspace/v1/" + providerID.String()))
		sandboxID, err := sandboxdomain.ParseID("sbx_" + hex.EncodeToString(digest[:16]))
		if err != nil {
			return fmt.Errorf("migrate Workspace: generated SandboxID: %w", err)
		}
		spec := sandboxdomain.Spec{ID: sandboxID, Name: "Workspace / " + providerID.String(), Backend: sandboxdomain.BackendLocalWorkspace, Isolation: isolation, LocalRoot: filepath.Clean(root), OwnerKind: string(owner.Kind()), OwnerRef: owner.Reference()}
		if _, err := sandboxdomain.NewSandbox(spec); err != nil {
			return fmt.Errorf("migrate Workspace %s: invalid Sandbox: %w", providerID, err)
		}
		run, err := sandboxdomain.NewRun(sandboxID, 1, sandboxdomain.StateReady, "local_workspace:"+sandboxID.String())
		if err != nil {
			return err
		}
		created := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx, `INSERT INTO sandboxes(id,display_name,backend,isolation,local_root,owner_kind,owner_ref,created_at) VALUES(?,?,?,?,?,?,?,?)`, spec.ID.String(), spec.Name, string(spec.Backend), string(spec.Isolation), spec.LocalRoot, spec.OwnerKind, spec.OwnerRef, created); err != nil {
			return fmt.Errorf("migrate Workspace %s: insert Sandbox: %w", providerID, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_runs(sandbox_id,generation,state,external_id,updated_at) VALUES(?,?,?,?,?)`, run.SandboxID.String(), run.Generation, string(run.State), run.ExternalID, created); err != nil {
			return fmt.Errorf("migrate Workspace %s: insert run: %w", providerID, err)
		}
		nextRaw, err := json.Marshal(struct {
			SandboxID string `json:"sandboxId"`
		}{SandboxID: sandboxID.String()})
		if err != nil {
			return err
		}
		next, err := providerplugin.NewConfiguration(providerID, workspaceplugin.ID, workspaceSandboxConfigVersion, nextRaw)
		if err != nil {
			return err
		}
		outcome, err := tx.ExecContext(ctx, `UPDATE provider_configs SET config_version = ?, revision = ?, config_json = ?, credential_refs_json = ?, connection_summary = ? WHERE provider_id = ? AND config_version = 1`, next.ConfigVersion, next.Revision, string(next.Config), "[]", "sandbox:"+sandboxID.String(), providerID.String())
		if err != nil {
			return fmt.Errorf("migrate Workspace %s: update config: %w", providerID, err)
		}
		updated, err := outcome.RowsAffected()
		if err != nil || updated != 1 {
			return fmt.Errorf("migrate Workspace %s: changed before update: %w", providerID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("migrate Workspace: commit: %w", err)
	}
	return nil
}
