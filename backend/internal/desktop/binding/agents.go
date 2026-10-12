package binding

import (
	"fmt"
	"time"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
)

type Agent struct {
	AgentID          string `json:"agentId"`
	Name             string `json:"name"`
	Backend          string `json:"backend"`
	PluginID         string `json:"pluginId"`
	ExtensionID      string `json:"extensionId"`
	WorkspaceRoot    string `json:"workspaceRoot"`
	Isolation        string `json:"isolation"`
	CreatedAt        string `json:"createdAt"`
	BackendAvailable bool   `json:"backendAvailable"`
	Issue            string `json:"issue,omitempty"`
}
type AgentList struct {
	Agents []Agent `json:"agents"`
}
type AgentTask struct {
	TaskID          string `json:"taskId"`
	AgentID         string `json:"agentId"`
	Status          string `json:"status"`
	Result          string `json:"result,omitempty"`
	Diagnostic      string `json:"diagnostic,omitempty"`
	CreatedAt       string `json:"createdAt"`
	UpdatedAt       string `json:"updatedAt"`
	CancelRequested bool   `json:"cancelRequested"`
}
type AgentTaskList struct {
	Tasks []AgentTask `json:"tasks"`
}
type AgentProfileGrants struct {
	ProfileIDs []string `json:"profileIds"`
}
type AgentCancelReceipt struct {
	TaskID          string `json:"taskId"`
	CancelRequested bool   `json:"cancelRequested"`
}
type CreateAgentInput struct {
	Name          string `json:"name"`
	Backend       string `json:"backend"`
	WorkspaceRoot string `json:"workspaceRoot"`
	Isolation     string `json:"isolation"`
}

func (s *Service) agentOwnerContext() (gov.Context, error) {
	if s == nil || s.runtime == nil {
		return gov.Context{}, fmt.Errorf("desktop Agent runtime unavailable")
	}
	return gov.NewContext(gov.OriginDesktop, s.runtime.OwnerPrincipal(), gov.CatalogScope())
}
func (s *Service) projectAgent(a agentdomain.Agent) Agent {
	spec := a.Spec()
	out := Agent{AgentID: string(spec.ID), Name: spec.Name, Backend: string(spec.Backend),
		PluginID: string(spec.PluginID), ExtensionID: string(spec.ExtensionID),
		WorkspaceRoot: spec.WorkspaceRoot, Isolation: spec.Isolation,
		CreatedAt: spec.CreatedAt.UTC().Format(time.RFC3339Nano)}
	if _, err := s.runtime.AgentAvailability(spec.Backend); err == nil {
		out.BackendAvailable = true
	} else {
		out.Issue = err.Error()
	}
	return out
}
func projectTask(t agentdomain.Task) AgentTask {
	return AgentTask{TaskID: string(t.ID), AgentID: string(t.AgentID), Status: string(t.Status),
		Result: t.Result, Diagnostic: t.Diagnostic, CreatedAt: t.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt: t.UpdatedAt.UTC().Format(time.RFC3339Nano), CancelRequested: t.CancelRequested}
}
func (s *Service) ListAgents() (AgentList, error) {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return AgentList{}, err
	}
	agents, err := s.runtime.Agents.List(s.ctx, caller)
	if err != nil {
		return AgentList{}, err
	}
	result := AgentList{Agents: make([]Agent, 0, len(agents))}
	for _, a := range agents {
		result.Agents = append(result.Agents, s.projectAgent(a))
	}
	return result, nil
}
func (s *Service) GetAgent(id string) (Agent, error) {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return Agent{}, err
	}
	parsed, err := agentdomain.ParseID(id)
	if err != nil {
		return Agent{}, err
	}
	a, err := s.runtime.Agents.Get(s.ctx, parsed, caller)
	if err != nil {
		return Agent{}, err
	}
	return s.projectAgent(a), nil
}
func (s *Service) CreateAgent(input CreateAgentInput) (Agent, error) {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return Agent{}, err
	}
	a, err := s.runtime.Agents.Create(s.ctx, agentapplication.CreateRequest{
		Name: input.Name, Backend: agentdomain.Backend(input.Backend), WorkspaceRoot: input.WorkspaceRoot, Isolation: input.Isolation,
	}, caller)
	if err != nil {
		return Agent{}, err
	}
	return s.projectAgent(a), nil
}
func (s *Service) DeleteAgent(id string) error {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return err
	}
	parsed, err := agentdomain.ParseID(id)
	if err != nil {
		return err
	}
	return s.runtime.Agents.Delete(s.ctx, parsed, caller)
}
func (s *Service) ListAgentTasks(id string) (AgentTaskList, error) {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return AgentTaskList{}, err
	}
	parsed, err := agentdomain.ParseID(id)
	if err != nil {
		return AgentTaskList{}, err
	}
	tasks, err := s.runtime.Agents.Tasks(s.ctx, parsed, caller)
	if err != nil {
		return AgentTaskList{}, err
	}
	out := AgentTaskList{Tasks: make([]AgentTask, 0, len(tasks))}
	for _, task := range tasks {
		out.Tasks = append(out.Tasks, projectTask(task))
	}
	return out, nil
}
func (s *Service) GetAgentTask(id string) (AgentTask, error) {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return AgentTask{}, err
	}
	parsed, err := agentdomain.ParseTaskID(id)
	if err != nil {
		return AgentTask{}, err
	}
	task, err := s.runtime.Agents.Task(s.ctx, parsed, caller)
	if err != nil {
		return AgentTask{}, err
	}
	return projectTask(task), nil
}
func (s *Service) CancelAgentTask(id string) (AgentCancelReceipt, error) {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return AgentCancelReceipt{}, err
	}
	parsed, err := agentdomain.ParseTaskID(id)
	if err != nil {
		return AgentCancelReceipt{}, err
	}
	if err := s.runtime.Agents.Cancel(s.ctx, parsed, caller); err != nil {
		return AgentCancelReceipt{}, err
	}
	return AgentCancelReceipt{TaskID: id, CancelRequested: true}, nil
}
func (s *Service) ListAgentGrants(id string) (AgentProfileGrants, error) {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return AgentProfileGrants{}, err
	}
	parsed, err := agentdomain.ParseID(id)
	if err != nil {
		return AgentProfileGrants{}, err
	}
	ids, err := s.runtime.Agents.Grants(s.ctx, parsed, caller)
	if err != nil {
		return AgentProfileGrants{}, err
	}
	return AgentProfileGrants{ProfileIDs: ids}, nil
}
func (s *Service) GrantAgentProfile(agentID, profileID string) error {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return err
	}
	parsed, err := agentdomain.ParseID(agentID)
	if err != nil {
		return err
	}
	return s.runtime.Agents.Grant(s.ctx, parsed, profileID, caller)
}
func (s *Service) RevokeAgentProfile(agentID, profileID string) error {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return err
	}
	parsed, err := agentdomain.ParseID(agentID)
	if err != nil {
		return err
	}
	return s.runtime.Agents.Revoke(s.ctx, parsed, profileID, caller)
}
