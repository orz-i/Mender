package resultguard

import (
	"context"
	"fmt"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
)

// Guard enforces canonical post-execution result invariants.
type Guard struct {
	validator invocationapplication.ArgumentValidator
}

func New(validator invocationapplication.ArgumentValidator) Guard {
	return Guard{validator: validator}
}

func (g Guard) Guard(
	ctx context.Context,
	tool catalogdomain.Tool,
	outcome invocationdomain.Outcome,
	maxBytes int,
) (invocationdomain.Outcome, error) {
	if err := ctx.Err(); err != nil {
		return invocationdomain.Outcome{}, err
	}
	if outcome.Kind() != invocationdomain.OutcomeKindComplete {
		return outcome, nil
	}

	if maxBytes <= 0 {
		return invocationdomain.Outcome{}, fmt.Errorf("result guard: max bytes must be positive")
	}
	content := outcome.Content()
	if len(content.Bytes()) > maxBytes {
		return invocationdomain.Failed(
			invocationdomain.FailureSourceResultGuard,
			invocationdomain.FailureResultTooLarge,
			"tool result exceeds the configured canonical result-size limit",
		)
	}

	outputSchema := tool.OutputSchema()
	if outputSchema == nil {
		return outcome, nil
	}
	if g.validator == nil {
		return invocationdomain.Outcome{}, fmt.Errorf("result guard: schema validator is unavailable")
	}

	valid, err := g.validator.Validate(ctx, *outputSchema, content)
	if err != nil {
		return invocationdomain.Outcome{}, fmt.Errorf("result guard: validate output schema: %w", err)
	}
	if !valid {
		return invocationdomain.Failed(
			invocationdomain.FailureSourceResultGuard,
			invocationdomain.FailureOutputSchemaMismatch,
			"tool result does not match the declared output schema",
		)
	}
	return outcome, nil
}
