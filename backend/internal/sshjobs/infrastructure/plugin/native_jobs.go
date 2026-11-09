package plugin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governance "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationapp "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocation "github.com/orz-i/Mender/backend/internal/invocation/domain"
	sandboxapp "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandbox "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sshremote "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sshremote"
	"github.com/orz-i/Mender/backend/internal/toolsuite/infrastructure/curated"
)

// NativeJobs executes the one host-owned native:jobs tool suite through the
// existing SSH Sandbox Job service. This is an adapter, not a Task scheduler,
// a Provider façade, or a second Invocation pipeline.
type NativeJobs struct {
	db   *sql.DB
	port Port
	get  func(context.Context, sandbox.ID) (sandbox.Sandbox, error)
}

func NewNativeJobs(ctx context.Context, db *sql.DB, port Port, get func(context.Context, sandbox.ID) (sandbox.Sandbox, error)) (*NativeJobs, error) {
	if db == nil || port == nil || get == nil {
		return nil, sandbox.ErrBackendUnavailable
	}
	for _, sqlQuery := range []string{
		"SELECT profile_id,tool_id,workspace_id,sandbox_id FROM profile_tool_target_scopes LIMIT 0",
		"SELECT sandbox_id,profile_id,capability FROM sandbox_profile_grants LIMIT 0",
	} {
		if _, err := db.ExecContext(ctx, sqlQuery); err != nil {
			return nil, fmt.Errorf("native SSH Jobs: typed authority unavailable: %w", err)
		}
	}
	return &NativeJobs{db: db, port: port, get: get}, nil
}

type nativeJobInput struct {
	SandboxID      string `json:"sandboxId"`
	IdempotencyKey string `json:"idempotencyKey"`
	Command        string `json:"command"`
	TaskID         string `json:"taskId"`
	Stream         string `json:"stream"`
	Cursor         int64  `json:"cursor"`
	Limit          int    `json:"limit"`
}

func nativeJobCapability(key string) (sandbox.Capability, error) {
	switch key {
	case SourceSubmit:
		return sandbox.CapabilityJobSubmit, nil
	case SourceStatus, SourceLogs, SourceResult:
		return sandbox.CapabilityJobObserve, nil
	case SourceCancel:
		return sandbox.CapabilityJobCancel, nil
	default:
		return "", sandbox.ErrCapabilityUnavailable
	}
}

