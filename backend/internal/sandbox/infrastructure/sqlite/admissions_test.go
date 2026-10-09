package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	policy "github.com/orz-i/Mender/backend/internal/policy/domain"
	app "github.com/orz-i/Mender/backend/internal/sandbox/application"
)

func TestSandboxAdmissionAuditDurable(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "admissions.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(ctx, testSandbox(t)); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	record := app.AdmissionAudit{OperationID: "sadm_0123456789abcdef0123456789abcdef", SandboxID: sandboxID, Action: gov.ActionSandboxAcquire, PrincipalKind: gov.PrincipalLocalUser, PrincipalRef: "local-owner", Origin: gov.OriginCLI, ScopeKind: gov.ScopeCatalog, PolicyKind: policy.DecisionAllow, PolicyReason: "policy.default.allow", Approval: "not_required", Status: "admitted", StartedAt: now, FinishedAt: now}
	if err := store.RecordSandboxAdmission(ctx, record); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	restored, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = restored.Close() }()
	rows, err := restored.ListSandboxAdmissions(ctx, sandboxID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].OperationID != record.OperationID || rows[0].PolicyKind != policy.DecisionAllow {
		t.Fatalf("restarted audit %+v", rows)
	}
	if err := restored.RecordSandboxAdmission(ctx, record); err == nil {
		t.Fatal("duplicate admission identity accepted")
	}
}
