// Package application owns the Agent resource and Task lifecycle without
// implementing, supervising, or planning an Agent's own reasoning loop.
package application

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

const (
	maxActivePerAgent = 2
	TaskTimeout       = 10 * time.Minute
)

// Repository is the Agent consuming-side storage port.
type Repository interface {
	CreateAgent(context.Context, agentdomain.Agent) error
	GetAgent(context.Context, agentdomain.ID) (agentdomain.Agent, error)
	ListAgents(context.Context) ([]agentdomain.Agent, error)
	DeleteAgent(context.Context, agentdomain.ID) error
	GetTask(context.Context, agentdomain.TaskID) (agentdomain.Task, error)
	ListTasks(context.Context, agentdomain.ID) ([]agentdomain.Task, error)
	CreateAdmittedTask(context.Context, agentdomain.Task, int, string) error
	Transition(context.Context, agentdomain.TaskID, uint64, agentdomain.TaskStatus, agentdomain.TaskStatus, string, string) error
	RequestCancel(context.Context, agentdomain.TaskID, uint64) error
	HasProfileGrant(context.Context, agentdomain.ID, string) (bool, error)
	ListProfileGrants(context.Context, agentdomain.ID) ([]string, error)
	ListAdmissions(context.Context, agentdomain.ID, int) ([]agentdomain.AdmissionEvent, error)
	GrantProfile(context.Context, agentdomain.ID, string) error
	RevokeProfile(context.Context, agentdomain.ID, string) error
	SetDefaults(context.Context, agentdomain.ID, string, string) error
	BindWorkspace(context.Context, agentdomain.ID, string, string) error
	Workspace(context.Context, agentdomain.ID, string) (string, error)
	HasWorkspaceGrant(context.Context, agentdomain.ID, string, string) (bool, error)
	ListWorkspaces(context.Context, agentdomain.ID) ([]agentdomain.WorkspaceBinding, error)
	UnbindWorkspace(context.Context, agentdomain.ID, string) error
	GrantWorkspace(context.Context, agentdomain.ID, string, string) error
	RevokeWorkspace(context.Context, agentdomain.ID, string, string) error
}

// Executor is supplied by the ACP Backend adapter, not by the upstream Tool.
type Executor interface {
	Execute(context.Context, agentdomain.Agent, string) (string, error)
}

// ConfiguredExecutor runs one admitted immutable selection without changing Agent identity.
type ConfiguredExecutor interface {
	ExecuteConfigured(context.Context, agentdomain.Agent, string, RunConfig) (string, error)
}
type RunConfig struct {
	WorkspaceRoot   string
	Model           string
	ReasoningEffort string
}

// WorkspaceTarget contains inert metadata about a registered local Sandbox.
type WorkspaceTarget struct {
	ID        string
	Root      string
	Backend   string
	Isolation string
	OwnerKind string
	OwnerRef  string
}
type WorkspaceResolver func(context.Context, string) (WorkspaceTarget, error)
type Admission func(context.Context, gov.Context, agentdomain.Agent, gov.Action) error
type ProfileExists func(context.Context, string) error

type Service struct {
	store             Repository
	runner            Executor
	admit             Admission
	profileExists     ProfileExists
	workspaceResolver WorkspaceResolver
	owner             gov.Principal
	rootContext       context.Context
	rootCancel        context.CancelFunc
	mu                sync.Mutex
	active            map[agentdomain.TaskID]context.CancelFunc
	closed            bool
	wg                sync.WaitGroup
}

