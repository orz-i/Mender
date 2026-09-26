package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMenderToolsSkillReferencesStableCLICommands(t *testing.T) {
	t.Parallel()

	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "../../.."))
	skillPath := filepath.Join(repoRoot, "skills", "mender-tools", "SKILL.md")

	//nolint:gosec // skillPath is derived from this repository's test source path.
	raw, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", skillPath, err)
	}
	content := string(raw)

	required := []string{
		"mender tool search",
		"mender tool describe",
		"mender tool call",
		"--output json",
	}
	for _, fragment := range required {
		if !strings.Contains(content, fragment) {
			t.Errorf("SKILL.md missing %q", fragment)
		}
	}

	root := NewRootCommand(Options{})
	for _, commandPath := range [][]string{
		{"tool", "search"},
		{"tool", "describe"},
		{"tool", "call"},
	} {
		command, _, err := root.Find(commandPath)
		if err != nil {
			t.Fatalf("CLI command %v not found: %v", commandPath, err)
		}
		if command == root {
			t.Fatalf("CLI command %v resolved to root", commandPath)
		}
	}
}
