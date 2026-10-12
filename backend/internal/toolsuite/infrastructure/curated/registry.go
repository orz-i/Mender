package curated

import (
	"github.com/orz-i/Mender/backend/internal/toolsuite/domain"
)

var definitions = []domain.Definition{
	{SuiteID: domain.AgentDelegation, DisplayName: "Agent Delegation",
		Description:  "Submit and inspect governed tasks of one explicitly selected Agent",
		ResourceKind: domain.AgentResource, RequiredCapabilities: []domain.Capability{domain.AgentTask},
		ToolSourceKeys: []string{"agent.delegate", "agent.status", "agent.result", "agent.cancel"}, Status: "builtin"},
	{SuiteID: domain.SandboxWorkspace, DisplayName: "Sandbox Workspace Tools",
		Description:          "Read, search, edit and execute within one governed Workspace Sandbox",
		ResourceKind:         domain.SandboxResource,
		RequiredCapabilities: []domain.Capability{domain.FileRead, domain.FileSearch, domain.FileEdit, domain.ProcessShell},
		ToolSourceKeys:       []string{"workspace.read", "workspace.search", "workspace.edit", "workspace.shell"}, Status: "builtin"},
	{SuiteID: domain.SandboxBrowser, DisplayName: "Browser Tools",
		Description:  "Browser-specific navigation, interaction and screenshots; requires a managed Browser Session backend",
		ResourceKind: domain.SandboxResource, RequiredCapabilities: []domain.Capability{domain.BrowserSession},
		ToolSourceKeys: []string{}, Status: "unavailable", Issue: "No verified managed Browser Session backend is registered"},
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
