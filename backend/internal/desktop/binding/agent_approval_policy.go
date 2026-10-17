package binding

import (
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

type AgentApprovalPolicy struct {
	AgentID      string `json:"agentId"`
	WorkspaceKey string `json:"workspaceKey"`
	Mode         string `json:"mode"`
	ExpiresAt    string `json:"expiresAt,omitempty"`
	UpdatedAt    string `json:"updatedAt,omitempty"`
}
type AgentApprovalPolicyList struct {
	Policies []AgentApprovalPolicy `json:"policies"`
}
type AgentPendingApproval struct {
	Approval     AgentApproval `json:"approval"`
	AgentName    string        `json:"agentName"`
	WorkspaceKey string        `json:"workspaceKey"`
}
type AgentApprovalReleaseReceipt struct {
	Released int `json:"released"`
}
type AgentPendingApprovalList struct {
	Approvals []AgentPendingApproval `json:"approvals"`
}

func projectApprovalPolicy(p agentdomain.ApprovalPolicy) AgentApprovalPolicy {
	result := AgentApprovalPolicy{AgentID: string(p.AgentID), WorkspaceKey: p.WorkspaceKey, Mode: string(p.Mode)}
	if !p.ExpiresAt.IsZero() {
		result.ExpiresAt = p.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	if !p.UpdatedAt.IsZero() {
		result.UpdatedAt = p.UpdatedAt.UTC().Format(time.RFC3339Nano)
	}
	return result
}
func (s *Service) ListPendingAgentApprovals() (AgentPendingApprovalList, error) {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return AgentPendingApprovalList{}, err
	}
	found, err := s.runtime.Agents.ListPendingApprovals(s.ctx, caller)
	if err != nil {
		return AgentPendingApprovalList{}, err
	}
	result := AgentPendingApprovalList{Approvals: make([]AgentPendingApproval, 0, len(found))}
	for _, item := range found {
		result.Approvals = append(result.Approvals, AgentPendingApproval{Approval: projectAgentApproval(item.Approval),
			AgentName: item.AgentName, WorkspaceKey: item.WorkspaceKey})
	}
	return result, nil
}
func (s *Service) GetAgentApprovalPolicy(agentID, workspaceKey string) (AgentApprovalPolicy, error) {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return AgentApprovalPolicy{}, err
	}
	id, err := agentdomain.ParseID(agentID)
	if err != nil {
		return AgentApprovalPolicy{}, err
	}
	item, err := s.runtime.Agents.GetApprovalPolicy(s.ctx, id, workspaceKey, caller)
	if err != nil {
		return AgentApprovalPolicy{}, err
	}
	return projectApprovalPolicy(item), nil
}
func (s *Service) ListAgentApprovalPolicies(agentID string) (AgentApprovalPolicyList, error) {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return AgentApprovalPolicyList{}, err
	}
	id, err := agentdomain.ParseID(agentID)
	if err != nil {
		return AgentApprovalPolicyList{}, err
	}
	items, err := s.runtime.Agents.ListApprovalPolicies(s.ctx, id, caller)
	if err != nil {
		return AgentApprovalPolicyList{}, err
	}
	out := AgentApprovalPolicyList{Policies: make([]AgentApprovalPolicy, 0, len(items))}
	for _, item := range items {
		out.Policies = append(out.Policies, projectApprovalPolicy(item))
	}
	return out, nil
}
func (s *Service) ReleasePendingAgentApprovals(agentID, workspaceKey string) (AgentApprovalReleaseReceipt, error) {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return AgentApprovalReleaseReceipt{}, err
	}
	id, err := agentdomain.ParseID(agentID)
	if err != nil {
		return AgentApprovalReleaseReceipt{}, err
	}
	n, err := s.runtime.Agents.AllowPendingWithCurrentPolicy(s.ctx, id, workspaceKey, caller)
	if err != nil {
		return AgentApprovalReleaseReceipt{}, err
	}
	return AgentApprovalReleaseReceipt{Released: n}, nil
}
func (s *Service) SetAgentApprovalPolicy(agentID, workspaceKey, mode string) (AgentApprovalPolicy, error) {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return AgentApprovalPolicy{}, err
	}
	id, err := agentdomain.ParseID(agentID)
	if err != nil {
		return AgentApprovalPolicy{}, err
	}
	result, err := s.runtime.Agents.SetApprovalPolicy(s.ctx, id, workspaceKey, agentdomain.ApprovalMode(mode), caller)
	if err != nil {
		return AgentApprovalPolicy{}, err
	}
	return projectApprovalPolicy(result), nil
}
