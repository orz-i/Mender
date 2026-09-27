package application

import (
	"context"
	"time"

	auditdomain "github.com/orz-i/Mender/backend/internal/audit/domain"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
)

// ToolLookup resolves canonical Catalog metadata.
type ToolLookup interface {
	GetTool(context.Context, catalogdomain.ToolID) (catalogdomain.Tool, error)
}

// ProfileScopeAuthorizer resolves Profile membership without exposing Profile persistence.
type ProfileScopeAuthorizer interface {
	AllowsTool(context.Context, string, catalogdomain.ToolID) (bool, error)
}

// ArgumentValidator checks one JSON value against a canonical Tool schema.
// A false result means caller input is invalid; a non-nil error means the
// validator could not reliably evaluate the schema/instance.
type ArgumentValidator interface {
	Validate(
		context.Context,
		catalogdomain.Schema,
		invocationdomain.JSONValue,
	) (bool, error)
}

// GovernanceFactExtractor derives bounded Policy facts from validated input.
type GovernanceFactExtractor interface {
	Extract(
		context.Context,
		catalogdomain.Tool,
		invocationdomain.JSONValue,
	) (invocationdomain.GovernanceFacts, error)
}

// PolicyEvaluator decides whether an invocation may execute.
type PolicyEvaluator interface {
	Evaluate(context.Context, PolicyRequest) (policydomain.Decision, error)
}

// ApprovalGate handles policy decisions that require explicit approval.
type ApprovalGate interface {
	Approve(context.Context, ApprovalRequest) (bool, error)
}

// ExecutionGuard controls entry into provider execution critical sections.
type ExecutionGuard interface {
	Acquire(context.Context, ExecutionGuardRequest) (release func(), err error)
}

// ResultGuard applies post-execution governance before adapters receive an outcome.
type ResultGuard interface {
	Guard(
		context.Context,
		catalogdomain.Tool,
		invocationdomain.Outcome,
	) (invocationdomain.Outcome, error)
}

// ExecutorRouter resolves the outbound executor for a canonical Tool.
type ExecutorRouter interface {
	ExecutorFor(context.Context, catalogdomain.Tool) (ToolExecutor, error)
}

// ToolExecutor invokes one provider-specific Tool.
//
// Credential resolution is exposed directly to the executor boundary so the
// orchestration layer never receives resolved secret bytes.
type ToolExecutor interface {
	Execute(context.Context, ExecutionRequest, CredentialResolver) (invocationdomain.Outcome, error)
}

// CredentialResolver resolves an opaque reference into secret bytes.
//
// Implementations should return a fresh byte slice. Consumers are responsible
// for minimizing the lifetime of returned bytes.
type CredentialResolver interface {
	Resolve(context.Context, credentialdomain.Reference) ([]byte, error)
}

// AuditRecorder persists one data-minimized terminal record.
type AuditRecorder interface {
	Record(context.Context, auditdomain.Record) error
}

// InvocationIDGenerator allocates stable Mender invocation identities.
type InvocationIDGenerator interface {
	NewInvocationID() (invocationdomain.InvocationID, error)
}

// Clock supplies time to application use cases.
type Clock interface {
	Now() time.Time
}

// PolicyRequest intentionally excludes raw arguments and secret material.
type PolicyRequest struct {
	InvocationID          invocationdomain.InvocationID
	Tool                  catalogdomain.Tool
	Context               invocationdomain.InvocationContext
	Facts                 invocationdomain.GovernanceFacts
	Caller                string
	HasCredentialResolver bool
}

// ApprovalRequest intentionally excludes arguments and secret material.
type ApprovalRequest struct {
	InvocationID invocationdomain.InvocationID
	Tool         catalogdomain.Tool
	Caller       string
	Decision     policydomain.Decision
}

// ExecutionGuardRequest identifies the governed execution entering a critical section.
type ExecutionGuardRequest struct {
	InvocationID invocationdomain.InvocationID
	Tool         catalogdomain.Tool
	Context      invocationdomain.InvocationContext
}

// ExecutionRequest is the protocol-independent executor input.
type ExecutionRequest struct {
	InvocationID invocationdomain.InvocationID
	Tool         catalogdomain.Tool
	Arguments    invocationdomain.JSONValue
}
