package binding

import (
	"fmt"
	"time"

	"github.com/orz-i/Mender/backend/internal/bootstrap"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
)

type Agent struct {
	AgentID                string `json:"agentId"`
	Name                   string `json:"name"`
	Backend                string `json:"backend"`
	PluginID               string `json:"pluginId"`
	ExtensionID            string `json:"extensionId"`
	WorkspaceRoot          string `json:"workspaceRoot"`
	DefaultModel           string `json:"defaultModel"`
	DefaultReasoningEffort string `json:"defaultReasoningEffort"`
	Isolation              string `json:"isolation"`
	CreatedAt              string `json:"createdAt"`
	BackendAvailable       bool   `json:"backendAvailable"`
	Issue                  string `json:"issue,omitempty"`
}
type AgentList struct {
	Agents []Agent `json:"agents"`
}
type AgentTool struct {
	ToolID    string `json:"toolId"`
	SourceKey string `json:"sourceKey"`
	Name      string `json:"name"`
	Title     string `json:"title"`
}
type AgentTools struct {
	SuiteID      string      `json:"suiteId"`
	Status       string      `json:"status"`
	ProviderID   string      `json:"providerId,omitempty"`
	ProviderName string      `json:"providerName,omitempty"`
	Tools        []AgentTool `json:"tools"`
}
type AgentTaskObservation struct {
	Phase                 string `json:"phase"`
	Activity              string `json:"activity"`
	LastObservedAt        string `json:"lastObservedAt"`
	LastNativeProgressAt  string `json:"lastNativeProgressAt,omitempty"`
	LastWorkerHeartbeatAt string `json:"lastWorkerHeartbeatAt,omitempty"`
	SessionRecorded       bool   `json:"sessionRecorded"`
	SessionRecoverability string `json:"sessionRecoverability"`
	LeaderReaped          bool   `json:"leaderReaped"`
}
type AgentTask struct {
	TaskID              string                `json:"taskId"`
	AgentID             string                `json:"agentId"`
	Status              string                `json:"status"`
	Model               string                `json:"model,omitempty"`
	ReasoningEffort     string                `json:"reasoningEffort,omitempty"`
	Workspace           string                `json:"workspace,omitempty"`
	Result              string                `json:"result,omitempty"`
	Diagnostic          string                `json:"diagnostic,omitempty"`
	CreatedAt           string                `json:"createdAt"`
	UpdatedAt           string                `json:"updatedAt"`
	CancelRequested     bool                  `json:"cancelRequested"`
	Observation         *AgentTaskObservation `json:"observation,omitempty"`
	ContinuedFromTaskID string                `json:"continuedFromTaskId,omitempty"`
}
type AgentTaskList struct {
	Tasks []AgentTask `json:"tasks"`
}
type AgentAdmissionEvent struct {
	AgentID       string `json:"agentId"`
	Action        string `json:"action"`
	PrincipalKind string `json:"principalKind"`
	Origin        string `json:"origin"`
	ScopeKind     string `json:"scopeKind"`
	PolicyKind    string `json:"policyKind"`
	PolicyReason  string `json:"policyReason"`
	Status        string `json:"status"`
	OccurredAt    string `json:"occurredAt"`
}
type AgentAdmissionList struct {
	Events []AgentAdmissionEvent `json:"events"`
}

