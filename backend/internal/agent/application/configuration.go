package application

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
)

// ModelChoice is host-reviewed, bounded ACP v1 session metadata.
// It is not a catalog of models that Mender can invoke independently.
type ModelChoice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type OptionChoices struct {
	Current string        `json:"current"`
	Choices []ModelChoice `json:"choices"`
}
type ModelChoices struct {
	Current         string         `json:"current"`
	Models          []ModelChoice  `json:"models"`
	ReasoningEffort *OptionChoices `json:"reasoningEffort,omitempty"`
}
type ModelInspector interface {
	InspectModels(context.Context, agentdomain.Agent) (ModelChoices, error)
}

// InspectModels is an explicit owner action, never executed during Agent listing.
// A bounded ACP session is created solely to inspect advertised model options;
// no Task, prompt, credentials or Agent defaults are mutated.
func (s *Service) InspectModels(ctx context.Context, id agentdomain.ID, caller gov.Context) (ModelChoices, error) {
	if err := s.requireOwner(caller); err != nil {
		return ModelChoices{}, err
	}
	agent, err := s.store.GetAgent(ctx, id)
	if err != nil {
		return ModelChoices{}, err
	}
	if !agent.OwnedBy(string(s.owner.Kind()), s.owner.Reference()) {
		return ModelChoices{}, agentdomain.ErrForbidden
	}
	inspector, ok := s.runner.(ModelInspector)
	if !ok {
		return ModelChoices{}, agentdomain.ErrUnavailable
	}
	probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	choices, err := inspector.InspectModels(probeCtx, agent)
	if err != nil {
		return ModelChoices{}, fmt.Errorf("%w: model options unavailable", agentdomain.ErrUnavailable)
	}
	return choices, nil
}

// canonicalLocalWorkspace is an explicit owner-selected directory, not a
// Sandbox resource or a path provided by an MCP task.
func canonicalLocalWorkspace(root string) (string, error) {
	if !filepath.IsAbs(root) {
		return "", fmt.Errorf("%w: local workspace must be an absolute path", agentdomain.ErrInvalid)
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("%w: workspace path unavailable: %w", agentdomain.ErrInvalid, err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", fmt.Errorf("%w: workspace path unavailable: %w", agentdomain.ErrInvalid, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w: workspace is not a directory", agentdomain.ErrInvalid)
	}
	return filepath.Clean(canonical), nil
}

// validateRegisteredWorkspaceRoot rechecks the approved root before task
// admission. A replaced symlink cannot silently redirect an old Profile grant.
func validateRegisteredWorkspaceRoot(root string) error {
	canonical, err := canonicalLocalWorkspace(root)
	if err != nil {
		return agentdomain.ErrUnavailable
	}
	if canonical != root {
		return fmt.Errorf("%w: approved workspace root changed", agentdomain.ErrUnavailable)
	}
	return nil
}

// UpdateDefaults changes the model and reasoning effort together, so tasks
// always admit one coherent defaults snapshot without a two-step write.
func (s *Service) UpdateDefaults(ctx context.Context, id agentdomain.ID, model, effort string, caller gov.Context) (agentdomain.Agent, error) {
	if err := s.requireOwner(caller); err != nil {
		return agentdomain.Agent{}, err
	}
	if err := agentdomain.ValidateModel(model); err != nil {
		return agentdomain.Agent{}, err
	}
	if err := agentdomain.ValidateReasoningEffort(effort); err != nil {
		return agentdomain.Agent{}, err
	}
	a, err := s.store.GetAgent(ctx, id)
	if err != nil {
		return agentdomain.Agent{}, err
	}
	if !a.OwnedBy(string(s.owner.Kind()), s.owner.Reference()) {
		return agentdomain.Agent{}, agentdomain.ErrForbidden
	}
	if err := s.store.SetDefaults(ctx, id, model, effort); err != nil {
		return agentdomain.Agent{}, err
	}
	return s.store.GetAgent(ctx, id)
}

// BindWorkspace registers an existing local directory, without creating a
// Sandbox, Agent or Provider. Only the Agent owner may approve a root.
func (s *Service) BindWorkspace(ctx context.Context, id agentdomain.ID, key, workspaceRoot string, caller gov.Context) error {
	if err := s.requireOwner(caller); err != nil {
		return err
	}
	if err := agentdomain.ValidateWorkspaceKey(key); err != nil {
		return err
	}
	a, err := s.store.GetAgent(ctx, id)
	if err != nil {
		return err
	}
	if !a.OwnedBy(string(s.owner.Kind()), s.owner.Reference()) {
		return agentdomain.ErrForbidden
	}
	canonical, err := canonicalLocalWorkspace(workspaceRoot)
	if err != nil {
		return err
	}
	if canonical == a.Spec().WorkspaceRoot {
		return fmt.Errorf("%w: %w", agentdomain.ErrConflict, agentdomain.ErrWorkspaceDefaultDirectory)
	}
	return s.store.BindWorkspace(ctx, id, key, canonical)
}
func (s *Service) Workspaces(ctx context.Context, id agentdomain.ID, caller gov.Context) ([]agentdomain.WorkspaceBinding, error) {
	if err := s.requireOwner(caller); err != nil {
		return nil, err
	}
	if _, err := s.store.GetAgent(ctx, id); err != nil {
		return nil, err
	}
	return s.store.ListWorkspaces(ctx, id)
}
func (s *Service) UnbindWorkspace(ctx context.Context, id agentdomain.ID, key string, caller gov.Context) error {
	if err := s.requireOwner(caller); err != nil {
		return err
	}
	if _, err := s.store.GetAgent(ctx, id); err != nil {
		return err
	}
	return s.store.UnbindWorkspace(ctx, id, key)
}
func (s *Service) GrantWorkspace(ctx context.Context, id agentdomain.ID, key, profileID string, caller gov.Context) error {
	if err := s.requireOwner(caller); err != nil {
		return err
	}
	if s.profileExists == nil {
		return agentdomain.ErrUnavailable
	}
	if _, err := s.store.Workspace(ctx, id, key); err != nil {
		return err
	}
	if err := s.profileExists(ctx, profileID); err != nil {
		return agentdomain.ErrInvalid
	}
	return s.store.GrantWorkspace(ctx, id, key, profileID)
}
func (s *Service) RevokeWorkspace(ctx context.Context, id agentdomain.ID, key, profileID string, caller gov.Context) error {
	if err := s.requireOwner(caller); err != nil {
		return err
	}
	if _, err := s.store.Workspace(ctx, id, key); err != nil {
		return err
	}
	return s.store.RevokeWorkspace(ctx, id, key, profileID)
}
