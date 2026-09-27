package domain

import (
	"fmt"
	"strings"
)

const (
	maxInvocationCallerLength = 255
	maxScopeReferenceLength   = 255
)

// Origin identifies the runtime/adapter entry point for one Invocation.
type Origin string

const (
	OriginCLI            Origin = "cli"
	OriginMCP            Origin = "mcp"
	OriginManagementHTTP Origin = "management_http"
	OriginDesktop        Origin = "desktop"
	OriginInternal       Origin = "internal"
)

// ScopeKind identifies the execution authorization boundary.
type ScopeKind string

const (
	ScopeCatalog ScopeKind = "catalog"
	ScopeProfile ScopeKind = "profile"
)

// ExecutionScope is the explicit authorization boundary for one Invocation.
type ExecutionScope struct {
	kind      ScopeKind
	reference string
}

// CatalogExecutionScope grants explicit canonical Catalog scope.
func CatalogExecutionScope() ExecutionScope {
	return ExecutionScope{kind: ScopeCatalog}
}

// ProfileExecutionScope grants access through one Profile reference.
func ProfileExecutionScope(reference string) (ExecutionScope, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return ExecutionScope{}, validationError("execution_scope.reference", "must not be empty")
	}
	if len(reference) > maxScopeReferenceLength {
		return ExecutionScope{}, validationError("execution_scope.reference", "is too long")
	}
	return ExecutionScope{
		kind:      ScopeProfile,
		reference: reference,
	}, nil
}

func (s ExecutionScope) validate() error {
	switch s.kind {
	case ScopeCatalog:
		if s.reference != "" {
			return validationError("execution_scope.reference", "must be empty for catalog scope")
		}
		return nil
	case ScopeProfile:
		if strings.TrimSpace(s.reference) == "" {
			return validationError("execution_scope.reference", "must not be empty for profile scope")
		}
		if len(s.reference) > maxScopeReferenceLength {
			return validationError("execution_scope.reference", "is too long")
		}
		return nil
	default:
		return validationError("execution_scope.kind", fmt.Sprintf("unknown kind %q", s.kind))
	}
}

func (s ExecutionScope) Kind() ScopeKind {
	return s.kind
}

func (s ExecutionScope) Reference() string {
	return s.reference
}

// InvocationContext is non-secret execution context shared by all Invocation stages.
type InvocationContext struct {
	origin Origin
	caller string
	scope  ExecutionScope
}

// NewInvocationContext constructs one explicit Invocation execution context.
func NewInvocationContext(
	origin Origin,
	caller string,
	scope ExecutionScope,
) (InvocationContext, error) {
	switch origin {
	case OriginCLI, OriginMCP, OriginManagementHTTP, OriginDesktop, OriginInternal:
	default:
		return InvocationContext{}, validationError("origin", fmt.Sprintf("unknown origin %q", origin))
	}

	caller = strings.TrimSpace(caller)
	if len(caller) > maxInvocationCallerLength {
		return InvocationContext{}, validationError("caller", "is too long")
	}
	if err := scope.validate(); err != nil {
		return InvocationContext{}, err
	}

	return InvocationContext{
		origin: origin,
		caller: caller,
		scope:  scope,
	}, nil
}

// Validate rejects a zero or malformed InvocationContext.
func (c InvocationContext) Validate() error {
	_, err := NewInvocationContext(c.origin, c.caller, c.scope)
	return err
}

func (c InvocationContext) Origin() Origin {
	return c.origin
}

func (c InvocationContext) Caller() string {
	return c.caller
}

func (c InvocationContext) Scope() ExecutionScope {
	return c.scope
}
