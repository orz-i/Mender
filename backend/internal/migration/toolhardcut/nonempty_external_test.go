package toolhardcut_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	"github.com/orz-i/Mender/backend/internal/migration/toolhardcut"
	sandbox "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	_ "modernc.org/sqlite"
)

type suiteFixture struct {
	path        string
	providers   []string
	sandboxIDs  []string
	readToolIDs []string
	roots       []string
}

func localSuiteFixture(t *testing.T, n int) suiteFixture {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "mender.sqlite")
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: path})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	owner, err := gov.NewContext(gov.OriginCLI, runtime.OwnerPrincipal(), gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	f := suiteFixture{path: path}
	for i := 0; i < n; i++ {
		root := t.TempDir()
		item, err := runtime.Sandboxes.CreateLocalSandbox(ctx, bootstrap.CreateLocalSandboxRequest{
			Name: "Migration Sandbox", Root: root, Isolation: sandbox.IsolationHost,
		}, owner)
		if err != nil {
			t.Fatal(err)
		}
		suite, err := runtime.EnsureSandboxWorkspaceTools(ctx, item.ID(), owner)
		if err != nil || suite.ProviderID == "" || len(suite.Tools) != 4 {
			t.Fatalf("managed workspace suite: %+v %v", suite, err)
		}
		f.providers = append(f.providers, suite.ProviderID)
		f.sandboxIDs = append(f.sandboxIDs, item.ID().String())
		f.roots = append(f.roots, root)
		for _, tool := range suite.Tools {
			if tool.SourceKey == "workspace.read" {
				f.readToolIDs = append(f.readToolIDs, tool.ToolID)
			}
		}
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestAssessLocalSandboxFacadeHasExactVerifiedDefaultPlacement(t *testing.T) {
	ctx := context.Background()
	f := localSuiteFixture(t, 1)
	report, err := toolhardcut.AssessPath(ctx, f.path)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "ready" {
		t.Fatalf("verified local source should be eligible: %+v", report.Conflicts)
	}
	if report.Counts.ToolsBefore != 4 || report.Counts.ToolsAfter != 9 {
		t.Fatalf("unexpected count: %+v", report.Counts)
	}
	if len(report.WorkspacePlacements) != 1 || report.WorkspacePlacements[0].Verification != "verified_local_root" ||
		report.WorkspacePlacements[0].SandboxID != f.sandboxIDs[0] {
		t.Fatalf("incorrect verified Workspace: %+v", report.WorkspacePlacements)
	}
	if len(report.ToolMappings) != 4 {
		t.Fatalf("mapping count=%d", len(report.ToolMappings))
	}
	for _, m := range report.ToolMappings {
		if m.NewSource != "native:workspace" || m.Action != "merge" ||
			len(m.AllowedTargetRefs) != 1 || m.AllowedTargetRefs[0] != f.sandboxIDs[0] ||
			!strings.HasPrefix(m.NewToolID, "tool_") {
			t.Fatalf("legacy native Tool mapping not strictly scoped: %+v", m)
		}
	}
	original, err := toolhardcut.AssessPath(ctx, f.path)
	if err != nil || report.ManifestDigest != original.ManifestDigest {
		t.Fatal("non-deterministic source assessment")
	}
}

func TestStageCopyRetiresExactPinnedLocalFacadeWithoutProfileGrant(t *testing.T) {
	ctx := context.Background()
	f := localSuiteFixture(t, 1)
	report, err := toolhardcut.AssessPath(ctx, f.path)
	if err != nil || report.Status != "ready" {
		t.Fatalf("source not ready: %+v %v", report.Conflicts, err)
	}
	artifact, err := toolhardcut.StageCopy(ctx, f.path, filepath.Join(t.TempDir(), "converted.sqlite"), report.ManifestDigest)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", artifact.CandidatePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	for _, tc := range []struct {
		q        string
		expected int
	}{
		{"SELECT COUNT(*) FROM tools", 9},
		{"SELECT COUNT(*) FROM tools_fts", 9},
		{"SELECT COUNT(*) FROM hardcut_legacy_tool_identities", 4},
		{"SELECT COUNT(*) FROM workspaces", 1},
		{"SELECT COUNT(*) FROM workspace_placements", 1},
		{"SELECT COUNT(*) FROM provider_configs WHERE provider_id='" + f.providers[0] + "'", 0},
		{"SELECT COUNT(*) FROM providers WHERE id='" + f.providers[0] + "'", 0},
		{"SELECT COUNT(*) FROM profile_tools", 0},
		{"SELECT COUNT(*) FROM profile_tool_target_scopes", 0},
	} {
		var n int
		if err := db.QueryRow(tc.q).Scan(&n); err != nil || n != tc.expected {
			t.Fatalf("%s = %d expected %d: %v", tc.q, n, tc.expected, err)
		}
	}
	var wsID string
	if err := db.QueryRow("SELECT workspace_id FROM workspaces").Scan(&wsID); err != nil || !strings.HasPrefix(wsID, "wrk_") {
		t.Fatal("workspace ref lost")
	}
	var sandboxID string
	if err := db.QueryRow("SELECT sandbox_id FROM workspace_placements WHERE workspace_id=?", wsID).Scan(&sandboxID); err != nil || sandboxID != f.sandboxIDs[0] {
		t.Fatal("sandbox placement lost")
	}
	if _, err := os.Stat(artifact.PreUpgradeBackupPath); err != nil {
		t.Fatal(err)
	}
	sourceAgain, err := toolhardcut.AssessPath(ctx, f.path)
	if err != nil || sourceAgain.ManifestDigest != report.ManifestDigest {
		t.Fatal("live source was modified")
	}
}

func TestAssessNativeProfileBindingRequiresReviewAndNoPrivilegeExpansion(t *testing.T) {
	f := localSuiteFixture(t, 2)
	db, err := sql.Open("sqlite", f.path)
	if err != nil {
		t.Fatal(err)
	}
	profileID := "prof_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if _, err := db.Exec(`INSERT INTO profiles(id,slug,display_name,revision)
  VALUES(?,'migration-check','Migration Check',0)`, profileID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO mcp_delivery_configs(profile_id,mode,tunnel_id,revision)
  VALUES(?,'hybrid','',0)`, profileID); err != nil {
		t.Fatal(err)
	}
	for i, id := range f.readToolIDs {
		alias := "same_read"
		if i == 1 {
			alias = "different_read"
		}
		if _, err := db.Exec(`INSERT INTO profile_tools
   (profile_id,tool_id,alias,title_override,description_override,policy_constraint,concurrency_constraint,credential_refs_json)
   VALUES(?,? ,?,'','','inherit','inherit','[]')`, profileID, id, alias); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	report, err := toolhardcut.AssessPath(context.Background(), f.path)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range report.Conflicts {
		if c.Code == toolhardcut.BlockBinding {
			found = true
		}
	}
	if !found || report.Status != "blocked" {
		t.Fatalf("incompatible native ToolBinding merge not blocked: %+v", report.Conflicts)
	}
	candidate := filepath.Join(t.TempDir(), "candidate.sqlite")
	if _, err := toolhardcut.StageCopy(context.Background(), f.path, candidate, report.ManifestDigest); err == nil {
		t.Fatal("blocked mapping staged")
	}
	if _, err := os.Stat(candidate); !os.IsNotExist(err) {
		t.Fatal("blocked assessment created staged candidate")
	}
}

func TestAssessMissingLocalRootIsNotEquivalentToAReadySandbox(t *testing.T) {
	f := localSuiteFixture(t, 1)
	if err := os.RemoveAll(f.roots[0]); err != nil {
		t.Fatal(err)
	}
	report, err := toolhardcut.AssessPath(context.Background(), f.path)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range report.Conflicts {
		if c.Code == toolhardcut.BlockPlacement {
			found = true
		}
	}
	if !found || report.Status != "blocked" {
		t.Fatalf("missing actual Workspace root silently considered verified: %+v", report.Conflicts)
	}
}
