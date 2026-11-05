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

func TestArchitectureRejectsWorkspacePresentationAndLegacyPorts(t *testing.T) {
	root := t.TempDir()
	writeArchitectureFixture(t, root, "backend/internal/workspace/application/json.go", "package application\nimport _ \"encoding/json\"\n")
	writeArchitectureFixture(t, root, "backend/internal/workspace/domain/outcome.go", "package domain\nimport _ \"github.com/orz-i/Mender/backend/internal/invocation/domain\"\n")
	writeArchitectureFixture(t, root, "backend/internal/sandbox/infrastructure/localworkspace/work.go", "package localworkspace\nimport inv \"github.com/orz-i/Mender/backend/internal/invocation/domain\"\nfunc ExecuteWorkspace() {}\n")
	writeArchitectureFixture(t, root, "backend/internal/sandbox/application/old.go", "package application\ntype WorkspaceSession interface{}\n")
	writeArchitectureFixture(t, root, "backend/internal/sandbox/infrastructure/external/workspace.go", "package external\nimport _ \"github.com/orz-i/Mender/backend/internal/invocation/domain\"\n")
	writeArchitectureFixture(t, root, "backend/internal/sandbox/infrastructure/external/configuration.go", "package external\nimport _ \"github.com/orz-i/Mender/backend/internal/invocation/infrastructure/jsonschema\"\n")
	violations, err := checkArchitecture(root)
	if err != nil || len(violations) != 6 {
		t.Fatalf("typed Workspace boundary: %v %v", violations, err)
	}
}

func TestArchitectureChecksPlatformFilesAndIgnoresTestFixtures(t *testing.T) {
	root := t.TempDir()
	writeArchitectureFixture(t, root, "backend/internal/example/application/leak_darwin.go", "package application\nimport _ \"database/sql\"\n")
	writeArchitectureFixture(t, root, "backend/internal/example/domain/source.go", "package domain\n// import \"os\" is only a comment\nimport \"fmt\"\n")
	writeArchitectureFixture(t, root, "backend/internal/example/application/source_test.go", "package application\nimport _ \"os\"\n")
	writeArchitectureFixture(t, root, "backend/internal/example/testdata/fixture.go", "not valid Go")
	violations, err := checkArchitecture(root)
	if err != nil || len(violations) != 1 || !strings.Contains(violations[0], "leak_darwin.go") {
		t.Fatalf("platform leak not isolated: violations=%v err=%v", violations, err)
	}
}

func TestArchitectureRejectsInfrastructureAndReverseApplicationEdges(t *testing.T) {
	root := t.TempDir()
	writeArchitectureFixture(t, root, "backend/internal/management/application/query/leak.go", "package query\nimport _ \"github.com/orz-i/Mender/backend/internal/catalog/infrastructure/sqlite\"\n")
	writeArchitectureFixture(t, root, "backend/internal/catalog/domain/leak.go", "package domain\nimport _ \"github.com/orz-i/Mender/backend/internal/catalog/application\"\n")
	writeArchitectureFixture(t, root, "backend/internal/providerplugin/leak.go", "package providerplugin\nimport _ \"github.com/orz-i/Mender/backend/internal/providerplugin/application\"\n")
	writeArchitectureFixture(t, root, "backend/internal/providerplugin/application/ports.go", "package application\nimport _ \"github.com/orz-i/Mender/backend/internal/catalog/domain\"\n")
	violations, err := checkArchitecture(root)
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
	violations, err := checkArchitecture(root)
	if err != nil || len(violations) != 5 {
		t.Fatalf("filesystem mechanisms not isolated from lexical/port operations: %v %v", violations, err)
	}
	for _, expected := range []string{"filepath.EvalSymlinks", "filepath.WalkDir", "filepath.Walk", "filepath.Abs", "dot-imports path/filepath"} {
		if !strings.Contains(strings.Join(violations, "\n"), expected) {
			t.Fatalf("missing %s violation: %v", expected, violations)
		}
	}
}

func TestArchitectureSeparatesProfileSelectionFromDeliveryImports(t *testing.T) {
	root := t.TempDir()
	for file, source := range map[string]string{
		"backend/internal/profile/domain/leak_windows.go":             "package domain\nimport channel \"github.com/orz-i/Mender/backend/internal/mcp/domain\"\n",
		"backend/internal/profile/application/delivery/leak_linux.go": "package delivery\nimport _ \"github.com/orz-i/Mender/backend/internal/tunnel/domain\"\n",
		"backend/internal/profile/application/leak.go":                "package application\nimport _ \"github.com/orz-i/Mender/backend/internal/mcp/application/delivery\"\n",
		"backend/internal/profile/infrastructure/sqlite/migration.go": "package sqlite\nimport _ \"github.com/orz-i/Mender/backend/internal/mcp/domain\"\n",
		"backend/internal/mcp/domain/config.go":                       "package domain\nimport _ \"github.com/orz-i/Mender/backend/internal/profile/domain\"\n",
	} {
		writeArchitectureFixture(t, root, file, source)
	}
	violations, err := checkArchitecture(root)
	if err != nil || len(violations) != 3 {
		t.Fatalf("Profile direction or migration scope incorrect: %v %v", violations, err)
	}
	for _, violation := range violations {
		if !strings.Contains(violation, "Profile selection/delivery boundary") {
			t.Fatalf("wrong boundary: %s", violation)
		}
	}
}

