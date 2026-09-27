package governance

import (
	"context"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
)

// UnknownExtractor is the safe default for Tools without a reviewed fact mapper.
type UnknownExtractor struct{}

func (UnknownExtractor) Extract(
	context.Context,
	catalogdomain.Tool,
	invocationdomain.JSONValue,
) (invocationdomain.GovernanceFacts, error) {
	return invocationdomain.UnknownGovernanceFacts(), nil
}
