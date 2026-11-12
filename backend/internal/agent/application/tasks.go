package application

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
)

const (
	maxActivePerAgent = 2
	TaskTimeout       = agentdomain.DefaultTimeoutSeconds * time.Second
)

// Executor receives only one immutable, admitted task configuration.
// There is no legacy unconfigured execution path; even default tasks get
// the reviewed canonical working directory and model/effort snapshot.
type Executor interface {
	ExecuteConfigured(context.Context, agentdomain.Agent, string, RunConfig) (string, error)
}
type RunConfig struct {
	WorkspaceRoot       string
	Model               string
	ReasoningEffort     string
	PermissionRequester PermissionRequester
	ToolSupply          ToolSupply                             // optional backend-neutral, Task-scoped tool delivery
	AllowNativeYOLO     bool                                   // owner-approved, unexpired Antigravity trusted workspace only
	Resume              *agentdomain.SessionContinuation       // host-resolved, never from Tool arguments
	Observe             func(agentdomain.ExecutionEvent) error // trusted, Task-scoped evidence sink
	RecordEvent         func(agentdomain.TaskEventType, map[string]any) error
}

// ToolSupplyResolver is the trusted Runtime's optional, backend-neutral
// capability issuer. It is never populated by a model/Tool request.
type ToolSupplyResolver func(context.Context, agentdomain.Task) (ToolSupply, func(), error)

// nativeApprovalUseCases is the Task service's consuming-side permission
// port. Task lifecycle storage does not expose owner approval-policy writes.
type nativeApprovalUseCases interface {
	requestPermission(context.Context, agentdomain.Task, gov.Context, PermissionRequest) (PermissionDecision, error)
	nativeYOLOAllowed(context.Context, agentdomain.Task) (bool, error)
}

// TaskService owns admission, Task/Session execution, evidence and cancellation.
// Approval waits and cancellation share one runtimeCoordinator instance.
type TaskService struct {
	*taskAccess
	*runtimeCoordinator
	*localWorkspaceRules
	store       TaskRepository
	runner      Executor
	admit       Admission
	approvals   nativeApprovalUseCases
	broadcaster *TaskEventBroadcaster
	taskSeqs    sync.Map
}

type SubmitOptions struct {
	Title           string `json:"title,omitempty"`
	Model           string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoningEffort,omitempty"`
	Workspace       string `json:"workspace,omitempty"`
	TimeoutMinutes  int    `json:"timeoutMinutes,omitempty"` // optional 10/20/30, only future Tasks
}

