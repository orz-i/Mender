package bootstrap

import (
	"context"
	"time"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sshjobsplugin "github.com/orz-i/Mender/backend/internal/sshjobs/infrastructure/plugin"
	workspaceplugin "github.com/orz-i/Mender/backend/internal/workspace/infrastructure/plugin"
)

// SSH transport negotiation and SFTP may take tens of seconds before the
// governed operation can begin. These are SHORT TOOL-CALL budgets, not the
// detached systemd job's maximum execution time. Only the exact verified SSH
// Workspace or SSH Jobs Tool receives an implicit extension; explicit shorter
// deadlines and the authoritative Runtime Budget always take precedence.
const (
	sshWorkspaceInvocationTimeout = 60 * time.Second
	sshJobsInvocationTimeout      = 90 * time.Second
)

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

// isSSHJobsTool checks both canonical SourceKey and the immutable, exact
// first-party Provider/config/Sandbox binding. A renamed arbitrary Provider,
// Workspace shell or third-party Sandbox can never request a longer timeout.
// No network, credential or remote process is accessed during this check.
func (r *Runtime) isSSHJobsTool(ctx context.Context, id catalogdomain.ToolID) bool {
	if r == nil || r.Sandboxes == nil || r.providerConfigs == nil {
		return false
	}
	tool, err := r.Catalog.DescribeTool.Execute(ctx, id)
	if err != nil {
		return false
	}
	switch tool.SourceKey() {
	case "jobs.submit", "jobs.status", "jobs.logs", "jobs.result", "jobs.cancel":
	default:
		return false
	}
	config, err := r.providerConfigs.GetProviderConfig(ctx, tool.ProviderID())
	if err != nil || config.PluginID != sshjobsplugin.ID || config.InstallationID != nil ||
		config.ConfigVersion != sshjobsplugin.ConfigVersion {
		return false
	}
	binding, err := sshjobsplugin.DecodeConfig(config.Config)
	if err != nil {
		return false
	}
	return r.Sandboxes.VerifySSHJobResource(ctx, sandboxdomain.ID(binding.SandboxID)) == nil
}

// This request adapter extends only an *implicit* Tool-call deadline.
// Explicit shorter caller timeouts and the authoritative Runtime Budget win.
func (r *Runtime) withSSHWorkspaceExecutionBudget(
	ctx context.Context,
	request invocationapplication.InvokeToolRequest,
) invocationapplication.InvokeToolRequest {
	if request.Timeout != 0 {
		return request
	}
	switch {
	case r.isSSHWorkspaceTool(ctx, request.ToolID):
		request.Timeout = sshWorkspaceInvocationTimeout
	case r.isSSHJobsTool(ctx, request.ToolID):
		request.Timeout = sshJobsInvocationTimeout
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
