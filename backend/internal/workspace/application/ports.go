package application

import (
	"context"

	governance "github.com/orz-i/Mender/backend/internal/governance/domain"
	workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"
)

// Session is the consuming-side typed execution port. Implementations own
// backend safety and uncertainty; they do not parse Tool arguments or Outcomes.
type Session interface {
	Read(context.Context, workspace.ReadRequest) (workspace.ReadResult, error)
	Search(context.Context, workspace.SearchRequest) (workspace.SearchResult, error)
	Edit(context.Context, workspace.EditRequest) (workspace.EditResult, error)
	Shell(context.Context, workspace.ShellRequest) (workspace.ShellResult, error)
	Close() error
}

type LeaseProvider interface {
	Acquire(context.Context, governance.Context, string, workspace.Operation) (*Lease, error)
}

// Compatibility describes existing transport limits, not authorization.
// External v1 retains omitted request defaults as part of its wire contract.
type Compatibility struct {
	PreserveDefaults         bool
	MaxArgumentBytes         int
	ArgumentLimitMessage     string
	ArgumentLimitIsToolError bool
}
