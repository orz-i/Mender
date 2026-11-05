// Run from the repository root: go run scripts/check-backend-architecture.go
// AST parsing includes platform-specific sources without executing project code.
package main

import (
	"fmt"
	"go/ast"
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

func main() {
	violations, err := checkArchitecture(".")
	if err != nil {
		fail(err)
	}
	if len(violations) != 0 {
		for _, violation := range violations {
			fmt.Fprintln(os.Stderr, violation)
		}
		os.Exit(1)
	}
	fmt.Println("backend architecture check: ok (no legacy import exceptions)")
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "backend architecture check:", err)
	os.Exit(1)
}

func checkArchitecture(root string) ([]string, error) {
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
		file, err := parser.ParseFile(fset, path, nil, 0)
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
			violations = append(violations, fmt.Sprintf("%s:%d: %s imports %s", relative, fset.Position(spec.Pos()).Line, reason, imported))
		}
		violations = append(violations, filepathMechanismViolations(relative, file, fset)...)
		violations = append(violations, workspaceLegacyViolations(relative, file, fset)...)
		violations = append(violations, profileDeliveryViolations(relative, file, fset)...)
		violations = append(violations, bootstrapManagedMCPViolations(relative, file, fset)...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.Sort(violations)
	return violations, nil
}

// filepath also contains filesystem operations. Allowing its lexical helpers
// must not implicitly allow directory walks, symlink resolution or cwd lookup.
func filepathMechanismViolations(path string, file *ast.File, fset *token.FileSet) []string {
	parts := strings.Split(path, "/")
	if !slices.Contains(parts, "application") && !slices.Contains(parts, "domain") {
		return nil
	}
	var violations []string
	aliases := map[string]bool{}
	for _, imported := range file.Imports {
		importPath, err := strconv.Unquote(imported.Path.Value)
		if err != nil || importPath != "path/filepath" {
			continue
		}
		alias := "filepath"
		if imported.Name != nil {
			alias = imported.Name.Name
		}
		if alias == "." {
			violations = append(violations, fmt.Sprintf("%s:%d: inward layer dot-imports path/filepath", path, fset.Position(imported.Pos()).Line))
		} else {
			aliases[alias] = true
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		qualifier, ok := selector.X.(*ast.Ident)
		if !ok || qualifier.Obj != nil || !aliases[qualifier.Name] {
			return true
		}
		switch selector.Sel.Name {
		case "Abs", "EvalSymlinks", "Glob", "Walk", "WalkDir":
			violations = append(violations, fmt.Sprintf("%s:%d: application/domain mechanism boundary uses filepath.%s", path, fset.Position(selector.Pos()).Line, selector.Sel.Name))
		}
		return true
	})
	return violations
}

