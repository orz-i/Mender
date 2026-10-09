package daytona

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	_ "modernc.org/sqlite"
)

// TemplateStore is a secondary, same-DB repository. Its single transaction
// reserves a bounded Daytona slot and persists resource, Run and immutable
// template before any remote provider/credential operation can take place.
type TemplateStore struct{ db *sql.DB }

func OpenTemplateStore(ctx context.Context, path string) (*TemplateStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("daytona template database path required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	uri := url.URL{Scheme: "file", Path: filepath.ToSlash(absolute)}
	params := uri.Query()
	params.Add("_pragma", "foreign_keys(1)")
	params.Add("_pragma", "busy_timeout(5000)")
	uri.RawQuery = params.Encode()
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, fmt.Errorf("daytona template database unavailable")
	}
	db.SetMaxOpenConns(1)
	var exists int
	if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sandbox_schema_migrations WHERE version=7`).Scan(&exists); err != nil || exists != 1 {
		_ = db.Close()
		return nil, fmt.Errorf("daytona requires Sandbox schema v7")
	}
	return &TemplateStore{db: db}, nil
}
func (s *TemplateStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}
func (s *TemplateStore) CreateDefinedSandbox(ctx context.Context, item sandboxdomain.Sandbox, t Template) error {
	if s == nil || s.db == nil {
		return sandboxdomain.ErrBackendUnavailable
	}
	if err := ValidateSandbox(item); err != nil {
		return err
	}
	if err := t.Validate(); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("daytona reserve: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	spec := item.Spec()
	var count int
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sandboxes s JOIN sandbox_runs r ON s.id=r.sandbox_id WHERE s.backend='daytona' AND s.owner_kind=? AND s.owner_ref=? AND r.state NOT IN ('destroyed','failed')`, spec.OwnerKind, spec.OwnerRef).Scan(&count)
	if err != nil {
		return fmt.Errorf("daytona reserve: capacity check: %w", err)
	}
	if count >= MaxActivePerPrincipal {
		return sandboxdomain.ErrConflict
	}
	timestamp := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err = tx.ExecContext(ctx, `INSERT INTO sandboxes(id,display_name,backend,isolation,local_root,owner_kind,owner_ref,created_at) VALUES(?,?,?,?,?,?,?,?)`, spec.ID.String(), spec.Name, string(spec.Backend), string(spec.Isolation), "", spec.OwnerKind, spec.OwnerRef, timestamp); err != nil {
		return fmt.Errorf("daytona reserve: Sandbox unavailable: %w", sandboxdomain.ErrConflict)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO sandbox_runs(sandbox_id,generation,state,external_id,updated_at) VALUES(?,1,'defined','',?)`, spec.ID.String(), timestamp); err != nil {
		return fmt.Errorf("daytona reserve: run: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO sandbox_daytona_templates(sandbox_id,credential_ref,image_digest,target,cpu,memory_gib,disk_gib,ttl_minutes,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, spec.ID.String(), t.CredentialRef.String(), t.ImageDigest, t.Target, t.CPU, t.MemoryGiB, t.DiskGiB, t.TTLMinutes, timestamp); err != nil {
		return fmt.Errorf("daytona reserve: template: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("daytona reserve: commit: %w", err)
	}
	return nil
}
func (s *TemplateStore) GetDaytonaTemplate(ctx context.Context, id sandboxdomain.ID) (Template, error) {
	if s == nil || s.db == nil {
		return Template{}, sandboxdomain.ErrBackendUnavailable
	}
	if _, err := sandboxdomain.ParseID(id.String()); err != nil {
		return Template{}, err
	}
	var result Template
	var ref string
	err := s.db.QueryRowContext(ctx, `SELECT credential_ref,image_digest,target,cpu,memory_gib,disk_gib,ttl_minutes FROM sandbox_daytona_templates WHERE sandbox_id=?`, id.String()).Scan(&ref, &result.ImageDigest, &result.Target, &result.CPU, &result.MemoryGiB, &result.DiskGiB, &result.TTLMinutes)
	if errors.Is(err, sql.ErrNoRows) {
		return Template{}, sandboxdomain.ErrNotFound
	}
	if err != nil {
		return Template{}, fmt.Errorf("daytona template lookup unavailable")
	}
	result.CredentialRef = credentialdomain.Reference(ref)
	if err := result.Validate(); err != nil {
		return Template{}, fmt.Errorf("daytona template corrupted: %w", sandboxdomain.ErrBackendUnavailable)
	}
	return result, nil
}
