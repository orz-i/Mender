package dockerengine

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	_ "modernc.org/sqlite"
)

// TemplateStore keeps reviewed, immutable Docker Sandbox parameters in the
// same Mender SQLite DB as Sandboxes and their Run generation. No daemon socket,
// privileged flags, registry secret or mutable image tag is persisted.
type TemplateStore struct{ db *sql.DB }

func OpenTemplateStore(ctx context.Context, path string) (*TemplateStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("docker template store: database path required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("docker template store: database path: %w", err)
	}
	uri := url.URL{Scheme: "file", Path: filepath.ToSlash(absolute)}
	params := uri.Query()
	params.Add("_pragma", "foreign_keys(1)")
	params.Add("_pragma", "busy_timeout(5000)")
	uri.RawQuery = params.Encode()
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, fmt.Errorf("docker template store: open: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("docker template store: connect: %w", err)
	}
	// Sandbox SQLite migrations run before the driver store can be opened; never
	// create a separate conflicting schema or accept unknown old DB layouts.
	var table string
	if err := db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name='sandbox_docker_templates'`).Scan(&table); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("docker template store: Sandbox schema v6 required: %w", err)
	}
	return &TemplateStore{db: db}, nil
}
func (s *TemplateStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// CreateDockerTemplate is a trusted, single-assignment management store
// operation. It works only on a recorded Docker Sandbox whose Run is defined.
// It is not a public tool, and D1d must call it after sandbox.provision policy.
func (s *TemplateStore) CreateDockerTemplate(ctx context.Context, id sandboxdomain.ID, t Template) error {
	if s == nil || s.db == nil {
		return sandboxdomain.ErrBackendUnavailable
	}
	if _, err := sandboxdomain.ParseID(id.String()); err != nil {
		return err
	}
	if err := t.Validate(); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO sandbox_docker_templates(sandbox_id,image_digest,user_id,group_id,cpu_milli,memory_mib,workspace_mib,pid_limit,created_at)
 SELECT ?,?,?,?,?,?,?,?,? WHERE EXISTS(
   SELECT 1 FROM sandboxes AS s JOIN sandbox_runs AS r ON r.sandbox_id=s.id
   WHERE s.id=? AND s.backend='docker' AND s.isolation='container' AND r.state='defined'
 )`, id.String(), t.ImageDigest, t.UserID, t.GroupID, t.CPUmilli, t.MemoryMiB, t.WorkspaceMiB, t.PIDs, time.Now().UTC().Format(time.RFC3339Nano), id.String())
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "constraint failed") || strings.Contains(err.Error(), "immutable") {
			return sandboxdomain.ErrConflict
		}
		return fmt.Errorf("docker template store: create: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("docker template store: rows: %w", err)
	}
	if count != 1 {
		return sandboxdomain.ErrConflict
	}
	return nil
}
func (s *TemplateStore) GetDockerTemplate(ctx context.Context, id sandboxdomain.ID) (Template, error) {
	if s == nil || s.db == nil {
		return Template{}, sandboxdomain.ErrBackendUnavailable
	}
	if _, err := sandboxdomain.ParseID(id.String()); err != nil {
		return Template{}, err
	}
	var t Template
	err := s.db.QueryRowContext(ctx, `SELECT image_digest,user_id,group_id,cpu_milli,memory_mib,workspace_mib,pid_limit FROM sandbox_docker_templates WHERE sandbox_id=?`, id.String()).Scan(&t.ImageDigest, &t.UserID, &t.GroupID, &t.CPUmilli, &t.MemoryMiB, &t.WorkspaceMiB, &t.PIDs)
	if errors.Is(err, sql.ErrNoRows) {
		return Template{}, sandboxdomain.ErrNotFound
	}
	if err != nil {
		return Template{}, fmt.Errorf("docker template store: get: %w", err)
	}
	if err := t.Validate(); err != nil {
		return Template{}, fmt.Errorf("docker template store: corrupt template: %w", sandboxdomain.ErrBackendUnavailable)
	}
	return t, nil
}

var _ TemplateLookup = (*TemplateStore)(nil)

// CreateDefinedSandbox persists one admitted Docker Sandbox, defined Run and
// immutable Template as ONE SQLite transaction. No daemon/container side
// effect occurs here. An interrupted create cannot leave half the contract.
func (s *TemplateStore) CreateDefinedSandbox(ctx context.Context, item sandboxdomain.Sandbox, template Template) error {
	if s == nil || s.db == nil {
		return sandboxdomain.ErrBackendUnavailable
	}
	if err := item.Validate(); err != nil {
		return err
	}
	if item.Backend() != sandboxdomain.BackendDocker || item.Isolation() != sandboxdomain.IsolationContainer {
		return sandboxdomain.ErrBackendUnavailable
	}
	if err := template.Validate(); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("docker template: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	spec := item.Spec()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = tx.ExecContext(ctx, `INSERT INTO sandboxes(id,display_name,backend,backend_plugin_id,backend_extension_id,backend_installation_id,isolation,local_root,owner_kind,owner_ref,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, spec.ID.String(), spec.Name, string(spec.Backend), spec.Binding.PluginID.String(), spec.Binding.ExtensionID.String(), spec.Binding.InstallationID.String(), string(spec.Isolation), spec.LocalRoot, spec.OwnerKind, spec.OwnerRef, now)
	if err != nil {
		return sandboxdomain.ErrConflict
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO sandbox_runs(sandbox_id,generation,state,external_id,updated_at) VALUES(?,1,'defined','',?)`, spec.ID.String(), now)
	if err != nil {
		return fmt.Errorf("docker template: run insert: %w", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO sandbox_docker_templates(sandbox_id,image_digest,user_id,group_id,cpu_milli,memory_mib,workspace_mib,pid_limit,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, spec.ID.String(), template.ImageDigest, template.UserID, template.GroupID, template.CPUmilli, template.MemoryMiB, template.WorkspaceMiB, template.PIDs, now)
	if err != nil {
		return fmt.Errorf("docker template: immutable config insert: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("docker template: atomic commit: %w", err)
	}
	return nil
}