func forbiddenDependency(file, imported string) string {
	if strings.HasPrefix(file, "backend/internal/profile/domain/") || strings.HasPrefix(file, "backend/internal/profile/application/") {
		if strings.HasPrefix(imported, internalPrefix+"mcp/") || strings.HasPrefix(imported, internalPrefix+"tunnel/") {
			return "Profile selection/delivery boundary"
		}
	}
	if strings.HasPrefix(file, "backend/internal/mcp/infrastructure/managed/") {
		for _, adapter := range []string{"bootstrap", "cli", "desktop", "management"} {
			if imported == internalPrefix+adapter || strings.HasPrefix(imported, internalPrefix+adapter+"/") {
				return "managed MCP composition boundary"
			}
		}
		parts := strings.Split(strings.TrimPrefix(imported, internalPrefix), "/")
		if strings.HasPrefix(imported, internalPrefix) {
			for i := 0; i+1 < len(parts); i++ {
				if parts[i] == "infrastructure" && parts[i+1] == "sqlite" {
					return "managed MCP persistence callback boundary"
				}
			}
		}
		if imported == "database/sql" || strings.HasPrefix(imported, "modernc.org/sqlite") {
			return "managed MCP persistence callback boundary"
		}
	}
	if strings.HasPrefix(file, "backend/internal/workspace/domain/") || strings.HasPrefix(file, "backend/internal/workspace/application/") {
		if imported == "encoding/json" || strings.HasPrefix(imported, internalPrefix+"invocation/") || strings.HasPrefix(imported, internalPrefix+"sandbox/") {
			return "Workspace typed boundary"
		}
	}
	if workspaceDriver(file) && (strings.HasPrefix(imported, internalPrefix+"invocation/domain") || strings.HasPrefix(imported, internalPrefix+"invocation/infrastructure/")) {
		// External configuration validation still reuses the existing JSON Schema
		// validator; the typed Workspace codec is checked independently.
		if file != "backend/internal/sandbox/infrastructure/external/configuration.go" {
			return "Workspace driver presentation boundary"
		}
	}
	if strings.HasPrefix(file, "backend/internal/sandbox/application/") &&
		(strings.HasPrefix(imported, internalPrefix+"workspace/") || strings.HasPrefix(imported, internalPrefix+"invocation/")) {
		return "Sandbox lifetime boundary"
	}
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

func workspaceDriver(file string) bool {
	for _, backend := range []string{"localworkspace", "dockerengine", "daytona", "sshremote", "external"} {
		if strings.HasPrefix(file, "backend/internal/sandbox/infrastructure/"+backend+"/") {
			return true
		}
	}
	return false
}

func workspaceLegacyViolations(path string, file *ast.File, fset *token.FileSet) []string {
	if !workspaceDriver(path) && !strings.HasPrefix(path, "backend/internal/sandbox/application/") {
		return nil
	}
	var violations []string
	ast.Inspect(file, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.FuncDecl:
			if value.Name.Name == "ExecuteWorkspace" {
				violations = append(violations, fmt.Sprintf("%s:%d: legacy JSON Workspace execution method", path, fset.Position(value.Pos()).Line))
			}
		case *ast.TypeSpec:
			if value.Name.Name == "WorkspaceSession" {
				violations = append(violations, fmt.Sprintf("%s:%d: legacy JSON Workspace session port", path, fset.Position(value.Pos()).Line))
			}
		}
		return true
	})
	return violations
}

// Only the Profile aggregate and its binding values are covered. A Mode method
// on another context (or a different Profile value) is not a delivery leak.
func profileDeliveryViolations(path string, file *ast.File, fset *token.FileSet) []string {
	if !strings.HasPrefix(path, "backend/internal/profile/domain/") {
		return nil
	}
	var violations []string
	aggregates := map[string]string{"Profile": "Profile", "ToolBinding": "ToolBinding", "ToolBindingSpec": "ToolBindingSpec"}
	// Resolve local aliases without depending on the host's platform/type build.
	for changed := true; changed; {
		changed = false
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range general.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok || aggregates[typeSpec.Name.Name] != "" {
					continue
				}
				if base := aggregates[localTypeName(typeSpec.Type)]; base != "" {
					aggregates[typeSpec.Name.Name] = base
					changed = true
				}
			}
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.FuncDecl:
			if value.Recv == nil || len(value.Recv.List) != 1 || aggregates[localTypeName(value.Recv.List[0].Type)] == "" {
				break
			}
			switch value.Name.Name {
			case "Mode", "TunnelID", "SetTunnel", "ClearTunnel", "Direct", "DirectToolIDs", "withDirect":
				violations = append(violations, fmt.Sprintf("%s:%d: legacy Profile delivery method %s", path, fset.Position(value.Pos()).Line, value.Name.Name))
			}
		case *ast.TypeSpec:
			if value.Name.Name == "ExposureMode" {
				violations = append(violations, fmt.Sprintf("%s:%d: legacy Profile MCP exposure mode", path, fset.Position(value.Pos()).Line))
			}
			fields, ok := value.Type.(*ast.StructType)
			if !ok || aggregates[value.Name.Name] == "" {
				break
			}
			for _, field := range fields.Fields.List {
				for _, name := range field.Names {
					switch strings.ToLower(name.Name) {
					case "mode", "exposuremode", "exposure_mode", "tunnelid", "tunnel_id", "direct", "directtoolids":
						violations = append(violations, fmt.Sprintf("%s:%d: legacy Profile delivery field %s.%s", path, fset.Position(name.Pos()).Line, value.Name.Name, name.Name))
					}
				}
			}
		}
		return true
	})
	return violations
}

