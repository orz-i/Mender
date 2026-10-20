// Package delegationprovider projects a configured Agent resource into
// ordinary governed Mender Tools. It never runs an Agent during discovery.
package delegationprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

const ID providerplugin.ID = "io.mender.provider.agent-delegation"

const (
	ConfigVersion  = 1
	sourceDelegate = "agent.delegate"
	sourceStatus   = "agent.status"
	sourceResult   = "agent.result"
	sourceCancel   = "agent.cancel"
	sourceResume   = "agent.resume"
)

var configSchema = json.RawMessage(`{"type":"object","additionalProperties":false,
	"required":["agentId"],"properties":{"agentId":{"type":"string","pattern":"^agent_[0-9a-f]{32}$"}}}`)

type Config struct {
	AgentID string `json:"agentId"`
}

func decodeConfig(raw []byte) (Config, error) {
	var c Config
	if err := strictDecode(raw, &c); err != nil {
		return c, err
	}
	if _, err := agentdomain.ParseID(c.AgentID); err != nil {
		return c, err
	}
	return c, nil
}
func strictDecode(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var next any
	if err := decoder.Decode(&next); err != io.EOF {
		return fmt.Errorf("unexpected trailing JSON")
	}
	return nil
}

type Plugin struct {
	service *agentapplication.Service
	owner   gov.Principal
}

func New(service *agentapplication.Service, owner gov.Principal) *Plugin {
	return &Plugin{service: service, owner: owner}
}
func (*Plugin) Descriptor() providerplugin.Descriptor {
	return providerplugin.Descriptor{ID: ID, Version: "1.0.0", DisplayName: "Agent Delegation Tools",
		Description:   "Delegate tasks to one explicitly configured Agent and retrieve governed results",
		ConfigVersion: ConfigVersion, ConfigSchema: slices.Clone(configSchema),
		Capabilities: []string{sourceDelegate, sourceStatus, sourceResult, sourceCancel, sourceResume}}
}
func (p *Plugin) ownerContext() (gov.Context, error) {
	return gov.NewContext(gov.OriginInternal, p.owner, gov.CatalogScope())
}
func (p *Plugin) ValidateConfig(ctx context.Context, raw json.RawMessage) error {
	c, err := decodeConfig(raw)
	if err != nil {
		return err
	}
	if p == nil || p.service == nil {
		return agentdomain.ErrUnavailable
	}
	owner, err := p.ownerContext()
	if err != nil {
		return err
	}
	// Pure metadata lookup: no process, no credential resolution, no
	// filesystem reads and no task submission during Provider activation.
	_, err = p.service.Get(ctx, agentdomain.ID(c.AgentID), owner)
	return err
}
func (p *Plugin) InspectConfig(ctx context.Context, raw json.RawMessage) (providerplugin.ConfigMetadata, error) {
	if err := p.ValidateConfig(ctx, raw); err != nil {
		return providerplugin.ConfigMetadata{}, err
	}
	c, _ := decodeConfig(raw)
	return providerplugin.NewConfigMetadataWithSummary(nil, "agent:"+c.AgentID)
}
func (p *Plugin) Open(ctx context.Context, req providerplugin.OpenRequest) (providerplugin.Instance, error) {
	if req.ConfigVersion != ConfigVersion {
		return nil, fmt.Errorf("agent delegation config version %d unsupported", req.ConfigVersion)
	}
	if err := p.ValidateConfig(ctx, req.Config); err != nil {
		return nil, err
	}
	c, _ := decodeConfig(req.Config)
	owner, err := p.ownerContext()
	if err != nil {
		return nil, err
	}
	a, err := p.service.Get(ctx, agentdomain.ID(c.AgentID), owner)
	if err != nil {
		return nil, err
	}
	// Retain the existing canonical resume ToolID even for unsupported native
	// placements: a legacy Profile may refer to it. Its discovered metadata
	// must explain the real supported contract instead of promising recovery.
	unsupported := a.Spec().Isolation != "host"
	return &Instance{service: p.service, agentID: agentdomain.ID(c.AgentID), backend: a.Spec().Backend, resumeUnsupported: unsupported}, nil
}

