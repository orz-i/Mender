package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeArchitectureFixture(t *testing.T, root, file, source string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(file))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestArchitectureChecksPlatformFilesAndIgnoresTestFixtures(t *testing.T) {
	root := t.TempDir()
	writeArchitectureFixture(t, root, "backend/internal/example/application/leak_darwin.go", "package application\nimport _ \"database/sql\"\n")
	writeArchitectureFixture(t, root, "backend/internal/example/domain/source.go", "package domain\n// import \"os\" is only a comment\nimport \"fmt\"\n")
	writeArchitectureFixture(t, root, "backend/internal/example/application/source_test.go", "package application\nimport _ \"os\"\n")
	writeArchitectureFixture(t, root, "backend/internal/example/testdata/fixture.go", "not valid Go")
	violations, err := checkArchitecture(root, nil)
	if err != nil || len(violations) != 1 || !strings.Contains(violations[0], "leak_darwin.go") {
		t.Fatalf("platform leak not isolated: violations=%v err=%v", violations, err)
	}
}

func TestArchitectureExceptionsAreExactAndBecomeStale(t *testing.T) {
	root := t.TempDir()
	file := "backend/internal/agent/application/old.go"
	writeArchitectureFixture(t, root, file, "package application\nimport _ \"os\"\n")
	exceptions := []architectureException{{File: file, Import: "os", Reason: "existing read", RemoveIn: "R3"}}
	if violations, err := checkArchitecture(root, exceptions); err != nil || len(violations) != 0 {
		t.Fatalf("exact exception failed: %v %v", violations, err)
	}
	writeArchitectureFixture(t, root, "backend/internal/agent/application/new.go", "package application\nimport _ \"os\"\n")
	if violations, err := checkArchitecture(root, exceptions); err != nil || len(violations) != 1 {
		t.Fatalf("new file inherited exception: %v %v", violations, err)
	}
	writeArchitectureFixture(t, root, file, "package application\n")
	violations, err := checkArchitecture(root, exceptions)
	if err != nil || len(violations) != 2 || !strings.Contains(strings.Join(violations, "\n"), "unused architecture exception") {
		t.Fatalf("stale exception not rejected: %v %v", violations, err)
	}
}

func TestArchitectureRejectsInfrastructureAndReverseApplicationEdges(t *testing.T) {
	root := t.TempDir()
	writeArchitectureFixture(t, root, "backend/internal/management/application/query/leak.go", "package query\nimport _ \"github.com/orz-i/Mender/backend/internal/catalog/infrastructure/sqlite\"\n")
	writeArchitectureFixture(t, root, "backend/internal/catalog/domain/leak.go", "package domain\nimport _ \"github.com/orz-i/Mender/backend/internal/catalog/application\"\n")
	writeArchitectureFixture(t, root, "backend/internal/providerplugin/leak.go", "package providerplugin\nimport _ \"github.com/orz-i/Mender/backend/internal/providerplugin/application\"\n")
	writeArchitectureFixture(t, root, "backend/internal/providerplugin/application/ports.go", "package application\nimport _ \"github.com/orz-i/Mender/backend/internal/catalog/domain\"\n")
	violations, err := checkArchitecture(root, nil)
	if err != nil || len(violations) != 3 {
		t.Fatalf("expected three bad edges and an allowed consumer port: %v %v", violations, err)
	}
}

func TestArchitectureRejectsFilepathMechanismsAndAliases(t *testing.T) {
	root := t.TempDir()
	writeArchitectureFixture(t, root, "backend/internal/example/application/resolve.go", "package application\nimport fp \"path/filepath\"\nvar resolve = fp.EvalSymlinks\n")
	writeArchitectureFixture(t, root, "backend/internal/example/domain/walk.go", "package domain\nimport \"path/filepath\"\nvar walk = filepath.WalkDir\n")
	writeArchitectureFixture(t, root, "backend/internal/example/application/cwd.go", "package application\nimport \"path/filepath\"\nvar absolute = filepath.Abs\n")
	writeArchitectureFixture(t, root, "backend/internal/example/application/dot.go", "package application\nimport . \"path/filepath\"\nvar resolve = EvalSymlinks\n")
	writeArchitectureFixture(t, root, "backend/internal/example/application/raw.go", "package application\nimport fp `path/filepath`\nvar walk = fp.Walk\n")
	writeArchitectureFixture(t, root, "backend/internal/example/application/lexical.go", "package application\nimport fp \"path/filepath\"\nvar clean = fp.Clean\nfunc scoped(fp interface { EvalSymlinks(string) (string, error) }) { _, _ = fp.EvalSymlinks(\"x\") }\n")
	writeArchitectureFixture(t, root, "backend/internal/example/infrastructure/resolve.go", "package infrastructure\nimport \"path/filepath\"\nvar resolve = filepath.EvalSymlinks\n")
	violations, err := checkArchitecture(root, nil)
	if err != nil || len(violations) != 5 {
		t.Fatalf("filesystem mechanisms not isolated from lexical/port operations: %v %v", violations, err)
	}
	for _, expected := range []string{"filepath.EvalSymlinks", "filepath.WalkDir", "filepath.Walk", "filepath.Abs", "dot-imports path/filepath"} {
		if !strings.Contains(strings.Join(violations, "\n"), expected) {
			t.Fatalf("missing %s violation: %v", expected, violations)
		}
	}
}
