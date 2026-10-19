package curated

import (
	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	"github.com/orz-i/Mender/backend/internal/toolsuite/domain"
)

func inlineContracts(names ...domain.Capability) []platformplugin.CapabilityContract {
	out := make([]platformplugin.CapabilityContract, 0, len(names))
	for _, name := range names {
		out = append(out, platformplugin.CapabilityContract{ID: name,
			Version: platformplugin.CapabilityVersion{Major: 1}, Mode: platformplugin.ExecutionInline})
	}
	return out
}

var definitions = []domain.Definition{
	{SuiteID: domain.AgentDelegation, DisplayName: "Agent Delegation",
		Description:  "Submit and inspect governed tasks of one explicitly selected Agent",
		ResourceKind: domain.AgentResource, RequiredCapabilities: []domain.Capability{domain.AgentTask},
		RequiredContracts: []platformplugin.CapabilityContract{{ID: domain.AgentTask, Version: platformplugin.CapabilityVersion{Major: 1}, Mode: platformplugin.ExecutionDetached}},
		ToolSourceKeys:    []string{"agent.delegate", "agent.status", "agent.result", "agent.cancel", "agent.resume"}, Status: "builtin"},
	{SuiteID: domain.SandboxWorkspace, DisplayName: "Sandbox Workspace Tools",
		Description:          "Read, search, edit and execute within one governed Workspace Sandbox",
		ResourceKind:         domain.SandboxResource,
		RequiredCapabilities: []domain.Capability{domain.FileRead, domain.FileSearch, domain.FileEdit, domain.ProcessShell},
		RequiredContracts:    inlineContracts(domain.FileRead, domain.FileSearch, domain.FileEdit, domain.ProcessShell),
		ToolSourceKeys:       []string{"workspace.read", "workspace.search", "workspace.edit", "workspace.shell"}, Status: "builtin"},
	{SuiteID: domain.SandboxJobs, DisplayName: "SSH Durable Jobs",
		Description:          "Submit, inspect, read logs and request cancellation for plugin-owned SSH tasks",
		ResourceKind:         domain.SandboxResource,
		RequiredCapabilities: []domain.Capability{domain.JobSubmit, domain.JobObserve, domain.JobCancel},
		RequiredContracts: []platformplugin.CapabilityContract{
			{ID: domain.JobSubmit, Version: platformplugin.CapabilityVersion{Major: 1}, Mode: platformplugin.ExecutionDetached,
				Guarantees: []platformplugin.ExecutionGuarantee{
					platformplugin.GuaranteeIdempotentSubmit, platformplugin.GuaranteeReconnectObservable, platformplugin.GuaranteePersistedResult}},
			{ID: domain.JobObserve, Version: platformplugin.CapabilityVersion{Major: 1}, Mode: platformplugin.ExecutionInline},
			{ID: domain.JobCancel, Version: platformplugin.CapabilityVersion{Major: 1}, Mode: platformplugin.ExecutionInline},
		},
		ToolSourceKeys: []string{"jobs.submit", "jobs.status", "jobs.logs", "jobs.result", "jobs.cancel"}, Status: "builtin"},
	{SuiteID: domain.SandboxBrowser, DisplayName: "Browser Tools",
		Description:  "Browser-specific navigation, interaction and screenshots; requires a managed Browser Session backend",
		ResourceKind: domain.SandboxResource, RequiredCapabilities: []domain.Capability{domain.BrowserSession},
		RequiredContracts: inlineContracts(domain.BrowserSession),
		ToolSourceKeys:    []string{}, Status: "unavailable", Issue: "No verified managed Browser Session backend is registered"},
}

// List is stable source-reviewed metadata; it never discovers external
// binaries, launches a driver or connects to a Browser service.
func List() []domain.Definition {
	out := make([]domain.Definition, 0, len(definitions))
	for _, item := range definitions {
		out = append(out, item.Clone())
	}
	return out
}
func Find(id domain.ID) (domain.Definition, bool) {
	for _, item := range definitions {
		if item.SuiteID == id {
			return item.Clone(), true
		}
	}
	return domain.Definition{}, false
}
