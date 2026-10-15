package localfs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
)

func TestStoreInstallsResolvesAndRemovesImmutableSkill(t *testing.T) {
	t.Parallel()

	managedRoot := filepath.Join(t.TempDir(), "managed")
	store, err := New(managedRoot)
	if err != nil {
		t.Fatal(err)
	}
	source := writeTestSkill(t, t.TempDir(), "example-skill", "1.0.0")

	id := mustInstallationID(t, "sinst_00112233445566778899aabbccddeeff")
	pkg, err := store.InstallLocal(context.Background(), source, id)
	if err != nil {
		t.Fatalf("InstallLocal() error = %v", err)
	}
	if pkg.Snapshot.Metadata.Name != "example-skill" ||
		pkg.Snapshot.Metadata.Values["version"] != "1.0.0" {
		t.Fatalf("snapshot metadata = %+v", pkg.Snapshot.Metadata)
	}
	absoluteSource, err := filepath.Abs(source)
	if err != nil {
		t.Fatal(err)
	}
	if pkg.SourceReference != absoluteSource {
		t.Fatalf("SourceReference = %q, want %q", pkg.SourceReference, absoluteSource)
	}

	skillID := mustSkillID(t, "skill_00112233445566778899aabbccddeeff")
	installation, err := skilldomain.NewInstallation(
		id,
		skillID,
		pkg.Snapshot.Metadata.Name,
		pkg.Snapshot.Metadata.Description,
		skilldomain.SourceLocalDirectory,
		pkg.SourceReference,
		pkg.Snapshot.Digest,
		pkg.Snapshot.Metadata.Values["version"],
		false,
		false,
		time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Mutating the source after installation does not alter the managed copy.
	if err := os.WriteFile(
		filepath.Join(source, "references", "REFERENCE.md"),
		[]byte("changed source\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	resolved, err := store.Resolve(context.Background(), installation)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.Digest != pkg.Snapshot.Digest {
		t.Fatalf("resolved digest = %s, want %s", resolved.Digest, pkg.Snapshot.Digest)
	}

	managedSkill := filepath.Join(
		managedRoot,
		id.String(),
		"example-skill",
		"SKILL.md",
	)
	info, err := os.Stat(managedSkill)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if got := info.Mode().Perm(); got != 0o400 {
			t.Fatalf("managed SKILL.md mode = %#o, want 0400", got)
		}
	}

	if err := store.Remove(context.Background(), id); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if _, err := os.Lstat(filepath.Join(managedRoot, id.String())); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("managed package still exists: %v", err)
	}
}

func TestStoreRejectsSourceInsideManagedRoot(t *testing.T) {
	t.Parallel()

	managedRoot := filepath.Join(t.TempDir(), "managed")
	store, err := New(managedRoot)
	if err != nil {
		t.Fatal(err)
	}
	source := writeTestSkill(t, managedRoot, "inside-skill", "")
	id := mustInstallationID(t, "sinst_11112222333344445555666677778888")

	_, err = store.InstallLocal(context.Background(), source, id)
	if err == nil || !strings.Contains(err.Error(), "outside managed skill root") {
		t.Fatalf("InstallLocal() error = %v, want managed-root rejection", err)
	}
}

func TestStoreResolveDetectsManagedContentDrift(t *testing.T) {
	t.Parallel()

	managedRoot := filepath.Join(t.TempDir(), "managed")
	store, err := New(managedRoot)
	if err != nil {
		t.Fatal(err)
	}
	source := writeTestSkill(t, t.TempDir(), "drift-skill", "")
	id := mustInstallationID(t, "sinst_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	pkg, err := store.InstallLocal(context.Background(), source, id)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Remove(context.Background(), id) })
	skillID := mustSkillID(t, "skill_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	installation, err := skilldomain.NewInstallation(
		id,
		skillID,
		pkg.Snapshot.Metadata.Name,
		pkg.Snapshot.Metadata.Description,
		skilldomain.SourceLocalDirectory,
		pkg.SourceReference,
		pkg.Snapshot.Digest,
		"",
		false,
		false,
		time.Now().UTC(),
	)
	if err != nil {
		t.Fatal(err)
	}

	managedSkill := filepath.Join(managedRoot, id.String(), "drift-skill", "SKILL.md")
	if err := os.Chmod(managedSkill, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		managedSkill,
		[]byte("---\nname: drift-skill\ndescription: tampered\n---\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	_, err = store.Resolve(context.Background(), installation)
	if err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("Resolve() error = %v, want digest mismatch", err)
	}
}

func writeTestSkill(
	t *testing.T,
	parent string,
	name string,
	version string,
) string {
	t.Helper()
	root := filepath.Join(parent, name)
	if err := os.MkdirAll(filepath.Join(root, "references"), 0o750); err != nil {
		t.Fatal(err)
	}
	metadata := ""
	if version != "" {
		metadata = "metadata:\n  version: \"" + version + "\"\n"
	}
	skill := "---\nname: " + name + "\ndescription: Managed Skill test.\n" +
		metadata + "---\n# Skill\n"
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(skill), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, "references", "REFERENCE.md"),
		[]byte("reference\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	return root
}

func mustInstallationID(t *testing.T, raw string) skilldomain.InstallationID {
	t.Helper()
	id, err := skilldomain.ParseInstallationID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func mustSkillID(t *testing.T, raw string) skilldomain.SkillID {
	t.Helper()
	id, err := skilldomain.ParseSkillID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
