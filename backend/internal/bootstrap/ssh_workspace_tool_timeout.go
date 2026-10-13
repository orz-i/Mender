package bootstrap

import (
	"context"
	"time"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	workspaceplugin "github.com/orz-i/Mender/backend/internal/workspace/infrastructure/plugin"
)

// SSH Workspace may need two pre-auth TCP attempts (up to 45 seconds overall)
// before its file/shell capability runs. This timeout applies ONLY to
// SSH-backed Workspace Tools, and is still capped by Runtime Budget and any
// tighter caller context; generic Providers retain their existing 30s default.
const sshWorkspaceInvocationTimeout = 60 * time.Second

func (r *Runtime) isSSHWorkspaceTool(ctx context.Context, id catalogdomain.ToolID) bool {
	if r == nil || r.Sandboxes == nil || r.providerConfigs == nil {
		return false
	}
	tool, err := r.Catalog.DescribeTool.Execute(ctx, id)
	if err != nil {
		return false
	}
	switch tool.SourceKey() {
	case "workspace.read", "workspace.search", "workspace.edit", "workspace.shell":
	default:
		return false
	}
	config, err := r.providerConfigs.GetProviderConfig(ctx, tool.ProviderID())
	if err != nil || config.PluginID != workspaceplugin.ID ||
		config.InstallationID != nil || config.ConfigVersion != workspaceplugin.ConfigVersion {
		return false
	}
	binding, err := workspaceplugin.DecodeConfig(config.Config)
	if err != nil {
		return false
	}
	sandbox, err := r.Sandboxes.Get(ctx, sandboxdomain.ID(binding.SandboxID))
	return err == nil && sandbox.Backend() == sandboxdomain.BackendSSHRemote
}

// This request adapter extends only the *implicit* execution deadline.
// Explicit shorter caller timeouts and the authoritative Runtime Budget win.
func (r *Runtime) withSSHWorkspaceExecutionBudget(
	ctx context.Context,
	request invocationapplication.InvokeToolRequest,
) invocationapplication.InvokeToolRequest {
	if request.Timeout == 0 && r.isSSHWorkspaceTool(ctx, request.ToolID) {
		request.Timeout = sshWorkspaceInvocationTimeout
	}
	return request
}

// This wrapper never changes Profile visibility, Provider activation,
// admission, Policy, resource authority or the caller's context deadline.
func (r *Runtime) invokeToolWithSSHTimeout(
	ctx context.Context,
	request invocationapplication.InvokeToolRequest,
) (invocationapplication.InvokeToolResponse, error) {
	return r.InvokeTool.Execute(ctx, r.withSSHWorkspaceExecutionBudget(ctx, request))
}
