package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	legacyfixture "github.com/orz-i/Mender/backend/internal/testfixtures/v2"
	_ "modernc.org/sqlite"
)

func cliDB(t *testing.T) string {
	t.Helper()
	return legacyfixture.EmptyDatabase(t)
}

func TestCLIReadOnlyAssessAndSeparateCandidate(t *testing.T) {
	db := cliDB(t)
	// #nosec G304 -- db is created under t.TempDir by this test fixture.
	before, err := os.ReadFile(db)
	if err != nil {
		t.Fatal(err)
	}
	var out, errs bytes.Buffer
	if code := run(context.Background(), []string{"assess", "--db", db, "--format", "json"}, &out, &errs); code != 0 {
		t.Fatalf("assessment exit %d: %s", code, errs.String())
	}
	var report struct {
		Status         string `json:"status"`
		ManifestDigest string `json:"manifestDigest"`
	}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil || report.Status != "ready" {
		t.Fatalf("CLI report invalid: %+v %v", report, err)
	}
	// #nosec G304 -- db is a private disposable test fixture path.
	if after, err := os.ReadFile(db); err != nil || !bytes.Equal(before, after) {
		t.Fatal("CLI assess changed original SQLite data")
	}
	candidate := filepath.Join(t.TempDir(), "candidate.sqlite")
	out.Reset()
	errs.Reset()
	code := run(context.Background(), []string{
		"stage-copy", "--db", db, "--destination", candidate, "--manifest", report.ManifestDigest,
	}, &out, &errs)
	if code != 0 {
		t.Fatalf("CLI stage-copy exit %d: %s", code, errs.String())
	}
	if _, err := os.Stat(candidate); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(candidate + ".preupgrade"); err != nil {
		t.Fatal(err)
	}
	// #nosec G304 -- db is a private disposable test fixture path.
	if after, err := os.ReadFile(db); err != nil || !bytes.Equal(before, after) {
		t.Fatal("CLI StageCopy modified source")
	}
	out.Reset()
	errs.Reset()
	if code := run(context.Background(), []string{
		"stage-copy", "--db", db, "--destination", filepath.Join(t.TempDir(), "refused.sqlite"),
		"--manifest", "sha256:deadbeef",
	}, &out, &errs); code != 3 {
		t.Fatalf("stale manifest did not fail closed: %d %s", code, errs.String())
	}
}

func TestCLIBlockedReportExitsNonzeroAndDoesNotRunStage(t *testing.T) {
	db := cliDB(t)
	raw, err := sql.Open("sqlite", db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec("DELETE FROM agent_schema_migrations WHERE version=19"); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	var out, errs bytes.Buffer
	code := run(context.Background(), []string{"assess", "--db", db}, &out, &errs)
	if code != 3 {
		t.Fatalf("blocked source exit %d: %s", code, errs.String())
	}
	var report struct {
		Status    string `json:"status"`
		Conflicts []any  `json:"conflicts"`
	}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil || report.Status != "blocked" || len(report.Conflicts) == 0 {
		t.Fatalf("blocked assessment not emitted: %+v %v", report, err)
	}
	if _, err := os.Stat(db + ".preupgrade"); !os.IsNotExist(err) {
		t.Fatal("CLI assessment created backup unexpectedly")
	}
}

func TestCLIArgumentsRequireExplicitPathsAndManifest(t *testing.T) {
	cases := [][]string{
		nil, {"unknown"}, {"stage-copy"}, {"assess"},
		{"assess", "--db", "/missing", "--format", "yaml"},
		{"stage-copy", "--db", "/missing", "--destination", "/tmp/unverified", "--manifest", ""},
		{"assess", "--db", "/missing", "positional"},
	}
	for _, args := range cases {
		var out, errs bytes.Buffer
		if code := run(context.Background(), args, &out, &errs); code == 0 {
			t.Fatalf("unsafe command %v was accepted", args)
		}
	}
	var out, errs bytes.Buffer
	if code := run(context.Background(), []string{"--help"}, &out, &errs); code != 0 || out.Len() == 0 {
		t.Fatal("CLI help unavailable")
	}
}
