package toolhardcut_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentapp "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	"github.com/orz-i/Mender/backend/internal/bootstrap"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	"github.com/orz-i/Mender/backend/internal/migration/toolhardcut"
	_ "modernc.org/sqlite"
)

type agentFixture struct {
	path       string
	agentID    string
	providerID string
	toolIDs    []string
}

func newAgentFixture(t *testing.T) agentFixture {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "mender.sqlite")
	r, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: path})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := gov.NewContext(gov.OriginCLI, r.OwnerPrincipal(), gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	a, err := r.CreateAgentWithTools(ctx, agentapp.CreateRequest{
		Name: "Migration review", Backend: agentdomain.BackendGrok, WorkspaceRoot: t.TempDir(),
	}, owner)
	if err != nil {
		_ = r.Close()
		t.Fatal(err)
	}
	state, err := r.AgentTools(ctx, a.ID(), owner)
	if err != nil {
		_ = r.Close()
		t.Fatal(err)
	}
	if state.ProviderID == "" || len(state.Tools) != 5 || state.Status != "ready" {
		_ = r.Close()
		t.Fatalf("Agent as Tool facade not ready: %+v", state)
	}
	f := agentFixture{path: path, agentID: string(a.ID()), providerID: state.ProviderID}
	for _, tool := range state.Tools {
		f.toolIDs = append(f.toolIDs, tool.ToolID)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestAssessAndStageAgentAsToolPreservesEveryOperationID(t *testing.T) {
	ctx := context.Background()
	f := newAgentFixture(t)
	report, err := toolhardcut.AssessPath(ctx, f.path)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "ready" {
		t.Fatalf("exact Agent facade failed the assessment: %+v", report.Conflicts)
	}
	if report.Counts.ToolsBefore != 5 || report.Counts.ToolsAfter != 14 {
		t.Fatalf("Agent mapping counts: %+v", report.Counts)
	}
	for _, m := range report.ToolMappings {
		if m.NewToolID != m.OldToolID || m.NewSource != "agent:"+f.agentID || m.Action != "keep" {
			t.Fatalf("AgentID or ToolID changed: %+v", m)
		}
	}
	artifact, err := toolhardcut.StageCopy(ctx, f.path, filepath.Join(t.TempDir(), "candidate.sqlite"), report.ManifestDigest)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", artifact.CandidatePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	for _, tc := range []struct {
		q    string
		want int
	}{
		{"SELECT COUNT(*) FROM tools WHERE source_kind='agent' AND source_id='" + f.agentID + "'", 5},
		{"SELECT COUNT(*) FROM providers WHERE id='" + f.providerID + "'", 0},
		{"SELECT COUNT(*) FROM provider_configs WHERE provider_id='" + f.providerID + "'", 0},
		{"SELECT COUNT(*) FROM agents WHERE agent_id='" + f.agentID + "'", 1},
		{"SELECT COUNT(*) FROM hardcut_legacy_tool_identities WHERE new_source_kind='agent'", 5},
	} {
		var actual int
		if err := db.QueryRow(tc.q).Scan(&actual); err != nil || actual != tc.want {
			t.Fatalf("converted Agent proof %s => %d/%d err %v", tc.q, actual, tc.want, err)
		}
	}
	if _, err := db.Exec("DELETE FROM agents WHERE agent_id=?", f.agentID); err == nil {
		t.Fatal("Agent with SourceRef-owned ToolIDs was deleted")
	}
}

func TestAssessUncertainAgentTaskBlocksWithoutReplaying(t *testing.T) {
	f := newAgentFixture(t)
	db, err := sql.Open("sqlite", f.path)
	if err != nil {
		t.Fatal(err)
	}
	// Only fixture metadata is inserted; no Agent process is ever spawned.
	_, err = db.Exec(`INSERT INTO agent_tasks(task_id,agent_id,status,origin,
  principal_kind,principal_ref,scope_kind,scope_ref,generation,created_at,updated_at)
  VALUES(?,?, 'unknown','cli','local_user','fixture','catalog','',1,'now','now')`,
		"task_11111111111111111111111111111111", f.agentID)
	if err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	_ = db.Close()
	report, err := toolhardcut.AssessPath(context.Background(), f.path)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range report.Conflicts {
		if c.Code == toolhardcut.BlockTask {
			found = true
		}
	}
	if !found || report.Status != "blocked" {
		t.Fatalf("unknown Task could be replayed by migration: %+v", report.Conflicts)
	}
	if _, err := toolhardcut.StageCopy(context.Background(), f.path, filepath.Join(t.TempDir(), "candidate.sqlite"), report.ManifestDigest); err == nil {
		t.Fatal("uncertain Task was automatically staged")
	}
}

func TestAssessPolicyReferringToOldFacadeCannotBeSilentlyRewritten(t *testing.T) {
	f := newAgentFixture(t)
	db, err := sql.Open("sqlite", f.path)
	if err != nil {
		t.Fatal(err)
	}
	ruleJSON, err := json.Marshal(map[string]any{
		"actions":     []string{"tool.invoke"},
		"providerIds": []string{f.providerID},
		"toolIds":     []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO policy_rules
  (rule_id,display_name,enabled,decision,match_json,created_at,updated_at)
  VALUES('rule_11111111111111111111111111111111','Do not delegate',1,'deny',?,'now','now')`,
		string(ruleJSON))
	if err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	_ = db.Close()
	report, err := toolhardcut.AssessPath(context.Background(), f.path)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range report.Conflicts {
		if c.Code == toolhardcut.BlockPolicy {
			found = true
		}
	}
	if !found || report.Status != "blocked" {
		t.Fatalf("lost deny Policy: %+v", report.Conflicts)
	}
	if !strings.HasPrefix(report.ManifestDigest, "sha256:") {
		t.Fatal("invalid deterministic manifest")
	}
}

func TestSingleLegacyProfileNativeBindingRequiresExplicitExportReview(t *testing.T) {
	f := localSuiteFixture(t, 1)
	db, err := sql.Open("sqlite", f.path)
	if err != nil {
		t.Fatal(err)
	}
	profileID := "prof_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	_, err = db.Exec(`INSERT INTO profiles(id,slug,display_name,revision)
  VALUES(?,'migration-check','Migration Check',0)`, profileID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO mcp_delivery_configs(profile_id,mode,tunnel_id,revision)
  VALUES(?,'hybrid','',0)`, profileID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO profile_tools
  (profile_id,tool_id,alias,title_override,description_override,policy_constraint,concurrency_constraint,credential_refs_json)
  VALUES(?,?,'read_scoped','','','inherit','inherit','[]')`, profileID, f.readToolIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	report, err := toolhardcut.AssessPath(context.Background(), f.path)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "requires_review" || report.Counts.UnresolvedBlockers != 0 ||
		len(report.MCPExportDiffs) != 1 || len(report.ProfileBindingDiffs) != 1 ||
		len(report.ProfileBindingDiffs[0].AllowedTargetRefs) != 1 ||
		report.ProfileBindingDiffs[0].AllowedTargetRefs[0] != f.sandboxIDs[0] {
		t.Fatalf("expected exact scope with mandatory MCP export review: %+v", report)
	}
	dest := filepath.Join(t.TempDir(), "candidate.sqlite")
	if _, err := toolhardcut.StageCopy(context.Background(), f.path, dest, report.ManifestDigest); err == nil {
		t.Fatal("review-required breaking MCP export was silently approved")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("unsafe staged candidate created without review")
	}
}
