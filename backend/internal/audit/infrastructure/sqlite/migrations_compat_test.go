package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	v3fixture "github.com/orz-i/Mender/backend/internal/testfixtures/v3"
)

func TestAuditV3RejectsFutureSchemaVersion(t *testing.T) {
	t.Parallel()

	path := v3fixture.EmptyDatabase(t)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO audit_schema_migrations(version,applied_at) VALUES(999,'future')`); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(context.Background(), path)
	if store != nil {
		_ = store.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "expected version 5") {
		t.Fatalf("Open() error = %v, want future schema rejection", err)
	}
}