func (s *TaskService) Submit(ctx context.Context, id agentdomain.ID, prompt string, caller gov.Context) (agentdomain.Task, error) {
	return s.SubmitWithOptions(ctx, id, prompt, SubmitOptions{}, caller)
}
func (s *TaskService) SubmitWithOptions(ctx context.Context, id agentdomain.ID, prompt string, options SubmitOptions, caller gov.Context) (agentdomain.Task, error) {
	if caller.Validate() != nil {
		return agentdomain.Task{}, agentdomain.ErrForbidden
	}
	if len(prompt) > agentdomain.MaxPromptBytes || strings.TrimSpace(prompt) == "" {
		return agentdomain.Task{}, agentdomain.ErrInvalid
	}
	if err := agentdomain.ValidateTaskTitle(options.Title); err != nil {
		return agentdomain.Task{}, err
	}
	agent, err := s.store.GetAgent(ctx, id)
	if err != nil {
		return agentdomain.Task{}, err
	}
	profileID, err := s.allowed(ctx, agent, caller)
	if err != nil {
		return agentdomain.Task{}, err
	}
	if options.TimeoutMinutes != 0 && options.TimeoutMinutes != 10 &&
		options.TimeoutMinutes != 20 && options.TimeoutMinutes != 30 {
		return agentdomain.Task{}, agentdomain.ErrInvalid
	}
	seconds := options.TimeoutMinutes * 60
	if seconds == 0 {
		seconds = agentdomain.DefaultTimeoutSeconds
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
		registeredRoot, err := s.store.Workspace(ctx, id, options.Workspace)
		if err != nil {
			return agentdomain.Task{}, err
		}
		// Registration does not authorize an arbitrary task-supplied path.
		// Verify the stored canonical directory before task admission.
		if err := s.validateRegisteredWorkspaceRoot(ctx, registeredRoot); err != nil {
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
		root = registeredRoot
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
		Title: options.Title, Model: model, ReasoningEffort: effort, TimeoutSeconds: seconds, WorkspaceKey: options.Workspace, WorkspaceRoot: root,
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
	if evtID, err := agentdomain.NewTaskEventID(); err == nil {
		_ = s.RecordTaskEvent(ctx, agentdomain.TaskEvent{
			ID:        evtID,
			TaskID:    taskID,
			Seq:       1,
			Type:      agentdomain.TaskEventUserPrompt,
			Payload:   map[string]any{"prompt": prompt},
			CreatedAt: now,
		})
	}
	runCtx, cancel := context.WithTimeout(s.rootContext, task.ExecutionTimeout())
	s.active[taskID] = cancel
	s.wg.Add(1)
	s.mu.Unlock()
	go s.run(runCtx, cancel, agent, task, prompt, caller, nil)
	return task, nil
}

// TaskLocallyActive fences a Task to the exact in-memory Runtime that
// admitted it. Other processes may read the same SQLite Task history, but
// they cannot mint governed Tool capabilities for an orphan/stale Task.
// ConfigureToolSupplyResolver is a one-time bootstrap operation before any
// Task can start. No user/Plugin API can replace the resolver at runtime.
func (s *TaskService) ConfigureToolSupplyResolver(resolve ToolSupplyResolver) error {
	if resolve == nil {
		return agentdomain.ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.toolSupplyResolver != nil || len(s.active) != 0 {
		return agentdomain.ErrConflict
	}
	s.toolSupplyResolver = resolve
	return nil
}
func (s *TaskService) TaskLocallyActive(id agentdomain.TaskID) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.closed && s.active[id] != nil
}

func (s *TaskService) run(ctx context.Context, cancel context.CancelFunc, agent agentdomain.Agent, task agentdomain.Task, prompt string, caller gov.Context, continuation *agentdomain.SessionContinuation) {
	defer s.wg.Done()
	defer func() {
		cancel()
		s.mu.Lock()
		delete(s.active, task.ID)
		s.mu.Unlock()
	}()
	recordEvent := func(eventType agentdomain.TaskEventType, payload map[string]any) error {
		return s.RecordTaskEvent(context.WithoutCancel(ctx), agentdomain.TaskEvent{
			TaskID:    task.ID,
			Type:      eventType,
			Payload:   payload,
			CreatedAt: time.Now().UTC(),
		})
	}
	// A concurrent cancellation may fence queued → running.
	if err := ctx.Err(); err != nil {
		_ = s.store.Transition(context.WithoutCancel(ctx), task.ID, task.Generation, agentdomain.TaskQueued, agentdomain.TaskCancelled, "", "")
		_ = recordEvent(agentdomain.TaskEventTerminal, map[string]any{"status": string(agentdomain.TaskCancelled), "diagnostic": "Task cancelled"})
		return
	}
	if err := s.store.Transition(context.WithoutCancel(ctx), task.ID, task.Generation, agentdomain.TaskQueued, agentdomain.TaskRunning, "", ""); err != nil {
		_ = s.store.Transition(context.WithoutCancel(ctx), task.ID, task.Generation, agentdomain.TaskQueued, agentdomain.TaskCancelled, "", "")
		_ = recordEvent(agentdomain.TaskEventTerminal, map[string]any{"status": string(agentdomain.TaskCancelled), "diagnostic": "Task cancelled"})
		return
	}
	// Reservation is durable before resolving capabilities or starting a CLI.
	// Failure to record provenance fails closed, without launching the Agent.
	if err := s.store.BeginAttempt(context.WithoutCancel(ctx), task, s.runtimeInstance); err != nil {
		// A caller can cancel after queued→running but before the durable
		// reservation. No process was started, so preserve cancellation.
		next, diagnostic := agentdomain.TaskFailed, "Agent execution evidence unavailable"
		if observed, readErr := s.store.GetTask(context.WithoutCancel(ctx), task.ID); readErr == nil && observed.CancelRequested {
			next, diagnostic = agentdomain.TaskCancelled, "Task cancelled"
		}
		_ = s.store.Transition(context.WithoutCancel(ctx), task.ID, task.Generation,
			agentdomain.TaskRunning, next, "", diagnostic)
		_ = recordEvent(agentdomain.TaskEventTerminal, map[string]any{"status": string(next), "diagnostic": diagnostic})
		return
	}
	config := RunConfig{WorkspaceRoot: task.WorkspaceRoot, Model: task.Model, ReasoningEffort: task.ReasoningEffort,
		Resume: continuation,
		Observe: func(event agentdomain.ExecutionEvent) error {
			writeCtx, done := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
			defer done()
			return s.store.RecordExecution(writeCtx, task, s.runtimeInstance, event)
		},
		RecordEvent: recordEvent,
		PermissionRequester: func(ctx context.Context, req PermissionRequest) (PermissionDecision, error) {
			return s.approvals.requestPermission(ctx, task, caller, req)
		}}
	// Runtime heartbeat reports only that this task's local supervisory worker
	// remains alive. It is separate from native Agent progress or completion
	// evidence; a stale/failed heartbeat never authorizes replay or recovery.
	heartbeatStop := make(chan struct{})
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatStop:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = config.Observe(agentdomain.ExecutionEvent{Kind: agentdomain.EventWorkerHeartbeat})
			}
		}
	}()
	defer func() { close(heartbeatStop); <-heartbeatDone }()
	var result string
	var err error
	// Antigravity has no ACP permission callback. The existing per-Agent,
	// per-workspace 24h trusted grant is the ONLY authority for headless YOLO.
	// Decide at task launch: revoking a grant affects future processes, not
	// already-running native actions. No Profile or model may set this flag.
	if agent.Spec().Backend == agentdomain.BackendAntigravity {
		config.AllowNativeYOLO, err = s.approvals.nativeYOLOAllowed(ctx, task)
	}
	supplyFailed := false
	if err == nil && s.toolSupplyResolver != nil {
		var supply ToolSupply
		var release func()
		supply, release, err = s.toolSupplyResolver(ctx, task)
		if err != nil {
			supplyFailed = true
		} else {
			if release != nil {
				defer release()
			}
			config.ToolSupply = supply
		}
	}
	if err == nil {
		result, err = s.runner.ExecuteConfigured(ctx, agent, prompt, config)
	}
	// A runner that reports success after cancellation cannot prove it
	// stopped executing side effects. Preserve the uncertainty instead.
	if ctx.Err() != nil && err == nil {
		_ = s.store.Transition(context.WithoutCancel(ctx), task.ID, task.Generation,
			agentdomain.TaskRunning, agentdomain.TaskUnknown, "", "Agent outcome unknown after cancellation")
		_ = recordEvent(agentdomain.TaskEventTerminal, map[string]any{"status": string(agentdomain.TaskUnknown), "diagnostic": "Agent outcome unknown after cancellation"})
		return
	}
	next := agentdomain.TaskCompleted
	diagnostic := ""
	if err != nil {
		next = agentdomain.TaskFailed
		diagnostic = agentFailureDiagnostic(err)
		if supplyFailed {
			diagnostic = "Governed tool execution unavailable"
		}
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
		if errors.Is(err, agentdomain.ErrOutcomeUncertain) {
			next = agentdomain.TaskUnknown
			diagnostic = "Agent outcome unknown after execution evidence loss"
		}
		result = ""
	}
	if len(result) > agentdomain.MaxResultBytes {
		next = agentdomain.TaskFailed
		result = ""
		diagnostic = "Agent output exceeded the permitted limit"
	}
	// A runner's nil error is not native completion proof. The durable Task
	// transition independently requires terminal_seen and leader_reaped for
	// this exact generation; a rejected success must remain uncertain instead
	// of leaving a finished worker reported as running with an asserted result.
	writeCtx := context.WithoutCancel(ctx)
	if transitionErr := s.store.Transition(writeCtx, task.ID, task.Generation,
		agentdomain.TaskRunning, next, result, diagnostic); transitionErr != nil && next == agentdomain.TaskCompleted {
		_ = s.store.Transition(writeCtx, task.ID, task.Generation, agentdomain.TaskRunning,
			agentdomain.TaskUnknown, "", "Agent outcome unknown after execution evidence loss")
	}
	terminalPayload := map[string]any{
		"status": string(next),
	}
	if result != "" {
		terminalPayload["result"] = result
	}
	if diagnostic != "" {
		terminalPayload["diagnostic"] = diagnostic
	}
	_ = recordEvent(agentdomain.TaskEventTerminal, terminalPayload)
}
func (s *TaskService) Task(ctx context.Context, id agentdomain.TaskID, caller gov.Context) (agentdomain.Task, error) {
	task, _, err := s.authorizedTask(ctx, id, caller)
	if err != nil {
		return task, err
	}
	return s.attachObservation(ctx, task)
}

