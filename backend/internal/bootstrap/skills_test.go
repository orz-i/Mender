package bootstrap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
)

func TestRuntimeSkillLibraryIncludesBuiltInAndManagedInstallations(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	temp := t.TempDir()
	runtime, err := NewRuntime(ctx, RuntimeOptions{
		DatabasePath: filepath.Join(temp, "mender.db"),
		SkillRoot:    filepath.Join(temp, "managed-skills"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	items, err := runtime.Skills.List(ctx)
	if err != nil {
		t.Fatalf("Skills.List() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("initial Skills.List() count = %d, want 1", len(items))
	}
	builtIn := items[0]
	if builtIn.Name != "mender-tools" || !builtIn.BuiltIn ||
		builtIn.SourceKind != SkillSourceBuiltIn ||
		!builtIn.Trusted || !builtIn.Enabled {
		t.Fatalf("built-in Skill = %+v", builtIn)
	}
	inspection, err := runtime.Skills.Inspect(ctx, builtIn.InstallationID)
	if err != nil {
		t.Fatalf("Inspect(built-in) error = %v", err)
	}
	if inspection.Metadata.Name != "mender-tools" ||
		len(inspection.Resources) != 1 ||
		inspection.Resources[0].Path != "SKILL.md" {
		t.Fatalf("built-in inspection = %+v", inspection)
	}
	if _, err := runtime.Skills.SetTrust(ctx, builtIn.InstallationID, false); !errors.Is(err, ErrBuiltInSkillImmutable) {
		t.Fatalf("SetTrust(built-in) error = %v", err)
	}
	if err := runtime.Skills.Remove(ctx, builtIn.InstallationID); !errors.Is(err, ErrBuiltInSkillImmutable) {
		t.Fatalf("Remove(built-in) error = %v", err)
	}

	source := writeRuntimeSkill(t, filepath.Join(temp, "example-skill"))
	managed, err := runtime.Skills.InstallLocal(ctx, source)
	if err != nil {
		t.Fatalf("Skills.InstallLocal() error = %v", err)
	}
	if managed.Trusted() || managed.Enabled() {
		t.Fatalf("managed initial state trusted=%v enabled=%v", managed.Trusted(), managed.Enabled())
	}

	items, err = runtime.Skills.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("Skills.List() after install count = %d, want 2", len(items))
	}
	var managedProjection SkillInstallation
	for _, item := range items {
		if item.InstallationID == managed.ID() {
			managedProjection = item
		}
	}
	if managedProjection.InstallationID == "" ||
		managedProjection.BuiltIn ||
		managedProjection.SourceKind != string(skilldomain.SourceLocalDirectory) ||
		managedProjection.Trusted ||
		managedProjection.Enabled {
		t.Fatalf("managed projection = %+v", managedProjection)
	}

	if _, err := runtime.Skills.SetEnabled(ctx, managed.ID(), true); err == nil {
		t.Fatal("SetEnabled(untrusted) error = nil")
	}
	if _, err := runtime.Skills.SetTrust(ctx, managed.ID(), true); err != nil {
		t.Fatalf("SetTrust(managed) error = %v", err)
	}
	enabled, err := runtime.Skills.SetEnabled(ctx, managed.ID(), true)
	if err != nil {
		t.Fatalf("SetEnabled(managed) error = %v", err)
	}
	if !enabled.Trusted() || !enabled.Enabled() {
		t.Fatalf("enabled managed state trusted=%v enabled=%v", enabled.Trusted(), enabled.Enabled())
	}

	managedInspection, err := runtime.Skills.Inspect(ctx, managed.ID())
	if err != nil {
		t.Fatalf("Inspect(managed) error = %v", err)
	}
	if managedInspection.Installation.InstallationID != managed.ID() ||
		managedInspection.Metadata.Name != "example-skill" {
		t.Fatalf("managed inspection = %+v", managedInspection)
	}

	if err := runtime.Skills.Remove(ctx, managed.ID()); err != nil {
		t.Fatalf("Skills.Remove(managed) error = %v", err)
	}
	items, err = runtime.Skills.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "mender-tools" {
		t.Fatalf("Skills.List() after remove = %+v", items)
	}
}

func TestRuntimeUsesDefaultSkillRootBesideDatabase(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	temp := t.TempDir()
	databasePath := filepath.Join(temp, "state", "mender.db")
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o750); err != nil {
		t.Fatal(err)
	}
	runtime, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: databasePath})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	source := writeRuntimeSkill(t, filepath.Join(temp, "source-skill"))
	item, err := runtime.Skills.InstallLocal(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(filepath.Dir(databasePath), "skills", item.ID().String())
	if info, err := os.Stat(expected); err != nil || !info.IsDir() {
		t.Fatalf("default managed Skill root %q: info=%v err=%v", expected, info, err)
	}
	if err := runtime.Skills.Remove(ctx, item.ID()); err != nil {
		t.Fatalf("Skills.Remove() cleanup error = %v", err)
	}
}

func writeRuntimeSkill(t *testing.T, root string) string {
	t.Helper()
	if err := os.MkdirAll(root, 0o750); err != nil {
		t.Fatal(err)
	}
	content := "---\n" +
		"name: " + filepath.Base(root) + "\n" +
		"description: Runtime Skill Library test.\n" +
		"---\n# Skill\n"
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}
