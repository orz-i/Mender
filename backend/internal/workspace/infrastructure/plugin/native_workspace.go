package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governance "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationapp "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocation "github.com/orz-i/Mender/backend/internal/invocation/domain"
	sandbox "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	workspaceapp "github.com/orz-i/Mender/backend/internal/workspace/application"
	workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"
	"github.com/orz-i/Mender/backend/internal/workspace/infrastructure/sandboxbridge"
)

// NativeWorkspaceTools is the host-composed Workspace facts and execution adapter.
// The existing typed Workspace operations and Sandbox Router remain the only
// execution implementation.
type NativeWorkspaceTools struct {
	Selection         workspaceapp.WorkspaceSelection
	Router            sandboxbridge.GuardedRouter
	RequiredIsolation sandbox.IsolationClass
	// PinnedSandboxID is an explicit owner-approved default (never a Tool
	// argument). An absent pin permits only an unambiguous eligible placement.
	PinnedSandboxID string
}

// Extract performs inert, caller-scoped Workspace and Sandbox selection BEFORE
// Policy. The returned frozen target must reach Execute unchanged through
// canonical Invocation admission. This is not an authority to run the Tool.
func (n NativeWorkspaceTools) Extract(ctx context.Context, tool catalog.Tool,
	args invocation.JSONValue, caller governance.Context,
) (invocationapp.GovernanceFactExtraction, error) {
	op, err := nativeWorkspaceOperation(tool)
	if err != nil {
		return invocationapp.GovernanceFactExtraction{}, err
	}
	var selection struct {
		WorkspaceID string `json:"workspaceId"`
	}
	if err := json.Unmarshal(args.Bytes(), &selection); err != nil {
		return invocationapp.GovernanceFactExtraction{}, fmt.Errorf("native Workspace: decode workspace selection: %w", err)
	}
	var requested workspace.Ref
	if selection.WorkspaceID != "" {
		requested, err = workspace.ParseRef(selection.WorkspaceID)
		if err != nil {
			return invocationapp.GovernanceFactExtraction{}, err
		}
	}
	ref, err := n.Selection.Resolve(ctx, caller, requested)
	if err != nil {
		return invocationapp.GovernanceFactExtraction{}, err
	}
	frozen, err := n.Router.Resolve(ctx, sandboxbridge.ResolveRequest{
		Caller: caller, ToolID: tool.ID(), Workspace: ref, Operation: op,
		PinnedSandboxID: n.PinnedSandboxID, RequiredIsolation: n.RequiredIsolation,
		Arguments: args.Bytes(),
	})
	if err != nil {
		return invocationapp.GovernanceFactExtraction{}, err
	}
	return invocationapp.GovernanceFactExtraction{Facts: frozen.Facts(), Target: frozen}, nil
}

// Execute never selects a target. Canonical Invocation must have supplied the
// exact pre-Policy FrozenPlacement and its attempt-local admitted proof. The
// GuardedRouter and the atomic Native lease store recheck live grants, backend
// run generation, root and TargetScope before acquiring the original Sandbox.
func (n NativeWorkspaceTools) Execute(ctx context.Context, request invocationapp.ExecutionRequest,
	_ invocationapp.CredentialResolver, _ invocationapp.ProgressReporter,
) (invocation.Outcome, error) {
	op, err := nativeWorkspaceOperation(request.Tool)
	if err != nil {
		return invocation.Outcome{}, err
	}
	frozen, ok := request.Target.(sandboxbridge.FrozenPlacement)
	if !ok || frozen.Operation() != op || frozen.ToolID() != request.Tool.ID() ||
		!bytes.Equal(frozen.Arguments(), request.Arguments.Bytes()) ||
		request.Context.Validate() != nil {
		return invocation.Outcome{}, sandbox.ErrNotAuthorized
	}
	// Remove only the routing-only workspaceId. The legacy typed operation
	// decoder must continue rejecting unrecognized execution parameters.
	cleaned, err := stripNativeWorkspaceID(request.Arguments)
	if err != nil {
		return invocation.Outcome{}, err
	}
	lease, err := n.Router.Acquire(ctx, frozen, request.InvocationID, request.AdmittedFacts)
	if err != nil {
		return invocation.Outcome{}, err
	}
	// Preserve the original canonical request's size when applying backend
	// argument limits and typed payload validation.
	return executeTypedWithArgumentSize(ctx, lease, op, cleaned, request.MaxResultBytes, len(request.Arguments.Bytes()))
}

func nativeWorkspaceOperation(tool catalog.Tool) (workspace.Operation, error) {
	if tool.SourceRef().Kind() != catalog.SourceNative || tool.SourceRef().ID() != "workspace" {
		return "", sandbox.ErrNotAuthorized
	}
	switch tool.SourceKey() {
	case "workspace.read":
		return workspace.Read, nil
	case "workspace.search":
		return workspace.Search, nil
	case "workspace.edit":
		return workspace.Edit, nil
	case "workspace.shell":
		return workspace.Shell, nil
	default:
		return "", sandbox.ErrCapabilityUnavailable
	}
}

func stripNativeWorkspaceID(args invocation.JSONValue) (invocation.JSONValue, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(args.Bytes(), &fields); err != nil || fields == nil {
		return invocation.JSONValue{}, fmt.Errorf("native Workspace: invalid operation arguments")
	}
	delete(fields, "workspaceId")
	raw, err := json.Marshal(fields)
	if err != nil {
		return invocation.JSONValue{}, err
	}
	return invocation.NewJSONValue(raw)
}

var _ invocationapp.GovernanceFactExtractor = NativeWorkspaceTools{}
var _ invocationapp.ToolExecutor = NativeWorkspaceTools{}
