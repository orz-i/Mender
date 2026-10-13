package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"

	catalogapp "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationapp "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sshremote "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sshremote"
)

const (
	SourceSubmit = "jobs.submit"
	SourceStatus = "jobs.status"
	SourceLogs   = "jobs.logs"
	SourceResult = "jobs.result"
	SourceCancel = "jobs.cancel"
)

var taskIDPattern = regexp.MustCompile(`^sjob_[0-9a-f]{32}$`)

// Port is implemented by the governed SSH Sandbox host service. The Provider
// facade owns NO TaskIDs, remote process, credentials, Scheduler or task table.
// Every call carries the canonical trusted Invocation Context, never a caller-
// selectable Principal or SandboxID from tool arguments.
type Port interface {
	VerifySSHJobResource(context.Context, sandboxdomain.ID) error
	SubmitSSHJob(context.Context, sandboxdomain.ID, gov.Context, sshremote.JobRequest) (sshremote.JobSnapshot, error)
	SSHJobStatus(context.Context, sandboxdomain.ID, gov.Context, string) (sshremote.JobSnapshot, error)
	SSHJobResult(context.Context, sandboxdomain.ID, gov.Context, string) (sshremote.JobSnapshot, error)
	SSHJobLogs(context.Context, sandboxdomain.ID, gov.Context, string, string, int64, int) (sshremote.JobLogs, error)
	SSHJobCancel(context.Context, sandboxdomain.ID, gov.Context, string) (sshremote.JobSnapshot, error)
}

type Plugin struct{ port Port }

