package application

import (
	"context"
	"errors"
	"fmt"

	auditdomain "github.com/orz-i/Mender/backend/internal/audit/domain"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
)

// invocationCompletion is the private terminal handoff from an ordered stage.
// Expected rejections/failures carry an outcome; unreliable governance carries
// an error. Neither the audit disposition nor metadata carries input payloads.
type invocationCompletion struct {
	tool              *catalogdomain.Tool
	outcome           invocationdomain.Outcome
	err               error
	audit             invocationAuditDisposition
	useParentContext  bool // Approval's audit retains parent cancellation.
	publishAfterAudit bool // Postflight output is unavailable if durable audit fails.
	skipAudit         bool // A canonical-outcome construction failure cannot be recorded.
	afterAuditEvent   invocationdomain.EventKind
}

// invocationAuditDisposition contains classification only, never arguments,
// result payloads, credential references or resolver capabilities.
type invocationAuditDisposition struct {
	policyKind    policydomain.DecisionKind
	policyReason  string
	approval      auditdomain.ApprovalOutcome
	status        auditdomain.Status
	resultKind    invocationdomain.ResultKind
	errorClass    auditdomain.ErrorClass
	summary       string
	outcomeKind   invocationdomain.OutcomeKind
	rejectionCode invocationdomain.RejectionCode
	failureSource invocationdomain.FailureSource
	failureCode   invocationdomain.FailureCode
}

func failedInvocation(tool *catalogdomain.Tool, class auditdomain.ErrorClass, err error) invocationCompletion {
	return invocationCompletion{tool: tool, err: err, audit: invocationAuditDisposition{
		approval: auditdomain.ApprovalNotRequired, status: auditdomain.StatusFailed, errorClass: class,
	}}
}

func rejectedInvocation(tool *catalogdomain.Tool, code invocationdomain.RejectionCode, message string,
	status auditdomain.Status, class auditdomain.ErrorClass,
) invocationCompletion {
	outcome, err := newRejectedOutcome(code, message)
	return invocationCompletion{tool: tool, outcome: outcome, err: err, skipAudit: err != nil,
		audit: invocationAuditDisposition{approval: auditdomain.ApprovalNotRequired, status: status, errorClass: class}}
}

func (c invocationCompletion) withAdmission(admitted admittedInvocation) invocationCompletion {
	c.audit.policyKind = admitted.decision.Kind()
	c.audit.policyReason = admitted.decision.ReasonCode()
	c.audit.approval = admitted.approval
	return c
}

type invocationFinalizer struct{ audit invocationAuditWriter }

func (f invocationFinalizer) finish(ctx context.Context, attempt invocationAttempt, completion invocationCompletion) (InvokeToolResponse, error) {
	response := InvokeToolResponse{InvocationID: attempt.metadata.id}
	if !completion.publishAfterAudit {
		response.Outcome = completion.outcome
	}
	if completion.skipAudit {
		return response, completion.err
	}
	disposition := completion.audit
	if outcome := completion.outcome; outcome.Kind() != "" {
		disposition.outcomeKind = outcome.Kind()
		disposition.rejectionCode = outcome.RejectionCode()
		disposition.failureSource = outcome.FailureSource()
		disposition.failureCode = outcome.FailureCode()
	}
	var providerID *catalogdomain.ProviderID
	metadata := attempt.metadata
	if completion.tool != nil {
		// Native and Agent-as-Tool own their canonical ToolID directly.
		// Never fabricate an empty ProviderID for their durable audit record.
		if completion.tool.SourceRef().Kind() == catalogdomain.SourceProvider {
			id := completion.tool.ProviderID()
			providerID = &id
		}
		metadata.toolID = completion.tool.ID()
	}
	auditCtx := ctx
	if !completion.useParentContext {
		auditCtx = context.WithoutCancel(ctx)
	}
	if err := f.audit.record(auditCtx, metadata, providerID, disposition); err != nil {
		if completion.err == nil {
			return response, err
		}
		return response, joinInvocationError(completion.err, err)
	}
	if completion.err != nil {
		return response, completion.err
	}
	if completion.afterAuditEvent != "" {
		attempt.events.lifecycle(completion.afterAuditEvent)
	}
	response.Outcome = completion.outcome
	return response, nil
}

func (invocationFinalizer) observeTerminal(events *eventEmitter, response InvokeToolResponse, err error) {
	if err == nil && response.Outcome.Kind() != "" {
		events.terminalOutcome(response.Outcome)
	}
}

type invocationAuditWriter struct {
	recorder AuditRecorder
	clock    Clock
}

// record accepts only the identity and bounded terminal classification derived
// by finalization. It cannot inspect input/result content or resolve secrets.
func (w invocationAuditWriter) record(ctx context.Context, metadata invocationMetadata,
	providerID *catalogdomain.ProviderID, disposition invocationAuditDisposition,
) error {
	record, err := auditdomain.NewRecord(metadata.id, metadata.toolID, providerID,
		metadata.startedAt, w.clock.Now(), disposition.policyKind, disposition.policyReason,
		disposition.approval, disposition.status, disposition.resultKind, disposition.errorClass)
	if err != nil {
		if providerID == nil {
			return fmt.Errorf("%w: construct unresolved tool record: %w", ErrAuditFailed, err)
		}
		return fmt.Errorf("%w: construct: %w", ErrAuditFailed, err)
	}
	record, err = record.WithExecutionContext(metadata.context)
	if err != nil {
		if providerID == nil {
			return fmt.Errorf("%w: unresolved execution metadata: %w", ErrAuditFailed, err)
		}
		return fmt.Errorf("%w: execution metadata: %w", ErrAuditFailed, err)
	}
	if disposition.outcomeKind != "" {
		record, err = record.WithOutcomeClassification(disposition.outcomeKind, disposition.rejectionCode,
			disposition.failureSource, disposition.failureCode)
		if err != nil {
			if providerID == nil {
				return fmt.Errorf("%w: unresolved outcome metadata: %w", ErrAuditFailed, err)
			}
			return fmt.Errorf("%w: outcome metadata: %w", ErrAuditFailed, err)
		}
	}
	if disposition.summary != "" {
		record = record.WithErrorSummary(disposition.summary)
	}
	if err := w.recorder.Record(ctx, record); err != nil {
		if providerID == nil {
			return fmt.Errorf("%w: persist unresolved tool record: %w", ErrAuditFailed, err)
		}
		return fmt.Errorf("%w: persist: %w", ErrAuditFailed, err)
	}
	return nil
}

func joinInvocationError(primary, auditErr error) error {
	if auditErr == nil {
		return primary
	}
	return errors.Join(primary, auditErr)
}
