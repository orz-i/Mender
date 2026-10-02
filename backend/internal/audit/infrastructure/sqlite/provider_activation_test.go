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
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
)

func TestProviderActivationAuditRoundTrip(t *testing.T) {
	t.Parallel()

	store, err := Open(context.Background(), filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	providerID, _ := catalogdomain.ParseProviderID(
		"prov_00000000000000000000000000000001",
	)
	principal, err := governancedomain.NewPrincipal(
		governancedomain.PrincipalLocalUser,
		"os:test",
	)
	if err != nil {
		t.Fatal(err)
	}
	governanceContext, err := governancedomain.NewContext(
		governancedomain.OriginCLI,
		principal,
		governancedomain.CatalogScope(),
	)
	if err != nil {
		t.Fatal(err)
	}
	startedAt := time.Date(2026, 10, 2, 1, 0, 0, 0, time.UTC)
	record, err := auditdomain.NewProviderActivationRecord(
		"act_00000000000000000000000000000001",
		providerID,
		governanceContext,
		startedAt,
		startedAt.Add(time.Millisecond),
		policydomain.DecisionAllow,
		"policy.default.allow",
		auditdomain.ApprovalNotRequired,
		auditdomain.ProviderActivationCompleted,
		auditdomain.ProviderActivationErrorNone,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordProviderActivation(context.Background(), record); err != nil {
		t.Fatal(err)
	}

	records, err := store.ListProviderActivations(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	got := records[0]
	if got.ActivationID() != record.ActivationID() ||
		got.ProviderID() != providerID ||
		got.PrincipalKind() != string(governancedomain.PrincipalLocalUser) ||
		got.PrincipalRef() != "os:test" ||
		got.Status() != auditdomain.ProviderActivationCompleted ||
		got.PolicyKind() != policydomain.DecisionAllow {
		t.Fatalf("activation audit = %#v", got)
	}
}

func TestProviderActivationAuditSchemaContainsNoSecretOrConfigPayloads(t *testing.T) {
	t.Parallel()

	store, err := Open(context.Background(), filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	rows, err := store.db.QueryContext(
		context.Background(),
		"PRAGMA table_info(provider_activation_audit)",
	)
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
		"config",
		"raw_config",
		"credential",
		"secret",
		"arguments",
		"result",
		"request",
		"response",
	} {
		if slices.Contains(columns, forbidden) {
			t.Fatalf(
				"provider activation audit contains forbidden column %q: %v",
				forbidden,
				columns,
			)
		}
	}
}