type Instance struct {
	service           *agentapplication.Service
	agentID           agentdomain.ID
	backend           agentdomain.Backend
	resumeUnsupported bool // truthful metadata, never an authorization decision
}

func (*Instance) Close() error { return nil }

var submitInput = []byte(`{"type":"object","additionalProperties":false,"required":["prompt"],
	"properties":{"prompt":{"type":"string","minLength":1,"maxLength":65536},
	"model":{"type":"string","maxLength":128},"reasoningEffort":{"type":"string","maxLength":64,"pattern":"^[a-z0-9_-]*$"},"workspace":{"type":"string","minLength":1,"maxLength":64,"pattern":"^[a-z0-9_-]+$"},"timeoutMinutes":{"type":"integer","enum":[10,20,30]}}}`)
var resumeInput = []byte(`{"type":"object","additionalProperties":false,"required":["taskId","prompt"],
	"properties":{"taskId":{"type":"string","pattern":"^atask_[0-9a-f]{32}$"},
	"prompt":{"type":"string","minLength":1,"maxLength":65536}}}`)
var resumeOutput = []byte(`{"type":"object","additionalProperties":false,
	"required":["taskId","status","continuedFromTaskId"],
	"properties":{"taskId":{"type":"string"},"status":{"type":"string"},
	"continuedFromTaskId":{"type":"string"}}}`)
var taskInput = []byte(`{"type":"object","additionalProperties":false,"required":["taskId"],
	"properties":{"taskId":{"type":"string","pattern":"^atask_[0-9a-f]{32}$"}}}`)
var submitOutput = []byte(`{"type":"object","additionalProperties":false,"required":["taskId","status"],
	"properties":{"taskId":{"type":"string"},"status":{"type":"string"},"timeoutSeconds":{"type":"integer"}}}`)
var taskOutput = []byte(`{"type":"object","additionalProperties":false,"required":["taskId","status","cancelRequested"],
	"properties":{"taskId":{"type":"string"},"status":{"type":"string"},"cancelRequested":{"type":"boolean"},
	"result":{"type":"string"},"diagnostic":{"type":"string"},"timeoutSeconds":{"type":"integer","enum":[600,1200,1800]},
	"continuedFromTaskId":{"type":"string"},
	"observation":{"type":"object","additionalProperties":false,
	"properties":{"phase":{"type":"string"},"activity":{"type":"string"},
	"lastObservedAt":{"type":"string"},"lastNativeProgressAt":{"type":"string"},"lastWorkerHeartbeatAt":{"type":"string"},"sessionRecorded":{"type":"boolean"},
	"sessionRecoverability":{"type":"string"},"leaderReaped":{"type":"boolean"}}}}}`)
var cancelOutput = []byte(`{"type":"object","additionalProperties":false,"required":["taskId","cancelRequested"],
	"properties":{"taskId":{"type":"string"},"cancelRequested":{"type":"boolean"}}}`)

