package domain

import (
	"fmt"
	"slices"

	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	profile "github.com/orz-i/Mender/backend/internal/profile/domain"
	tunnel "github.com/orz-i/Mender/backend/internal/tunnel/domain"
)

type Mode string

const (
	ModeDirect  Mode = "direct"
	ModeDynamic Mode = "dynamic"
	ModeHybrid  Mode = "hybrid"
)

// Config owns desired MCP delivery settings, independently of the selected
// Profile's membership revision and of any running listener or Tunnel process.
type Config struct {
	profileID profile.ProfileID
	mode      Mode
	tunnelID  tunnel.TunnelID
	direct    []catalog.ToolID
	revision  uint64
}

func NewConfig(profileID profile.ProfileID, mode Mode, tunnelID tunnel.TunnelID, direct []catalog.ToolID) (Config, error) {
	if _, err := profile.ParseProfileID(profileID.String()); err != nil {
		return Config{}, fmt.Errorf("MCP delivery Profile: %w", err)
	}
	if mode != ModeDirect && mode != ModeDynamic && mode != ModeHybrid {
		return Config{}, fmt.Errorf("MCP delivery: unknown mode %q", mode)
	}
	if tunnelID != "" {
		if _, err := tunnel.ParseTunnelID(tunnelID.String()); err != nil {
			return Config{}, fmt.Errorf("MCP delivery Tunnel: %w", err)
		}
	}
	selected := slices.Clone(direct)
	slices.Sort(selected)
	for i, id := range selected {
		if _, err := catalog.ParseToolID(id.String()); err != nil {
			return Config{}, fmt.Errorf("MCP direct Tool: %w", err)
		}
		if i > 0 && selected[i-1] == id {
			return Config{}, fmt.Errorf("MCP delivery: duplicate direct Tool %s", id)
		}
	}
	// Direct mode derives all current Profile members; dynamic mode derives
	// none. Only hybrid mode has an explicitly configured direct subset.
	if mode != ModeHybrid {
		selected = nil
	}
	return Config{profileID: profileID, mode: mode, tunnelID: tunnelID, direct: selected}, nil
}

func (c Config) ProfileID() profile.ProfileID        { return c.profileID }
func (c Config) Mode() Mode                          { return c.mode }
func (c Config) TunnelID() (tunnel.TunnelID, bool)   { return c.tunnelID, c.tunnelID != "" }
func (c Config) DirectToolIDs() []catalog.ToolID     { return slices.Clone(c.direct) }
func (c Config) Revision() uint64                    { return c.revision }
func (c Config) WithRevision(revision uint64) Config { c.revision = revision; return c }
