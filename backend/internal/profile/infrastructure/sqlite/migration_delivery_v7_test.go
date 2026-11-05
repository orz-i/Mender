package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	profile "github.com/orz-i/Mender/backend/internal/profile/domain"
	skill "github.com/orz-i/Mender/backend/internal/skill/domain"
	skillsqlite "github.com/orz-i/Mender/backend/internal/skill/infrastructure/sqlite"
	_ "modernc.org/sqlite"
)

func legacyDeliveryDatabase(t *testing.T, path string) *sql.DB {
	t.Helper()
	skillStore, err := skillsqlite.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = skillStore.Close() }()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, statement := range []string{
		`PRAGMA foreign_keys=ON`,
		`CREATE TABLE profile_schema_migrations(version INTEGER PRIMARY KEY,applied_at TEXT NOT NULL)`,
		`INSERT INTO profile_schema_migrations VALUES(1,'old'),(2,'old'),(3,'old'),(4,'old'),(5,'old'),(6,'old')`,
		`CREATE TABLE profiles(id TEXT PRIMARY KEY,slug TEXT NOT NULL UNIQUE,display_name TEXT NOT NULL,exposure_mode TEXT NOT NULL,tunnel_id TEXT NOT NULL DEFAULT '',revision INTEGER NOT NULL DEFAULT 0 CHECK(revision>=0))`,
		`CREATE TABLE profile_tools(profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,tool_id TEXT NOT NULL,direct INTEGER NOT NULL CHECK(direct IN(0,1)),alias TEXT NOT NULL DEFAULT '',title_override TEXT NOT NULL DEFAULT '',description_override TEXT NOT NULL DEFAULT '',policy_constraint TEXT NOT NULL DEFAULT 'inherit',concurrency_constraint TEXT NOT NULL DEFAULT 'inherit',credential_refs_json TEXT NOT NULL DEFAULT '[]',PRIMARY KEY(profile_id,tool_id))`,
		`CREATE TABLE profile_skills(profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,installation_id TEXT NOT NULL,PRIMARY KEY(profile_id,installation_id))`,
		`CREATE INDEX idx_profile_skills_installation ON profile_skills(installation_id)`,
		`CREATE TABLE tools(id TEXT PRIMARY KEY)`,
		`CREATE TABLE tunnels(id TEXT PRIMARY KEY)`,
		`INSERT INTO tunnels VALUES('tun_00000000000000000000000000000001')`,
		`CREATE TRIGGER profile_tool_reference_insert_guard BEFORE INSERT ON profile_tools WHEN NOT EXISTS(SELECT 1 FROM tools WHERE id=NEW.tool_id) BEGIN SELECT RAISE(ABORT,'Profile Tool reference unavailable'); END`,
		`CREATE TABLE profile_change_probe(profile_id TEXT,revision INTEGER)`,
		`CREATE TRIGGER foreign_profile_observer AFTER UPDATE OF revision ON profiles BEGIN INSERT INTO profile_change_probe VALUES(NEW.id,NEW.revision); END`,
	} {
		if _, err := db.Exec(statement); err != nil {
			_ = db.Close()
			t.Fatal(err)
		}
	}
	for i, mode := range []string{"direct", "dynamic", "hybrid"} {
		id := []string{"prof_00000000000000000000000000000001", "prof_00000000000000000000000000000002", "prof_00000000000000000000000000000003"}[i]
		if _, err := db.Exec(`INSERT INTO profiles VALUES(?,?,?,?,?,?)`, id, mode, mode, mode, "tun_00000000000000000000000000000001", 41+i); err != nil {
			t.Fatal(err)
		}
		for j, tool := range []string{"tool_00000000000000000000000000000001", "tool_00000000000000000000000000000002"} {
			if _, err := db.Exec("INSERT OR IGNORE INTO tools VALUES(?)", tool); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`INSERT INTO profile_tools VALUES(?,?,?,?,?,?,?,?,?)`, id, tool, j, "alias"+string(rune('a'+j)), "Title", "Description", "require_approval", "serial", `["credential://github/work"]`); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := db.Exec("INSERT INTO profile_skills VALUES(?,?)", id, "sinst_00000000000000000000000000000001"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO skill_installations(installation_id,skill_id,name,description,source_kind,source_reference,digest,version_hint,trusted,enabled,installed_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, "sinst_00000000000000000000000000000001", "skill_00000000000000000000000000000001", "Legacy Skill", "Workflow", "local", "legacy", "sha256:"+strings.Repeat("a", 64), "v1", 1, 1, "2026-10-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for i, owned := range []bool{true, false} {
		id := []skill.DeploymentID{"sdep_00000000000000000000000000000001", "sdep_00000000000000000000000000000002"}[i]
		name := []string{"owned", "unowned"}[i]
		item, err := skill.NewDeployment(id, "sinst_00000000000000000000000000000001", skill.DeploymentAgentsProject, root, filepath.Join(root, name), "sha256:"+strings.Repeat("a", 64), time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
		if err != nil {
			t.Fatal(err)
		}
		if owned {
			item, err = item.WithProfile("prof_00000000000000000000000000000001", 41)
			if err != nil {
				t.Fatal(err)
			}
		}
		if err := skillStore.CreateDeployment(context.Background(), item); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func TestProfileV7MigratesNonemptyDeliveriesAndPreservesForeignGuards(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	old := legacyDeliveryDatabase(t, path)
	if err := (&Store{db: old}).EnsureSkillReferenceGuards(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	for i, mode := range []string{"direct", "dynamic", "hybrid"} {
		slug, _ := profile.NewSlug(mode)
		p, err := s.GetProfileBySlug(context.Background(), slug)
		if err != nil {
			t.Fatal(err)
		}
		if p.Revision() != uint64(41+i) || len(p.Bindings()) != 2 || len(p.Skills()) != 1 || p.Bindings()[0].Alias() != "aliasa" || p.Bindings()[0].PolicyConstraint() != profile.PolicyRequireApproval || len(p.Bindings()[0].CredentialRefs()) != 1 {
			t.Fatalf("lost selection/governance: %+v", p)
		}
		var gotMode, tunnel string
		var revision, direct int
		if err := s.db.QueryRow(`SELECT mode,tunnel_id,revision FROM mcp_delivery_configs WHERE profile_id=?`, p.ID().String()).Scan(&gotMode, &tunnel, &revision); err != nil {
			t.Fatal(err)
		}
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM mcp_delivery_direct_tools WHERE profile_id=?`, p.ID().String()).Scan(&direct); err != nil {
			t.Fatal(err)
		}
		want := 0
		if mode == "hybrid" {
			want = 1
		}
		if gotMode != mode || tunnel != "tun_00000000000000000000000000000001" || revision != 0 || direct != want {
			t.Fatalf("delivery semantics changed: %s %s %d %d", gotMode, tunnel, revision, direct)
		}
	}
	skillStore, err := skillsqlite.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = skillStore.Close() }()
	for _, owned := range []bool{true, false} {
		id := skill.DeploymentID("sdep_00000000000000000000000000000002")
		if owned {
			id = "sdep_00000000000000000000000000000001"
		}
		item, err := skillStore.GetDeployment(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if owned && (item.OwnerProfileID() != "prof_00000000000000000000000000000001" || item.ProfileRevision() != 41) || !owned && (item.OwnerProfileID() != "" || item.ProfileRevision() != 0) {
			t.Fatalf("Skill owner/revision changed: %+v", item)
		}
	}
	var indexes int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name IN('idx_profile_skills_installation','idx_skill_deployments_owner','idx_mcp_delivery_direct_tool','idx_mcp_delivery_tunnel')").Scan(&indexes); err != nil || indexes != 4 {
		t.Fatalf("handoff lost indexes: %d %v", indexes, err)
	}
	for _, query := range []string{"SELECT exposure_mode FROM profiles", "SELECT tunnel_id FROM profiles", "SELECT direct FROM profile_tools"} {
		if _, err := s.db.Exec(query); err == nil {
			t.Fatalf("legacy authority remains: %s", query)
		}
	}
	var owner, digest string
	var revision int
	if err := s.db.QueryRow("SELECT owner_profile_id,profile_revision,digest FROM skill_deployments WHERE deployment_id='sdep_00000000000000000000000000000002'").Scan(&owner, &revision, &digest); err != nil {
		t.Fatal(err)
	}
	if owner != "" || revision != 0 || digest != "sha256:"+strings.Repeat("a", 64) {
		t.Fatal("legacy unowned deployment was adopted or changed")
	}
	if _, err := s.db.Exec("UPDATE skill_installations SET enabled=0"); err == nil {
		t.Fatal("Skill membership guard disappeared")
	}
	if _, err := s.db.Exec(`INSERT INTO skill_deployments(deployment_id,installation_id,owner_profile_id,profile_revision,target,root,destination,digest,deployed_at) VALUES('stale','sinst_00000000000000000000000000000001','prof_00000000000000000000000000000001',40,'agents-project','root','destination','digest','2026-10-01T00:00:00Z')`); err == nil {
		t.Fatal("Skill selection-revision fence disappeared")
	}
	if _, err := s.db.Exec(`INSERT INTO profile_tools(profile_id,tool_id) VALUES('prof_00000000000000000000000000000001','tool_00000000000000000000000000000099')`); err == nil {
		t.Fatal("Catalog reference trigger disappeared")
	}
	if _, err := s.db.Exec("UPDATE profiles SET revision=revision+1 WHERE slug='direct'"); err != nil {
		t.Fatal(err)
	}
	var probes int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM profile_change_probe").Scan(&probes); err != nil || probes != 1 {
		t.Fatalf("foreign trigger lost: %d %v", probes, err)
	}
	var selectionVersion, deliveryVersion int
	if err := s.db.QueryRow("SELECT MAX(version) FROM profile_schema_migrations").Scan(&selectionVersion); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow("SELECT MAX(version) FROM mcp_delivery_schema_migrations").Scan(&deliveryVersion); err != nil {
		t.Fatal(err)
	}
	if selectionVersion != 7 || deliveryVersion != 1 {
		t.Fatalf("version handoff incomplete: %d/%d", selectionVersion, deliveryVersion)
	}
	// This connection has never opened the new MCP store. Migration itself
	// must already fence existing Tunnel references at its atomic commit.
	other, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.Close() }()
	if _, err := other.Exec("DELETE FROM tunnels"); err == nil || !strings.Contains(err.Error(), "MCP delivery Tunnel in use") {
		t.Fatalf("migration published unguarded Tunnel references: %v", err)
	}
}

func TestProfileV7FailureRollsBackBothSchemasAndOriginalColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollback.db")
	db := legacyDeliveryDatabase(t, path)
	if _, err := db.Exec(`CREATE TRIGGER incompatible_legacy_observer AFTER UPDATE OF exposure_mode ON profiles BEGIN SELECT NEW.exposure_mode; END`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if store, err := Open(context.Background(), path); err == nil {
		_ = store.Close()
		t.Fatal("incompatible legacy observer should stop migration")
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var version, created, rows int
	if err := db.QueryRow("SELECT MAX(version) FROM profile_schema_migrations").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE name IN('mcp_delivery_schema_migrations','mcp_delivery_configs','mcp_delivery_direct_tools')").Scan(&created); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM profile_tools WHERE direct IN(0,1)").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if version != 6 || created != 0 || rows != 6 {
		t.Fatalf("partial migration escaped rollback: version=%d created=%d rows=%d", version, created, rows)
	}
	var mode string
	if err := db.QueryRow("SELECT exposure_mode||tunnel_id FROM profiles WHERE slug='hybrid'").Scan(&mode); err != nil || !strings.HasPrefix(mode, "hybridtun_") {
		t.Fatalf("original data lost: %q %v", mode, err)
	}
}
