package binding

import (
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

type AgentWorkspace struct {
	Key        string   `json:"key"`
	SandboxID  string   `json:"sandboxId"`
	ProfileIDs []string `json:"profileIds"`
}
type AgentWorkspaceList struct {
	Workspaces []AgentWorkspace `json:"workspaces"`
}

func (s *Service) UpdateAgentSettings(id, defaultModel string) (Agent, error) {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return Agent{}, err
	}
	parsed, err := agentdomain.ParseID(id)
	if err != nil {
		return Agent{}, err
	}
	a, err := s.runtime.Agents.UpdateDefaultModel(s.ctx, parsed, defaultModel, caller)
	if err != nil {
		return Agent{}, err
	}
	return s.projectAgent(a), nil
}
func (s *Service) ListAgentWorkspaces(id string) (AgentWorkspaceList, error) {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return AgentWorkspaceList{}, err
	}
	parsed, err := agentdomain.ParseID(id)
	if err != nil {
		return AgentWorkspaceList{}, err
	}
	bindings, err := s.runtime.Agents.Workspaces(s.ctx, parsed, caller)
	if err != nil {
		return AgentWorkspaceList{}, err
	}
	result := AgentWorkspaceList{Workspaces: make([]AgentWorkspace, 0, len(bindings))}
	for _, item := range bindings {
		result.Workspaces = append(result.Workspaces, AgentWorkspace{Key: item.Key, SandboxID: item.SandboxID, ProfileIDs: item.ProfileIDs})
	}
	return result, nil
}
func (s *Service) BindAgentWorkspace(id, key, sandboxID string) error {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return err
	}
	parsed, err := agentdomain.ParseID(id)
	if err != nil {
		return err
	}
	return s.runtime.Agents.BindWorkspace(s.ctx, parsed, key, sandboxID, caller)
}
func (s *Service) UnbindAgentWorkspace(id, key string) error {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return err
	}
	parsed, err := agentdomain.ParseID(id)
	if err != nil {
		return err
	}
	return s.runtime.Agents.UnbindWorkspace(s.ctx, parsed, key, caller)
}
func (s *Service) GrantAgentWorkspace(id, key, profileID string) error {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return err
	}
	parsed, err := agentdomain.ParseID(id)
	if err != nil {
		return err
	}
	return s.runtime.Agents.GrantWorkspace(s.ctx, parsed, key, profileID, caller)
}
func (s *Service) RevokeAgentWorkspace(id, key, profileID string) error {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return err
	}
	parsed, err := agentdomain.ParseID(id)
	if err != nil {
		return err
	}
	return s.runtime.Agents.RevokeWorkspace(s.ctx, parsed, key, profileID, caller)
}
