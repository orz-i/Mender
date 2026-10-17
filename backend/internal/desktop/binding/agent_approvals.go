package binding

import (
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

type AgentApproval struct {
	ApprovalID string `json:"approvalId"`
	TaskID     string `json:"taskId"`
	AgentID    string `json:"agentId"`
	Kind       string `json:"kind"`
	Target     string `json:"target,omitempty"`
	CanAllow   bool   `json:"canAllow"`
	Status     string `json:"status"`
	CreatedAt  string `json:"createdAt"`
	ExpiresAt  string `json:"expiresAt"`
	ResolvedAt string `json:"resolvedAt,omitempty"`
}
type AgentApprovalList struct {
	Approvals []AgentApproval `json:"approvals"`
}

func projectAgentApproval(value agentdomain.Approval) AgentApproval {
	record := AgentApproval{
		ApprovalID: string(value.ID), TaskID: string(value.TaskID), AgentID: string(value.AgentID),
		Kind: value.Kind, Target: value.Target, CanAllow: value.CanAllow, Status: string(value.Status),
		CreatedAt: value.CreatedAt.UTC().Format(time.RFC3339Nano),
		ExpiresAt: value.ExpiresAt.UTC().Format(time.RFC3339Nano),
	}
	if !value.ResolvedAt.IsZero() {
		record.ResolvedAt = value.ResolvedAt.UTC().Format(time.RFC3339Nano)
	}
	return record
}

// Task approval methods are intentionally exposed ONLY through the owner
// desktop binding. A delegated Profile/MCP Tool cannot call these methods.
func (s *Service) ListAgentApprovals(taskID string) (AgentApprovalList, error) {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return AgentApprovalList{}, err
	}
	id, err := agentdomain.ParseTaskID(taskID)
	if err != nil {
		return AgentApprovalList{}, err
	}
	values, err := s.runtime.Agents.ListApprovals(s.ctx, id, caller)
	if err != nil {
		return AgentApprovalList{}, err
	}
	result := AgentApprovalList{Approvals: make([]AgentApproval, 0, len(values))}
	for _, a := range values {
		result.Approvals = append(result.Approvals, projectAgentApproval(a))
	}
	return result, nil
}
func (s *Service) DecideAgentApproval(approvalID, choice string) (AgentApproval, error) {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return AgentApproval{}, err
	}
	id, err := agentdomain.ParseApprovalID(approvalID)
	if err != nil {
		return AgentApproval{}, err
	}
	result, err := s.runtime.Agents.DecideApproval(s.ctx, id, agentdomain.ApprovalChoice(choice), caller)
	if err != nil {
		return AgentApproval{}, err
	}
	return projectAgentApproval(result), nil
}
