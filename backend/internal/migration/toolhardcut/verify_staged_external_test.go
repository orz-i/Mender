package toolhardcut_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	migration "github.com/orz-i/Mender/backend/internal/migration/toolhardcut"
	_ "modernc.org/sqlite"
)

func stageVerifiedCandidate(t *testing.T) migration.StagedCopy {
	t.Helper()
	ctx := context.Background()
	source := emptyRuntimeFixture(t)
	plan, err := migration.AssessPath(ctx, source)
	if err != nil || plan.Status != "ready" {
		t.Fatalf("blank source: %+v %v", plan.Conflicts, err)
	}
	artifact, err := migration.StageCopy(ctx, source, filepath.Join(t.TempDir(), "candidate.sqlite"), plan.ManifestDigest)
	if err != nil {
		t.Fatal(err)
	}
	if err := migration.VerifyStagedCopy(ctx, artifact); err != nil {
		t.Fatalf("unmodified candidate rejected: %v", err)
	}
	return artifact
}

func testRehashCandidate(t *testing.T, a *migration.StagedCopy) {
	t.Helper()
	// #nosec G304 -- disposable test's Stage E candidate only.
	raw, err := os.ReadFile(a.CandidatePath)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(raw)
	a.AfterFileSHA256 = "sha256:" + hex.EncodeToString(hash[:])
}

func TestVerifyStagedCopyRefusesTamperedCandidateEvenWithRestatedHash(t *testing.T) {
	for _, tc := range []struct{ name, sql, expected string }{
		{"hardcut-marker", "UPDATE tool_hardcut_schema_migrations SET before_manifest_digest='sha256:forged'", "marker"},
		{"future-context-version", "INSERT INTO profile_schema_migrations(version,applied_at) VALUES(9,'tampered')", "version"},
		{"legacy-facade-reintroduced", "CREATE TABLE agent_tool_facades(agent_id TEXT,provider_id TEXT)", "facade"},
		{"fts-out-of-sync", "DELETE FROM tools_fts WHERE tool_id=(SELECT id FROM tools LIMIT 1)", "FTS"},
		{"profile-authz-expansion", "INSERT INTO profiles(id,slug,display_name,revision) VALUES('prof_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','forged','Forged',0)", "profiles"},
		{"policy-default-tamper", "UPDATE policy_settings SET default_decision='deny' WHERE singleton_id=1", "policy_settings"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			artifact := stageVerifiedCandidate(t)
			db, err := sql.Open("sqlite", artifact.CandidatePath)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(tc.sql); err != nil {
				_ = db.Close()
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			// Even if somebody replaces the advertised file digest, independent
			// post-commit SQL provenance and schema evidence still rejects it.
			testRehashCandidate(t, &artifact)
			if err := migration.VerifyStagedCopy(context.Background(), artifact); err == nil ||
				!strings.Contains(err.Error(), tc.expected) {
				t.Fatalf("tampered candidate passed or wrong rejection: %v", err)
			}
		})
	}
}

func TestVerifyStagedCopyProtectsPreUpgradeBackupAndWALIndependence(t *testing.T) {
	t.Run("backup-review-drift", func(t *testing.T) {
		artifact := stageVerifiedCandidate(t)
		db, err := sql.Open("sqlite", artifact.PreUpgradeBackupPath)
		if err != nil {
			t.Fatal(err)
		}
		_, err = db.Exec(`INSERT INTO providers(id,namespace,display_name,description,created_at,updated_at)
			VALUES('prov_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','drift','Drift','','now','now')`)
		if closeErr := db.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			t.Fatal(err)
		}
		if err := migration.VerifyStagedCopy(context.Background(), artifact); err == nil ||
			!strings.Contains(err.Error(), "backup differs") {
			t.Fatalf("unreviewed backup was accepted: %v", err)
		}
	})
	t.Run("journal-sidecar", func(t *testing.T) {
		artifact := stageVerifiedCandidate(t)
		if err := os.WriteFile(artifact.CandidatePath+"-wal", []byte("not a valid WAL"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := migration.VerifyStagedCopy(context.Background(), artifact); err == nil ||
			!strings.Contains(err.Error(), "sidecar") {
			t.Fatalf("non-self-contained candidate was accepted: %v", err)
		}
	})
	t.Run("cancelled", func(t *testing.T) {
		artifact := stageVerifiedCandidate(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := migration.VerifyStagedCopy(ctx, artifact); err == nil {
			t.Fatal("cancelled verification accepted")
		}
	})
}
