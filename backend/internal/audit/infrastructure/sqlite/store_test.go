package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"slices"
	"testing"
	"time"

	auditdomain "github.com/orz-i/Mender/backend/internal/audit/domain"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
)

func TestAuditRecordRoundTrip(t *testing.T) {
	t.Parallel()

	store := openTestStore(t)
	record := completedRecord(t)
	executionContext, err := invocationdomain.NewInvocationContext(
		invocationdomain.OriginCLI,
		"cli",
		invocationdomain.CatalogExecutionScope(),
	)
	if err != nil {
		t.Fatal(err)
	}
	record, err = record.WithExecutionContext(executionContext)
	if err != nil {
		t.Fatal(err)
	}
	content, err := invocationdomain.NewJSONValue([]byte(`{"ok":true}`))
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := invocationdomain.Complete(content)
	if err != nil {
		t.Fatal(err)
	}
	record, err = record.WithOutcome(outcome)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.Record(context.Background(), record); err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	loaded, err := store.Get(context.Background(), record.InvocationID())
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if loaded.InvocationID() != record.InvocationID() ||
		loaded.ToolID() != record.ToolID() ||
		loaded.Status() != record.Status() ||
		loaded.ResultKind() != record.ResultKind() {
		t.Fatalf("loaded Record = %#v, want %#v", loaded, record)
	}
	if loaded.ProviderID() == nil || *loaded.ProviderID() != *record.ProviderID() {
		t.Fatalf("ProviderID() = %v, want %v", loaded.ProviderID(), record.ProviderID())
	}
	if loaded.Origin() != invocationdomain.OriginCLI ||
		loaded.ScopeKind() != invocationdomain.ScopeCatalog ||
		loaded.ScopeReference() != "" ||
		loaded.OutcomeKind() != invocationdomain.OutcomeKindComplete {
		t.Fatalf(
			"governance metadata = origin %q scope %q/%q outcome %q",
			loaded.Origin(),
			loaded.ScopeKind(),
			loaded.ScopeReference(),
			loaded.OutcomeKind(),
		)
	}
}

func TestAuditErrorSummaryRoundTrip(t *testing.T) {
	t.Parallel()

	store := openTestStore(t)
	invocationID, _ := invocationdomain.ParseInvocationID("inv_00000000000000000000000000000003")
	toolID, _ := catalogdomain.ParseToolID("tool_00000000000000000000000000000003")
	providerID, _ := catalogdomain.ParseProviderID("prov_00000000000000000000000000000003")
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

	record, err := auditdomain.NewRecord(
		invocationID,
		toolID,
		&providerID,
		"desktop",
		now,
		now.Add(time.Millisecond),
		policydomain.DecisionAllow,
		"local.default",
		auditdomain.ApprovalNotRequired,
		auditdomain.StatusFailed,
		"",
		auditdomain.ErrorExecution,
	)
	if err != nil {
		t.Fatal(err)
	}
	record = record.WithErrorSummary("provider rejected input")

	if err := store.Record(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Get(context.Background(), invocationID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ErrorSummary() != "provider rejected input" {
		t.Fatalf("ErrorSummary() = %q", loaded.ErrorSummary())
	}
}

func TestAuditAllowsUnknownProviderForUnresolvedTool(t *testing.T) {
	t.Parallel()

	store := openTestStore(t)
	invocationID, _ := invocationdomain.ParseInvocationID("inv_00000000000000000000000000000002")
	toolID, _ := catalogdomain.ParseToolID("tool_00000000000000000000000000000002")
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

	record, err := auditdomain.NewRecord(
		invocationID,
		toolID,
		nil,
		"mcp",
		now,
		now.Add(time.Millisecond),
		"",
		"",
		auditdomain.ApprovalNotRequired,
		auditdomain.StatusFailed,
		"",
		auditdomain.ErrorToolNotFound,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Record(context.Background(), record); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.Get(context.Background(), invocationID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ProviderID() != nil {
		t.Fatalf("ProviderID() = %v, want nil", loaded.ProviderID())
	}
}

func TestAuditSchemaContainsNoPayloadOrCredentialColumns(t *testing.T) {
	t.Parallel()

	store := openTestStore(t)
	rows, err := store.db.QueryContext(context.Background(), "PRAGMA table_info(invocation_audit)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()

	var columns []string
	for rows.Next() {
		var (
			cid          int
			name         string
			columnType   string
			notNull      int
			defaultValue sql.NullString
			primaryKey   int
		)
		if err := rows.Scan(
			&cid,
			&name,
			&columnType,
			&notNull,
			&defaultValue,
			&primaryKey,
		); err != nil {
			t.Fatal(err)
		}
		columns = append(columns, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	for _, forbidden := range []string{
		"arguments",
		"result",
		"payload",
		"credential",
		"secret",
		"request",
		"response",
		"governance_facts",
		"progress",
	} {
		if slices.Contains(columns, forbidden) {
			t.Fatalf("audit schema contains forbidden column %q: %v", forbidden, columns)
		}
	}
}

func completedRecord(t *testing.T) auditdomain.Record {
	t.Helper()

	invocationID, _ := invocationdomain.ParseInvocationID("inv_00000000000000000000000000000001")
	toolID, _ := catalogdomain.ParseToolID("tool_00000000000000000000000000000001")
	providerID, _ := catalogdomain.ParseProviderID("prov_00000000000000000000000000000001")
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

	record, err := auditdomain.NewRecord(
		invocationID,
		toolID,
		&providerID,
		"cli",
		now,
		now.Add(10*time.Millisecond),
		policydomain.DecisionAllow,
		"local.default",
		auditdomain.ApprovalNotRequired,
		auditdomain.StatusCompleted,
		invocationdomain.ResultKindComplete,
		auditdomain.ErrorNone,
	)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func openTestStore(t *testing.T) *Store {
	t.Helper()

	store, err := Open(context.Background(), filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	return store
}
