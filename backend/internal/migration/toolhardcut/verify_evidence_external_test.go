package toolhardcut_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	migration "github.com/orz-i/Mender/backend/internal/migration/toolhardcut"
	_ "modernc.org/sqlite"
)

func TestVerifyStagedCopyRefusesPostUpgradeToolGrantExpansion(t *testing.T) {
	ctx := context.Background()
	source := emptyRuntimeFixture(t)
	db, err := sql.Open("sqlite", source)
	if err != nil {
		t.Fatal(err)
	}
	id := "prof_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if _, err := db.Exec(`INSERT INTO profiles(id,slug,display_name,revision)
		VALUES(?,'limited','Limited',0)`, id); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	report, err := migration.AssessPath(ctx, source)
	if err != nil || report.Status != "ready" {
		t.Fatalf("Profile source not ready: %+v %v", report.Conflicts, err)
	}
	artifact, err := migration.StageCopy(ctx, source, filepath.Join(t.TempDir(), "candidate.sqlite"), report.ManifestDigest)
	if err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", artifact.CandidatePath)
	if err != nil {
		t.Fatal(err)
	}
	var native string
	if err := db.QueryRow(`SELECT id FROM tools WHERE source_kind='native' AND source_key='workspace.read'`).Scan(&native); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	// Even a valid canonical native ToolID cannot be slipped into a Profile
	// during cutover merely because its Schema exists.
	_, err = db.Exec(`INSERT INTO profile_tools
		(profile_id,tool_id,alias,title_override,description_override,policy_constraint,concurrency_constraint,credential_refs_json)
		VALUES(?,?,'','','','inherit','inherit','[]')`, id, native)
	if closeErr := db.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	testRehashCandidate(t, &artifact)
	err = migration.VerifyStagedCopy(ctx, artifact)
	if err == nil || !strings.Contains(err.Error(), "profile_tools") {
		t.Fatalf("new unauthorized Profile binding was accepted: %v", err)
	}
}

func TestVerifyStagedCopyPreservesUncertainTaskEvidenceWithoutReplay(t *testing.T) {
	ctx := context.Background()
	// A completed Task is terminal, so a source can be safely migrated.
	// The candidate must not acquire any unreviewed uncertain Task evidence.
	fixture := newAgentFixture(t)
	db, err := sql.Open("sqlite", fixture.path)
	if err != nil {
		t.Fatal(err)
	}
	taskID := "task_11111111111111111111111111111111"
	_, err = db.Exec(`INSERT INTO agent_tasks(task_id,agent_id,status,origin,
		principal_kind,principal_ref,scope_kind,scope_ref,generation,created_at,updated_at)
		VALUES(?,?,'completed','cli','local_user','fixture','catalog','',1,'now','now')`, taskID, fixture.agentID)
	if closeErr := db.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	report, err := migration.AssessPath(ctx, fixture.path)
	if err != nil || report.Status != "ready" {
		t.Fatalf("completed Agent Task not migratable: %+v %v", report.Conflicts, err)
	}
	artifact, err := migration.StageCopy(ctx, fixture.path, filepath.Join(t.TempDir(), "candidate.sqlite"), report.ManifestDigest)
	if err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", artifact.CandidatePath)
	if err != nil {
		t.Fatal(err)
	}
	// The terminal Task cannot be updated by SQLite's immutable trigger;
	// inserting a new unknown Task behind the assessor is also forbidden by
	// the post-commit evidence fingerprint, even if its file digest is restated.
	_, err = db.Exec("UPDATE agent_tasks SET status='unknown' WHERE task_id=?", taskID)
	if err == nil {
		_ = db.Close()
		t.Fatal("terminal Task was mutable after hard cut")
	}
	_, err = db.Exec(`INSERT INTO agent_tasks(task_id,agent_id,status,origin,
		principal_kind,principal_ref,scope_kind,scope_ref,generation,created_at,updated_at)
		VALUES('task_22222222222222222222222222222222',?,'unknown','cli','local_user','tampered','catalog','',1,'now','now')`, fixture.agentID)
	if closeErr := db.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	testRehashCandidate(t, &artifact)
	err = migration.VerifyStagedCopy(ctx, artifact)
	if err == nil || !strings.Contains(err.Error(), "agent_tasks") {
		t.Fatalf("Task evidence changed across migration: %v", err)
	}
}

// ToolID lineage is historical authorization evidence. A candidate that
// retains the same canonical ToolIDs but rewrites the assessed old->new owner
// relationship is not eligible for the one-way cut.
func TestVerifyStagedCopyRejectsReassignedAgentToolLineage(t *testing.T) {
	ctx := context.Background()
	f := newAgentFixture(t)
	report, err := migration.AssessPath(ctx, f.path)
	if err != nil || report.Status != "ready" {
		t.Fatalf("source readiness: %+v %v", report.Conflicts, err)
	}
	artifact, err := migration.StageCopy(ctx, f.path, filepath.Join(t.TempDir(), "future.sqlite"), report.ManifestDigest)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", artifact.CandidatePath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec("UPDATE hardcut_legacy_tool_identities SET operation='agent.delegate' WHERE operation='agent.status'")
	if closeErr := db.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	testRehashCandidate(t, &artifact)
	if err := migration.VerifyStagedCopy(ctx, artifact); err == nil ||
		!strings.Contains(err.Error(), "lineage") {
		t.Fatalf("unapproved Agent ToolID operation remapping: %v", err)
	}
}
