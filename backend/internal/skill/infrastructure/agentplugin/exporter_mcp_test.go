package agentplugin

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	skillapplication "github.com/orz-i/Mender/backend/internal/skill/application"
)

func TestAgentPluginExporterSupportsExactSkillsAndRemoteHTTPSMCP(t *testing.T) {
	t.Parallel()
	id := mustInstallationID(t, "sinst_00112233445566778899aabbccddeeff")
	fixture := newFixture(t, "docs-review")
	exporter := New(fixtureMaterializer{id: fixture})
	for _, tc := range []struct {
		name     string
		hasSkill bool
		hasMCP   bool
	}{
		{name: "skills-only", hasSkill: true},
		{name: "mcp-only", hasMCP: true},
		{name: "combined", hasSkill: true, hasMCP: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parent := t.TempDir()
			destination := filepath.Join(parent, "output")
			request := skillapplication.AgentPluginPackage{
				Name: "mender.work", Version: "1.2.3", Destination: destination,
			}
			if tc.hasSkill {
				request.Skills = []skillapplication.AgentPluginSkill{{
					InstallationID: id, Name: fixture.name, Digest: fixture.digest,
				}}
			}
			if tc.hasMCP {
				request.MCP = &skillapplication.AgentPluginMCP{URL: "https://mcp.example.com/mcp"}
			}
			if err := exporter.ExportAgentPlugin(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			var manifest map[string]any
			if err := json.Unmarshal(readRootFile(t, destination, "plugin.json"), &manifest); err != nil {
				t.Fatal(err)
			}
			if manifest["$schema"] != "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json" ||
				manifest["name"] != "mender.work" {
				t.Fatalf("unexpected manifest = %#v", manifest)
			}
			var mcp struct {
				Schema  string `json:"$schema"`
				Servers map[string]struct {
					Type string `json:"type"`
					URL  string `json:"url"`
				} `json:"mcpServers"`
			}
			//nolint:gosec // Destination is a controlled TempDir under this test.
			rawMCP, mcpErr := os.ReadFile(filepath.Join(destination, "mcp.json"))
			if tc.hasMCP {
				if mcpErr != nil {
					t.Fatal(mcpErr)
				}
				if err := json.Unmarshal(rawMCP, &mcp); err != nil {
					t.Fatal(err)
				}
				if mcp.Schema != "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json" ||
					len(mcp.Servers) != 1 ||
					mcp.Servers["mender"].Type != "streamable-http" ||
					mcp.Servers["mender"].URL != "https://mcp.example.com/mcp" {
					t.Fatalf("unexpected MCP schema: %+v", mcp)
				}
				if strings.Contains(string(rawMCP), "Authorization") ||
					strings.Contains(string(rawMCP), "secret") {
					t.Fatalf("MCP config echoed a credential: %s", rawMCP)
				}
			} else if !errors.Is(mcpErr, os.ErrNotExist) {
				t.Fatalf("unexpected mcp.json: %v", mcpErr)
			}
			_, skillsErr := os.Stat(filepath.Join(destination, "skills"))
			if tc.hasSkill && skillsErr != nil {
				t.Fatal(skillsErr)
			}
			if !tc.hasSkill && !errors.Is(skillsErr, os.ErrNotExist) {
				t.Fatalf("MCP-only package included skills: %v", skillsErr)
			}
		})
	}
}

func TestAgentPluginExporterRejectsUnsafeRemoteMCPBeforeWriting(t *testing.T) {
	t.Parallel()
	exporter := New(fixtureMaterializer{})
	for _, unsafeURL := range []string{
		"", "http://public.example.com/mcp", "https://localhost:8443/mcp",
		"https://127.0.0.1/mcp", "https://[::1]/mcp",
		"https://10.2.3.4/mcp", "https://192.168.1.8/mcp",
		"https://[fd00::1]/mcp", "https://[fe80::1]/mcp",
		"https://private.local/mcp",
		"https://admin:secret@mcp.example.com/mcp",
		"https://mcp.example.com/mcp?token=secret",
		"https://mcp.example.com/mcp#token",
		"file:///private/data",
		" https://mcp.example.com/mcp",
	} {
		t.Run(unsafeURL, func(t *testing.T) {
			parent := t.TempDir()
			destination := filepath.Join(parent, "plugin")
			err := exporter.ExportAgentPlugin(context.Background(), skillapplication.AgentPluginPackage{
				Name: "safe-name", Destination: destination,
				MCP: &skillapplication.AgentPluginMCP{URL: unsafeURL},
			})
			if !errors.Is(err, skillapplication.ErrInvalidAgentPluginPackage) {
				t.Fatalf("invalid url %q: %v", unsafeURL, err)
			}
			if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("invalid URL wrote destination: %v", err)
			}
		})
	}
}
