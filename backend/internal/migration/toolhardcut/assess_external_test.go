package toolhardcut_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/orz-i/Mender/backend/internal/migration/toolhardcut"
	legacyfixture "github.com/orz-i/Mender/backend/internal/testfixtures/v2"
)

func emptyRuntimeFixture(t *testing.T) string {
	t.Helper()
	return legacyfixture.EmptyDatabase(t)
}

func TestAssessReadOnlyAndDeterministicOnDisposableRuntime(t *testing.T) {
	path := emptyRuntimeFixture(t)
	// #nosec G304 -- path is a private t.TempDir SQLite fixture.
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	first, err := toolhardcut.AssessPath(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := toolhardcut.AssessPath(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("assessment changed without DB writes: %+v vs %+v", first.Snapshot, second.Snapshot)
	}
	if first.ReportVersion != 1 || first.Mode != "assessment" || !strings.HasPrefix(first.ManifestDigest, "sha256:") {
		t.Fatalf("invalid manifest: %+v", first)
	}
	if first.Snapshot.RuntimeWriterQuiesced {
		t.Fatal("read-only assessment claimed writer quiescence")
	}
	// #nosec G304 -- read-only assertion on a private t.TempDir fixture.
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("read-only assessment modified DB bytes")
	}
	if len(first.Snapshot.SchemaVersions) != 7 {
		t.Fatalf("unrecognized disposable schema: %+v conflicts %+v", first.Snapshot.SchemaVersions, first.Conflicts)
	}
	encoded, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "config_json") || strings.Contains(string(encoded), "result_text") {
		t.Fatal("raw sensitive database column leaked to report")
	}
	t.Logf("assessment status=%s, tools=%d, blockers=%d", first.Status, first.Counts.ToolsBefore, len(first.Conflicts))
}

func TestAssessDoesNotCreateMissingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.sqlite")
	if _, err := toolhardcut.AssessPath(context.Background(), path); err == nil {
		t.Fatal("missing DB opened or created")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("assess created DB")
	}
}
