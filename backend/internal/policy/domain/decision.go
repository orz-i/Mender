package domain

import (
	"fmt"
	"regexp"
	"strings"
)

// DecisionKind is the canonical policy outcome.
type DecisionKind string

const (
	DecisionAllow           DecisionKind = "allow"
	DecisionDeny            DecisionKind = "deny"
	DecisionRequireApproval DecisionKind = "require_approval"
)

var reasonCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,127}$`)

// Decision is a protocol-independent policy decision.
type Decision struct {
	kind        DecisionKind
	reasonCode  string
	explanation string
}

func Allow(reasonCode, explanation string) (Decision, error) {
	return newDecision(DecisionAllow, reasonCode, explanation)
}

func Deny(reasonCode, explanation string) (Decision, error) {
	return newDecision(DecisionDeny, reasonCode, explanation)
}

func RequireApproval(reasonCode, explanation string) (Decision, error) {
	return newDecision(DecisionRequireApproval, reasonCode, explanation)
}

func newDecision(kind DecisionKind, reasonCode, explanation string) (Decision, error) {
	reasonCode = strings.TrimSpace(reasonCode)
	if !reasonCodePattern.MatchString(reasonCode) {
		return Decision{}, fmt.Errorf("reason_code: must be a lowercase stable code")
	}

	return Decision{
		kind:        kind,
		reasonCode:  reasonCode,
		explanation: strings.TrimSpace(explanation),
	}, nil
}

func (d Decision) Kind() DecisionKind {
	return d.kind
}

func (d Decision) ReasonCode() string {
	return d.reasonCode
}

func (d Decision) Explanation() string {
	return d.explanation
}
