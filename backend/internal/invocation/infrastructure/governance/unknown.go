package governance

import (
	"context"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governance "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
)

// UnknownExtractor is the safe default for Tools without a reviewed fact mapper.
type UnknownExtractor struct{}

func (UnknownExtractor) Extract(
	context.Context,
	catalogdomain.Tool,
	invocationdomain.JSONValue,
	governance.Context,
) (invocationapplication.GovernanceFactExtraction, error) {
	return invocationapplication.GovernanceFactExtraction{Facts: invocationdomain.UnknownGovernanceFacts()}, nil
}
