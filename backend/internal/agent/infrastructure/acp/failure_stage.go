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
	stageProcessLaunch               = "process_launch"
	stageInitialize                  = "initialize"
	stageSessionCreate               = "session_create"
	stageSessionResume               = "session_resume"
	stageConfiguration               = "configuration"
	stageConfigModelUnavailable      = "configuration_model_unavailable"
	stageConfigReasoningUnavailable  = "configuration_reasoning_unavailable"
	stageConfigConfirmationFailed    = "configuration_confirmation_failed"
	stageConfigRequestFailed         = "configuration_request_failed"
	stagePermission                  = "permission"
	stagePermissionRequestInvalid    = "permission_request_invalid"
	stagePermissionClientUnavailable = "permission_client_capability_unavailable"
	stagePromptRejected              = "prompt_rejected"
	stagePromptTransport             = "prompt_transport"
	// Fixed, non-sensitive prompt transport sub-stages. They are bounded to
	// the observed Mender ACP boundary and never identify which model,
	// plugin, process or tool produced the failure.
	stagePromptStreamClosed   = "prompt_stream_closed"
	stagePromptMalformedFrame = "prompt_malformed_frame"
	stagePromptReadFailed     = "prompt_read_failed"
	stagePromptWriteFailed    = "prompt_write_failed"
	stageProtocol             = "protocol"
	stageResult               = "result"
)

// recognizedFailureStages is the closed set accepted by tagFailure.
// Anything else fails closed and stays untagged (generic application stage).
var recognizedFailureStages = map[string]struct{}{
	stageProcessLaunch:               {},
	stageInitialize:                  {},
	stageSessionCreate:               {},
	stageSessionResume:               {},
	stageConfiguration:               {},
	stageConfigModelUnavailable:      {},
	stageConfigReasoningUnavailable:  {},
	stageConfigConfirmationFailed:    {},
	stageConfigRequestFailed:         {},
	stagePermission:                  {},
	stagePermissionRequestInvalid:    {},
	stagePermissionClientUnavailable: {},
	stagePromptRejected:              {},
	stagePromptTransport:             {},
	stagePromptStreamClosed:          {},
	stagePromptMalformedFrame:        {},
	stagePromptReadFailed:            {},
	stagePromptWriteFailed:           {},
	stageProtocol:                    {},
	stageResult:                      {},
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

// These fixed local sentinels distinguish failures at Mender's ACP boundary,
// not the Agent's untrusted method, payload, or alleged intent. In particular,
// an invalid request never creates an approvable owner record.
var (
	errACPInvalidPermissionRequest    = errors.New("ACP permission request invalid")
	errACPUnsupportedClientCapability = errors.New("ACP client capability unavailable")
)

func tagPermissionFailure(err error) error {
	switch {
	case errors.Is(err, errACPInvalidPermissionRequest):
		return tagFailure(stagePermissionRequestInvalid, err)
	case errors.Is(err, errACPUnsupportedClientCapability):
		return tagFailure(stagePermissionClientUnavailable, err)
	default:
		return tagFailure(stagePermission, err)
	}
}

// tagSessionTransport tags setup/initialization/session-creation transport
// failures of wire.call. Permission failures retain their exact safe local
// cause; caller cancellation/deadline remain unchanged.
func tagSessionTransport(stage string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, errClientPermissionDenied) {
		return tagPermissionFailure(err)
	}
	return tagUnlessCancelled(stage, err)
}

// rpcRejectedError marks an explicit JSON-RPC error object returned by the
// remote Agent for one of our requests. The remote error text is never
// retained. The user-visible message is a fixed phrase independent of the
// trusted internal method name; method is kept only for type classification.
type rpcRejectedError struct{ method string }

// Fixed, method-independent message constants. The trusted method name keeps
// existing call sites working, but user-visible text never interpolates it.
const (
	rpcRejectedErrorMessage      = "ACP request rejected by Agent"
	malformedOutcomeErrorMessage = "ACP returned a malformed result"
)

func (e *rpcRejectedError) Error() string { return rpcRejectedErrorMessage }

// malformedOutcomeError marks a response that is not the bounded shape the
// protocol requires. The remote payload is never echoed.
type malformedOutcomeError struct{ method string }

func (e *malformedOutcomeError) Error() string { return malformedOutcomeErrorMessage }

// Fixed, backend-neutral prompt transport sentinels. Their names classify the
// failure shape only; messages are literal constants with no interpolation of
// remote RPC text, stdout contents, stderr, bearer tokens, paths or commands.
var (
	// errACPStreamClosed marks an ACP stdout stream that closed before a
	// matching session/prompt response arrived.
	errACPStreamClosed = errors.New("acp prompt stream closed before completion")
	// errACPInvalidFrame marks a frame that is not valid JSON.
	errACPInvalidFrame = errors.New("acp prompt frame was malformed")
	// errACPReadFailed marks a scanner I/O or frame-size read failure.
	errACPReadFailed = errors.New("acp prompt stream read failed")
	// errACPWriteFailed marks a failure writing the outbound request frame.
	errACPWriteFailed = errors.New("acp prompt request write failed")
)

// stagePromptError classifies a failed session/prompt interaction without
// ever exposing remote text:
//   - unresolved/rejected permission  -> permission
//   - explicit remote RPC rejection   -> prompt_rejected
//   - malformed final outcome         -> result
//   - caller cancellation/deadline    -> unchanged (established semantics)
//   - malformed stdout JSON, scanner IO, pipe write, premature EOF -> fixed
//     prompt transport sub-stages; other causes -> prompt_transport
func stagePromptError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, errClientPermissionDenied) {
		return tagPermissionFailure(err)
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
	// Bounded transport sub-classification. errors.Is matches through any
	// number of wrapped layers; stagePromptError already returned for
	// cancellation, permission, existing precise stages and typed outcomes,
	// so only unclassified transport errors reach this switch.
	// A scanner failure closes the same channel observed by wire.call.
	// The combined error therefore carries both the specific scanner cause
	// and the generic EOF. Prefer the original defect, never the secondary EOF.
	switch {
	case errors.Is(err, errACPInvalidFrame):
		return tagFailure(stagePromptMalformedFrame, err)
	case errors.Is(err, errACPReadFailed):
		return tagFailure(stagePromptReadFailed, err)
	case errors.Is(err, errACPWriteFailed):
		return tagFailure(stagePromptWriteFailed, err)
	case errors.Is(err, errACPStreamClosed):
		return tagFailure(stagePromptStreamClosed, err)
	}
	return tagFailure(stagePromptTransport, err)
}
