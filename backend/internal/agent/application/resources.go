package application

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

// ResourceService manages owner-selected Agent resources, defaults and grants.
// It never admits a Task or participates in a native permission handoff.
type ResourceService struct {
	*agentAuthority
	*localWorkspaceRules
	store         ResourceRepository
	inspector     ModelInspector
	profileExists ProfileExists
}

type CreateRequest struct {
	Name                   string              `json:"name"`
	Backend                agentdomain.Backend `json:"backend"`
	WorkspaceRoot          string              `json:"workspaceRoot"`
	Isolation              string              `json:"isolation"`
	DefaultModel           string              `json:"defaultModel,omitempty"`
	DefaultReasoningEffort string              `json:"defaultReasoningEffort,omitempty"`
}

// Create stores one owner-selected backend and canonical local directory;
// unlike status listing, this may inspect the filesystem after owner admission.
func (s *ResourceService) Create(ctx context.Context, request CreateRequest, caller gov.Context) (agentdomain.Agent, error) {
	if err := s.requireOwner(caller); err != nil {
		return agentdomain.Agent{}, err
	}
	if request.Backend.PluginID() == "" {
		return agentdomain.Agent{}, agentdomain.ErrInvalid
	}
	if err := agentdomain.ValidateModel(request.DefaultModel); err != nil {
		return agentdomain.Agent{}, err
	}
	if err := agentdomain.ValidateReasoningEffort(request.DefaultReasoningEffort); err != nil {
		return agentdomain.Agent{}, err
	}
	if !filepath.IsAbs(request.WorkspaceRoot) {
		return agentdomain.Agent{}, agentdomain.ErrInvalid
	}
	resolved, err := s.workspace.ResolvePath(ctx, request.WorkspaceRoot)
	if err != nil {
		return agentdomain.Agent{}, fmt.Errorf("%w: Agent workspace unavailable: %w", agentdomain.ErrInvalid, err)
	}
	directory, err := s.workspace.IsDirectory(ctx, resolved)
	if err != nil {
		return agentdomain.Agent{}, fmt.Errorf("%w: Agent workspace stat: %w", agentdomain.ErrInvalid, err)
	}
	if !directory {
		return agentdomain.Agent{}, agentdomain.ErrInvalid
	}
	id, err := agentdomain.NewID()
	if err != nil {
		return agentdomain.Agent{}, err
	}
	isolation := request.Isolation
	if isolation == "" {
		isolation = "restricted_host"
	}
	agent, err := agentdomain.NewAgent(agentdomain.Spec{ID: id, Name: request.Name, Backend: request.Backend,
		PluginID: request.Backend.PluginID(), ExtensionID: "main", WorkspaceRoot: filepath.Clean(resolved),
		Isolation: isolation, DefaultModel: request.DefaultModel, DefaultReasoningEffort: request.DefaultReasoningEffort, OwnerKind: string(s.owner.Kind()), OwnerRef: s.owner.Reference(),
		CreatedAt: time.Now().UTC()})
	if err != nil {
		return agentdomain.Agent{}, err
	}
	if err := s.store.CreateAgent(ctx, agent); err != nil {
		return agentdomain.Agent{}, err
	}
	return agent, nil
}
func (s *ResourceService) List(ctx context.Context, caller gov.Context) ([]agentdomain.Agent, error) {
	if err := s.requireOwner(caller); err != nil {
		return nil, err
	}
	return s.store.ListAgents(ctx)
}
func (s *ResourceService) Get(ctx context.Context, id agentdomain.ID, caller gov.Context) (agentdomain.Agent, error) {
	if err := s.requireOwner(caller); err != nil {
		return agentdomain.Agent{}, err
	}
	return s.store.GetAgent(ctx, id)
}
func (s *ResourceService) Delete(ctx context.Context, id agentdomain.ID, caller gov.Context) error {
	if err := s.requireOwner(caller); err != nil {
		return err
	}
	return s.store.DeleteAgent(ctx, id)
}

func (s *ResourceService) Grant(ctx context.Context, id agentdomain.ID, profileID string, caller gov.Context) error {
	if err := s.requireOwner(caller); err != nil {
		return err
	}
	if s.profileExists == nil {
		return agentdomain.ErrUnavailable
	}
	if _, err := profiledomain.ParseProfileID(profileID); err != nil {
		return agentdomain.ErrInvalid
	}
	a, err := s.store.GetAgent(ctx, id)
	if err != nil {
		return err
	}
	if !a.OwnedBy(string(s.owner.Kind()), s.owner.Reference()) {
		return agentdomain.ErrForbidden
	}
	if err := s.profileExists(ctx, profileID); err != nil {
		return fmt.Errorf("agent Profile grant: %w", err)
	}
	return s.store.GrantProfile(ctx, id, profileID)
}
func (s *ResourceService) Revoke(ctx context.Context, id agentdomain.ID, profileID string, caller gov.Context) error {
	if err := s.requireOwner(caller); err != nil {
		return err
	}
	if _, err := profiledomain.ParseProfileID(profileID); err != nil {
		return agentdomain.ErrInvalid
	}
	if _, err := s.store.GetAgent(ctx, id); err != nil {
		return err
	}
	return s.store.RevokeProfile(ctx, id, profileID)
}
func (s *ResourceService) Grants(ctx context.Context, id agentdomain.ID, caller gov.Context) ([]string, error) {
	if err := s.requireOwner(caller); err != nil {
		return nil, err
	}
	if _, err := s.store.GetAgent(ctx, id); err != nil {
		return nil, err
	}
	return s.store.ListProfileGrants(ctx, id)
}

// Admissions is an owner-only audit view, not an MCP task capability.
func (s *ResourceService) Admissions(ctx context.Context, id agentdomain.ID, caller gov.Context, limit int) ([]agentdomain.AdmissionEvent, error) {
	if err := s.requireOwner(caller); err != nil {
		return nil, err
	}
	if _, err := s.store.GetAgent(ctx, id); err != nil {
		return nil, err
	}
	return s.store.ListAdmissions(ctx, id, limit)
}
