package toolhardcut_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/orz-i/Mender/backend/internal/migration/toolhardcut"
	_ "modernc.org/sqlite"
)

func TestAgentSourceNamespaceChangeRequiresDirectMCPReviewEvenWhenToolIDRemains(t *testing.T) {
	f := newAgentFixture(t)
	db, err := sql.Open("sqlite", f.path)
	if err != nil {
		t.Fatal(err)
	}
	profileID := "prof_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	for _, cmd := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO profiles(id,slug,display_name,revision)
   VALUES(?,'direct-agent','Direct Agent',0)`, []any{profileID}},
		{`INSERT INTO mcp_delivery_configs(profile_id,mode,tunnel_id,revision)
   VALUES(?,'direct','',0)`, []any{profileID}},
		{`INSERT INTO profile_tools(profile_id,tool_id,alias,title_override,description_override,
   policy_constraint,concurrency_constraint,credential_refs_json)
   VALUES(?,?,'delegate','','','inherit','inherit','[]')`, []any{profileID, f.toolIDs[0]}},
	} {
		if _, err := db.Exec(cmd.sql, cmd.args...); err != nil {
			_ = db.Close()
			t.Fatal(err)
		}
	}
	_ = db.Close()
	report, err := toolhardcut.AssessPath(context.Background(), f.path)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "requires_review" || report.Counts.UnresolvedBlockers != 0 ||
		len(report.MCPExportDiffs) != 1 || !report.MCPExportDiffs[0].Direct ||
		report.MCPExportDiffs[0].OldToolID != report.MCPExportDiffs[0].NewToolID ||
		report.MCPExportDiffs[0].Action != "source_namespace_change_requires_client_review" {
		t.Fatalf("Agent namespace was silently reclassified: %+v", report.MCPExportDiffs)
	}
	if _, err := toolhardcut.StageCopy(context.Background(), f.path, filepath.Join(t.TempDir(), "candidate.sqlite"), report.ManifestDigest); err == nil {
		t.Fatal("Agent source change bypassed direct MCP export review")
	}
}
