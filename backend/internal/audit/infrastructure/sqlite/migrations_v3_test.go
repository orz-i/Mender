package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"

	_ "modernc.org/sqlite"
)

func TestOpenMigratesAuditV2AndPreservesLegacyRows(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "audit.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE audit_schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE invocation_audit (
		invocation_id TEXT PRIMARY KEY,
		tool_id TEXT NOT NULL,
		provider_id TEXT,
		caller TEXT NOT NULL,
		started_at TEXT NOT NULL,
		finished_at TEXT NOT NULL,
		policy_kind TEXT NOT NULL,
		policy_reason TEXT NOT NULL,
		approval TEXT NOT NULL,
		status TEXT NOT NULL,
		result_kind TEXT NOT NULL,
		error_class TEXT NOT NULL,
		error_summary TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		"INSERT INTO audit_schema_migrations(version, applied_at) VALUES (1, 'v1'), (2, 'v2')",
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO invocation_audit(
		invocation_id,
		tool_id,
		provider_id,
		caller,
		started_at,
		finished_at,
		policy_kind,
		policy_reason,
		approval,
		status,
		result_kind,
		error_class,
		error_summary
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"inv_00000000000000000000000000000001",
		"tool_00000000000000000000000000000001",
		"prov_00000000000000000000000000000001",
		"legacy-cli",
		"2026-09-25T12:00:00Z",
		"2026-09-25T12:00:01Z",
		"allow",
		"local.default",
		"not_required",
		"completed",
		"complete",
		"",
		"",
	); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = store.Close() }()

	var version int
	if err := store.db.QueryRowContext(
		context.Background(),
		"SELECT MAX(version) FROM audit_schema_migrations",
	).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 3 {
		t.Fatalf("schema version = %d, want 3", version)
	}

	invocationID, err := invocationdomain.ParseInvocationID(
		"inv_00000000000000000000000000000001",
	)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Get(context.Background(), invocationID)
	if err != nil {
		t.Fatalf("Get() legacy row error = %v", err)
	}
	if record.Origin() != "" ||
		record.ScopeKind() != "" ||
		record.ScopeReference() != "" ||
		record.OutcomeKind() != "" ||
		record.RejectionCode() != "" ||
		record.FailureSource() != "" ||
		record.FailureCode() != "" {
		t.Fatalf(
			"legacy governance metadata = origin %q scope %q/%q outcome %q rejection %q failure %q/%q",
			record.Origin(),
			record.ScopeKind(),
			record.ScopeReference(),
			record.OutcomeKind(),
			record.RejectionCode(),
			record.FailureSource(),
			record.FailureCode(),
		)
	}
}