func New(port Port) *Plugin                           { return &Plugin{port: port} }
func (*Plugin) Descriptor() providerplugin.Descriptor { return Descriptor() }
func (p *Plugin) ValidateConfig(ctx context.Context, raw json.RawMessage) error {
	cfg, err := DecodeConfig(raw)
	if err != nil {
		return err
	}
	if p == nil || p.port == nil {
		return sandboxdomain.ErrBackendUnavailable
	}
	return p.port.VerifySSHJobResource(ctx, sandboxdomain.ID(cfg.SandboxID))
}
func (*Plugin) InspectConfig(ctx context.Context, raw json.RawMessage) (providerplugin.ConfigMetadata, error) {
	return InspectConfig(ctx, raw)
}
func (p *Plugin) Open(_ context.Context, request providerplugin.OpenRequest) (providerplugin.Instance, error) {
	if p == nil || p.port == nil || request.ConfigVersion != ConfigVersion {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	cfg, err := DecodeConfig(request.Config)
	if err != nil {
		return nil, err
	}
	// Opening/Discovering a Provider never dials SSH or claims a Sandbox lease.
	return &Instance{sandboxID: sandboxdomain.ID(cfg.SandboxID), port: p.port, tools: discoveredJobTools()}, nil
}

type Instance struct {
	sandboxID sandboxdomain.ID
	port      Port
	tools     []catalogapp.DiscoveredTool
}

func (i *Instance) DiscoverTools(_ context.Context, _ catalogdomain.Provider) ([]catalogapp.DiscoveredTool, error) {
	if i == nil {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	out := make([]catalogapp.DiscoveredTool, 0, len(i.tools))
	for _, t := range i.tools {
		out = append(out, catalogapp.DiscoveredTool{SourceKey: t.SourceKey, Name: t.Name, Title: t.Title, Description: t.Description,
			InputSchema: slices.Clone(t.InputSchema), OutputSchema: slices.Clone(t.OutputSchema), Tags: slices.Clone(t.Tags)})
	}
	return out, nil
}
func (*Instance) Close() error { return nil }
func discoveredJobTools() []catalogapp.DiscoveredTool {
	return []catalogapp.DiscoveredTool{
		{SourceKey: SourceSubmit, Name: "submit", Title: "Submit durable SSH job",
			Description: "Submit one bounded command to this Sandbox with a stable idempotency key. Detached systemd execution; unknown remote ACK is never automatically replayed.",
			InputSchema: submitInputSchema, OutputSchema: taskOutputSchema, Tags: []string{"jobs", "ssh", "execute"}},
		{SourceKey: SourceStatus, Name: "status", Title: "Observe SSH job status",
			Description: "Read plugin-owned TaskID status without taking an exclusive Sandbox lease, including when prior submission was uncertain.",
			InputSchema: taskInputSchema, OutputSchema: taskOutputSchema, Tags: []string{"jobs", "ssh", "observe"}},
		{SourceKey: SourceLogs, Name: "logs", Title: "Read SSH job output",
			Description: "Read a bounded stdout/stderr chunk at a cursor; caller explicitly handles pagination and output sensitivity.",
			InputSchema: logInputSchema, OutputSchema: logOutputSchema, Tags: []string{"jobs", "ssh", "observe"}},
		{SourceKey: SourceResult, Name: "result", Title: "Inspect SSH job result",
			Description: "Read durable terminal result or report running/unknown; no automatic remote command replay.",
			InputSchema: taskInputSchema, OutputSchema: taskOutputSchema, Tags: []string{"jobs", "ssh", "observe"}},
		{SourceKey: SourceCancel, Name: "cancel", Title: "Request SSH job cancellation",
			Description: "Request systemd stop with a separate permission. Acceptance does not mean confirmed terminal cancellation.",
			InputSchema: taskInputSchema, OutputSchema: taskOutputSchema, Tags: []string{"jobs", "ssh", "cancel"}},
	}
}
func strictDecode(raw []byte, target any) error {
	if len(raw) > 1<<20 {
		return sshremote.ErrJobInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return sshremote.ErrJobInvalid
	}
	var rest any
	if err := decoder.Decode(&rest); err != io.EOF {
		return sshremote.ErrJobInvalid
	}
	return nil
}
func complete(value any) (invocationdomain.Outcome, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return invocationdomain.Outcome{}, err
	}
	payload, err := invocationdomain.NewJSONValue(raw)
	if err != nil {
		return invocationdomain.Outcome{}, err
	}
	return invocationdomain.Complete(payload)
}
func (i *Instance) Execute(ctx context.Context, request invocationapp.ExecutionRequest, _ invocationapp.CredentialResolver, _ invocationapp.ProgressReporter) (invocationdomain.Outcome, error) {
	if i == nil || i.port == nil {
		return invocationdomain.Outcome{}, sandboxdomain.ErrBackendUnavailable
	}
	if err := request.Context.Validate(); err != nil {
		return invocationdomain.Outcome{}, err
	}
	var output sshremote.JobSnapshot
	var err error
	switch request.Tool.SourceKey() {
	case SourceSubmit:
		var input struct {
			IdempotencyKey string `json:"idempotencyKey"`
			Command        string `json:"command"`
		}
		if e := strictDecode(request.Arguments.Bytes(), &input); e != nil {
			return invocationdomain.Outcome{}, e
		}
		job := sshremote.JobRequest{IdempotencyKey: input.IdempotencyKey, Command: input.Command}
		if e := job.Validate(); e != nil {
			return invocationdomain.Outcome{}, e
		}
		output, err = i.port.SubmitSSHJob(ctx, i.sandboxID, request.Context, job)
	case SourceStatus, SourceResult, SourceCancel:
		var input struct {
			TaskID string `json:"taskId"`
		}
		if e := strictDecode(request.Arguments.Bytes(), &input); e != nil || !taskIDPattern.MatchString(input.TaskID) {
			return invocationdomain.Outcome{}, sshremote.ErrJobInvalid
		}
		switch request.Tool.SourceKey() {
		case SourceStatus:
			output, err = i.port.SSHJobStatus(ctx, i.sandboxID, request.Context, input.TaskID)
		case SourceResult:
			output, err = i.port.SSHJobResult(ctx, i.sandboxID, request.Context, input.TaskID)
		case SourceCancel:
			output, err = i.port.SSHJobCancel(ctx, i.sandboxID, request.Context, input.TaskID)
		}
		if errors.Is(err, sshremote.ErrJobUnknown) {
			output.TaskID = input.TaskID
			output.State = sshremote.JobUnknown
		}
	case SourceLogs:
		var input struct {
			TaskID string `json:"taskId"`
			Stream string `json:"stream"`
			Cursor int64  `json:"cursor"`
			Limit  int    `json:"limit"`
		}
		if e := strictDecode(request.Arguments.Bytes(), &input); e != nil || !taskIDPattern.MatchString(input.TaskID) || input.Cursor < 0 || input.Limit < 0 || input.Limit > 65536 {
			return invocationdomain.Outcome{}, sshremote.ErrJobInvalid
		}
		if input.Stream == "" {
			input.Stream = "stdout"
		}
		if input.Stream != "stdout" && input.Stream != "stderr" {
			return invocationdomain.Outcome{}, sshremote.ErrJobInvalid
		}
		if input.Limit == 0 {
			input.Limit = 4096
		}
		result, e := i.port.SSHJobLogs(ctx, i.sandboxID, request.Context, input.TaskID, input.Stream, input.Cursor, input.Limit)
		if e != nil {
			return invocationdomain.Outcome{}, e
		}
		return complete(result)
	default:
		return invocationdomain.Outcome{}, sandboxdomain.ErrCapabilityUnavailable
	}
	// Lost ACK/remote transport can still yield a trusted TaskID; JSON Outcome
	// exposes unknown safely to regular MCP clients without native MCP Tasks.
	if err != nil && (!errors.Is(err, sshremote.ErrJobUnknown) || !taskIDPattern.MatchString(output.TaskID)) {
		return invocationdomain.Outcome{}, fmt.Errorf("SSH job operation unavailable: %w", err)
	}
	if output.State == "" {
		output.State = sshremote.JobUnknown
	}
	return complete(output)
}

var _ providerplugin.Plugin = (*Plugin)(nil)
var _ providerplugin.Instance = (*Instance)(nil)
