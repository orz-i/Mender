package application

import (
	"context"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

// Repository is the composition root's aggregate storage dependency. Use cases
// consume the smaller ports below; aggregate writes keep their original atomic
// persistence boundaries instead of being reconstructed from smaller calls.
type Repository interface {
	ResourceRepository
	TaskRepository
	ApprovalRepository
	ProfileGrantLookup
}

type ProfileGrantLookup interface {
	HasProfileGrant(context.Context, agentdomain.ID, string) (bool, error)
}

type TaskLookup interface {
	GetAgent(context.Context, agentdomain.ID) (agentdomain.Agent, error)
	GetTask(context.Context, agentdomain.TaskID) (agentdomain.Task, error)
}

type ResourceRepository interface {
	CreateAgent(context.Context, agentdomain.Agent) error
	GetAgent(context.Context, agentdomain.ID) (agentdomain.Agent, error)
	ListAgents(context.Context) ([]agentdomain.Agent, error)
	DeleteAgent(context.Context, agentdomain.ID) error
	ListProfileGrants(context.Context, agentdomain.ID) ([]string, error)
	ListAdmissions(context.Context, agentdomain.ID, int) ([]agentdomain.AdmissionEvent, error)
	GrantProfile(context.Context, agentdomain.ID, string) error
	RevokeProfile(context.Context, agentdomain.ID, string) error
	SetDefaults(context.Context, agentdomain.ID, string, string) error
	BindWorkspace(context.Context, agentdomain.ID, string, string) error
	Workspace(context.Context, agentdomain.ID, string) (string, error)
	ListWorkspaces(context.Context, agentdomain.ID) ([]agentdomain.WorkspaceBinding, error)
	UnbindWorkspace(context.Context, agentdomain.ID, string) error
	GrantWorkspace(context.Context, agentdomain.ID, string, string) error
	RevokeWorkspace(context.Context, agentdomain.ID, string, string) error
}

type TaskRepository interface {
	TaskLookup
	ListTasks(context.Context, agentdomain.ID) ([]agentdomain.Task, error)
	CreateAdmittedTask(context.Context, agentdomain.Task, int, string) error
	ResumePlan(context.Context, agentdomain.TaskID) (agentdomain.SessionContinuation, error)
	CreateResumedTask(context.Context, agentdomain.Task, agentdomain.SessionContinuation, int, string) error
	TaskParent(context.Context, agentdomain.TaskID) (agentdomain.TaskID, error)
	BeginAttempt(context.Context, agentdomain.Task, string) error
	RecordExecution(context.Context, agentdomain.Task, string, agentdomain.ExecutionEvent) error
	TaskObservation(context.Context, agentdomain.TaskID) (*agentdomain.TaskObservation, error)
	Transition(context.Context, agentdomain.TaskID, uint64, agentdomain.TaskStatus, agentdomain.TaskStatus, string, string) error
	RequestCancel(context.Context, agentdomain.TaskID, uint64) error
	Workspace(context.Context, agentdomain.ID, string) (string, error)
	HasWorkspaceGrant(context.Context, agentdomain.ID, string, string) (bool, error)
}

type ApprovalRepository interface {
	TaskLookup
	Workspace(context.Context, agentdomain.ID, string) (string, error)
	CreateApproval(context.Context, agentdomain.Approval) error
	GetApproval(context.Context, agentdomain.ApprovalID) (agentdomain.Approval, error)
	ListTaskApprovals(context.Context, agentdomain.TaskID) ([]agentdomain.Approval, error)
	ResolveApproval(context.Context, agentdomain.ApprovalID, agentdomain.ApprovalStatus, int64) error
	InvalidateApproval(context.Context, agentdomain.ApprovalID) error
	ApprovalPolicy(context.Context, agentdomain.ID, string) (agentdomain.ApprovalPolicy, error)
	ListApprovalPolicies(context.Context, agentdomain.ID) ([]agentdomain.ApprovalPolicy, error)
	SetApprovalPolicy(context.Context, agentdomain.ApprovalPolicy) error
	ClearApprovalPolicy(context.Context, agentdomain.ID, string) error
	CurrentPolicyRevision(context.Context) (int64, error)
	RecordAutomaticApproval(context.Context, agentdomain.Approval, agentdomain.ApprovalMode, int64) error
	ResolveApprovalByPolicy(context.Context, agentdomain.ApprovalID, agentdomain.ApprovalMode, int64) error
	ListPendingApprovals(context.Context, int) ([]agentdomain.PendingApproval, error)
	ListPendingApprovalsForWorkspace(context.Context, agentdomain.ID, string, agentdomain.ApprovalMode, agentdomain.ApprovalID, int) ([]agentdomain.PendingApproval, error)
}
