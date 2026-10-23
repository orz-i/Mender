package agentplugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	skillapplication "github.com/orz-i/Mender/backend/internal/skill/application"
	"github.com/orz-i/Mender/backend/internal/skill/infrastructure/agentskill"
)

const schemaID = "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"

var pluginNamePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]{0,62}[a-z0-9])?$`)

// Materializer copies one exact Skill installation to a new destination.
type Materializer interface {
	MaterializeSkill(
		context.Context,
		skillapplication.AgentPluginSkill,
		string,
	) error
}

// Exporter writes deterministic Agent Plugins 1.0 Skill and/or HTTPS MCP packages.
type Exporter struct {
	materializer Materializer
}

// New constructs an Agent Plugin exporter around an exact Skill materializer.
func New(materializer Materializer) *Exporter {
	return &Exporter{materializer: materializer}
}

type manifest struct {
	Schema      string `json:"$schema"`
	Name        string `json:"name"`
	Version     string `json:"version,omitempty"`
	Description string `json:"description,omitempty"`
}

// ExportAgentPlugin writes one portable Agent Plugin package.
//
// The target directory must not already exist. The package is prepared in a
// sibling staging directory then renamed into place.
func (e *Exporter) ExportAgentPlugin(
	ctx context.Context,
	pkg skillapplication.AgentPluginPackage,
) error {
	if e == nil || e.materializer == nil {
		return fmt.Errorf("%w: materializer is required", skillapplication.ErrInvalidAgentPluginPackage)
	}
	if err := validatePluginName(pkg.Name); err != nil {
		return err
	}
	if len(pkg.Skills) == 0 && pkg.MCP == nil {
		return fmt.Errorf("%w: at least one Skill or MCP connection is required", skillapplication.ErrInvalidAgentPluginPackage)
	}
	if pkg.MCP != nil {
		if err := validateRemoteMCPURL(pkg.MCP.URL); err != nil {
			return err
		}
	}

	destination := strings.TrimSpace(pkg.Destination)
	if destination == "" {
		return fmt.Errorf(
			"%w: destination must not be empty",
			skillapplication.ErrInvalidAgentPluginPackage,
		)
	}
	absolute, err := filepath.Abs(destination)
	if err != nil {
		return fmt.Errorf("resolve Agent Plugin destination: %w", err)
	}
	parent := filepath.Dir(absolute)
	parentInfo, err := os.Lstat(parent)
	if err != nil {
		return fmt.Errorf("inspect Agent Plugin parent: %w", err)
	}
	if parentInfo.Mode()&os.ModeSymlink != 0 || !parentInfo.IsDir() {
		return fmt.Errorf(
			"%w: destination parent must be a real directory",
			skillapplication.ErrInvalidAgentPluginPackage,
		)
	}
	if _, err := os.Lstat(absolute); err == nil {
		return fmt.Errorf(
			"%w: %s",
			skillapplication.ErrAgentPluginTargetExists,
			absolute,
		)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect Agent Plugin destination: %w", err)
	}

	staging, err := os.MkdirTemp(parent, ".mender-agent-plugin-")
	if err != nil {
		return fmt.Errorf("create Agent Plugin staging directory: %w", err)
	}
	published := false
	defer func() {
		if !published {
			_ = os.RemoveAll(staging)
		}
	}()

	rawManifest, err := json.MarshalIndent(manifest{
		Schema:      schemaID,
		Name:        pkg.Name,
		Version:     pkg.Version,
		Description: pkg.Description,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode Agent Plugin manifest: %w", err)
	}
	rawManifest = append(rawManifest, '\n')
	if err := os.WriteFile(filepath.Join(staging, "plugin.json"), rawManifest, 0o600); err != nil {
		return fmt.Errorf("write Agent Plugin manifest: %w", err)
	}

	skillsRoot := filepath.Join(staging, "skills")
	if len(pkg.Skills) > 0 {
		if err := os.Mkdir(skillsRoot, 0o700); err != nil {
			return fmt.Errorf("create Agent Plugin skills directory: %w", err)
		}
	}
	seenNames := make(map[string]struct{}, len(pkg.Skills))
	for _, skill := range pkg.Skills {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, exists := seenNames[skill.Name]; exists {
			return fmt.Errorf(
				"%w: duplicate Skill name %q",
				skillapplication.ErrInvalidAgentPluginPackage,
				skill.Name,
			)
		}
		seenNames[skill.Name] = struct{}{}
		target := filepath.Join(skillsRoot, skill.Name)
		if err := e.materializer.MaterializeSkill(ctx, skill, target); err != nil {
			return fmt.Errorf(
				"materialize Agent Plugin Skill %s: %w",
				skill.InstallationID,
				err,
			)
		}
		snapshot, err := agentskill.LoadDirectory(target)
		if err != nil {
			return fmt.Errorf(
				"verify Agent Plugin Skill %s: %w",
				skill.InstallationID,
				err,
			)
		}
		if snapshot.Metadata.Name != skill.Name || snapshot.Digest != skill.Digest {
			return fmt.Errorf(
				"%w: materialized Skill %s does not match exact installation",
				skillapplication.ErrInvalidAgentPluginPackage,
				skill.InstallationID,
			)
		}
	}

	if pkg.MCP != nil {
		config := struct {
			Schema     string                     `json:"$schema"`
			MCPServers map[string]remoteMCPServer `json:"mcpServers"`
		}{
			Schema: "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json",
			MCPServers: map[string]remoteMCPServer{
				"mender": {Type: "streamable-http", URL: pkg.MCP.URL},
			},
		}
		rawMCP, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			return fmt.Errorf("encode Agent Plugin MCP: %w", err)
		}
		if err := os.WriteFile(filepath.Join(staging, "mcp.json"), append(rawMCP, byte(10)), 0o600); err != nil {
			return fmt.Errorf("write Agent Plugin MCP: %w", err)
		}
	}

	if _, err := os.Lstat(absolute); err == nil {
		return fmt.Errorf(
			"%w: %s",
			skillapplication.ErrAgentPluginTargetExists,
			absolute,
		)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("recheck Agent Plugin destination: %w", err)
	}
	if err := os.Rename(staging, absolute); err != nil {
		return fmt.Errorf("publish Agent Plugin package: %w", err)
	}
	published = true
	return nil
}

type remoteMCPServer struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

// validateRemoteMCPURL excludes local-only URLs and credentials from portable
// packages; authorization is always configured by the receiving client.
func validateRemoteMCPURL(raw string) error {
	if strings.TrimSpace(raw) != raw || len(raw) > 2048 {
		return fmt.Errorf("%w: invalid remote MCP URL", skillapplication.ErrInvalidAgentPluginPackage)
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil || !parsed.IsAbs() ||
		parsed.Scheme != "https" || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery ||
		parsed.Fragment != "" || parsed.RawFragment != "" {
		return fmt.Errorf("%w: MCP URL must be a credential-free HTTPS endpoint without query or fragment",
			skillapplication.ErrInvalidAgentPluginPackage)
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".localhost") ||
		strings.HasSuffix(host, ".local") {
		return fmt.Errorf("%w: local-only MCP endpoint is not portable", skillapplication.ErrInvalidAgentPluginPackage)
	}
	if ip := net.ParseIP(host); ip != nil &&
		(!ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLinkLocalUnicast()) {
		return fmt.Errorf("%w: private or local-only MCP endpoint is not portable",
			skillapplication.ErrInvalidAgentPluginPackage)
	}
	return nil
}

func validatePluginName(name string) error {
	if len(name) < 1 ||
		len(name) > 64 ||
		!pluginNamePattern.MatchString(name) ||
		strings.Contains(name, "--") ||
		strings.Contains(name, "..") {
		return fmt.Errorf(
			"%w: plugin name must be 1-64 lowercase letters, digits, hyphens, or periods; begin and end alphanumeric; repeated hyphen or period segments are not allowed",
			skillapplication.ErrInvalidAgentPluginPackage,
		)
	}
	return nil
}
