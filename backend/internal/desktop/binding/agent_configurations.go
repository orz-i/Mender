package binding

import (
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

type AgentModelChoice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type AgentSelectChoices struct {
	Current string             `json:"current"`
	Choices []AgentModelChoice `json:"choices"`
}
type AgentModelChoices struct {
	Current         string              `json:"current"`
	Models          []AgentModelChoice  `json:"models"`
	ReasoningEffort *AgentSelectChoices `json:"reasoningEffort,omitempty"`
}

func (s *Service) InspectAgentModels(id string) (AgentModelChoices, error) {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return AgentModelChoices{}, err
	}
	parsed, err := agentdomain.ParseID(id)
	if err != nil {
		return AgentModelChoices{}, err
	}
	value, err := s.runtime.Agents.InspectModels(s.ctx, parsed, caller)
	if err != nil {
		return AgentModelChoices{}, err
	}
	response := AgentModelChoices{Current: value.Current, Models: make([]AgentModelChoice, 0, len(value.Models))}
	for _, m := range value.Models {
		response.Models = append(response.Models, AgentModelChoice{ID: m.ID, Name: m.Name})
	}
	if value.ReasoningEffort != nil {
		choices := &AgentSelectChoices{Current: value.ReasoningEffort.Current, Choices: make([]AgentModelChoice, 0, len(value.ReasoningEffort.Choices))}
		for _, option := range value.ReasoningEffort.Choices {
			choices.Choices = append(choices.Choices, AgentModelChoice{ID: option.ID, Name: option.Name})
		}
		response.ReasoningEffort = choices
	}
	return response, nil
}

type AgentWorkspace struct {
	Key           string   `json:"key"`
	WorkspaceRoot string   `json:"workspaceRoot"`
	ProfileIDs    []string `json:"profileIds"`
}
type AgentWorkspaceList struct {
	Workspaces []AgentWorkspace `json:"workspaces"`
}

func (s *Service) UpdateAgentSettings(id, defaultModel, defaultReasoningEffort string) (Agent, error) {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return Agent{}, err
	}
	parsed, err := agentdomain.ParseID(id)
	if err != nil {
		return Agent{}, err
	}
	a, err := s.runtime.Agents.UpdateDefaults(s.ctx, parsed, defaultModel, defaultReasoningEffort, caller)
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
		result.Workspaces = append(result.Workspaces, AgentWorkspace{Key: item.Key, WorkspaceRoot: item.WorkspaceRoot, ProfileIDs: item.ProfileIDs})
	}
	return result, nil
}
func (s *Service) BindAgentWorkspace(id, key, workspaceRoot string) error {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return err
	}
	parsed, err := agentdomain.ParseID(id)
	if err != nil {
		return err
	}
	return s.runtime.Agents.BindWorkspace(s.ctx, parsed, key, workspaceRoot, caller)
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
