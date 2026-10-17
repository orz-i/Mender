package application

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
type PermissionDecision struct {
	Choice agentdomain.ApprovalChoice
	// Only the trusted application service may set this after a current,
	// owner-authored, atomic approval-policy decision (never ACP metadata).
	AuthorizedByPolicy bool
}
type PermissionRequester func(context.Context, PermissionRequest) (PermissionDecision, error)
type pendingApproval struct {
	offers   chan *pendingApprovalHandoff // unbuffered, handoff requires a live request consumer
	done     chan struct{}                // closes when the ACP request leaves its waiter
	reserved bool                         // only one owner/policy delivery can commit
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
	if decision, accepted, err := s.tryAutomaticApproval(ctx, task, a); err != nil || accepted {
		return decision, err
	}
	waiter := &pendingApproval{offers: make(chan *pendingApprovalHandoff), done: make(chan struct{})}
	s.mu.Lock()
	if s.closed || s.active[task.ID] == nil {
		s.mu.Unlock()
		return none, agentdomain.ErrUnavailable
	}
	s.pending[approvalID] = waiter
	s.mu.Unlock()
	defer func() {
		close(waiter.done) // signal detachment before waiting for the Service mutex
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
	for {
		select {
		case <-ctx.Done():
			return none, ctx.Err()
		case <-timer.C:
			return none, agentdomain.ErrUnavailable
		case offer := <-waiter.offers:
			decision, retry, err := waitForApprovalCommit(ctx, timer.C, offer)
			if retry {
				// Policy changed while the owner was reviewing: the original
				// pending request remains live and eligible for fresh review.
				continue
			}
			return decision, err
		}
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
	var policyRevision int64 // zero for owner rejection; never grants authority
	if choice == agentdomain.ApprovalAllowOnce {
		if !approval.CanAllow || approval.Kind != "read" {
			return agentdomain.Approval{}, agentdomain.ErrForbidden
		}
		// Recheck the originally previewed lexical path. A workspace or
		// symlink changed since the ACP request must not be approved.
		if err := revalidateReadPreview(task.WorkspaceRoot, approval.Target); err != nil {
			return agentdomain.Approval{}, err
		}
		// An allow-once must satisfy the ORIGINAL submitting principal's
		// current grants and Policy; the owner is only the approver.
		original, err := approvalCaller(task)
		if err != nil {
			return agentdomain.Approval{}, err
		}
		// Capture Policy before admission, fence the same version in the
		// durable SQLite decision; a concurrent mutation cannot reuse the
		// earlier Policy check result.
		policyRevision, err = s.store.CurrentPolicyRevision(ctx)
		if err != nil {
			return agentdomain.Approval{}, err
		}
		if err := s.allowedForApproval(ctx, agent, original); err != nil {
			return agentdomain.Approval{}, err
		}
		if err := s.admit(ctx, original, agent, gov.ActionAgentTaskSubmit); err != nil {
			return agentdomain.Approval{}, err
		}
	}
	// A reject needs no Policy revision: it can never grant authority.
	// A positive decision must match its prior admission generation.
	// All decisions remain individually task bound and owner only.
	// Reject is always an owner action and cannot grant new authority.
	// It remains available after Profile/workspace grants are revoked.
	status := agentdomain.ApprovalRejected
	if choice == agentdomain.ApprovalAllowOnce {
		status = agentdomain.ApprovalAllowedOnce
	}
	if err := s.resolveWaitingApproval(ctx, id, task.ID, PermissionDecision{Choice: choice}, func(commitCtx context.Context) error {
		return s.store.ResolveApproval(commitCtx, id, status, policyRevision)
	}); err != nil {
		return agentdomain.Approval{}, err
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

// revalidateReadPreview is a last-moment cooperative boundary check, not
// filesystem isolation. OpenCode still executes its own native read and
// could deviate from reported ACP metadata in host mode.
func revalidateReadPreview(root, relative string) error {
	if relative == "" || filepath.IsAbs(relative) || filepath.Clean(relative) != relative ||
		relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) ||
		strings.ContainsAny(relative, "\r\n\x00") {
		return agentdomain.ErrForbidden
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil || canonicalRoot != root {
		return agentdomain.ErrForbidden
	}
	canonicalTarget, err := filepath.EvalSymlinks(filepath.Join(root, relative))
	if err != nil {
		return agentdomain.ErrForbidden
	}
	inside, err := filepath.Rel(root, canonicalTarget)
	if err != nil || inside == "." || inside == ".." ||
		strings.HasPrefix(inside, ".."+string(filepath.Separator)) ||
		filepath.IsAbs(inside) {
		return agentdomain.ErrForbidden
	}
	info, err := os.Stat(canonicalTarget)
	if err != nil || !info.Mode().IsRegular() {
		return agentdomain.ErrForbidden
	}
	return nil
}
