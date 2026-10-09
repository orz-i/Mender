package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	sandboxapplication "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// Plugin is a statically discoverable Tool facade over an explicitly bound Sandbox.
// It neither creates nor opens a Sandbox during Provider activation.
type Plugin struct {
	descriptor providerplugin.Descriptor
	router     sandboxapplication.ExecutionRouter
}

func New(router sandboxapplication.ExecutionRouter) *Plugin {
	return &Plugin{descriptor: Descriptor(), router: router}
}
func (p *Plugin) Descriptor() providerplugin.Descriptor {
	if p == nil {
		return Descriptor()
	}
	return p.descriptor.Clone()
}
func (p *Plugin) ValidateConfig(ctx context.Context, raw json.RawMessage) error {
	config, err := DecodeConfig(raw)
	if err != nil {
		return err
	}
	if p == nil || p.router == nil {
		return fmt.Errorf("validate Sandbox Workspace config: execution router unavailable")
	}
	// Resolve reads persisted placement metadata without acquiring a Sandbox,
	// starting a process or reading the backend filesystem. Reject dangling
	// bindings during Provider creation and update rather than at first invoke.
	placement, err := p.router.Resolve(ctx, sandboxdomain.ID(config.SandboxID), sandboxdomain.CapabilityRead)
	if err != nil {
		return fmt.Errorf("validate Sandbox Workspace config: lookup: %w", err)
	}
	if placement.Sandbox.ID().String() != config.SandboxID {
		return fmt.Errorf("validate Sandbox Workspace config: placement identity conflict: %w", sandboxdomain.ErrConflict)
	}
	return nil
}
func (*Plugin) InspectConfig(ctx context.Context, raw json.RawMessage) (providerplugin.ConfigMetadata, error) {
	return InspectConfig(ctx, raw)
}
func (p *Plugin) Open(_ context.Context, request providerplugin.OpenRequest) (providerplugin.Instance, error) {
	if p == nil || p.router == nil {
		return nil, fmt.Errorf("open Sandbox Workspace Tools: Sandbox Router unavailable")
	}
	if request.ConfigVersion != ConfigVersion {
		return nil, fmt.Errorf("open Sandbox Workspace Tools: unsupported ConfigVersion %d", request.ConfigVersion)
	}
	config, err := DecodeConfig(request.Config)
	if err != nil {
		return nil, fmt.Errorf("open Sandbox Workspace Tools: %w", err)
	}
	// Do not resolve or acquire Sandbox on Provider activation or discovery.
	return &Instance{sandboxID: sandboxdomain.ID(config.SandboxID), router: p.router, tools: discoveredFileTools()}, nil
}

type Instance struct {
	sandboxID sandboxdomain.ID
	router    sandboxapplication.ExecutionRouter
	tools     []catalogapplication.DiscoveredTool
}

