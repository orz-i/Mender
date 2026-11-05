// Package application owns Agent resources, Task/Session lifecycle and
// native approvals, without implementing the Agent's own reasoning loop.
package application

import (
	"context"
	"errors"
	"strings"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
)

// Service preserves the management and delegation API while delegating to
// resource, Task/Session and native approval use cases with separate ports.
type Service struct {
	resources *ResourceService
	tasks     *TaskService
	approvals *ApprovalService
}

func NewService(store Repository, runner Executor, admit Admission, owner gov.Principal, profileExists ProfileExists, verifier LocalWorkspaceVerifier) (*Service, error) {
	if store == nil || runner == nil || admit == nil || verifier == nil || owner.Validate() != nil ||
		owner.Kind() != gov.PrincipalLocalUser {
		return nil, errors.New("agent service: missing trusted dependencies")
	}
	runtimeID, err := agentdomain.NewID()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	runtime := &runtimeCoordinator{
		runtimeInstance: "arun_" + strings.TrimPrefix(string(runtimeID), "agent_"),
		rootContext:     ctx, rootCancel: cancel,
		active:  map[agentdomain.TaskID]context.CancelFunc{},
		pending: map[agentdomain.ApprovalID]*pendingApproval{},
	}
	authority := &agentAuthority{owner: owner, grants: store}
	access := &taskAccess{agentAuthority: authority, store: store}
	workspace := &localWorkspaceRules{workspace: verifier}
	approvals := &ApprovalService{taskAccess: access, runtimeCoordinator: runtime,
		localWorkspaceRules: workspace, store: store, admit: admit}
	tasks := &TaskService{taskAccess: access, runtimeCoordinator: runtime,
		localWorkspaceRules: workspace, store: store, runner: runner, admit: admit, approvals: approvals}
	inspector, _ := runner.(ModelInspector)
	resources := &ResourceService{agentAuthority: authority, localWorkspaceRules: workspace,
		store: store, inspector: inspector, profileExists: profileExists}
	return &Service{resources: resources, tasks: tasks, approvals: approvals}, nil
}

func (s *Service) Create(ctx context.Context, request CreateRequest, caller gov.Context) (agentdomain.Agent, error) {
	return s.resources.Create(ctx, request, caller)
}

func (s *Service) List(ctx context.Context, caller gov.Context) ([]agentdomain.Agent, error) {
	return s.resources.List(ctx, caller)
}

func (s *Service) Get(ctx context.Context, id agentdomain.ID, caller gov.Context) (agentdomain.Agent, error) {
	return s.resources.Get(ctx, id, caller)
}

func (s *Service) Delete(ctx context.Context, id agentdomain.ID, caller gov.Context) error {
	return s.resources.Delete(ctx, id, caller)
}

func (s *Service) Grant(ctx context.Context, id agentdomain.ID, profileID string, caller gov.Context) error {
	return s.resources.Grant(ctx, id, profileID, caller)
}

func (s *Service) Revoke(ctx context.Context, id agentdomain.ID, profileID string, caller gov.Context) error {
	return s.resources.Revoke(ctx, id, profileID, caller)
}

func (s *Service) Grants(ctx context.Context, id agentdomain.ID, caller gov.Context) ([]string, error) {
	return s.resources.Grants(ctx, id, caller)
}

func (s *Service) Admissions(ctx context.Context, id agentdomain.ID, caller gov.Context, limit int) ([]agentdomain.AdmissionEvent, error) {
	return s.resources.Admissions(ctx, id, caller, limit)
}

func (s *Service) Submit(ctx context.Context, id agentdomain.ID, prompt string, caller gov.Context) (agentdomain.Task, error) {
	return s.tasks.Submit(ctx, id, prompt, caller)
}

func (s *Service) SubmitWithOptions(ctx context.Context, id agentdomain.ID, prompt string, options SubmitOptions, caller gov.Context) (agentdomain.Task, error) {
	return s.tasks.SubmitWithOptions(ctx, id, prompt, options, caller)
}

func (s *Service) ConfigureToolSupplyResolver(resolve ToolSupplyResolver) error {
	return s.tasks.ConfigureToolSupplyResolver(resolve)
}