func NewService(store Repository, runner Executor, admit Admission, owner gov.Principal, profileExists ProfileExists) (*Service, error) {
	if store == nil || runner == nil || admit == nil || owner.Validate() != nil ||
		owner.Kind() != gov.PrincipalLocalUser {
		return nil, errors.New("agent service: missing trusted dependencies")
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{store: store, runner: runner, admit: admit, owner: owner,
		profileExists: profileExists, rootContext: ctx, rootCancel: cancel,
		active: map[agentdomain.TaskID]context.CancelFunc{}}, nil
}

func (s *Service) isOwner(caller gov.Context) bool {
	return caller.Validate() == nil && caller.Principal() == s.owner &&
		caller.Scope().Kind() == gov.ScopeCatalog &&
		(caller.Origin() == gov.OriginCLI || caller.Origin() == gov.OriginDesktop ||
			caller.Origin() == gov.OriginManagementHTTP || caller.Origin() == gov.OriginInternal)
}
func (s *Service) requireOwner(caller gov.Context) error {
	if !s.isOwner(caller) {
		return agentdomain.ErrForbidden
	}
	return nil
}
func (s *Service) isTrustedProfile(caller gov.Context) string {
	if caller.Validate() != nil || caller.Origin() != gov.OriginMCP ||
		caller.Principal().Kind() != gov.PrincipalService || caller.Scope().Kind() != gov.ScopeProfile {
		return ""
	}
	id := caller.Scope().Reference()
	if _, err := profiledomain.ParseProfileID(id); err != nil || caller.Principal().Reference() != "profile:"+id {
		return ""
	}
	return id
}
func (s *Service) allowed(ctx context.Context, agent agentdomain.Agent, caller gov.Context) (string, error) {
	if s.isOwner(caller) && agent.OwnedBy(string(s.owner.Kind()), s.owner.Reference()) {
		return "", nil
	}
	profileID := s.isTrustedProfile(caller)
	if profileID == "" {
		return "", agentdomain.ErrForbidden
	}
	grant, err := s.store.HasProfileGrant(ctx, agent.ID(), profileID)
	if err != nil {
		return "", err
	}
	if !grant {
		return "", agentdomain.ErrForbidden
	}
	return profileID, nil
}

type CreateRequest struct {
	Name                   string              `json:"name"`
	Backend                agentdomain.Backend `json:"backend"`
	WorkspaceRoot          string              `json:"workspaceRoot"`
	Isolation              string              `json:"isolation"`
	DefaultModel           string              `json:"defaultModel,omitempty"`
	DefaultReasoningEffort string              `json:"defaultReasoningEffort,omitempty"`
}

// Create stores one owner-selected backend and canonical local directory;
// unlike status listing, this may inspect the filesystem after owner admission.
func (s *Service) Create(ctx context.Context, request CreateRequest, caller gov.Context) (agentdomain.Agent, error) {
	if err := s.requireOwner(caller); err != nil {
		return agentdomain.Agent{}, err
	}
	if request.Backend.PluginID() == "" {
		return agentdomain.Agent{}, agentdomain.ErrInvalid
	}
	if err := agentdomain.ValidateModel(request.DefaultModel); err != nil {
		return agentdomain.Agent{}, err
	}
	if err := agentdomain.ValidateReasoningEffort(request.DefaultReasoningEffort); err != nil {
		return agentdomain.Agent{}, err
	}
	if !filepath.IsAbs(request.WorkspaceRoot) {
		return agentdomain.Agent{}, agentdomain.ErrInvalid
	}
	resolved, err := filepath.EvalSymlinks(request.WorkspaceRoot)
	if err != nil {
		return agentdomain.Agent{}, fmt.Errorf("%w: Agent workspace unavailable: %w", agentdomain.ErrInvalid, err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return agentdomain.Agent{}, fmt.Errorf("%w: Agent workspace stat: %w", agentdomain.ErrInvalid, err)
	}
	if !info.IsDir() {
		return agentdomain.Agent{}, agentdomain.ErrInvalid
	}
	id, err := agentdomain.NewID()
	if err != nil {
		return agentdomain.Agent{}, err
	}
	isolation := request.Isolation
	if isolation == "" {
		isolation = "restricted_host"
	}
	agent, err := agentdomain.NewAgent(agentdomain.Spec{ID: id, Name: request.Name, Backend: request.Backend,
		PluginID: request.Backend.PluginID(), ExtensionID: "main", WorkspaceRoot: filepath.Clean(resolved),
		Isolation: isolation, DefaultModel: request.DefaultModel, DefaultReasoningEffort: request.DefaultReasoningEffort, OwnerKind: string(s.owner.Kind()), OwnerRef: s.owner.Reference(),
		CreatedAt: time.Now().UTC()})
	if err != nil {
		return agentdomain.Agent{}, err
	}
	if err := s.store.CreateAgent(ctx, agent); err != nil {
		return agentdomain.Agent{}, err
	}
	return agent, nil
}
func (s *Service) List(ctx context.Context, caller gov.Context) ([]agentdomain.Agent, error) {
	if err := s.requireOwner(caller); err != nil {
		return nil, err
	}
	return s.store.ListAgents(ctx)
}
func (s *Service) Get(ctx context.Context, id agentdomain.ID, caller gov.Context) (agentdomain.Agent, error) {
	if err := s.requireOwner(caller); err != nil {
		return agentdomain.Agent{}, err
	}
	return s.store.GetAgent(ctx, id)
}
func (s *Service) Delete(ctx context.Context, id agentdomain.ID, caller gov.Context) error {
	if err := s.requireOwner(caller); err != nil {
		return err
	}
	return s.store.DeleteAgent(ctx, id)
}

func (s *Service) Grant(ctx context.Context, id agentdomain.ID, profileID string, caller gov.Context) error {
	if err := s.requireOwner(caller); err != nil {
		return err
	}
	if s.profileExists == nil {
		return agentdomain.ErrUnavailable
	}
	if _, err := profiledomain.ParseProfileID(profileID); err != nil {
		return agentdomain.ErrInvalid
	}
	a, err := s.store.GetAgent(ctx, id)
	if err != nil {
		return err
	}
	if !a.OwnedBy(string(s.owner.Kind()), s.owner.Reference()) {
		return agentdomain.ErrForbidden
	}
	if err := s.profileExists(ctx, profileID); err != nil {
		return fmt.Errorf("agent Profile grant: %w", err)
	}
	return s.store.GrantProfile(ctx, id, profileID)
}
func (s *Service) Revoke(ctx context.Context, id agentdomain.ID, profileID string, caller gov.Context) error {
	if err := s.requireOwner(caller); err != nil {
		return err
	}
	if _, err := profiledomain.ParseProfileID(profileID); err != nil {
		return agentdomain.ErrInvalid
	}
	if _, err := s.store.GetAgent(ctx, id); err != nil {
		return err
	}
	return s.store.RevokeProfile(ctx, id, profileID)
}
func (s *Service) Grants(ctx context.Context, id agentdomain.ID, caller gov.Context) ([]string, error) {
	if err := s.requireOwner(caller); err != nil {
		return nil, err
	}
	if _, err := s.store.GetAgent(ctx, id); err != nil {
		return nil, err
	}
	return s.store.ListProfileGrants(ctx, id)
}

// Admissions is an owner-only audit view, not an MCP task capability.
func (s *Service) Admissions(ctx context.Context, id agentdomain.ID, caller gov.Context, limit int) ([]agentdomain.AdmissionEvent, error) {
	if err := s.requireOwner(caller); err != nil {
		return nil, err
	}
	if _, err := s.store.GetAgent(ctx, id); err != nil {
		return nil, err
	}
	return s.store.ListAdmissions(ctx, id, limit)
}

type SubmitOptions struct {
	Model           string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoningEffort,omitempty"`
	Workspace       string `json:"workspace,omitempty"`
}

func (s *Service) Submit(ctx context.Context, id agentdomain.ID, prompt string, caller gov.Context) (agentdomain.Task, error) {
	return s.SubmitWithOptions(ctx, id, prompt, SubmitOptions{}, caller)
}
func (s *Service) SubmitWithOptions(ctx context.Context, id agentdomain.ID, prompt string, options SubmitOptions, caller gov.Context) (agentdomain.Task, error) {
	if caller.Validate() != nil {
		return agentdomain.Task{}, agentdomain.ErrForbidden
	}
	if len(prompt) > agentdomain.MaxPromptBytes || strings.TrimSpace(prompt) == "" {
		return agentdomain.Task{}, agentdomain.ErrInvalid
	}
	agent, err := s.store.GetAgent(ctx, id)
	if err != nil {
		return agentdomain.Task{}, err
	}
	profileID, err := s.allowed(ctx, agent, caller)
	if err != nil {
		return agentdomain.Task{}, err
	}
	if err := agentdomain.ValidateModel(options.Model); err != nil {
		return agentdomain.Task{}, err
	}
	model := options.Model
	if model == "" {
		model = agent.Spec().DefaultModel
	}
	if err := agentdomain.ValidateReasoningEffort(options.ReasoningEffort); err != nil {
		return agentdomain.Task{}, err
	}
	effort := options.ReasoningEffort
	if effort == "" {
		effort = agent.Spec().DefaultReasoningEffort
	}
	root := agent.Spec().WorkspaceRoot
	if options.Workspace != "" {
		if err := agentdomain.ValidateWorkspaceKey(options.Workspace); err != nil {
			return agentdomain.Task{}, err
		}
		if s.workspaceResolver == nil {
			return agentdomain.Task{}, agentdomain.ErrUnavailable
		}
		sandboxID, err := s.store.Workspace(ctx, id, options.Workspace)
		if err != nil {
			return agentdomain.Task{}, err
		}
		target, err := s.workspaceResolver(ctx, sandboxID)
		if err != nil {
			return agentdomain.Task{}, err
		}
		if err := s.validateTarget(agent, target); err != nil {
			return agentdomain.Task{}, err
		}
		if profileID != "" {
			granted, err := s.store.HasWorkspaceGrant(ctx, id, options.Workspace, profileID)
			if err != nil {
				return agentdomain.Task{}, err
			}
			if !granted {
				return agentdomain.Task{}, agentdomain.ErrForbidden
			}
		}
		root = target.Root
	}
	if (model != "" || effort != "" || options.Workspace != "") && s.runner == nil {
		return agentdomain.Task{}, agentdomain.ErrUnavailable
	}
	if model != "" || effort != "" || options.Workspace != "" {
		if _, ok := s.runner.(ConfiguredExecutor); !ok {
			return agentdomain.Task{}, agentdomain.ErrUnavailable
		}
	}
	// Admission is before process, CLI/account, workspace and credential access.
	for _, action := range []gov.Action{gov.ActionAgentActivate, gov.ActionAgentTaskSubmit} {
		if err := s.admit(ctx, caller, agent, action); err != nil {
			return agentdomain.Task{}, err
		}
	}
	taskID, err := agentdomain.NewTaskID()
	if err != nil {
		return agentdomain.Task{}, err
	}
	now := time.Now().UTC()
	task := agentdomain.Task{ID: taskID, AgentID: id, Status: agentdomain.TaskQueued,
		Model: model, ReasoningEffort: effort, WorkspaceKey: options.Workspace, WorkspaceRoot: root,
		Origin: string(caller.Origin()), PrincipalKind: string(caller.Principal().Kind()),
		PrincipalRef: caller.Principal().Reference(), ScopeKind: string(caller.Scope().Kind()),
		ScopeRef: caller.Scope().Reference(), Generation: 1, CreatedAt: now, UpdatedAt: now}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return agentdomain.Task{}, agentdomain.ErrUnavailable
	}
	if err := s.store.CreateAdmittedTask(ctx, task, maxActivePerAgent, profileID); err != nil {
		s.mu.Unlock()
		return agentdomain.Task{}, err
	}
	runCtx, cancel := context.WithTimeout(s.rootContext, TaskTimeout)
	s.active[taskID] = cancel
	s.wg.Add(1)
	s.mu.Unlock()
	go s.run(runCtx, cancel, agent, task, prompt)
	return task, nil
}

func (s *Service) run(ctx context.Context, cancel context.CancelFunc, agent agentdomain.Agent, task agentdomain.Task, prompt string) {
	defer s.wg.Done()
	defer func() {
		cancel()
		s.mu.Lock()
		delete(s.active, task.ID)
		s.mu.Unlock()
	}()
	// A concurrent cancellation may fence queued → running.
	if err := ctx.Err(); err != nil {
		_ = s.store.Transition(context.WithoutCancel(ctx), task.ID, task.Generation, agentdomain.TaskQueued, agentdomain.TaskCancelled, "", "")
		return
	}
	if err := s.store.Transition(context.WithoutCancel(ctx), task.ID, task.Generation, agentdomain.TaskQueued, agentdomain.TaskRunning, "", ""); err != nil {
		_ = s.store.Transition(context.WithoutCancel(ctx), task.ID, task.Generation, agentdomain.TaskQueued, agentdomain.TaskCancelled, "", "")
		return
	}
	var result string
	var err error
	if runner, ok := s.runner.(ConfiguredExecutor); ok {
		result, err = runner.ExecuteConfigured(ctx, agent, prompt, RunConfig{WorkspaceRoot: task.WorkspaceRoot, Model: task.Model, ReasoningEffort: task.ReasoningEffort})
	} else {
		result, err = s.runner.Execute(ctx, agent, prompt)
	}
	// A runner that reports success after cancellation cannot prove it
	// stopped executing side effects. Preserve the uncertainty instead.
	if ctx.Err() != nil && err == nil {
		_ = s.store.Transition(context.WithoutCancel(ctx), task.ID, task.Generation,
			agentdomain.TaskRunning, agentdomain.TaskUnknown, "", "Agent outcome unknown after cancellation")
		return
	}
	next := agentdomain.TaskCompleted
	diagnostic := ""
	if err != nil {
		next = agentdomain.TaskFailed
		diagnostic = "Agent execution failed"
		if errors.Is(err, context.Canceled) {
			// The request to stop can originate from the caller or from
			// Mender shutting down. Only a caller-initiated cancellation
			// has a persisted CancelRequested bit.
			persisted, readErr := s.store.GetTask(context.WithoutCancel(ctx), task.ID)
			if readErr == nil && persisted.CancelRequested {
				next = agentdomain.TaskCancelled
				diagnostic = "Task cancelled"
			} else {
				next = agentdomain.TaskUnknown
				diagnostic = "Agent outcome unknown after runtime shutdown"
			}
		}
		if errors.Is(err, context.DeadlineExceeded) {
			next = agentdomain.TaskUnknown
			diagnostic = "Agent outcome unknown after timeout"
		}
		result = ""
	}
	if len(result) > agentdomain.MaxResultBytes {
		next = agentdomain.TaskFailed
		result = ""
		diagnostic = "Agent output exceeded the permitted limit"
	}
	_ = s.store.Transition(context.WithoutCancel(ctx), task.ID, task.Generation, agentdomain.TaskRunning, next, result, diagnostic)
}
func (s *Service) authorizedTask(ctx context.Context, id agentdomain.TaskID, caller gov.Context) (agentdomain.Task, agentdomain.Agent, error) {
	task, err := s.store.GetTask(ctx, id)
	if err != nil {
		return task, agentdomain.Agent{}, err
	}
	agent, err := s.store.GetAgent(ctx, task.AgentID)
	if err != nil {
		return task, agent, err
	}
	if s.isOwner(caller) && agent.OwnedBy(string(s.owner.Kind()), s.owner.Reference()) {
		return task, agent, nil
	}
	profileID, err := s.allowed(ctx, agent, caller)
	if err != nil {
		return task, agent, err
	}
	if profileID == "" || !task.OwnedBy(string(caller.Principal().Kind()), caller.Principal().Reference(),
		string(caller.Scope().Kind()), caller.Scope().Reference()) {
		return task, agent, agentdomain.ErrForbidden
	}
	return task, agent, nil
}
func (s *Service) Task(ctx context.Context, id agentdomain.TaskID, caller gov.Context) (agentdomain.Task, error) {
	task, _, err := s.authorizedTask(ctx, id, caller)
	return task, err
}
func (s *Service) Tasks(ctx context.Context, id agentdomain.ID, caller gov.Context) ([]agentdomain.Task, error) {
	if err := s.requireOwner(caller); err != nil {
		return nil, err
	}
	if _, err := s.store.GetAgent(ctx, id); err != nil {
		return nil, err
	}
	return s.store.ListTasks(ctx, id)
}
func (s *Service) Cancel(ctx context.Context, id agentdomain.TaskID, caller gov.Context) error {
	task, agent, err := s.authorizedTask(ctx, id, caller)
	if err != nil {
		return err
	}
	if err := s.admit(ctx, caller, agent, gov.ActionAgentTaskCancel); err != nil {
		return err
	}
	if err := s.store.RequestCancel(ctx, id, task.Generation); err != nil {
		return err
	}
	s.mu.Lock()
	cancel := s.active[id]
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	// No claim of termination here. Only the owned runner may mark terminal
	// after observing child exit.
	return nil
}
func (s *Service) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.rootCancel()
	for _, cancel := range s.active {
		cancel()
	}
	s.mu.Unlock()
	s.wg.Wait()
	return nil
}
