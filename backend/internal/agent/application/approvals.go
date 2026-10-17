package application

import (
	"context"
	"fmt"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
)

const ApprovalWaitLimit = 2 * time.Minute

// PermissionRequest is validated from a live ACP stream and never grants
// authority on its own. Only offered *one-shot* options are passed through.
type PermissionRequest struct {
	SessionID      string
	ToolCallID     string
	Kind           string // read, edit, execute or other; for owner display ONLY
	Target         string // bounded, sanitized relative path for owner review
	CanAllow       bool   // only explicitly reviewable read/edit targets
	AllowOptionID  string
	RejectOptionID string
}
type PermissionDecision struct{ Choice agentdomain.ApprovalChoice }
type PermissionRequester func(context.Context, PermissionRequest) (PermissionDecision, error)
type pendingApproval struct {
	decision chan agentdomain.ApprovalChoice
}

// requestPermission starts one bounded owner decision wait, without blocking
// the ACP message pump. The runner will echo only the originally offered
// ACP optionId after the local owner's verified decision.
func (s *Service) requestPermission(ctx context.Context, task agentdomain.Task, caller gov.Context, req PermissionRequest) (PermissionDecision, error) {
	var none PermissionDecision
	if req.SessionID == "" || len(req.SessionID) > 512 ||
		req.ToolCallID == "" || len(req.ToolCallID) > 128 ||
		req.AllowOptionID == "" || req.RejectOptionID == "" ||
		len(req.AllowOptionID) > 128 || len(req.RejectOptionID) > 128 ||
		req.AllowOptionID == req.RejectOptionID {
		return none, agentdomain.ErrInvalid
	}
	kind := req.Kind
	switch kind {
	case "read", "edit", "execute":
	default:
		kind = "other"
	}
	limit := time.Now().Add(ApprovalWaitLimit)
	if deadline, ok := ctx.Deadline(); ok && deadline.Before(limit) {
		limit = deadline
	}
	if !limit.After(time.Now()) {
		return none, agentdomain.ErrUnavailable
	}
	approvalID, err := agentdomain.NewApprovalID()
	if err != nil {
		return none, err
	}
	a := agentdomain.Approval{ID: approvalID, TaskID: task.ID, AgentID: task.AgentID, Generation: task.Generation,
		SessionID: req.SessionID, ToolCallID: req.ToolCallID, Kind: kind,
		Target: req.Target, CanAllow: req.CanAllow,
		AllowOptionID: req.AllowOptionID, RejectOptionID: req.RejectOptionID,
		Status: agentdomain.ApprovalPending, CreatedAt: time.Now().UTC(), ExpiresAt: limit.UTC()}
	waiter := &pendingApproval{decision: make(chan agentdomain.ApprovalChoice, 1)}
	s.mu.Lock()
	if s.closed || s.active[task.ID] == nil {
		s.mu.Unlock()
		return none, agentdomain.ErrUnavailable
	}
	s.pending[approvalID] = waiter
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.pending, approvalID)
		s.mu.Unlock()
		expireCtx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		_ = s.store.InvalidateApproval(expireCtx, approvalID)
	}()
	if err := s.store.CreateApproval(ctx, a); err != nil {
		return none, err
	}
	timer := time.NewTimer(time.Until(limit))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return none, ctx.Err()
	case <-timer.C:
		return none, agentdomain.ErrUnavailable
	case choice := <-waiter.decision:
		if ctx.Err() != nil {
			return none, ctx.Err()
		}
		return PermissionDecision{Choice: choice}, nil
	}
}

// ListApprovals is for the local owner only. A model/Profile cannot browse
// another task's permission requests using a guessed TaskID.
func (s *Service) ListApprovals(ctx context.Context, taskID agentdomain.TaskID, caller gov.Context) ([]agentdomain.Approval, error) {
	if err := s.requireOwner(caller); err != nil {
		return nil, err
	}
	task, agent, err := s.authorizedTask(ctx, taskID, caller)
	if err != nil {
		return nil, err
	}
	if !agent.OwnedBy(string(s.owner.Kind()), s.owner.Reference()) {
		return nil, agentdomain.ErrForbidden
	}
	return s.store.ListTaskApprovals(ctx, task.ID)
}

