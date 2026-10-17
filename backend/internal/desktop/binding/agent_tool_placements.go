package binding

import (
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

type AgentToolPlacement struct {
	AgentID       string   `json:"agentId"`
	WorkspaceKey  string   `json:"workspaceKey"`
	WorkspaceRoot string   `json:"workspaceRoot"`
	SandboxID     string   `json:"sandboxId"`
	ToolIDs       []string `json:"toolIds"`
	UpdatedAt     string   `json:"updatedAt"`
}
type AgentToolPlacementList struct {
	Placements []AgentToolPlacement `json:"placements"`
}

func projectAgentToolPlacement(p agentdomain.ToolPlacement) AgentToolPlacement {
	return AgentToolPlacement{
		AgentID: string(p.AgentID), WorkspaceKey: p.WorkspaceKey, WorkspaceRoot: p.WorkspaceRoot,
		SandboxID: p.SandboxID, ToolIDs: p.ToolIDs, UpdatedAt: p.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}
func (s *Service) ListAgentToolPlacements(agentID string) (AgentToolPlacementList, error) {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return AgentToolPlacementList{}, err
	}
	id, err := agentdomain.ParseID(agentID)
	if err != nil {
		return AgentToolPlacementList{}, err
	}
	items, err := s.runtime.ListAgentToolPlacements(s.ctx, id, caller)
	if err != nil {
		return AgentToolPlacementList{}, err
	}
	result := AgentToolPlacementList{Placements: make([]AgentToolPlacement, 0, len(items))}
	for _, item := range items {
		result.Placements = append(result.Placements, projectAgentToolPlacement(item))
	}
	return result, nil
}
func (s *Service) ConfigureAgentToolPlacement(agentID, workspaceKey, sandboxID string, toolIDs []string) (AgentToolPlacement, error) {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return AgentToolPlacement{}, err
	}
	id, err := agentdomain.ParseID(agentID)
	if err != nil {
		return AgentToolPlacement{}, err
	}
	configured, err := s.runtime.ConfigureAgentToolPlacement(s.ctx, id, workspaceKey, sandboxID, toolIDs, caller)
	if err != nil {
		return AgentToolPlacement{}, err
	}
	return projectAgentToolPlacement(configured), nil
}
func (s *Service) DisableAgentToolPlacement(agentID, workspaceKey string) error {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return err
	}
	id, err := agentdomain.ParseID(agentID)
	if err != nil {
		return err
	}
	return s.runtime.DisableAgentToolPlacement(s.ctx, id, workspaceKey, caller)
}
