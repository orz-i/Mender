package domain

import (
	"fmt"
	"strings"
	"time"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
)

type ProviderActivationStatus string

const (
	ProviderActivationCompleted ProviderActivationStatus = "completed"
	ProviderActivationDenied    ProviderActivationStatus = "denied"
	ProviderActivationFailed    ProviderActivationStatus = "failed"
)

type ProviderActivationErrorClass string

const (
	ProviderActivationErrorNone     ProviderActivationErrorClass = ""
	ProviderActivationErrorPolicy   ProviderActivationErrorClass = "policy"
	ProviderActivationErrorApproval ProviderActivationErrorClass = "approval"
	ProviderActivationErrorBudget   ProviderActivationErrorClass = "budget"
	ProviderActivationErrorRuntime  ProviderActivationErrorClass = "runtime"
	ProviderActivationErrorAudit    ProviderActivationErrorClass = "audit"
)

type ProviderActivationRecord struct {
	activationID   string
	providerID     catalogdomain.ProviderID
	principalKind  string
	principalRef   string
	origin         governancedomain.Origin
	scopeKind      governancedomain.ScopeKind
	scopeReference string
	startedAt      time.Time
	finishedAt     time.Time
	policyKind     policydomain.DecisionKind
	policyReason   string
	approval       ApprovalOutcome
	status         ProviderActivationStatus
	errorClass     ProviderActivationErrorClass
	errorSummary   string
}

func NewProviderActivationRecord(
	activationID string,
	providerID catalogdomain.ProviderID,
	governanceContext governancedomain.Context,
	startedAt time.Time,
	finishedAt time.Time,
	policyKind policydomain.DecisionKind,
	policyReason string,
	approval ApprovalOutcome,
	status ProviderActivationStatus,
	errorClass ProviderActivationErrorClass,
) (ProviderActivationRecord, error) {
	activationID = strings.TrimSpace(activationID)
	if !strings.HasPrefix(activationID, "act_") || len(activationID) != 36 {
		return ProviderActivationRecord{}, fmt.Errorf("provider activation id is invalid")
	}
	if _, err := catalogdomain.ParseProviderID(providerID.String()); err != nil {
		return ProviderActivationRecord{}, err
	}
	if err := governanceContext.Validate(); err != nil {
		return ProviderActivationRecord{}, fmt.Errorf("provider activation governance context: %w", err)
	}
	if startedAt.IsZero() || finishedAt.IsZero() || finishedAt.Before(startedAt) {
		return ProviderActivationRecord{}, fmt.Errorf("provider activation timestamps are invalid")
	}
	if policyKind != "" {
		if err := policydomain.ValidateDecisionKind(policyKind); err != nil {
			return ProviderActivationRecord{}, err
		}
	}
	switch approval {
	case ApprovalNotRequired, ApprovalApproved, ApprovalDenied:
	default:
		return ProviderActivationRecord{}, fmt.Errorf("provider activation approval is invalid")
	}
	switch status {
	case ProviderActivationCompleted:
		if errorClass != ProviderActivationErrorNone {
			return ProviderActivationRecord{}, fmt.Errorf("completed provider activation cannot contain error class")
		}
	case ProviderActivationDenied, ProviderActivationFailed:
		if errorClass == ProviderActivationErrorNone {
			return ProviderActivationRecord{}, fmt.Errorf("non-completed provider activation requires error class")
		}
	default:
		return ProviderActivationRecord{}, fmt.Errorf("provider activation status is invalid")
	}

	principal := governanceContext.Principal()
	scope := governanceContext.Scope()
	return ProviderActivationRecord{
		activationID:   activationID,
		providerID:     providerID,
		principalKind:  string(principal.Kind()),
		principalRef:   principal.Reference(),
		origin:         governanceContext.Origin(),
		scopeKind:      scope.Kind(),
		scopeReference: scope.Reference(),
		startedAt:      startedAt.UTC(),
		finishedAt:     finishedAt.UTC(),
		policyKind:     policyKind,
		policyReason:   strings.TrimSpace(policyReason),
		approval:       approval,
		status:         status,
		errorClass:     errorClass,
	}, nil
}

func (r ProviderActivationRecord) WithErrorSummary(summary string) ProviderActivationRecord {
	r.errorSummary = normalizeErrorSummary(summary)
	return r
}

func (r ProviderActivationRecord) ActivationID() string                     { return r.activationID }
func (r ProviderActivationRecord) ProviderID() catalogdomain.ProviderID     { return r.providerID }
func (r ProviderActivationRecord) PrincipalKind() string                    { return r.principalKind }
func (r ProviderActivationRecord) PrincipalRef() string                     { return r.principalRef }
func (r ProviderActivationRecord) Origin() governancedomain.Origin          { return r.origin }
func (r ProviderActivationRecord) ScopeKind() governancedomain.ScopeKind    { return r.scopeKind }
func (r ProviderActivationRecord) ScopeReference() string                   { return r.scopeReference }
func (r ProviderActivationRecord) StartedAt() time.Time                     { return r.startedAt }
func (r ProviderActivationRecord) FinishedAt() time.Time                    { return r.finishedAt }
func (r ProviderActivationRecord) PolicyKind() policydomain.DecisionKind    { return r.policyKind }
func (r ProviderActivationRecord) PolicyReason() string                     { return r.policyReason }
func (r ProviderActivationRecord) Approval() ApprovalOutcome                { return r.approval }
func (r ProviderActivationRecord) Status() ProviderActivationStatus         { return r.status }
func (r ProviderActivationRecord) ErrorClass() ProviderActivationErrorClass { return r.errorClass }
func (r ProviderActivationRecord) ErrorSummary() string                     { return r.errorSummary }