// DecideApproval is Wails-owner-only in Phase B/C, not part of the
// Profile-exposed MCP Delegation tool suite.
func (s *Service) DecideApproval(ctx context.Context, id agentdomain.ApprovalID, choice agentdomain.ApprovalChoice, caller gov.Context) (agentdomain.Approval, error) {
	if err := s.requireOwner(caller); err != nil {
		return agentdomain.Approval{}, err
	}
	if !choice.Valid() {
		return agentdomain.Approval{}, agentdomain.ErrInvalid
	}
	approval, err := s.store.GetApproval(ctx, id)
	if err != nil {
		return agentdomain.Approval{}, err
	}
	task, agent, err := s.authorizedTask(ctx, approval.TaskID, caller)
	if err != nil {
		return agentdomain.Approval{}, err
	}
	if !agent.OwnedBy(string(s.owner.Kind()), s.owner.Reference()) ||
		approval.AgentID != task.AgentID || approval.Generation != task.Generation ||
		approval.Status != agentdomain.ApprovalPending ||
		task.Status != agentdomain.TaskRunning || task.CancelRequested || !approval.ExpiresAt.After(time.Now()) {
		return agentdomain.Approval{}, agentdomain.ErrConflict
	}
	s.mu.Lock()
	waiter := s.pending[id]
	active := s.active[task.ID] != nil && !s.closed
	s.mu.Unlock()
	if !active || waiter == nil {
		return agentdomain.Approval{}, agentdomain.ErrConflict
	}
	if choice == agentdomain.ApprovalAllowOnce && !approval.CanAllow {
		return agentdomain.Approval{}, agentdomain.ErrForbidden
	}
	// Re-evaluate current restrictive policy using the ORIGINAL submitting
	// caller, never the local owner's more privileged Principal as a substitute.
	original, err := approvalCaller(task)
	if err != nil {
		return agentdomain.Approval{}, err
	}
	if err := s.allowedForApproval(ctx, agent, original); err != nil {
		return agentdomain.Approval{}, err
	}
	if err := s.admit(ctx, original, agent, gov.ActionAgentTaskSubmit); err != nil {
		return agentdomain.Approval{}, err
	}
	status := agentdomain.ApprovalRejected
	if choice == agentdomain.ApprovalAllowOnce {
		status = agentdomain.ApprovalAllowedOnce
	}
	if err := s.store.ResolveApproval(ctx, id, status); err != nil {
		return agentdomain.Approval{}, err
	}
	// A buffered channel is deliberately nonblocking: expiry, cancellation
	// or shutdown may have detached the waiter concurrently with owner action.
	select {
	case waiter.decision <- choice:
	default:
	}
	approval.Status = status
	approval.ResolvedAt = time.Now().UTC()
	return approval, nil
}
func approvalCaller(task agentdomain.Task) (gov.Context, error) {
	principal, err := gov.NewPrincipal(gov.PrincipalKind(task.PrincipalKind), task.PrincipalRef)
	if err != nil {
		return gov.Context{}, err
	}
	scope := gov.CatalogScope()
	if task.ScopeKind == string(gov.ScopeProfile) {
		scope, err = gov.ProfileScope(task.ScopeRef)
		if err != nil {
			return gov.Context{}, err
		}
	} else if task.ScopeKind != string(gov.ScopeCatalog) {
		return gov.Context{}, fmt.Errorf("%w: invalid original Agent scope", agentdomain.ErrInvalid)
	}
	return gov.NewContext(gov.Origin(task.Origin), principal, scope)
}
func (s *Service) allowedForApproval(ctx context.Context, agent agentdomain.Agent, caller gov.Context) error {
	_, err := s.allowed(ctx, agent, caller)
	if err != nil {
		return err
	}
	return nil
}
