package delivery

import (
	"context"
	"fmt"

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

func (s *Service) LocalProfileCLI(ctx context.Context, slug profiledomain.Slug) (LocalProfileCLI, error) {
	profile, err := s.profiles.Snapshot(ctx, slug)
	if err != nil {
		return LocalProfileCLI{}, fmt.Errorf("profile CLI delivery: %w", err)
	}
	host, err := s.environment.LocalContext(ctx)
	if err != nil {
		return LocalProfileCLI{}, fmt.Errorf("profile CLI delivery: %w", err)
	}
	dbPath := host.DatabasePath
	base := []string{"mender", "--db", dbPath}
	profileSlug := profile.Slug.String()
	return LocalProfileCLI{
		ProfileSlug: profileSlug, DatabasePath: dbPath,
		Platform: host.Platform,
		Search:   append(append([]string(nil), base...), "profile", "tool", "search", profileSlug, "<query>"),
		Describe: append(append([]string(nil), base...), "profile", "tool", "describe", profileSlug, "<tool-id>"),
		Call:     append(append([]string(nil), base...), "profile", "tool", "call", profileSlug, "<tool-id>", "--args", "{}"),
		MCPStdio: append(append([]string(nil), base...), "serve", "mcp", "--profile", profileSlug, "--stdio"),
	}, nil
}
