package acp

import (
	"context"
	"errors"
)

// Finite, immutable stage vocabulary. These are the only values this package
// ever reports through SafeAgentFailureStage; the Agent Application Service
// maps exactly these codes to owner-facing diagnostics and treats every other
// error (including free-form injected labels) as a generic failure.
const (
	stageProcessLaunch   = "process_launch"
	stageInitialize      = "initialize"
	stageSessionCreate   = "session_create"
	stageConfiguration   = "configuration"
	stagePermission      = "permission"
	stagePromptRejected  = "prompt_rejected"
	stagePromptTransport = "prompt_transport"
	stageProtocol        = "protocol"
	stageResult          = "result"
)

// recognizedFailureStages is the closed set accepted by tagFailure.
// Anything else fails closed and stays untagged (generic application stage).
var recognizedFailureStages = map[string]struct{}{
	stageProcessLaunch:   {},
	stageInitialize:      {},
	stageSessionCreate:   {},
	stageConfiguration:   {},
	stagePermission:      {},
	stagePromptRejected:  {},
	stagePromptTransport: {},
	stageProtocol:        {},
	stageResult:          {},
}

type stagedAgentError struct {
	stage string
	err   error
}

// SafeAgentFailureStage is the backend-neutral hook read by the Agent
// Application Service. It is only ever a constant from the fixed table.
func (e *stagedAgentError) SafeAgentFailureStage() string { return e.stage }

// Error is opaque and fixed: the underlying cause, remote RPC error text,
// prompts, paths, stderr, bearer tokens or any other raw payload NEVER enter
// the message. Where and whether something failed is safe; what it said is not.
func (e *stagedAgentError) Error() string { return "Agent task failed at stage " + e.stage }

// Unwrap preserves established errors.Is semantics, in particular caller
// cancellation (context.Canceled / context.DeadlineExceeded) and the
// permission-denied sentinel.
func (e *stagedAgentError) Unwrap() error { return e.err }

// tagFailure attaches a fixed stage to err. A stage label outside the
// closed vocabulary is ignored fail-closed, so no caller can smuggle a
// free-form stage through. The message stays opaque by construction.
func tagFailure(stage string, err error) error {
	if err == nil {
		return nil
	}
	if _, ok := recognizedFailureStages[stage]; !ok {
		return err
	}
	return &stagedAgentError{stage: stage, err: err}
}

// safeStage returns the stage of the first staged error in the chain.
func safeStage(err error) (string, bool) {
	var staged *stagedAgentError
	if errors.As(err, &staged) {
		return staged.stage, true
	}
	return "", false
}

// isCancellation reports a caller-side cancellation or deadline. Such errors
// keep their established semantics and are never renamed into a stage.
func isCancellation(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// tagUnlessCancelled tags err with a fixed stage unless the chain is caller
// cancellation, which retains its priority and never becomes a stage fault.
func tagUnlessCancelled(stage string, err error) error {
	if err == nil || isCancellation(err) {
		return err
	}
	return tagFailure(stage, err)
}

// tagSessionTransport tags setup/initialization/session-creation transport
// failures of wire.call. An unresolved client-permission sentinel keeps the
// dedicated permission stage; caller cancellation always wins unchanged.
func tagSessionTransport(stage string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, errClientPermissionDenied) {
		return tagFailure(stagePermission, err)
	}
	return tagUnlessCancelled(stage, err)
}

// rpcRejectedError marks an explicit JSON-RPC error object returned by the
// remote Agent for one of our requests. The remote error text is never
// retained; only the locally fixed method name appears in the message.
type rpcRejectedError struct{ method string }

func (e *rpcRejectedError) Error() string { return "ACP " + e.method + " request rejected by Agent" }

// malformedOutcomeError marks a response that is not the bounded shape the
// protocol requires. The remote payload is never echoed.
type malformedOutcomeError struct{ method string }

func (e *malformedOutcomeError) Error() string {
	return "ACP " + e.method + " returned a malformed result"
}

// stagePromptError classifies a failed session/prompt interaction without
// ever exposing remote text:
//   - unresolved/rejected permission  -> permission
//   - explicit remote RPC rejection   -> prompt_rejected
//   - malformed final outcome         -> result
//   - caller cancellation/deadline    -> unchanged (established semantics)
//   - EOF, teardown, read or other transport failure -> prompt_transport
func stagePromptError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, errClientPermissionDenied) {
		return tagFailure(stagePermission, err)
	}
	if isCancellation(err) {
		return err
	}
	if _, ok := safeStage(err); ok {
		// Already a precise, non-sensitive stage (for example a malformed
		// session update tagged by the update handler); keep it.
		return err
	}
	var rejected *rpcRejectedError
	if errors.As(err, &rejected) {
		return tagFailure(stagePromptRejected, err)
	}
	var malformed *malformedOutcomeError
	if errors.As(err, &malformed) {
		return tagFailure(stageResult, err)
	}
	return tagFailure(stagePromptTransport, err)
}
