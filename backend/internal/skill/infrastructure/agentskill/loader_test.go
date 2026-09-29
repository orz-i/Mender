package agentskill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
)

func TestLoadDirectoryValidatesAndInventoriesPortableSkill(t *testing.T) {
	t.Parallel()

	root := makeSkillDir(t, "example-skill", `---
name: example-skill
description: Use this Skill for deterministic test workflows.
license: Apache-2.0
compatibility: Requires git
metadata:
  author: example
  version: "1.2.3"
allowed-tools: Bash(git:*) Read
---

# Example
`)
	writeSkillFile(t, root, "scripts/run.sh", "#!/bin/sh\necho ok\n", 0o755)
	writeSkillFile(t, root, "references/REFERENCE.md", "# Reference\n", 0o644)
	writeSkillFile(t, root, "assets/template.txt", "template\n", 0o644)
	writeSkillFile(t, root, "LICENSE.txt", "license\n", 0o644)

	first, err := LoadDirectory(root)
	if err != nil {
		t.Fatalf("LoadDirectory() error = %v", err)
	}
	second, err := LoadDirectory(root)
	if err != nil {
		t.Fatalf("second LoadDirectory() error = %v", err)
	}

	if first.Digest == "" || first.Digest != second.Digest {
		t.Fatalf("digest = %q second=%q, want stable non-empty digest", first.Digest, second.Digest)
	}
	if first.Metadata.Name != "example-skill" ||
		first.Metadata.Description != "Use this Skill for deterministic test workflows." ||
		first.Metadata.License != "Apache-2.0" ||
		first.Metadata.Compatibility != "Requires git" ||
		first.Metadata.Values["version"] != "1.2.3" ||
		first.Metadata.AllowedTools != "Bash(git:*) Read" {
		t.Fatalf("metadata = %+v", first.Metadata)
	}
	if len(first.Resources) != 5 {
		t.Fatalf("resources count = %d, want 5", len(first.Resources))
	}

	kinds := make(map[string]skilldomain.ResourceKind, len(first.Resources))
	executable := false
	for _, resource := range first.Resources {
		kinds[resource.Path] = resource.Kind
		if resource.Path == "scripts/run.sh" {
			executable = resource.Executable
		}
		if !strings.HasPrefix(resource.SHA256, "sha256:") {
			t.Fatalf("resource %q hash = %q", resource.Path, resource.SHA256)
		}
	}
	if kinds["SKILL.md"] != skilldomain.ResourceManifest ||
		kinds["scripts/run.sh"] != skilldomain.ResourceScript ||
		kinds["references/REFERENCE.md"] != skilldomain.ResourceReference ||
		kinds["assets/template.txt"] != skilldomain.ResourceAsset ||
		kinds["LICENSE.txt"] != skilldomain.ResourceOther {
		t.Fatalf("resource kinds = %+v", kinds)
	}
	if !executable {
		t.Fatal("scripts/run.sh executable = false, want true")
	}

	writeSkillFile(t, root, "references/REFERENCE.md", "# Changed\n", 0o644)
	changed, err := LoadDirectory(root)
	if err != nil {
		t.Fatalf("changed LoadDirectory() error = %v", err)
	}
	if changed.Digest == first.Digest {
		t.Fatal("digest did not change after content mutation")
	}
}

func TestLoadDirectoryRejectsInvalidFrontmatter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		directory string
		content   string
		want      string
	}{
		{
			name:      "missing frontmatter",
			directory: "bad-skill",
			content:   "# no frontmatter\n",
			want:      "must start with YAML frontmatter",
		},
		{
			name:      "invalid name",
			directory: "bad-skill",
			content:   "---\nname: Bad_Skill\ndescription: bad\n---\n",
			want:      "name must be 1-64",
		},
		{
			name:      "directory mismatch",
			directory: "bad-skill",
			content:   "---\nname: other-skill\ndescription: bad\n---\n",
			want:      "must match directory",
		},
		{
			name:      "description type",
			directory: "bad-skill",
			content:   "---\nname: bad-skill\ndescription: 42\n---\n",
			want:      "frontmatter description must be a string",
		},
		{
			name:      "metadata value type",
			directory: "bad-skill",
			content:   "---\nname: bad-skill\ndescription: bad\nmetadata:\n  version: 2\n---\n",
			want:      "metadata value for",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := makeSkillDir(t, test.directory, test.content)
			_, err := LoadDirectory(root)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("LoadDirectory() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestLoadDirectoryEnforcesFilesystemBounds(t *testing.T) {
	t.Parallel()

	t.Run("file count", func(t *testing.T) {
		t.Parallel()
		root := makeSkillDir(t, "bounded-skill", validSkill("bounded-skill"))
		writeSkillFile(t, root, "references/a.md", "a", 0o644)
		limits := DefaultLimits()
		limits.MaxFiles = 1
		_, err := LoadDirectoryWithLimits(root, limits)
		if err == nil || !strings.Contains(err.Error(), "more than 1 files") {
			t.Fatalf("error = %v, want file count error", err)
		}
	})

	t.Run("file size", func(t *testing.T) {
		t.Parallel()
		root := makeSkillDir(t, "bounded-skill", validSkill("bounded-skill"))
		limits := DefaultLimits()
		limits.MaxFileBytes = 32
		limits.MaxTotalBytes = 64
		_, err := LoadDirectoryWithLimits(root, limits)
		if err == nil || !strings.Contains(err.Error(), "SKILL.md exceeds") {
			t.Fatalf("error = %v, want file size error", err)
		}
	})

	t.Run("total size", func(t *testing.T) {
		t.Parallel()
		root := makeSkillDir(t, "bounded-skill", validSkill("bounded-skill"))
		writeSkillFile(t, root, "assets/blob.txt", strings.Repeat("x", 80), 0o644)
		limits := DefaultLimits()
		limits.MaxFileBytes = 128
		limits.MaxTotalBytes = 128
		_, err := LoadDirectoryWithLimits(root, limits)
		if err == nil || !strings.Contains(err.Error(), "total bytes") {
			t.Fatalf("error = %v, want total size error", err)
		}
	})

	t.Run("resource symlink", func(t *testing.T) {
		t.Parallel()
		root := makeSkillDir(t, "linked-skill", validSkill("linked-skill"))
		target := filepath.Join(root, "target.txt")
		if err := os.WriteFile(target, []byte("target"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(root, "references")); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		_, err := LoadDirectory(root)
		if err == nil || !strings.Contains(err.Error(), "must not be a symlink") {
			t.Fatalf("error = %v, want symlink error", err)
		}
	})

	t.Run("root symlink", func(t *testing.T) {
		t.Parallel()
		root := makeSkillDir(t, "real-skill", validSkill("real-skill"))
		link := filepath.Join(t.TempDir(), "real-skill")
		if err := os.Symlink(root, link); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		_, err := LoadDirectory(link)
		if err == nil || !strings.Contains(err.Error(), "root must not be a symlink") {
			t.Fatalf("error = %v, want root symlink error", err)
		}
	})
}

func makeSkillDir(t *testing.T, name, skill string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(root, 0o750); err != nil {
		t.Fatal(err)
	}
	writeSkillFile(t, root, "SKILL.md", skill, 0o644)
	return root
}

func writeSkillFile(t *testing.T, root, path, content string, mode os.FileMode) {
	t.Helper()
	fullPath := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func validSkill(name string) string {
	return "---\nname: " + name + "\ndescription: A valid Skill used by tests.\n---\n"
}
