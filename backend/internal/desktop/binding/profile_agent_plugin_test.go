package binding

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
)

func TestDesktopProfileCLIDeliveryAndExactAgentPluginExport(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	r, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: filepath.Join(root, "mender.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	service := NewService(ctx, r)
	profile, err := service.CreateProfile("docs-delivery", "Docs Delivery")
	if err != nil {
		t.Fatal(err)
	}
	cli, err := service.GetProfileCLIInstructions(profile.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if cli.DatabasePath != filepath.Join(root, "mender.db") ||
		cli.ProfileSlug != profile.Slug || len(cli.Call) == 0 || len(cli.MCPStdio) == 0 {
		t.Fatalf("Desktop CLI instructions = %+v", cli)
	}
	installed, err := r.Skills.List(ctx)
	if err != nil || len(installed) == 0 {
		t.Fatalf("Skills = %v %v", installed, err)
	}
	chosen, err := service.ReplaceProfileSkills(profile.Slug, []string{installed[0].InstallationID.String()})
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.ExportProfileAgentPlugin(profile.Slug, "docs.bundle", "1.0.0",
		"Docs workflow", root, "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Destination != filepath.Join(root, "docs.bundle") ||
		result.ProfileRevision != chosen.Revision || result.HasMCP {
		t.Fatalf("exported Profile Agent Plugin = %+v", result)
	}
	if _, err := os.Stat(filepath.Join(result.Destination, "plugin.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(result.Destination, "skills", installed[0].Name, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ExportProfileAgentPlugin(profile.Slug, "../bad", "", "", root, ""); err == nil {
		t.Fatal("unsafe package name accepted")
	}
	if _, err := service.ExportProfileAgentPlugin(profile.Slug, "docs.bad", "", "", "", ""); err == nil {
		t.Fatal("missing owner selected parent accepted")
	}
	if _, err := service.ExportProfileAgentPlugin(profile.Slug, "docs.localhost", "", "",
		root, "http://127.0.0.1:8888/mcp"); err == nil {
		t.Fatal("unsafe remote endpoint accepted")
	}
}
