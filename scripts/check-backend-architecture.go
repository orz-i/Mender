// Run from the repository root: go run scripts/check-backend-architecture.go
// AST parsing includes platform-specific sources without executing project code.
package main

import (
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const internalPrefix = "github.com/orz-i/Mender/backend/internal/"

type architectureException struct {
	File     string `json:"file"`
	Import   string `json:"import"`
	Reason   string `json:"reason"`
	RemoveIn string `json:"removeIn"`
}

func main() {
	data, err := os.ReadFile("scripts/backend-architecture-exceptions.json")
	if err != nil {
		fail(err)
	}
	var exceptions []architectureException
	if err := json.Unmarshal(data, &exceptions); err != nil {
		fail(err)
	}
	violations, err := checkArchitecture(".", exceptions)
	if err != nil {
		fail(err)
	}
	if len(violations) != 0 {
		for _, violation := range violations {
			fmt.Fprintln(os.Stderr, violation)
		}
		os.Exit(1)
	}
	fmt.Printf("backend architecture check: ok (%d exact legacy exceptions)\n", len(exceptions))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "backend architecture check:", err)
	os.Exit(1)
}

func checkArchitecture(root string, exceptions []architectureException) ([]string, error) {
	allowed := make(map[string]bool, len(exceptions))
	for _, exception := range exceptions {
		key := exception.File + ":" + exception.Import
		if exception.Reason == "" || exception.RemoveIn == "" ||
			!strings.HasPrefix(exception.File, "backend/internal/") ||
			forbiddenDependency(exception.File, exception.Import) == "" {
			return nil, fmt.Errorf("invalid architecture exception %s", key)
		}
		if _, duplicate := allowed[key]; duplicate {
			return nil, fmt.Errorf("duplicate architecture exception %s", key)
		}
		allowed[key] = false
	}
	var violations []string
	err := filepath.WalkDir(filepath.Join(root, "backend/internal"), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range file.Imports {
			imported, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			reason := forbiddenDependency(relative, imported)
			if reason == "" {
				continue
			}
			key := relative + ":" + imported
			if _, exists := allowed[key]; exists {
				allowed[key] = true
				continue
			}
			violations = append(violations, fmt.Sprintf("%s:%d: %s imports %s", relative, fset.Position(spec.Pos()).Line, reason, imported))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for key, used := range allowed {
		if !used {
			violations = append(violations, "unused architecture exception: "+key)
		}
	}
	slices.Sort(violations)
	return violations, nil
}

func forbiddenDependency(file, imported string) string {
	parts := strings.Split(file, "/")
	domain := slices.Contains(parts, "domain")
	application := slices.Contains(parts, "application")
	if domain || application {
		if strings.HasPrefix(imported, internalPrefix) {
			dependency := strings.TrimPrefix(imported, internalPrefix)
			segments := strings.Split(dependency, "/")
			if slices.Contains(segments, "infrastructure") ||
				segments[0] == "bootstrap" || segments[0] == "cli" ||
				strings.HasPrefix(dependency, "desktop/binding") ||
				strings.HasPrefix(dependency, "management/httpapi") ||
				dependency == "providerplugin/configstore" ||
				dependency == "providerplugin/external" ||
				dependency == "providerplugin/builtin" ||
				dependency == "plugin/process" || dependency == "providerhttp" {
				return "inward layer"
			}
			if domain && slices.Contains(segments, "application") {
				return "domain"
			}
		}
		if imported == "os" || imported == "os/exec" || imported == "database/sql" || imported == "net/http" ||
			strings.HasPrefix(imported, "modernc.org/sqlite") ||
			strings.HasPrefix(imported, "github.com/modelcontextprotocol/") ||
			strings.HasPrefix(imported, "github.com/wailsapp/") ||
			strings.HasPrefix(imported, "google.golang.org/grpc") ||
			strings.HasPrefix(imported, "github.com/orz-i/Mender/sdk/") {
			return "application/domain mechanism boundary"
		}
	}
	if filepath.ToSlash(filepath.Dir(file)) == "backend/internal/providerplugin" &&
		strings.HasPrefix(imported, internalPrefix+"providerplugin/application") {
		return "Provider runtime reverse edge"
	}
	return ""
}
