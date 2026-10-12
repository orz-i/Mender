package binding

import (
	"github.com/orz-i/Mender/backend/internal/bootstrap"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

type ToolSuiteDefinition struct {
	SuiteID              string   `json:"suiteId"`
	DisplayName          string   `json:"displayName"`
	Description          string   `json:"description"`
	ResourceKind         string   `json:"resourceKind"`
	RequiredCapabilities []string `json:"requiredCapabilities"`
	ToolSourceKeys       []string `json:"toolSourceKeys"`
	Status               string   `json:"status"`
	Issue                string   `json:"issue,omitempty"`
}
type ToolSuiteDefinitionList struct {
	Suites []ToolSuiteDefinition `json:"suites"`
}
type SandboxTool struct {
	ToolID    string `json:"toolId"`
	SourceKey string `json:"sourceKey"`
	Name      string `json:"name"`
	Title     string `json:"title"`
}
type SandboxToolSuite struct {
	SuiteID      string        `json:"suiteId"`
	SandboxID    string        `json:"sandboxId"`
	Status       string        `json:"status"`
	ProviderID   string        `json:"providerId,omitempty"`
	ProviderName string        `json:"providerName,omitempty"`
	Available    bool          `json:"available"`
	Issue        string        `json:"issue,omitempty"`
	Tools        []SandboxTool `json:"tools"`
}

func (s *Service) ListToolSuites() (ToolSuiteDefinitionList, error) {
	definitions, err := s.runtime.ListToolSuiteDefinitions(s.ctx)
	if err != nil {
		return ToolSuiteDefinitionList{}, err
	}
	items := make([]ToolSuiteDefinition, 0, len(definitions))
	for _, d := range definitions {
		caps := make([]string, 0, len(d.RequiredCapabilities))
		for _, cap := range d.RequiredCapabilities {
			caps = append(caps, string(cap))
		}
		items = append(items, ToolSuiteDefinition{SuiteID: string(d.SuiteID),
			DisplayName: d.DisplayName, Description: d.Description, ResourceKind: string(d.ResourceKind),
			RequiredCapabilities: caps, ToolSourceKeys: d.ToolSourceKeys, Status: d.Status, Issue: d.Issue})
	}
	return ToolSuiteDefinitionList{Suites: items}, nil
}
func projectSandboxToolSuite(value bootstrap.SandboxToolSuite) SandboxToolSuite {
	result := SandboxToolSuite{
		SuiteID: string(value.SuiteID), SandboxID: value.SandboxID.String(),
		Status: value.Status, ProviderID: value.ProviderID, ProviderName: value.ProviderName,
		Available: value.Available, Issue: value.Issue, Tools: make([]SandboxTool, 0, len(value.Tools)),
	}
	for _, tool := range value.Tools {
		result.Tools = append(result.Tools, SandboxTool{ToolID: tool.ToolID, SourceKey: tool.SourceKey, Name: tool.Name, Title: tool.Title})
	}
	return result
}
func (s *Service) GetSandboxTools(id string) (SandboxToolSuite, error) {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return SandboxToolSuite{}, err
	}
	parsed, err := sandboxdomain.ParseID(id)
	if err != nil {
		return SandboxToolSuite{}, err
	}
	item, err := s.runtime.SandboxWorkspaceTools(s.ctx, parsed, caller)
	if err != nil {
		return SandboxToolSuite{}, err
	}
	return projectSandboxToolSuite(item), nil
}
func (s *Service) EnsureSandboxTools(id string) (SandboxToolSuite, error) {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return SandboxToolSuite{}, err
	}
	parsed, err := sandboxdomain.ParseID(id)
	if err != nil {
		return SandboxToolSuite{}, err
	}
	item, err := s.runtime.EnsureSandboxWorkspaceTools(s.ctx, parsed, caller)
	if err != nil {
		return SandboxToolSuite{}, err
	}
	return projectSandboxToolSuite(item), nil
}
func (s *Service) RemoveSandboxTools(id string) error {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return err
	}
	parsed, err := sandboxdomain.ParseID(id)
	if err != nil {
		return err
	}
	return s.runtime.RemoveSandboxWorkspaceTools(s.ctx, parsed, caller)
}
