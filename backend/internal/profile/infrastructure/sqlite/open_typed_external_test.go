package sqlite_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	sourcecatalog "github.com/orz-i/Mender/backend/internal/catalog/infrastructure/sourcecatalog"
	credential "github.com/orz-i/Mender/backend/internal/credential/domain"
	migration "github.com/orz-i/Mender/backend/internal/migration/toolhardcut"
	profile "github.com/orz-i/Mender/backend/internal/profile/domain"
	profilesqlite "github.com/orz-i/Mender/backend/internal/profile/infrastructure/sqlite"
	typedsupply "github.com/orz-i/Mender/backend/internal/profile/infrastructure/typedsupply"
	"github.com/orz-i/Mender/backend/internal/toolsuite/infrastructure/curated"
	_ "modernc.org/sqlite"
)

func typedProfileCandidate(t *testing.T) (string, string) {
	t.Helper()
	ctx := context.Background()
	legacy := filepath.Join(t.TempDir(), "legacy.sqlite")
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: legacy})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	plan, err := migration.AssessPath(ctx, legacy)
	if err != nil || plan.Status != "ready" {
		t.Fatalf("empty legacy fixture is not eligible: %+v, %v", plan.Conflicts, err)
	}
	artifact, err := migration.StageCopy(ctx, legacy, filepath.Join(t.TempDir(), "candidate.sqlite"), plan.ManifestDigest)
	if err != nil {
		t.Fatal(err)
	}
	return legacy, artifact.CandidatePath
}

func TestOpenTypedReusesProfilePersistenceWithoutCreatingAnyGrants(t *testing.T) {
	ctx := context.Background()
	source, candidate := typedProfileCandidate(t)
	store, err := profilesqlite.OpenTyped(ctx, candidate)
	if err != nil {
		t.Fatalf("open typed Profile candidate: %v", err)
	}
	id, _ := profile.ParseProfileID("prof_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	slug, _ := profile.NewSlug("typed")
	ref, _ := credential.ParseReference("credential://github/operations")
	natives, err := curated.PlannedNativeTools()
	if err != nil {
		t.Fatal(err)
	}
	var nativeID catalog.ToolID
	for _, entry := range natives {
		if entry.SourceKey == "workspace.read" {
			nativeID = entry.ToolID
		}
	}
	if nativeID == "" {
		t.Fatal("missing canonical native Tool")
	}
	binding, err := profile.NewToolBinding(profile.ToolBindingSpec{
		ToolID: nativeID, Alias: "read", PolicyConstraint: profile.PolicyRequireApproval,
		ConcurrencyConstraint: profile.ConcurrencySerial, CredentialRefs: []credential.Reference{ref},
	})
	if err != nil {
		t.Fatal(err)
	}
	model, err := profile.NewProfileWithBindings(id, slug, "Typed Profile", []profile.ToolBinding{binding})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateProfile(ctx, model); err != nil {
		t.Fatal(err)
	}
	restored, err := store.GetProfileByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	restoredBinding, ok := restored.Binding(nativeID)
	if !ok || restoredBinding.PolicyConstraint() != profile.PolicyRequireApproval ||
		restoredBinding.ConcurrencyConstraint() != profile.ConcurrencySerial ||
		len(restoredBinding.CredentialRefs()) != 1 || restoredBinding.CredentialRefs()[0] != ref {
		t.Fatalf("typed Profile persistence lost binding constraints: %+v", restoredBinding)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	// Creating/selecting a Tool never creates Workspace/Sandbox grants,
	// target scope, or Agent permission. Native execute remains unavailable.
	db, err := sql.Open("sqlite", candidate)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	// The canonical v3 Catalog and existing Profile CRUD read exactly the
	// same typed ToolID. A ToolBinding cannot manufacture an executor target.
	catalogue, err := sourcecatalog.NewRepository(ctx, db, testNativeOwner{})
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := catalogue.GetTool(ctx, nativeID)
	if err != nil || canonical.SourceRef().String() != "native:workspace" || canonical.SourceKey() != "workspace.read" {
		t.Fatalf("Profile binding and v3 Catalog disagree: %+v %v", canonical, err)
	}
	projection, err := typedsupply.Open(ctx, candidate)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = projection.Close() }()
	if _, allowed, err := projection.ResolveBindingGovernance(ctx, id.String(), nativeID); err != nil || allowed {
		t.Fatalf("Profile ToolBinding without target grant became executable: allowed=%v err=%v", allowed, err)
	}
	for _, query := range []string{
		"SELECT COUNT(*) FROM profile_tool_target_scopes",
		"SELECT COUNT(*) FROM workspace_profile_grants",
		"SELECT COUNT(*) FROM sandbox_profile_grants",
		"SELECT COUNT(*) FROM agent_grants",
	} {
		var n int
		if err := db.QueryRowContext(ctx, query).Scan(&n); err != nil || n != 0 {
			t.Fatalf("typed Profile create minted authority (%s): n=%d err=%v", query, n, err)
		}
	}
	var oldTables int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='agent_tool_facades'").Scan(&oldTables); err != nil || oldTables != 0 {
		t.Fatalf("typed Profile store reintroduced legacy facade: %d %v", oldTables, err)
	}
	// The original legacy source is untouched; it can be inspected by the
	// existing reader while the disposable candidate is used for tests only.
	plan, err := migration.AssessPath(ctx, source)
	if err != nil || plan.Status != "ready" {
		t.Fatalf("typed Profile Store changed legacy source: %v status=%s", err, plan.Status)
	}
}

func TestOpenTypedRefusesLegacyPartialAndMissingDatabases(t *testing.T) {
	ctx := context.Background()
	legacy, candidate := typedProfileCandidate(t)
	if _, err := profilesqlite.OpenTyped(ctx, legacy); err == nil {
		t.Fatal("typed Store accepted Provider-v2 database")
	}
	missing := filepath.Join(t.TempDir(), "missing.sqlite")
	if _, err := profilesqlite.OpenTyped(ctx, missing); err == nil {
		t.Fatal("typed Store created an absent DB")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("absent database was created or error unexpected: %v", err)
	}
	// A partial future schema must not be repaired by old per-context migrate.
	db, err := sql.Open("sqlite", candidate)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO profile_schema_migrations(version,applied_at) VALUES(9,'tampered')"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := profilesqlite.OpenTyped(ctx, candidate); err == nil {
		t.Fatal("typed Store accepted unreviewed Profile v9")
	}
}

func TestOpenTypedRefusesReintroducedResourceFacade(t *testing.T) {
	ctx := context.Background()
	_, candidate := typedProfileCandidate(t)
	db, err := sql.Open("sqlite", candidate)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "CREATE TABLE agent_tool_facades(agent_id TEXT, provider_id TEXT)"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := profilesqlite.OpenTyped(ctx, candidate); err == nil {
		t.Fatal("typed Profile Store accepted Provider facade resurrection")
	}
}

// OwnerVerifier is never used for writes in this integration test; the
// migrated native catalog already contains host-reviewed canonical Tools.
type testNativeOwner struct{}

func (testNativeOwner) VerifySourceOwner(context.Context, catalog.SourceRef) error { return nil }
