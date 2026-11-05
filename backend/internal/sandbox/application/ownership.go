package application

import (
	"context"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
)

type ToolSuiteOwnershipLookup interface {
	IsManagedSandboxSuiteProvider(context.Context, string) (bool, error)
}

// ToolSuiteOwnership reads the exact Sandbox-owned Provider relationship. It
// neither activates the facade nor grants access to the configured resource.
type ToolSuiteOwnership struct{ lookup ToolSuiteOwnershipLookup }

func NewToolSuiteOwnership(lookup ToolSuiteOwnershipLookup) ToolSuiteOwnership {
	return ToolSuiteOwnership{lookup: lookup}
}

func (q ToolSuiteOwnership) Execute(ctx context.Context, providerID catalogdomain.ProviderID) (bool, error) {
	return q.lookup.IsManagedSandboxSuiteProvider(ctx, providerID.String())
}