func localTypeName(expr ast.Expr) string {
	switch value := expr.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.StarExpr:
		return localTypeName(value.X)
	case *ast.ParenExpr:
		return localTypeName(value.X)
	}
	return ""
}

// Profile-managed listener ownership belongs to mcp/infrastructure/managed.
// Agent Task capability proxies and local Provider identity sets are separate.
func bootstrapManagedMCPViolations(path string, file *ast.File, fset *token.FileSet) []string {
	if !strings.HasPrefix(path, "backend/internal/bootstrap/") {
		return nil
	}
	var violations []string
	base := filepath.Base(path)
	managedFile := base == "mcp_servers.go" || strings.HasPrefix(base, "mcp_servers_")
	aliases := map[string]string{}
	if managedFile {
		for _, imported := range file.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil || (importPath != "net" && importPath != "net/http") {
				continue
			}
			alias := filepath.Base(importPath)
			if imported.Name != nil {
				alias = imported.Name.Name
			}
			if alias == "." {
				violations = append(violations, fmt.Sprintf("%s:%d: Profile-managed bootstrap dot-imports listener mechanism %s", path, fset.Position(imported.Pos()).Line, importPath))
			} else {
				aliases[alias] = importPath
			}
		}
	}
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range general.Specs {
			switch value := spec.(type) {
			case *ast.TypeSpec:
				if value.Name.Name == "managedMCPServer" {
					violations = append(violations, fmt.Sprintf("%s:%d: legacy Profile-managed bootstrap server state", path, fset.Position(value.Pos()).Line))
				}
				fields, ok := value.Type.(*ast.StructType)
				if !ok || value.Name.Name != "Runtime" {
					continue
				}
				for _, field := range fields.Fields.List {
					for _, name := range field.Names {
						if name.Name == "mcpServers" || name.Name == "mcpServersMu" {
							violations = append(violations, fmt.Sprintf("%s:%d: legacy Profile-managed Runtime registry field %s", path, fset.Position(name.Pos()).Line, name.Name))
						}
					}
				}
			case *ast.ValueSpec:
				if !managedFile || general.Tok != token.VAR {
					continue
				}
				// Package-level maps would restore shared registry ownership. The
				// Provider activation method's local deduplication map is allowed.
				ast.Inspect(value, func(node ast.Node) bool {
					if _, closure := node.(*ast.FuncLit); closure {
						return false
					}
					if _, registry := node.(*ast.MapType); registry {
						violations = append(violations, fmt.Sprintf("%s:%d: Profile-managed bootstrap package map state", path, fset.Position(node.Pos()).Line))
					}
					return true
				})
			}
		}
	}
	if managedFile {
		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			qualifier, ok := selector.X.(*ast.Ident)
			if !ok || qualifier.Obj != nil {
				return true
			}
			mechanism := aliases[qualifier.Name]
			if mechanism == "net" && slices.Contains([]string{"Listener", "Listen", "ListenConfig", "ListenTCP", "ListenUnix"}, selector.Sel.Name) ||
				mechanism == "net/http" && slices.Contains([]string{"Server", "Serve", "ServeTLS", "ListenAndServe", "ListenAndServeTLS"}, selector.Sel.Name) {
				violations = append(violations, fmt.Sprintf("%s:%d: Profile-managed bootstrap listener mechanism %s.%s", path, fset.Position(selector.Pos()).Line, mechanism, selector.Sel.Name))
			}
			return true
		})
	}
	return violations
}