func (n *NativeJobs) admit(ctx context.Context, tool catalog.Tool, args invocation.JSONValue, caller governance.Context) (nativeJobInput, sandbox.ID, error) {
	if n == nil || n.db == nil || n.port == nil || n.get == nil || caller.Validate() != nil {
		return nativeJobInput{}, "", sandbox.ErrNotAuthorized
	}
	source, err := catalog.NativeSource("jobs")
	if err != nil || tool.SourceRef() != source {
		return nativeJobInput{}, "", sandbox.ErrNotAuthorized
	}
	capability, err := nativeJobCapability(tool.SourceKey())
	if err != nil {
		return nativeJobInput{}, "", err
	}
	reviewed, err := curated.PlannedNativeTools()
	if err != nil {
		return nativeJobInput{}, "", err
	}
	valid := false
	for _, item := range reviewed {
		if item.ToolID == tool.ID() && item.Source == source && item.SourceKey == tool.SourceKey() {
			valid = true
			break
		}
	}
	if !valid {
		return nativeJobInput{}, "", sandbox.ErrNotAuthorized
	}
	var input nativeJobInput
	if strictDecode(args.Bytes(), &input) != nil {
		return nativeJobInput{}, "", sshremote.ErrJobInvalid
	}
	id, err := sandbox.ParseID(input.SandboxID)
	if err != nil {
		return nativeJobInput{}, "", sshremote.ErrJobInvalid
	}
	switch tool.SourceKey() {
	case SourceSubmit:
		job := sshremote.JobRequest{IdempotencyKey: input.IdempotencyKey, Command: input.Command}
		if err := job.Validate(); err != nil || input.TaskID != "" || input.Stream != "" || input.Cursor != 0 || input.Limit != 0 {
			return nativeJobInput{}, "", sshremote.ErrJobInvalid
		}
	case SourceStatus, SourceResult, SourceCancel:
		if !taskIDPattern.MatchString(input.TaskID) || input.IdempotencyKey != "" || input.Command != "" ||
			input.Stream != "" || input.Cursor != 0 || input.Limit != 0 {
			return nativeJobInput{}, "", sshremote.ErrJobInvalid
		}
	case SourceLogs:
		if !taskIDPattern.MatchString(input.TaskID) || input.IdempotencyKey != "" || input.Command != "" ||
			input.Cursor < 0 || input.Limit < 0 || input.Limit > 65536 {
			return nativeJobInput{}, "", sshremote.ErrJobInvalid
		}
		if input.Stream == "" {
			input.Stream = "stdout"
		}
		if input.Stream != "stdout" && input.Stream != "stderr" {
			return nativeJobInput{}, "", sshremote.ErrJobInvalid
		}
		if input.Limit == 0 {
			input.Limit = 4096
		}
	}
	item, err := n.get(ctx, id)
	if err != nil {
		return nativeJobInput{}, "", err
	}
	if item.Validate() != nil {
		return nativeJobInput{}, "", sandbox.ErrNotAuthorized
	}
	if profileID, ok := sandboxapp.ProfileServiceID(caller); ok {
		// The ToolBinding TargetScope is the preserved per-Sandbox authority;
		// a broad Sandbox grant does not permit invoking an unselected Tool.
		var allowed int
		err = n.db.QueryRowContext(ctx, `SELECT EXISTS(
 SELECT 1 FROM profile_tool_target_scopes s
 JOIN profile_tools b ON b.profile_id=s.profile_id AND b.tool_id=s.tool_id
 JOIN tools t ON t.id=s.tool_id
 JOIN sandbox_profile_grants g ON g.sandbox_id=s.sandbox_id AND g.profile_id=s.profile_id
 WHERE s.profile_id=? AND s.tool_id=? AND s.workspace_id='' AND s.sandbox_id=?
 AND t.source_kind='native' AND t.source_id='jobs'
 AND g.capability=?)`,
			profileID, tool.ID().String(), id.String(), string(capability)).Scan(&allowed)
		if err != nil || allowed != 1 {
			return nativeJobInput{}, "", sandbox.ErrNotAuthorized
		}
	} else if caller.Scope().Kind() != governance.ScopeCatalog ||
		caller.Principal().Kind() != governance.PrincipalLocalUser ||
		(caller.Origin() != governance.OriginCLI && caller.Origin() != governance.OriginDesktop &&
			caller.Origin() != governance.OriginManagementHTTP) ||
		!sandboxapp.IsOwner(caller, item) {
		return nativeJobInput{}, "", sandbox.ErrNotAuthorized
	}
	return input, id, nil
}

// Extract is inert: no SSH session, backend opening, job reservation, or
// credential resolution before the canonical Policy/Approval/Budget gate.
func (n *NativeJobs) Extract(ctx context.Context, tool catalog.Tool, args invocation.JSONValue, caller governance.Context) (invocationapp.GovernanceFactExtraction, error) {
	input, id, err := n.admit(ctx, tool, args, caller)
	if err != nil {
		return invocationapp.GovernanceFactExtraction{}, err
	}
	effects := []invocation.EffectFact{invocation.EffectFact(tool.SourceKey())}
	target, err := invocation.NewResourceFact("sandbox_id", id.String())
	if err != nil {
		return invocationapp.GovernanceFactExtraction{}, err
	}
	resources := []invocation.ResourceFact{target}
	if input.TaskID != "" {
		task, err := invocation.NewResourceFact("ssh_job_id", input.TaskID)
		if err != nil {
			return invocationapp.GovernanceFactExtraction{}, err
		}
		resources = append(resources, task)
	}
	facts, err := invocation.KnownGovernanceFacts(effects, resources)
	if err != nil {
		return invocationapp.GovernanceFactExtraction{}, err
	}
	return invocationapp.GovernanceFactExtraction{Facts: facts}, nil
}

