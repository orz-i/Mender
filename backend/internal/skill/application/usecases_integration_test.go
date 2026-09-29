package application_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	skillapplication "github.com/orz-i/Mender/backend/internal/skill/application"
	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
	skilllocalfs "github.com/orz-i/Mender/backend/internal/skill/infrastructure/localfs"
	skillsqlite "github.com/orz-i/Mender/backend/internal/skill/infrastructure/sqlite"
)

func TestManagedSkillInstallationLifecycle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	temp := t.TempDir()
	repository, err := skillsqlite.Open(ctx, filepath.Join(temp, "mender.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })

	managedRoot := filepath.Join(temp, "skills")
	packages, err := skilllocalfs.New(managedRoot)
	if err != nil {
		t.Fatal(err)
	}
	ids := &sequenceIDs{values: []skilldomain.InstallationID{
		mustInstallationID(t, "sinst_00112233445566778899aabbccddeeff"),
		mustInstallationID(t, "sinst_11112222333344445555666677778888"),
		mustInstallationID(t, "sinst_22223333444455556666777788889999"),
	}}
	clock := fixedClock{value: time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)}
	install := skillapplication.NewInstallLocal(repository, packages, ids, clock)

	source := filepath.Join(temp, "source", "example-skill")
	writeSkill(t, source, "1.0.0", "first reference")
	first, err := install.Execute(ctx, source)
	if err != nil {
		t.Fatalf("InstallLocal(first) error = %v", err)
	}
	if first.Trusted() || first.Enabled() {
		t.Fatalf("first state trusted=%v enabled=%v", first.Trusted(), first.Enabled())
	}
	if first.VersionHint() != "1.0.0" {
		t.Fatalf("first version = %q", first.VersionHint())
	}

	writeSkill(t, source, "1.1.0", "second reference")
	second, err := install.Execute(ctx, source)
	if err != nil {
		t.Fatalf("InstallLocal(second) error = %v", err)
	}
	if second.SkillID() != first.SkillID() {
		t.Fatalf("SkillID changed across same source lineage: first=%s second=%s", first.SkillID(), second.SkillID())
	}
	if second.Digest() == first.Digest() {
		t.Fatal("digest did not change for new Skill content")
	}
	if second.VersionHint() != "1.1.0" {
		t.Fatalf("second version = %q", second.VersionHint())
	}

	list := skillapplication.NewListInstallations(repository)
	items, err := list.Execute(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("ListInstallations() count = %d, want 2", len(items))
	}

	inspect := skillapplication.NewInspectInstallation(repository, packages)
	inspection, err := inspect.Execute(ctx, second.ID())
	if err != nil {
		t.Fatalf("InspectInstallation() error = %v", err)
	}
	if inspection.Snapshot.Digest != second.Digest() ||
		inspection.Snapshot.Metadata.Values["version"] != "1.1.0" {
		t.Fatalf("inspection = %+v", inspection)
	}

	setTrust := skillapplication.NewSetTrust(repository)
	trusted, err := setTrust.Execute(ctx, second.ID(), true)
	if err != nil {
		t.Fatalf("SetTrust() error = %v", err)
	}
	if !trusted.Trusted() || trusted.Enabled() {
		t.Fatalf("trusted state trusted=%v enabled=%v", trusted.Trusted(), trusted.Enabled())
	}
	setEnabled := skillapplication.NewSetEnabled(repository)
	enabled, err := setEnabled.Execute(ctx, second.ID(), true)
	if err != nil {
		t.Fatalf("SetEnabled() error = %v", err)
	}
	if !enabled.Trusted() || !enabled.Enabled() {
		t.Fatalf("enabled state trusted=%v enabled=%v", enabled.Trusted(), enabled.Enabled())
	}
	untrusted, err := setTrust.Execute(ctx, second.ID(), false)
	if err != nil {
		t.Fatalf("SetTrust(false) error = %v", err)
	}
	if untrusted.Trusted() || untrusted.Enabled() {
		t.Fatalf("untrusted state trusted=%v enabled=%v", untrusted.Trusted(), untrusted.Enabled())
	}

	_, err = install.Execute(ctx, source)
	if !errors.Is(err, skillapplication.ErrPackageAlreadyInstalled) {
		t.Fatalf("duplicate InstallLocal() error = %v", err)
	}
	if _, err := os.Lstat(filepath.Join(managedRoot, ids.values[2].String())); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("duplicate package cleanup left files: %v", err)
	}

	remove := skillapplication.NewRemoveInstallation(repository, packages, nil)
	if err := remove.Execute(ctx, first.ID()); err != nil {
		t.Fatalf("RemoveInstallation() error = %v", err)
	}
	get := skillapplication.NewGetInstallation(repository)
	if _, err := get.Execute(ctx, first.ID()); !errors.Is(err, skillapplication.ErrInstallationNotFound) {
		t.Fatalf("GetInstallation(removed) error = %v", err)
	}
	if _, err := os.Lstat(filepath.Join(managedRoot, first.ID().String())); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("removed package still exists: %v", err)
	}
	if err := remove.Execute(ctx, second.ID()); err != nil {
		t.Fatalf("RemoveInstallation(second) error = %v", err)
	}
}

type sequenceIDs struct {
	values []skilldomain.InstallationID
	index  int
}

func (s *sequenceIDs) NewInstallationID() (skilldomain.InstallationID, error) {
	value := s.values[s.index]
	s.index++
	return value, nil
}

type fixedClock struct {
	value time.Time
}

func (c fixedClock) Now() time.Time {
	return c.value
}

func mustInstallationID(t *testing.T, raw string) skilldomain.InstallationID {
	t.Helper()
	id, err := skilldomain.ParseInstallationID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func writeSkill(t *testing.T, root, version, reference string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "references"), 0o750); err != nil {
		t.Fatal(err)
	}
	content := "---\n" +
		"name: example-skill\n" +
		"description: Managed application lifecycle Skill.\n" +
		"metadata:\n" +
		"  version: \"" + version + "\"\n" +
		"---\n" +
		"# Example\n"
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, "references", "REFERENCE.md"),
		[]byte(reference+"\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
}