func (s *Service) TaskLocallyActive(id agentdomain.TaskID) bool {
	if s == nil {
		return false
	}
	return s.tasks.TaskLocallyActive(id)
}

func (s *Service) Task(ctx context.Context, id agentdomain.TaskID, caller gov.Context) (agentdomain.Task, error) {
	return s.tasks.Task(ctx, id, caller)
}

func (s *Service) Tasks(ctx context.Context, id agentdomain.ID, caller gov.Context) ([]agentdomain.Task, error) {
	return s.tasks.Tasks(ctx, id, caller)
}

func (s *Service) Cancel(ctx context.Context, id agentdomain.TaskID, caller gov.Context) error {
	return s.tasks.Cancel(ctx, id, caller)
}

func (s *Service) Close() error {
	return s.tasks.Close()
}

func (s *Service) InspectModels(ctx context.Context, id agentdomain.ID, caller gov.Context) (ModelChoices, error) {
	return s.resources.InspectModels(ctx, id, caller)
}

func (s *Service) UpdateDefaults(ctx context.Context, id agentdomain.ID, model, effort string, caller gov.Context) (agentdomain.Agent, error) {
	return s.resources.UpdateDefaults(ctx, id, model, effort, caller)
}

func (s *Service) BindWorkspace(ctx context.Context, id agentdomain.ID, key, workspaceRoot string, caller gov.Context) error {
	return s.resources.BindWorkspace(ctx, id, key, workspaceRoot, caller)
}

func (s *Service) Workspaces(ctx context.Context, id agentdomain.ID, caller gov.Context) ([]agentdomain.WorkspaceBinding, error) {
	return s.resources.Workspaces(ctx, id, caller)
}

func (s *Service) UnbindWorkspace(ctx context.Context, id agentdomain.ID, key string, caller gov.Context) error {
	return s.resources.UnbindWorkspace(ctx, id, key, caller)
}

func (s *Service) GrantWorkspace(ctx context.Context, id agentdomain.ID, key, profileID string, caller gov.Context) error {
	return s.resources.GrantWorkspace(ctx, id, key, profileID, caller)
}

func (s *Service) RevokeWorkspace(ctx context.Context, id agentdomain.ID, key, profileID string, caller gov.Context) error {
	return s.resources.RevokeWorkspace(ctx, id, key, profileID, caller)
}

func (s *Service) ListApprovals(ctx context.Context, taskID agentdomain.TaskID, caller gov.Context) ([]agentdomain.Approval, error) {
	return s.approvals.ListApprovals(ctx, taskID, caller)
}

func (s *Service) DecideApproval(ctx context.Context, id agentdomain.ApprovalID, choice agentdomain.ApprovalChoice, caller gov.Context) (agentdomain.Approval, error) {
	return s.approvals.DecideApproval(ctx, id, choice, caller)
}

func (s *Service) GetApprovalPolicy(ctx context.Context, id agentdomain.ID, key string, owner gov.Context) (agentdomain.ApprovalPolicy, error) {
	return s.approvals.GetApprovalPolicy(ctx, id, key, owner)
}

func (s *Service) SetApprovalPolicy(ctx context.Context, id agentdomain.ID, key string, mode agentdomain.ApprovalMode, owner gov.Context) (agentdomain.ApprovalPolicy, error) {
	return s.approvals.SetApprovalPolicy(ctx, id, key, mode, owner)
}

func (s *Service) ListPendingApprovals(ctx context.Context, owner gov.Context) ([]agentdomain.PendingApproval, error) {
	return s.approvals.ListPendingApprovals(ctx, owner)
}

func (s *Service) ListApprovalPolicies(ctx context.Context, id agentdomain.ID, owner gov.Context) ([]agentdomain.ApprovalPolicy, error) {
	return s.approvals.ListApprovalPolicies(ctx, id, owner)
}

func (s *Service) AllowPendingWithCurrentPolicy(ctx context.Context, id agentdomain.ID, key string, owner gov.Context) (int, error) {
	return s.approvals.AllowPendingWithCurrentPolicy(ctx, id, key, owner)
}

func (s *Service) Resume(ctx context.Context, id agentdomain.TaskID, prompt string,
	agentID agentdomain.ID, caller gov.Context) (agentdomain.Task, error) {
	return s.tasks.Resume(ctx, id, prompt, agentID, caller)
}