func (s *TaskService) attachObservation(ctx context.Context, task agentdomain.Task) (agentdomain.Task, error) {
	obs, err := s.store.TaskObservation(ctx, task.ID)
	if err != nil {
		return task, err
	}
	if obs != nil && task.Status == agentdomain.TaskRunning && s.TaskLocallyActive(task.ID) &&
		obs.Phase != "reserved" && !obs.LeaderReaped {
		obs.Activity = "local_active"
	}
	task.Observation = obs
	parent, err := s.store.TaskParent(ctx, task.ID)
	if err != nil {
		return task, err
	}
	task.ContinuedFromTaskID = parent
	return task, nil
}
func (s *TaskService) Tasks(ctx context.Context, id agentdomain.ID, caller gov.Context) ([]agentdomain.Task, error) {
	if err := s.requireOwner(caller); err != nil {
		return nil, err
	}
	if _, err := s.store.GetAgent(ctx, id); err != nil {
		return nil, err
	}
	tasks, err := s.store.ListTasks(ctx, id)
	if err != nil {
		return nil, err
	}
	for i := range tasks {
		if tasks[i], err = s.attachObservation(ctx, tasks[i]); err != nil {
			return nil, err
		}
	}
	return tasks, nil
}
func (s *TaskService) Cancel(ctx context.Context, id agentdomain.TaskID, caller gov.Context) error {
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
func (s *TaskService) Close() error {
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

func (s *TaskService) nextEventSeq(ctx context.Context, taskID agentdomain.TaskID) int64 {
	val, loaded := s.taskSeqs.Load(taskID)
	if !loaded {
		var maxSeq int64
		if events, err := s.store.ListTaskEvents(ctx, taskID); err == nil && len(events) > 0 {
			maxSeq = events[len(events)-1].Seq
		}
		seq := &atomic.Int64{}
		seq.Store(maxSeq)
		actual, _ := s.taskSeqs.LoadOrStore(taskID, seq)
		val = actual
	}
	return val.(*atomic.Int64).Add(1)
}

func (s *TaskService) trackEventSeq(taskID agentdomain.TaskID, seq int64) {
	val, _ := s.taskSeqs.LoadOrStore(taskID, &atomic.Int64{})
	counter := val.(*atomic.Int64)
	for {
		curr := counter.Load()
		if seq <= curr {
			break
		}
		if counter.CompareAndSwap(curr, seq) {
			break
		}
	}
}

func (s *TaskService) RecordTaskEvent(ctx context.Context, event agentdomain.TaskEvent) error {
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	if event.ID == "" {
		id, err := agentdomain.NewTaskEventID()
		if err != nil {
			return err
		}
		event.ID = id
	}
	if event.Seq <= 0 {
		event.Seq = s.nextEventSeq(ctx, event.TaskID)
	} else {
		s.trackEventSeq(event.TaskID, event.Seq)
	}
	if err := s.store.RecordTaskEvent(ctx, event); err != nil {
		return err
	}
	if s.broadcaster != nil {
		s.broadcaster.Broadcast(event)
	}
	return nil
}

func (s *TaskService) SubscribeTaskEvents(ctx context.Context, id agentdomain.TaskID, caller gov.Context) (<-chan agentdomain.TaskEvent, func(), error) {
	if _, _, err := s.authorizedTask(ctx, id, caller); err != nil {
		return nil, nil, err
	}
	if s.broadcaster == nil {
		return nil, nil, agentdomain.ErrUnavailable
	}
	ch, unsub := s.broadcaster.Subscribe(id)
	go func() {
		select {
		case <-ctx.Done():
			unsub()
		}
	}()
	return ch, unsub, nil
}

func (s *TaskService) GetTaskTranscript(ctx context.Context, id agentdomain.TaskID, caller gov.Context) ([]agentdomain.TaskEvent, error) {
	if _, _, err := s.authorizedTask(ctx, id, caller); err != nil {
		return nil, err
	}
	return s.store.ListTaskEvents(ctx, id)
}