func TestArchitectureRejectsLegacyProfileAuthorityWithPreciseASTScope(t *testing.T) {
	root := t.TempDir()
	writeArchitectureFixture(t, root, "backend/internal/profile/domain/legacy.go", `package domain
type Profile struct { mode string; tunnelID string }
type ToolBinding struct { direct bool }
type ToolBindingSpec struct { Direct bool }
type ExposureMode string
type profileAlias = Profile
func (profileAlias) Mode() string { return "" }
func (*Profile) TunnelID() string { return "" }
func (Profile) DirectToolIDs() []string { return nil }
func (ToolBinding) Direct() bool { return false }
func (ToolBinding) withDirect(bool) ToolBinding { return ToolBinding{} }
`)
	writeArchitectureFixture(t, root, "backend/internal/profile/domain/legal.go", `package domain
type PolicyConstraint string
func (PolicyConstraint) Mode() string { return "inherit" }
// type Profile struct { mode string } is historical prose, not authority.
`)
	writeArchitectureFixture(t, root, "backend/internal/mcp/domain/config.go", "package domain\ntype Config struct { mode string }\nfunc (Config) Mode() string { return \"direct\" }\n")
	violations, err := checkArchitecture(root)
	if err != nil || len(violations) != 10 {
		t.Fatalf("legacy Profile authority not isolated from legal Mode methods: %v %v", violations, err)
	}
	for _, violation := range violations {
		if !strings.Contains(violation, "profile/domain/legacy.go") {
			t.Fatalf("legal scope rejected: %s", violation)
		}
	}
}

func TestArchitectureKeepsManagedMCPOnCallbacksWithoutRejectingHTTPTypes(t *testing.T) {
	root := t.TempDir()
	for i, imported := range []string{
		internalPrefix + "bootstrap", internalPrefix + "cli", internalPrefix + "desktop/binding",
		internalPrefix + "management/httpapi", internalPrefix + "profile/infrastructure/sqlite",
		internalPrefix + "mcp/infrastructure/sqlite", internalPrefix + "catalog/infrastructure/sqlite/query",
		"database/sql", "modernc.org/sqlite",
	} {
		file := "backend/internal/mcp/infrastructure/managed/leak_" + string(rune('a'+i)) + "_darwin.go"
		writeArchitectureFixture(t, root, file, "package managed\nimport adapter \""+imported+"\"\n")
	}
	writeArchitectureFixture(t, root, "backend/internal/mcp/infrastructure/managed/ports.go", `package managed
import web "net/http"
import channel "github.com/orz-i/Mender/backend/internal/mcp/domain"
import selection "github.com/orz-i/Mender/backend/internal/profile/domain"
type Dependencies struct { Build func(selection.Profile,channel.Config) web.Handler }
`)
	violations, err := checkArchitecture(root)
	if err != nil || len(violations) != 9 {
		t.Fatalf("managed callback boundary: %v %v", violations, err)
	}
	for _, violation := range violations {
		if strings.Contains(violation, "ports.go") {
			t.Fatalf("legal HTTP Handler dependency rejected: %s", violation)
		}
	}
}

func TestArchitectureRejectsProfileManagedBootstrapStateButAllowsTaskProxy(t *testing.T) {
	root := t.TempDir()
	writeArchitectureFixture(t, root, "backend/internal/bootstrap/mcp_servers_linux.go", `package bootstrap
import n "net"
import web "net/http"
var servers = make(map[string]any)
type listenerState struct { listener n.Listener; server *web.Server }
var listen = n.Listen
func activateProviderSet() { ids := make(map[string]struct{}); _ = ids }
func callback(n interface{ Listen() }) { n.Listen() }
type Builder func() web.Handler
`)
	writeArchitectureFixture(t, root, "backend/internal/bootstrap/runtime.go", `package bootstrap
type Runtime struct { mcpServers map[string]any; mcpServersMu any; managedMCP any; agentServers map[string]any }
`)
	writeArchitectureFixture(t, root, "backend/internal/bootstrap/old.go", "package bootstrap\ntype managedMCPServer struct{}\n")
	writeArchitectureFixture(t, root, "backend/internal/bootstrap/agent_task_tool_mcp_windows.go", `package bootstrap
import "net"
import "net/http"
type taskScopedMCPServer struct { listener net.Listener; server *http.Server }
var taskProxies = make(map[string]*taskScopedMCPServer)
var taskListen = net.Listen
`)
	violations, err := checkArchitecture(root)
	if err != nil || len(violations) != 7 {
		t.Fatalf("Profile listener ownership scope: %v %v", violations, err)
	}
	for _, violation := range violations {
		if strings.Contains(violation, "agent_task_tool_mcp") {
			t.Fatalf("Agent Task proxy was folded into Profile managed scope: %s", violation)
		}
	}
}

func TestArchitectureRejectsDotImportedProfileListenerMechanism(t *testing.T) {
	root := t.TempDir()
	writeArchitectureFixture(t, root, "backend/internal/bootstrap/mcp_servers_windows.go", "package bootstrap\nimport . \"net\"\nimport . \"net/http\"\n")
	violations, err := checkArchitecture(root)
	if err != nil || len(violations) != 2 {
		t.Fatalf("dot-import listener escape: %v %v", violations, err)
	}
}
