package builtin

import (
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
	"github.com/orz-i/Mender/backend/internal/skill/infrastructure/agentskill"
)

func TestAllIncludesValidatedMenderToolsSnapshot(t *testing.T) {
	t.Parallel()

	entries, err := All()
	if err != nil {
		t.Fatalf("All() error = %v", err)
	}
	if got, want := len(entries), 1; got != want {
		t.Fatalf("All() count = %d, want %d", got, want)
	}
	entry := entries[0]
	if entry.SkillID != MenderToolsSkillID {
		t.Fatalf("SkillID = %s, want %s", entry.SkillID, MenderToolsSkillID)
	}
	if _, err := skilldomain.ParseSkillID(entry.SkillID.String()); err != nil {
		t.Fatalf("SkillID invalid: %v", err)
	}
	if _, err := skilldomain.ParseInstallationID(entry.InstallationID.String()); err != nil {
		t.Fatalf("InstallationID invalid: %v", err)
	}
	if entry.SourceKind != SourceKind || !entry.Trusted || !entry.Enabled {
		t.Fatalf("built-in state = %+v", entry)
	}
	if entry.Snapshot.Metadata.Name != "mender-tools" {
		t.Fatalf("Skill name = %q, want mender-tools", entry.Snapshot.Metadata.Name)
	}
	if entry.Snapshot.Digest == "" {
		t.Fatal("built-in digest is empty")
	}
	if len(entry.Snapshot.Resources) != 1 ||
		entry.Snapshot.Resources[0].Path != "SKILL.md" {
		t.Fatalf("resources = %+v, want only SKILL.md", entry.Snapshot.Resources)
	}
}

func TestEmbeddedMenderToolsMatchesRepositorySkill(t *testing.T) {
	t.Parallel()

	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "../../../.."))
	repositorySnapshot, err := agentskill.LoadDirectory(
		filepath.Join(repoRoot, "skills", "mender-tools"),
	)
	if err != nil {
		t.Fatalf("load repository mender-tools: %v", err)
	}
	entries, err := All()
	if err != nil {
		t.Fatalf("All() error = %v", err)
	}
	embeddedSnapshot := entries[0].Snapshot
	if !reflect.DeepEqual(embeddedSnapshot, repositorySnapshot) {
		t.Fatalf(
			"embedded mender-tools differs from repository source\nembedded=%+v\nrepository=%+v",
			embeddedSnapshot,
			repositorySnapshot,
		)
	}
}

func TestAllReturnsIndependentSnapshots(t *testing.T) {
	t.Parallel()

	first, err := All()
	if err != nil {
		t.Fatal(err)
	}
	first[0].Snapshot.Metadata.Values["mutated"] = "true"
	first[0].Snapshot.Resources[0].Path = "changed"

	second, err := All()
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := second[0].Snapshot.Metadata.Values["mutated"]; exists {
		t.Fatal("All() leaked metadata mutation")
	}
	if second[0].Snapshot.Resources[0].Path != "SKILL.md" {
		t.Fatal("All() leaked resource mutation")
	}
}