// Execute rechecks exactly the approved ToolID/target grants immediately
// before handing the operation to the existing governed Sandbox Job service.
func (n *NativeJobs) Execute(ctx context.Context, request invocationapp.ExecutionRequest,
	_ invocationapp.CredentialResolver, _ invocationapp.ProgressReporter) (invocation.Outcome, error) {
	input, id, err := n.admit(ctx, request.Tool, request.Arguments, request.Context)
	if err != nil {
		return invocation.Outcome{}, err
	}
	// A different Sandbox/Job cannot be substituted between Policy and Execute.
	expected, err := n.Extract(ctx, request.Tool, request.Arguments, request.Context)
	if err != nil || expected.Facts.Validate() != nil || request.AdmittedFacts.Validate() != nil {
		return invocation.Outcome{}, sandbox.ErrNotAuthorized
	}
	if !sameNativeJobFacts(expected.Facts, request.AdmittedFacts) {
		return invocation.Outcome{}, sandbox.ErrNotAuthorized
	}
	switch request.Tool.SourceKey() {
	case SourceSubmit:
		snapshot, err := n.port.SubmitSSHJob(ctx, id, request.Context,
			sshremote.JobRequest{IdempotencyKey: input.IdempotencyKey, Command: input.Command})
		if err != nil && (!errors.Is(err, sshremote.ErrJobUnknown) || !taskIDPattern.MatchString(snapshot.TaskID)) {
			return invocation.Outcome{}, err
		}
		if snapshot.State == "" {
			snapshot.State = sshremote.JobUnknown
		}
		return complete(snapshot)
	case SourceStatus, SourceResult, SourceCancel:
		var snapshot sshremote.JobSnapshot
		switch request.Tool.SourceKey() {
		case SourceStatus:
			snapshot, err = n.port.SSHJobStatus(ctx, id, request.Context, input.TaskID)
		case SourceResult:
			snapshot, err = n.port.SSHJobResult(ctx, id, request.Context, input.TaskID)
		case SourceCancel:
			snapshot, err = n.port.SSHJobCancel(ctx, id, request.Context, input.TaskID)
		}
		if errors.Is(err, sshremote.ErrJobUnknown) {
			snapshot.TaskID, snapshot.State = input.TaskID, sshremote.JobUnknown
		} else if err != nil {
			return invocation.Outcome{}, err
		}
		if snapshot.State == "" {
			snapshot.State = sshremote.JobUnknown
		}
		return complete(snapshot)
	case SourceLogs:
		logs, err := n.port.SSHJobLogs(ctx, id, request.Context, input.TaskID, input.Stream, input.Cursor, input.Limit)
		if err != nil {
			return invocation.Outcome{}, err
		}
		return complete(logs)
	}
	return invocation.Outcome{}, sandbox.ErrCapabilityUnavailable
}

func sameNativeJobFacts(a, b invocation.GovernanceFacts) bool {
	// Preserve exactly the approved target and effects, not just ToolID.
	if a.Knowledge() != b.Knowledge() {
		return false
	}
	ae, be := a.Effects(), b.Effects()
	ar, br := a.Resources(), b.Resources()
	if len(ae) != len(be) || len(ar) != len(br) {
		return false
	}
	for i := range ae {
		if ae[i] != be[i] {
			return false
		}
	}
	for i := range ar {
		if ar[i].Kind() != br[i].Kind() || ar[i].Value() != br[i].Value() {
			return false
		}
	}
	return true
}

var _ invocationapp.ToolExecutor = (*NativeJobs)(nil)
var _ invocationapp.GovernanceFactExtractor = (*NativeJobs)(nil)
