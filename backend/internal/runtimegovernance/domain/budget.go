package domain

import (
	"errors"
	"fmt"
	"time"
)

var ErrBudgetExceeded = errors.New("runtime budget exceeded")

type BudgetExceededError struct {
	Dimension string
}

func (e BudgetExceededError) Error() string {
	return fmt.Sprintf("%s: %s", ErrBudgetExceeded, e.Dimension)
}

func (e BudgetExceededError) Unwrap() error { return ErrBudgetExceeded }

const (
	DefaultMaxInvocations          = 32
	DefaultMaxPrincipalInvocations = 8
	DefaultMaxProviderInvocations  = 8
	DefaultMaxToolInvocations      = 4
	DefaultMaxProviderActivations  = 4
	DefaultMaxExecutionTimeoutMS   = 120000
	DefaultMaxArgumentBytes        = 1 << 20
	DefaultMaxResultBytes          = 1 << 20

	maxConcurrencyLimit = 4096
	maxByteLimit        = 64 << 20
	maxTimeout          = 24 * time.Hour
)

// Budget is the immutable Runtime capacity snapshot applied to one new action.
type Budget struct {
	maxInvocations          int
	maxPrincipalInvocations int
	maxProviderInvocations  int
	maxToolInvocations      int
	maxProviderActivations  int
	maxExecutionTimeoutMS   int64
	maxArgumentBytes        int
	maxResultBytes          int
}

type BudgetSpec struct {
	MaxInvocations          int
	MaxPrincipalInvocations int
	MaxProviderInvocations  int
	MaxToolInvocations      int
	MaxProviderActivations  int
	MaxExecutionTimeoutMS   int64
	MaxArgumentBytes        int
	MaxResultBytes          int
}

func DefaultBudget() Budget {
	value, err := NewBudget(BudgetSpec{
		MaxInvocations:          DefaultMaxInvocations,
		MaxPrincipalInvocations: DefaultMaxPrincipalInvocations,
		MaxProviderInvocations:  DefaultMaxProviderInvocations,
		MaxToolInvocations:      DefaultMaxToolInvocations,
		MaxProviderActivations:  DefaultMaxProviderActivations,
		MaxExecutionTimeoutMS:   DefaultMaxExecutionTimeoutMS,
		MaxArgumentBytes:        DefaultMaxArgumentBytes,
		MaxResultBytes:          DefaultMaxResultBytes,
	})
	if err != nil {
		panic("runtime governance: invalid default budget: " + err.Error())
	}
	return value
}

func NewBudget(spec BudgetSpec) (Budget, error) {
	for field, value := range map[string]int{
		"max_invocations":           spec.MaxInvocations,
		"max_principal_invocations": spec.MaxPrincipalInvocations,
		"max_provider_invocations":  spec.MaxProviderInvocations,
		"max_tool_invocations":      spec.MaxToolInvocations,
		"max_provider_activations":  spec.MaxProviderActivations,
	} {
		if value < 1 || value > maxConcurrencyLimit {
			return Budget{}, fmt.Errorf("%s: must be between 1 and %d", field, maxConcurrencyLimit)
		}
	}
	for field, value := range map[string]int{
		"max_argument_bytes": spec.MaxArgumentBytes,
		"max_result_bytes":   spec.MaxResultBytes,
	} {
		if value < 1 || value > maxByteLimit {
			return Budget{}, fmt.Errorf("%s: must be between 1 and %d", field, maxByteLimit)
		}
	}
	timeout := time.Duration(spec.MaxExecutionTimeoutMS) * time.Millisecond
	if spec.MaxExecutionTimeoutMS < 1 || timeout <= 0 || timeout > maxTimeout {
		return Budget{}, fmt.Errorf(
			"max_execution_timeout_ms: must be between 1 and %d",
			maxTimeout.Milliseconds(),
		)
	}
	return Budget{
		maxInvocations:          spec.MaxInvocations,
		maxPrincipalInvocations: spec.MaxPrincipalInvocations,
		maxProviderInvocations:  spec.MaxProviderInvocations,
		maxToolInvocations:      spec.MaxToolInvocations,
		maxProviderActivations:  spec.MaxProviderActivations,
		maxExecutionTimeoutMS:   spec.MaxExecutionTimeoutMS,
		maxArgumentBytes:        spec.MaxArgumentBytes,
		maxResultBytes:          spec.MaxResultBytes,
	}, nil
}

func (b Budget) Validate() error {
	_, err := NewBudget(b.Spec())
	return err
}

func (b Budget) Spec() BudgetSpec {
	return BudgetSpec{
		MaxInvocations:          b.maxInvocations,
		MaxPrincipalInvocations: b.maxPrincipalInvocations,
		MaxProviderInvocations:  b.maxProviderInvocations,
		MaxToolInvocations:      b.maxToolInvocations,
		MaxProviderActivations:  b.maxProviderActivations,
		MaxExecutionTimeoutMS:   b.maxExecutionTimeoutMS,
		MaxArgumentBytes:        b.maxArgumentBytes,
		MaxResultBytes:          b.maxResultBytes,
	}
}

func (b Budget) MaxInvocations() int          { return b.maxInvocations }
func (b Budget) MaxPrincipalInvocations() int { return b.maxPrincipalInvocations }
func (b Budget) MaxProviderInvocations() int  { return b.maxProviderInvocations }
func (b Budget) MaxToolInvocations() int      { return b.maxToolInvocations }
func (b Budget) MaxProviderActivations() int  { return b.maxProviderActivations }
func (b Budget) MaxExecutionTimeout() time.Duration {
	return time.Duration(b.maxExecutionTimeoutMS) * time.Millisecond
}
func (b Budget) MaxExecutionTimeoutMS() int64 { return b.maxExecutionTimeoutMS }
func (b Budget) MaxArgumentBytes() int        { return b.maxArgumentBytes }
func (b Budget) MaxResultBytes() int          { return b.maxResultBytes }
