package application

import (
	"context"
	"time"

	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
)

type Repository interface {
	GetPolicySet(context.Context) (policydomain.PolicySet, error)
	SetDefaultDecision(context.Context, policydomain.DecisionKind, time.Time) error
	CreateRule(context.Context, policydomain.Rule) error
	GetRule(context.Context, policydomain.RuleID) (policydomain.Rule, error)
	UpdateRule(context.Context, policydomain.Rule) error
	DeleteRule(context.Context, policydomain.RuleID) error
}

type RuleIDGenerator interface {
	NewRuleID() (policydomain.RuleID, error)
}

type Clock interface {
	Now() time.Time
}

type ReferenceLookup interface {
	ProfileExists(context.Context, string) (bool, error)
	ProviderExists(context.Context, string) (bool, error)
	ToolExists(context.Context, string) (bool, error)
}
