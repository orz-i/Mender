package application

import "errors"

// This interface is implemented by a trusted native Agent runner. The failure
// stage is an opaque fixed code, not an error string or user/model-provided
// input. The Application never formats an arbitrary error into a persisted
// Task diagnostic, Profile output or Wails response.
type safeAgentFailureStage interface {
	SafeAgentFailureStage() string
}

func agentFailureDiagnostic(err error) string {
	const generic = "Agent execution failed"
	var staged safeAgentFailureStage
	if !errors.As(err, &staged) {
		return generic
	}
	// Strict allowlist: even a malformed runner (or unexpected future
	// backend) cannot turn an arbitrary stage into leaked task content.
	switch staged.SafeAgentFailureStage() {
	case "process_launch":
		return "Agent process could not start"
	case "initialize":
		return "ACP initialization failed"
	case "session_create":
		return "ACP session creation failed"
	case "configuration":
		return "Agent model or reasoning configuration failed"
	case "permission":
		return "Agent permission request was rejected"
	case "prompt_rejected":
		return "Agent rejected the ACP prompt"
	case "prompt_transport":
		return "ACP prompt transport failed"
	case "prompt_stream_closed":
		return "ACP prompt stream closed before completion"
	case "prompt_malformed_frame":
		return "ACP prompt frame was malformed"
	case "prompt_read_failed":
		return "ACP prompt stream read failed"
	case "prompt_write_failed":
		return "ACP prompt request write failed"
	case "protocol":
		return "ACP protocol validation failed"
	case "result":
		return "Agent returned an invalid completion"
	default:
		return generic
	}
}
