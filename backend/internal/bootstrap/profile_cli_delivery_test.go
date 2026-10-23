package bootstrap

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

func TestProfileCLIRecipesPinLocalDatabaseAndNeverFallBackToCatalogScope(t *testing.T) {
	r, ctx, root, p := deliveryFixture(t)
	instructions, err := r.LocalProfileCLI(ctx, p.Slug())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "test.db")
	if instructions.DatabasePath != path || instructions.ProfileSlug != p.Slug().String() {
		t.Fatalf("local CLI target = %+v", instructions)
	}
	for label, got := range map[string][]string{
		"search":   instructions.Search,
		"describe": instructions.Describe,
		"call":     instructions.Call,
		"stdio":    instructions.MCPStdio,
	} {
		if len(got) < 7 || !reflect.DeepEqual(got[:3], []string{"mender", "--db", path}) {
			t.Fatalf("%s lacks exact local DB selection: %v", label, got)
		}
		if label == "stdio" {
			if !reflect.DeepEqual(got[3:], []string{
				"serve", "mcp", "--profile", p.Slug().String(), "--stdio",
			}) {
				t.Fatalf("stdio lacks mandatory Profile: %v", got)
			}
			continue
		}
		if !reflect.DeepEqual(got[3:6], []string{"profile", "tool", label}) ||
			got[6] != p.Slug().String() {
			t.Fatalf("%s is not a strict Profile-only CLI command: %v", label, got)
		}
	}
	unknown, _ := profiledomain.NewSlug("not-here")
	if _, err := r.LocalProfileCLI(context.Background(), unknown); err == nil {
		t.Fatal("CLI instructions generated for unknown Profile")
	}
}
