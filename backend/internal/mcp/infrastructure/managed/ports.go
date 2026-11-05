// Package managed owns local MCP listeners, tunnel processes and atomically
// refreshed HTTP projections. Composition supplies authority and durable reads.
package managed

import (
	"context"
	"errors"
	"net/http"
	"sync"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	mcpdomain "github.com/orz-i/Mender/backend/internal/mcp/domain"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	tunnelapplication "github.com/orz-i/Mender/backend/internal/tunnel/application"
	tunneldomain "github.com/orz-i/Mender/backend/internal/tunnel/domain"
)

type Snapshot struct {
	Profile profiledomain.Profile
	Config  mcpdomain.Config
}

func (s Snapshot) ID() profiledomain.ProfileID             { return s.Profile.ID() }
func (s Snapshot) Slug() profiledomain.Slug                { return s.Profile.Slug() }
func (s Snapshot) Revision() uint64                        { return s.Profile.Revision() }
func (s Snapshot) ToolIDs() []catalogdomain.ToolID         { return s.Profile.ToolIDs() }
func (s Snapshot) TunnelID() (tunneldomain.TunnelID, bool) { return s.Config.TunnelID() }
func (s Snapshot) validate() error {
	if s.Profile.ID() != s.Config.ProfileID() {
		return errors.New("managed MCP delivery does not match Profile identity")
	}
	tunnelID, _ := s.Config.TunnelID()
	_, err := mcpdomain.NewConfig(s.Config.ProfileID(), s.Config.Mode(), tunnelID, s.Config.DirectToolIDs())
	return err
}

type Versions struct {
	Profile  uint64
	Delivery uint64
}

type Dependencies struct {
	LoadBySlug      func(context.Context, profiledomain.Slug) (Snapshot, error)
	LoadByID        func(context.Context, profiledomain.ProfileID) (Snapshot, error)
	Revisions       func(context.Context, profiledomain.ProfileID) (Versions, error)
	BuildHTTP       func(context.Context, Snapshot, governancedomain.Context) (http.Handler, error)
	DescribeTool    func(context.Context, catalogdomain.ToolID) (catalogdomain.Tool, error)
	CatalogRevision func(context.Context) (int64, error)
	StartTunnel     func(context.Context, tunneldomain.TunnelID, string) (tunnelapplication.Process, tunneldomain.Tunnel, error)
}

type Manager struct {
	deps    Dependencies
	mu      sync.Mutex
	servers map[string]*runningServer
}

func New(deps Dependencies) (*Manager, error) {
	if deps.LoadBySlug == nil || deps.LoadByID == nil || deps.Revisions == nil ||
		deps.BuildHTTP == nil || deps.DescribeTool == nil || deps.CatalogRevision == nil || deps.StartTunnel == nil {
		return nil, errors.New("managed MCP requires all read/build/tunnel dependencies")
	}
	return &Manager{deps: deps, servers: make(map[string]*runningServer)}, nil
}

func (r *Manager) LoadSnapshot(ctx context.Context, id profiledomain.ProfileID) (Snapshot, error) {
	snapshot, err := r.deps.LoadByID(ctx, id)
	if err != nil {
		return Snapshot{}, err
	}
	if err := snapshot.validate(); err != nil {
		return Snapshot{}, err
	}
	if snapshot.ID() != id {
		return Snapshot{}, errors.New("managed MCP Profile lookup identity changed")
	}
	if err := r.ConfirmSnapshot(ctx, snapshot); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func (r *Manager) ConfirmSnapshot(ctx context.Context, snapshot Snapshot) error {
	versions, err := r.deps.Revisions(ctx, snapshot.ID())
	if err != nil {
		return err
	}
	if versions.Profile != snapshot.Profile.Revision() || versions.Delivery != snapshot.Config.Revision() {
		return errors.New("managed MCP Profile or Delivery changed while building projection")
	}
	return nil
}
