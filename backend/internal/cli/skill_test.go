package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillCommandSurface(t *testing.T) {
	t.Parallel()

	root := NewRootCommand(Options{})
	for _, path := range [][]string{
		{"skill", "list"},
		{"skill", "inspect"},
		{"skill", "validate"},
		{"skill", "install"},
		{"skill", "trust"},
		{"skill", "untrust"},
		{"skill", "enable"},
		{"skill", "disable"},
		{"skill", "remove"},
	} {
		command, _, err := root.Find(path)
		if err != nil {
			t.Fatalf("Find(%v) error = %v", path, err)
		}
		if command == root {
			t.Fatalf("Find(%v) resolved to root", path)
		}
	}
}

func TestSkillCLIManagedLifecycle(t *testing.T) {
	t.Parallel()

	temp := t.TempDir()
	dbPath := filepath.Join(temp, "mender.db")
	source := filepath.Join(temp, "source", "example-skill")
	writeCLISkill(t, source, "1.0.0", "first")

	execute := func(args ...string) map[string]any {
		t.Helper()
		value, err := executeSkillJSON(dbPath, args...)
		if err != nil {
			t.Fatalf("%v: error = %v", args, err)
		}
		return value
	}

	initial := execute("skill", "list")
	initialItems := jsonArray(t, initial["installations"])
	if len(initialItems) != 1 {
		t.Fatalf("initial installations = %#v", initialItems)
	}
	builtIn := jsonObject(t, initialItems[0])
	if builtIn["name"] != "mender-tools" ||
		builtIn["sourceKind"] != "builtin" ||
		builtIn["trusted"] != true ||
		builtIn["enabled"] != true ||
		builtIn["builtIn"] != true {
		t.Fatalf("built-in installation = %#v", builtIn)
	}
	builtInID := stringValue(t, builtIn["installationId"])

	validated := execute("skill", "validate", source)
	validatedSkill := jsonObject(t, validated["skill"])
	if validatedSkill["name"] != "example-skill" ||
		validatedSkill["digest"] == "" {
		t.Fatalf("validated Skill = %#v", validatedSkill)
	}
	if _, err := os.Stat(filepath.Join(temp, "skills")); !os.IsNotExist(err) {
		t.Fatalf("validate unexpectedly materialized managed root: %v", err)
	}

	firstResult := execute("skill", "install", source)
	first := jsonObject(t, firstResult["installation"])
	if first["name"] != "example-skill" ||
		first["versionHint"] != "1.0.0" ||
		first["trusted"] != false ||
		first["enabled"] != false ||
		first["builtIn"] != false {
		t.Fatalf("first installation = %#v", first)
	}
	firstID := stringValue(t, first["installationId"])
	skillID := stringValue(t, first["skillId"])

	if _, err := executeSkillJSON(dbPath, "skill", "enable", firstID); err == nil ||
		!strings.Contains(err.Error(), "trust is required") {
		t.Fatalf("enable untrusted error = %v", err)
	}

	trusted := jsonObject(t, execute("skill", "trust", firstID)["installation"])
	if trusted["trusted"] != true || trusted["enabled"] != false {
		t.Fatalf("trusted installation = %#v", trusted)
	}
	enabled := jsonObject(t, execute("skill", "enable", firstID)["installation"])
	if enabled["trusted"] != true || enabled["enabled"] != true {
		t.Fatalf("enabled installation = %#v", enabled)
	}

	bySkillID := jsonObject(t, execute("skill", "inspect", skillID)["skill"])
	if bySkillID["installationId"] != firstID {
		t.Fatalf("inspect SkillID = %#v", bySkillID)
	}
	resources := jsonArray(t, bySkillID["resources"])
	if len(resources) != 2 {
		t.Fatalf("inspect resources = %#v", resources)
	}

	writeCLISkill(t, source, "1.1.0", "second")
	secondResult := execute("skill", "install", source)
	second := jsonObject(t, secondResult["installation"])
	secondID := stringValue(t, second["installationId"])
	if secondID == firstID || second["skillId"] != skillID {
		t.Fatalf("second installation = %#v first=%#v", second, first)
	}
	if second["versionHint"] != "1.1.0" {
		t.Fatalf("second version = %#v", second)
	}

	if _, err := executeSkillJSON(dbPath, "skill", "inspect", skillID); err == nil ||
		!strings.Contains(err.Error(), "matches 2 installations") {
		t.Fatalf("ambiguous SkillID inspect error = %v", err)
	}
	exact := jsonObject(t, execute("skill", "inspect", secondID)["skill"])
	if exact["installationId"] != secondID {
		t.Fatalf("exact inspect = %#v", exact)
	}

	untrusted := jsonObject(t, execute("skill", "untrust", firstID)["installation"])
	if untrusted["trusted"] != false || untrusted["enabled"] != false {
		t.Fatalf("untrusted installation = %#v", untrusted)
	}

	if _, err := executeSkillJSON(dbPath, "skill", "remove", builtInID); err == nil ||
		!strings.Contains(err.Error(), "built-in skill installation is immutable") {
		t.Fatalf("remove built-in error = %v", err)
	}

	execute("skill", "remove", firstID)
	execute("skill", "remove", secondID)
	final := execute("skill", "list")
	finalItems := jsonArray(t, final["installations"])
	if len(finalItems) != 1 || jsonObject(t, finalItems[0])["name"] != "mender-tools" {
		t.Fatalf("final installations = %#v", finalItems)
	}
}

func executeSkillJSON(
	dbPath string,
	args ...string,
) (map[string]any, error) {
	var stdout, stderr bytes.Buffer
	root := NewRootCommand(Options{
		Stdin:  strings.NewReader(""),
		Stdout: &stdout,
		Stderr: &stderr,
	})
	root.SetArgs(append(
		[]string{"--db", dbPath, "--output", "json"},
		args...,
	))
	if err := root.ExecuteContext(context.Background()); err != nil {
		return nil, err
	}
	var value map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &value); err != nil {
		return nil, err
	}
	return value, nil
}

func writeCLISkill(
	t *testing.T,
	root string,
	version string,
	reference string,
) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "references"), 0o750); err != nil {
		t.Fatal(err)
	}
	content := "---\n" +
		"name: example-skill\n" +
		"description: Skill CLI lifecycle test.\n" +
		"metadata:\n" +
		"  version: \"" + version + "\"\n" +
		"---\n" +
		"# Example Skill\n"
	if err := os.WriteFile(
		filepath.Join(root, "SKILL.md"),
		[]byte(content),
		0o600,
	); err != nil {
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

func jsonObject(t *testing.T, value any) map[string]any {
	t.Helper()
	result, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("value = %#v, want object", value)
	}
	return result
}

func jsonArray(t *testing.T, value any) []any {
	t.Helper()
	result, ok := value.([]any)
	if !ok {
		t.Fatalf("value = %#v, want array", value)
	}
	return result
}

func stringValue(t *testing.T, value any) string {
	t.Helper()
	result, ok := value.(string)
	if !ok || result == "" {
		t.Fatalf("value = %#v, want non-empty string", value)
	}
	return result
}