func (i *Instance) DiscoverTools(_ context.Context, _ catalogdomain.Provider) ([]catalogapplication.DiscoveredTool, error) {
	if i == nil {
		return nil, fmt.Errorf("discover Sandbox Workspace Tools: nil instance")
	}
	result := make([]catalogapplication.DiscoveredTool, 0, len(i.tools))
	for _, t := range i.tools {
		result = append(result, catalogapplication.DiscoveredTool{SourceKey: t.SourceKey, Name: t.Name, Title: t.Title, Description: t.Description, InputSchema: append([]byte(nil), t.InputSchema...), OutputSchema: append([]byte(nil), t.OutputSchema...), Tags: slices.Clone(t.Tags)})
	}
	return result, nil
}
func (i *Instance) Execute(ctx context.Context, request invocationapplication.ExecutionRequest, _ invocationapplication.CredentialResolver, _ invocationapplication.ProgressReporter) (invocationdomain.Outcome, error) {
	if i == nil || i.router == nil {
		return invocationdomain.Outcome{}, fmt.Errorf("execute Sandbox Workspace Tool: Router unavailable")
	}
	if err := request.Context.Validate(); err != nil {
		return invocationdomain.Outcome{}, fmt.Errorf("execute Sandbox Workspace Tool: trusted context: %w", err)
	}
	var capability sandboxdomain.Capability
	switch request.Tool.SourceKey() {
	case sourceKeyRead:
		capability = sandboxdomain.CapabilityRead
	case sourceKeySearch:
		capability = sandboxdomain.CapabilitySearch
	case sourceKeyEdit:
		capability = sandboxdomain.CapabilityEdit
	case sourceKeyShell:
		capability = sandboxdomain.CapabilityShell
	default:
		return invocationdomain.Outcome{}, sandboxdomain.ErrCapabilityUnavailable
	}
	placement, err := i.router.Resolve(ctx, i.sandboxID, capability)
	if err != nil {
		return invocationdomain.Outcome{}, fmt.Errorf("resolve Sandbox Workspace execution: %w", err)
	}
	lease, err := i.router.Acquire(ctx, request.Context, placement)
	if err != nil {
		return invocationdomain.Outcome{}, fmt.Errorf("acquire Sandbox Workspace execution: %w", err)
	}
	if lease == nil {
		return invocationdomain.Outcome{}, fmt.Errorf("acquire Sandbox Workspace execution: missing lease")
	}
	session, ok := lease.Session.(sandboxapplication.WorkspaceSession)
	if !ok {
		return invocationdomain.Outcome{}, errors.Join(sandboxdomain.ErrCapabilityUnavailable, lease.Close())
	}
	result, runErr := session.ExecuteWorkspace(ctx, capability, request.Arguments)
	closeErr := lease.Close()
	if closeErr != nil {
		return invocationdomain.Outcome{}, errors.Join(runErr, fmt.Errorf("release Sandbox Workspace lease: %w", closeErr))
	}
	return result, runErr
}
func (*Instance) Close() error { return nil } // lifecycle owned by Sandbox Router leases

const (
	sourceKeyRead   = "workspace.read"
	sourceKeySearch = "workspace.search"
	sourceKeyEdit   = "workspace.edit"
	sourceKeyShell  = "workspace.shell"
)

func discoveredFileTools() []catalogapplication.DiscoveredTool {
	return []catalogapplication.DiscoveredTool{
		{
			SourceKey:    sourceKeyRead,
			Name:         "read",
			Title:        "Read workspace file",
			Description:  "Read a bounded range of lines from one text file inside the configured workspace.",
			InputSchema:  readInputSchema,
			OutputSchema: readOutputSchema,
			Tags:         []string{"workspace", "filesystem", "read"},
		},
		{
			SourceKey:    sourceKeySearch,
			Name:         "search",
			Title:        "Search workspace",
			Description:  "Find workspace files or search bounded text content without invoking a shell.",
			InputSchema:  searchInputSchema,
			OutputSchema: searchOutputSchema,
			Tags:         []string{"workspace", "filesystem", "search"},
		},
		{
			SourceKey:    sourceKeyEdit,
			Name:         "edit",
			Title:        "Edit workspace file",
			Description:  "Create, deterministically replace text in, or delete one regular file inside the workspace.",
			InputSchema:  editInputSchema,
			OutputSchema: editOutputSchema,
			Tags:         []string{"workspace", "filesystem", "write"},
		},
		{
			SourceKey:    sourceKeyShell,
			Name:         "shell",
			Title:        "Run workspace shell command",
			Description:  "Run a bounded host shell command from a workspace-contained initial directory. This is not an OS sandbox.",
			InputSchema:  shellInputSchema,
			OutputSchema: shellOutputSchema,
			Tags:         []string{"workspace", "process", "shell"},
		},
	}
}

func decodeStrict(raw []byte, target any, label string) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("%s: multiple JSON values", label)
		}
		return fmt.Errorf("%s: trailing data: %w", label, err)
	}
	return nil
}

var _ providerplugin.Plugin = (*Plugin)(nil)
var _ providerplugin.Instance = (*Instance)(nil)