type AgentProfileGrants struct {
	ProfileIDs []string `json:"profileIds"`
}
type AgentCancelReceipt struct {
	TaskID          string `json:"taskId"`
	CancelRequested bool   `json:"cancelRequested"`
}
type CreateAgentInput struct {
	Name                   string `json:"name"`
	Backend                string `json:"backend"`
	WorkspaceRoot          string `json:"workspaceRoot"`
	DefaultModel           string `json:"defaultModel"`
	DefaultReasoningEffort string `json:"defaultReasoningEffort"`
	Isolation              string `json:"isolation"`
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
		WorkspaceRoot: spec.WorkspaceRoot, DefaultModel: spec.DefaultModel, DefaultReasoningEffort: spec.DefaultReasoningEffort, Isolation: spec.Isolation,
		CreatedAt: spec.CreatedAt.UTC().Format(time.RFC3339Nano)}
	if _, err := s.runtime.AgentAvailability(spec.Backend); err == nil {
		out.BackendAvailable = true
	} else {
		out.Issue = err.Error()
	}
	return out
}
func projectTask(t agentdomain.Task) AgentTask {
	out := AgentTask{TaskID: string(t.ID), AgentID: string(t.AgentID), Status: string(t.Status), Model: t.Model, ReasoningEffort: t.ReasoningEffort, Workspace: t.WorkspaceKey,
		Result: t.Result, Diagnostic: t.Diagnostic, CreatedAt: t.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt: t.UpdatedAt.UTC().Format(time.RFC3339Nano), CancelRequested: t.CancelRequested}
	if t.ContinuedFromTaskID != "" {
		out.ContinuedFromTaskID = string(t.ContinuedFromTaskID)
	}
	if o := t.Observation; o != nil {
		out.Observation = &AgentTaskObservation{
			Phase: o.Phase, Activity: o.Activity, LastObservedAt: o.LastObservedAt.UTC().Format(time.RFC3339Nano),
			SessionRecorded: o.SessionRecorded, SessionRecoverability: o.SessionRecoverability,
			LeaderReaped: o.LeaderReaped,
		}
		if o.LastNativeProgressAt != nil {
			out.Observation.LastNativeProgressAt = o.LastNativeProgressAt.UTC().Format(time.RFC3339Nano)
		}
		if o.LastWorkerHeartbeatAt != nil {
			out.Observation.LastWorkerHeartbeatAt = o.LastWorkerHeartbeatAt.UTC().Format(time.RFC3339Nano)
		}
	}
	return out
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
	a, err := s.runtime.CreateAgentWithTools(s.ctx, agentapplication.CreateRequest{
		Name: input.Name, Backend: agentdomain.Backend(input.Backend), WorkspaceRoot: input.WorkspaceRoot, DefaultModel: input.DefaultModel, DefaultReasoningEffort: input.DefaultReasoningEffort, Isolation: input.Isolation,
	}, caller)
	if err != nil {
		return Agent{}, err
	}
	return s.projectAgent(a), nil
}
func projectAgentTools(value bootstrap.AgentTools) AgentTools {
	out := AgentTools{
		SuiteID: value.SuiteID, Status: value.Status, ProviderID: value.ProviderID,
		ProviderName: value.ProviderName, Tools: make([]AgentTool, 0, len(value.Tools)),
	}
	for _, tool := range value.Tools {
		out.Tools = append(out.Tools, AgentTool{
			ToolID: tool.ToolID, SourceKey: tool.SourceKey, Name: tool.Name, Title: tool.Title,
		})
	}
	return out
}
func (s *Service) agentTools(id string, ensure bool) (AgentTools, error) {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return AgentTools{}, err
	}
	parsed, err := agentdomain.ParseID(id)
	if err != nil {
		return AgentTools{}, err
	}
	var value bootstrap.AgentTools
	if ensure {
		value, err = s.runtime.EnsureAgentTools(s.ctx, parsed, caller)
	} else {
		value, err = s.runtime.AgentTools(s.ctx, parsed, caller)
	}
	if err != nil {
		return AgentTools{}, err
	}
	return projectAgentTools(value), nil
}
func (s *Service) GetAgentTools(id string) (AgentTools, error)    { return s.agentTools(id, false) }
func (s *Service) EnsureAgentTools(id string) (AgentTools, error) { return s.agentTools(id, true) }
func (s *Service) DeleteAgent(id string) error {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return err
	}
	parsed, err := agentdomain.ParseID(id)
	if err != nil {
		return err
	}
	return s.runtime.DeleteAgentWithTools(s.ctx, parsed, caller)
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
func (s *Service) ListAgentAdmissions(id string) (AgentAdmissionList, error) {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return AgentAdmissionList{}, err
	}
	parsed, err := agentdomain.ParseID(id)
	if err != nil {
		return AgentAdmissionList{}, err
	}
	items, err := s.runtime.Agents.Admissions(s.ctx, parsed, caller, 100)
	if err != nil {
		return AgentAdmissionList{}, err
	}
	out := AgentAdmissionList{Events: make([]AgentAdmissionEvent, 0, len(items))}
	for _, ev := range items {
		out.Events = append(out.Events, AgentAdmissionEvent{
			AgentID: string(ev.AgentID), Action: ev.Action, PrincipalKind: ev.PrincipalKind,
			Origin: ev.Origin, ScopeKind: ev.ScopeKind, PolicyKind: ev.PolicyKind,
			PolicyReason: ev.PolicyReason, Status: ev.Status,
			OccurredAt: ev.OccurredAt.UTC().Format(time.RFC3339Nano),
		})
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
