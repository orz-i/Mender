package agentplugin

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	skillapplication "github.com/orz-i/Mender/backend/internal/skill/application"
	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
	"github.com/orz-i/Mender/backend/internal/skill/infrastructure/agentskill"
)

func TestExporterWritesDeterministicSkillsOnlyAgentPlugin(t *testing.T) {
	t.Parallel()

	firstID := mustInstallationID(t, "sinst_00112233445566778899aabbccddeeff")
	secondID := mustInstallationID(t, "sinst_11112222333344445555666677778888")
	first := newFixture(t, "a-skill")
	second := newFixture(t, "b-skill")
	materializer := fixtureMaterializer{
		firstID:  first,
		secondID: second,
	}
	exporter := New(materializer)
	parent := t.TempDir()
	destinationA := filepath.Join(parent, "plugin-a")
	destinationB := filepath.Join(parent, "plugin-b")

	base := skillapplication.AgentPluginPackage{
		Name:        "mender.skills",
		Version:     "1.0.0",
		Description: "Portable Mender Skills.",
		Skills: []skillapplication.AgentPluginSkill{
			{InstallationID: secondID, Name: "b-skill", Digest: second.digest},
			{InstallationID: firstID, Name: "a-skill", Digest: first.digest},
		},
	}
	requestA := base
	requestA.Destination = destinationA
	if err := exporter.ExportAgentPlugin(context.Background(), requestA); err != nil {
		t.Fatalf("ExportAgentPlugin(A) error = %v", err)
	}
	requestB := base
	requestB.Destination = destinationB
	if err := exporter.ExportAgentPlugin(context.Background(), requestB); err != nil {
		t.Fatalf("ExportAgentPlugin(B) error = %v", err)
	}

	wantManifest := "{\n" +
		"  \"$schema\": \"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json\",\n" +
		"  \"name\": \"mender.skills\",\n" +
		"  \"version\": \"1.0.0\",\n" +
		"  \"description\": \"Portable Mender Skills.\"\n" +
		"}\n"
	if got := string(readRootFile(t, destinationA, "plugin.json")); got != wantManifest {
		t.Fatalf("plugin.json = %q, want %q", got, wantManifest)
	}
	rootA, err := os.OpenRoot(destinationA)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rootA.Lstat("mcp.json"); !errors.Is(err, os.ErrNotExist) {
		_ = rootA.Close()
		t.Fatalf("mcp.json exists or inspect failed: %v", err)
	}
	if err := rootA.Close(); err != nil {
		t.Fatal(err)
	}

	for _, fixture := range []skillFixture{first, second} {
		pathA := filepath.Join(destinationA, "skills", fixture.name)
		snapshot, err := agentskill.LoadDirectory(pathA)
		if err != nil {
			t.Fatalf("LoadDirectory(%s) error = %v", fixture.name, err)
		}
		if snapshot.Digest != fixture.digest {
			t.Fatalf(
				"exported %s digest = %s, want %s",
				fixture.name,
				snapshot.Digest,
				fixture.digest,
			)
		}
		pathB := filepath.Join(destinationB, "skills", fixture.name)
		if got, want := readRootFile(t, pathA, "SKILL.md"), readRootFile(t, pathB, "SKILL.md"); string(got) != string(want) {
			t.Fatalf("exported %s differs across deterministic exports", fixture.name)
		}
	}
	if got, want := readRootFile(t, destinationA, "plugin.json"), readRootFile(t, destinationB, "plugin.json"); string(got) != string(want) {
		t.Fatal("plugin.json differs across deterministic exports")
	}
}

func TestExporterRejectsInvalidManifestAndExistingDestination(t *testing.T) {
	t.Parallel()

	id := mustInstallationID(t, "sinst_00112233445566778899aabbccddeeff")
	fixture := newFixture(t, "example-skill")
	exporter := New(fixtureMaterializer{id: fixture})

	for _, name := range []string{
		"Bad",
		"-bad",
		"bad-",
		"bad--name",
		"bad..name",
		"bad_name",
		strings.Repeat("a", 65),
	} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := exporter.ExportAgentPlugin(
				context.Background(),
				skillapplication.AgentPluginPackage{
					Name:        name,
					Destination: filepath.Join(t.TempDir(), "plugin"),
					Skills: []skillapplication.AgentPluginSkill{
						{InstallationID: id, Name: fixture.name, Digest: fixture.digest},
					},
				},
			)
			if !errors.Is(err, skillapplication.ErrInvalidAgentPluginPackage) {
				t.Fatalf("ExportAgentPlugin(%q) error = %v", name, err)
			}
		})
	}

	parent := t.TempDir()
	destination := filepath.Join(parent, "existing")
	if err := os.Mkdir(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	err := exporter.ExportAgentPlugin(
		context.Background(),
		skillapplication.AgentPluginPackage{
			Name:        "valid.plugin",
			Destination: destination,
			Skills: []skillapplication.AgentPluginSkill{
				{InstallationID: id, Name: fixture.name, Digest: fixture.digest},
			},
		},
	)
	if !errors.Is(err, skillapplication.ErrAgentPluginTargetExists) {
		t.Fatalf("ExportAgentPlugin(existing) error = %v", err)
	}
}

func TestExporterRejectsMaterializedSkillMismatch(t *testing.T) {
	t.Parallel()

	id := mustInstallationID(t, "sinst_00112233445566778899aabbccddeeff")
	fixture := newFixture(t, "example-skill")
	exporter := New(fixtureMaterializer{id: fixture})
	err := exporter.ExportAgentPlugin(
		context.Background(),
		skillapplication.AgentPluginPackage{
			Name:        "valid.plugin",
			Destination: filepath.Join(t.TempDir(), "plugin"),
			Skills: []skillapplication.AgentPluginSkill{
				{
					InstallationID: id,
					Name:           fixture.name,
					Digest:         "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
				},
			},
		},
	)
	if !errors.Is(err, skillapplication.ErrInvalidAgentPluginPackage) {
		t.Fatalf("ExportAgentPlugin(mismatch) error = %v", err)
	}
}

type skillFixture struct {
	name    string
	content string
	digest  string
}

func newFixture(t *testing.T, name string) skillFixture {
	t.Helper()
	content := "---\nname: " + name + "\ndescription: Agent Plugin exporter test.\n---\n# " + name + "\n"
	root := filepath.Join(t.TempDir(), name)
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	writeRootFile(t, root, "SKILL.md", []byte(content), 0o600)
	snapshot, err := agentskill.LoadDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	return skillFixture{
		name:    name,
		content: content,
		digest:  snapshot.Digest,
	}
}

type fixtureMaterializer map[skilldomain.InstallationID]skillFixture

func (m fixtureMaterializer) MaterializeSkill(
	_ context.Context,
	skill skillapplication.AgentPluginSkill,
	destination string,
) error {
	fixture, ok := m[skill.InstallationID]
	if !ok {
		return errors.New("fixture not found")
	}
	if err := os.Mkdir(destination, 0o700); err != nil {
		return err
	}
	root, err := os.OpenRoot(destination)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	file, err := root.OpenFile("SKILL.md", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write([]byte(fixture.content)); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func writeRootFile(
	t *testing.T,
	rootPath string,
	name string,
	content []byte,
	mode os.FileMode,
) {
	t.Helper()
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func readRootFile(t *testing.T, rootPath, name string) []byte {
	t.Helper()
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	file, err := root.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	raw, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func mustInstallationID(t *testing.T, raw string) skilldomain.InstallationID {
	t.Helper()
	id, err := skilldomain.ParseInstallationID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
