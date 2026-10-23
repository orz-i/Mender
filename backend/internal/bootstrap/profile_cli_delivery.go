package bootstrap

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"

	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

// LocalProfileCLI is a local-host command recipe, not an installed process,
// capability grant, Agent Loop, or remote service. Token arrays avoid shell
// concatenation and let each client quote for its own environment.
type LocalProfileCLI struct {
	ProfileSlug  string   `json:"profileSlug"`
	DatabasePath string   `json:"databasePath"`
	Platform     string   `json:"platform"`
	Search       []string `json:"search"`
	Describe     []string `json:"describe"`
	Call         []string `json:"call"`
	MCPStdio     []string `json:"mcpStdio"`
}

func (r *Runtime) LocalProfileCLI(ctx context.Context, slug profiledomain.Slug) (LocalProfileCLI, error) {
	profile, err := r.Profiles.Get.Execute(ctx, slug)
	if err != nil {
		return LocalProfileCLI{}, fmt.Errorf("profile CLI delivery: %w", err)
	}
	if r.providerDatabasePath == "" {
		return LocalProfileCLI{}, fmt.Errorf("profile CLI delivery: local database path unavailable")
	}
	dbPath, err := filepath.Abs(r.providerDatabasePath)
	if err != nil {
		return LocalProfileCLI{}, fmt.Errorf("profile CLI delivery: local database path: %w", err)
	}
	base := []string{"mender", "--db", dbPath}
	profileSlug := profile.Slug().String()
	return LocalProfileCLI{
		ProfileSlug: profileSlug, DatabasePath: dbPath,
		Platform: runtime.GOOS,
		Search:   append(append([]string(nil), base...), "profile", "tool", "search", profileSlug, "<query>"),
		Describe: append(append([]string(nil), base...), "profile", "tool", "describe", profileSlug, "<tool-id>"),
		Call:     append(append([]string(nil), base...), "profile", "tool", "call", profileSlug, "<tool-id>", "--args", "{}"),
		MCPStdio: append(append([]string(nil), base...), "serve", "mcp", "--profile", profileSlug, "--stdio"),
	}, nil
}