func (i *Instance) DiscoverTools(_ context.Context, _ catalogdomain.Provider) ([]catalogapplication.DiscoveredTool, error) {
	if i == nil {
		return nil, agentdomain.ErrUnavailable
	}
	resumeTitle := "Continue proven-complete ACP session"
	resumeDescription := "Continue only from an owned, completed ACP session with verified native capability and terminal evidence. Never replays unknown Tasks."
	resumeTags := []string{"agent", "session"}
	if !i.resumeUnsupported && i.backend == agentdomain.BackendAntigravity {
		resumeTitle = "Continue verified Agy conversation"
		resumeDescription = "Continue an owned, proven-complete Agy conversation. Its exact conversation ID must match the native init before a new prompt is sent; never retries an unknown Task."
	}
	if i.resumeUnsupported {
		resumeTitle = "Session continuation unavailable"
		resumeDescription = "Unavailable for this Agent backend/isolation. No safe native session restoration; never retries unknown Tasks."
		resumeTags = append(resumeTags, "unavailable")
	}
	return []catalogapplication.DiscoveredTool{
		{SourceKey: sourceDelegate, Name: "delegate", Title: "Delegate to Agent",
			Description: "Submit a bounded task to this configured Agent. Optional timeoutMinutes: 10 (default), 20 or 30. Returns TaskID; no automatic retry.",
			InputSchema: slices.Clone(submitInput), OutputSchema: slices.Clone(submitOutput), Tags: []string{"agent", "task"}},
		{SourceKey: sourceStatus, Name: "status", Title: "Check Agent task",
			Description: "Query one delegated task without reading its result.",
			InputSchema: slices.Clone(taskInput), OutputSchema: slices.Clone(taskOutput), Tags: []string{"agent", "task"}},
		{SourceKey: sourceResult, Name: "result", Title: "Read Agent task result",
			Description: "Read the bounded result and terminal state of this Agent's task.",
			InputSchema: slices.Clone(taskInput), OutputSchema: slices.Clone(taskOutput), Tags: []string{"agent", "task"}},
		{SourceKey: sourceCancel, Name: "cancel", Title: "Cancel Agent task",
			Description: "Request cancellation of an in-flight task; cancellation confirmation is asynchronous.",
			InputSchema: slices.Clone(taskInput), OutputSchema: slices.Clone(cancelOutput), Tags: []string{"agent", "task"}},
		{SourceKey: sourceResume, Name: "resume", Title: resumeTitle,
			Description: resumeDescription,
			InputSchema: slices.Clone(resumeInput), OutputSchema: slices.Clone(resumeOutput), Tags: resumeTags},
	}, nil
}
func complete(value any) (invocationdomain.Outcome, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return invocationdomain.Outcome{}, err
	}
	jsonValue, err := invocationdomain.NewJSONValue(raw)
	if err != nil {
		return invocationdomain.Outcome{}, err
	}
	return invocationdomain.Complete(jsonValue)
}
func (i *Instance) Execute(ctx context.Context, req invocationapplication.ExecutionRequest,
	_ invocationapplication.CredentialResolver, _ invocationapplication.ProgressReporter) (invocationdomain.Outcome, error) {
	if i == nil || i.service == nil {
		return invocationdomain.Outcome{}, agentdomain.ErrUnavailable
	}
	if err := req.Context.Validate(); err != nil {
		return invocationdomain.Outcome{}, err
	}
	switch req.Tool.SourceKey() {
	case sourceDelegate:
		var args struct {
			Prompt          string `json:"prompt"`
			Model           string `json:"model"`
			ReasoningEffort string `json:"reasoningEffort"`
			Workspace       string `json:"workspace"`
			TimeoutMinutes  int    `json:"timeoutMinutes"`
		}
		if err := strictDecode(req.Arguments.Bytes(), &args); err != nil {
			return invocationdomain.Outcome{}, err
		}
		task, err := i.service.SubmitWithOptions(ctx, i.agentID, args.Prompt, agentapplication.SubmitOptions{Model: args.Model, ReasoningEffort: args.ReasoningEffort, Workspace: args.Workspace, TimeoutMinutes: args.TimeoutMinutes}, req.Context)
		if err != nil {
			return invocationdomain.Outcome{}, err
		}
		return complete(map[string]any{"taskId": task.ID, "status": task.Status, "timeoutSeconds": int(task.ExecutionTimeout() / time.Second)})
	case sourceResume:
		var args struct {
			TaskID string `json:"taskId"`
			Prompt string `json:"prompt"`
		}
		if err := strictDecode(req.Arguments.Bytes(), &args); err != nil {
			return invocationdomain.Outcome{}, err
		}
		id, err := agentdomain.ParseTaskID(args.TaskID)
		if err != nil {
			return invocationdomain.Outcome{}, err
		}
		task, err := i.service.Resume(ctx, id, args.Prompt, i.agentID, req.Context)
		if errors.Is(err, agentdomain.ErrResumeUnavailable) {
			// The Service has already checked the caller and task ownership.
			// Do not leak arbitrary native Agent errors through the shared
			// Invocation diagnostic channel; this known restriction has a
			// fixed, safe message and never starts a replacement task.
			return invocationdomain.Failed(invocationdomain.FailureSourceTool,
				invocationdomain.FailureToolError,
				"Session continuation is unavailable for this task or Agent. No new task was created; do not replay the previous prompt.")
		}
		if err != nil {
			return invocationdomain.Outcome{}, err
		}
		return complete(map[string]any{"taskId": task.ID, "status": task.Status, "continuedFromTaskId": task.ContinuedFromTaskID})
	case sourceStatus, sourceResult, sourceCancel:
		var args struct {
			TaskID string `json:"taskId"`
		}
		if err := strictDecode(req.Arguments.Bytes(), &args); err != nil {
			return invocationdomain.Outcome{}, err
		}
		id, err := agentdomain.ParseTaskID(args.TaskID)
		if err != nil {
			return invocationdomain.Outcome{}, err
		}
		task, err := i.service.Task(ctx, id, req.Context)
		if err != nil {
			return invocationdomain.Outcome{}, err
		}
		// A Profile's ToolBinding to Agent A never authorizes Agent B's task,
		// even if both Agents are granted to the same Profile.
		if task.AgentID != i.agentID {
			return invocationdomain.Outcome{}, agentdomain.ErrForbidden
		}
		if req.Tool.SourceKey() == sourceCancel {
			if err := i.service.Cancel(ctx, id, req.Context); err != nil {
				return invocationdomain.Outcome{}, err
			}
			return complete(map[string]any{"taskId": id, "cancelRequested": true})
		}
		data := map[string]any{"taskId": id, "status": task.Status, "cancelRequested": task.CancelRequested, "timeoutSeconds": int(task.ExecutionTimeout() / time.Second)}
		if task.ContinuedFromTaskID != "" {
			data["continuedFromTaskId"] = task.ContinuedFromTaskID
		}
		if task.Observation != nil {
			data["observation"] = task.Observation
		}
		if req.Tool.SourceKey() == sourceResult {
			if task.Result != "" {
				data["result"] = task.Result
			}
			if task.Diagnostic != "" {
				data["diagnostic"] = task.Diagnostic
			}
		}
		return complete(data)
	default:
		return invocationdomain.Outcome{}, agentdomain.ErrInvalid
	}
}
func (p *Plugin) ExtractGovernanceFacts(_ context.Context, raw json.RawMessage, tool catalogdomain.Tool,
	_ invocationdomain.JSONValue) (invocationdomain.GovernanceFacts, error) {
	config, err := decodeConfig(raw)
	if err != nil {
		return invocationdomain.GovernanceFacts{}, err
	}
	switch tool.SourceKey() {
	case sourceDelegate, sourceStatus, sourceResult, sourceCancel, sourceResume:
	default:
		return invocationdomain.UnknownGovernanceFacts(), nil
	}
	resource, err := invocationdomain.NewResourceFact("agent_id", config.AgentID)
	if err != nil {
		return invocationdomain.GovernanceFacts{}, err
	}
	return invocationdomain.KnownGovernanceFacts(
		[]invocationdomain.EffectFact{invocationdomain.EffectFact(tool.SourceKey())},
		[]invocationdomain.ResourceFact{resource})
}

var _ providerplugin.Plugin = (*Plugin)(nil)
var _ providerplugin.Instance = (*Instance)(nil)
var _ providerplugin.GovernanceFactProvider = (*Plugin)(nil)
