package domain

import (
	"fmt"
	"strings"
)

const (
	maxPrincipalReferenceLength = 255
	maxScopeReferenceLength     = 255
)

// Action identifies one operation class governed by Policy.
type Action string

const (
	ActionToolInvoke       Action = "tool.invoke"
	ActionProviderActivate Action = "provider.activate"
	ActionSandboxAcquire   Action = "sandbox.acquire"
	ActionSandboxProvision Action = "sandbox.provision"
	ActionSandboxStart     Action = "sandbox.start"
	ActionSandboxStop      Action = "sandbox.stop"
	ActionSandboxDestroy   Action = "sandbox.destroy"
	ActionSandboxReconcile Action = "sandbox.reconcile"
)

func (a Action) Validate() error {
	switch a {
	case ActionToolInvoke, ActionProviderActivate,
		ActionSandboxAcquire, ActionSandboxProvision, ActionSandboxStart,
		ActionSandboxStop, ActionSandboxDestroy, ActionSandboxReconcile:
		return nil
	default:
		return fmt.Errorf("governance action: unknown value %q", a)
	}
}

// PrincipalKind identifies the assurance/source class of one live Principal.
type PrincipalKind string

const (
	PrincipalLocalUser PrincipalKind = "local_user"
	PrincipalService   PrincipalKind = "service"
)

// Principal is one explicit non-anonymous security subject.
type Principal struct {
	kind      PrincipalKind
	reference string
}

func NewPrincipal(kind PrincipalKind, reference string) (Principal, error) {
	switch kind {
	case PrincipalLocalUser, PrincipalService:
	default:
		return Principal{}, fmt.Errorf("principal.kind: unknown value %q", kind)
	}
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return Principal{}, fmt.Errorf("principal.reference: must not be empty")
	}
	if len(reference) > maxPrincipalReferenceLength {
		return Principal{}, fmt.Errorf("principal.reference: is too long")
	}
	return Principal{kind: kind, reference: reference}, nil
}

func (p Principal) Validate() error {
	_, err := NewPrincipal(p.kind, p.reference)
	return err
}

func (p Principal) Kind() PrincipalKind { return p.kind }
func (p Principal) Reference() string   { return p.reference }

// Origin identifies the trusted adapter/runtime entry point.
type Origin string

const (
	OriginCLI            Origin = "cli"
	OriginMCP            Origin = "mcp"
	OriginManagementHTTP Origin = "management_http"
	OriginDesktop        Origin = "desktop"
	OriginInternal       Origin = "internal"
)

func validateOrigin(origin Origin) error {
	switch origin {
	case OriginCLI, OriginMCP, OriginManagementHTTP, OriginDesktop, OriginInternal:
		return nil
	default:
		return fmt.Errorf("origin: unknown value %q", origin)
	}
}

// ScopeKind identifies the authorization boundary for a governed action.
type ScopeKind string

const (
	ScopeCatalog ScopeKind = "catalog"
	ScopeProfile ScopeKind = "profile"
)

// Scope is one explicit Catalog or Profile governance boundary.
type Scope struct {
	kind      ScopeKind
	reference string
}

func CatalogScope() Scope {
	return Scope{kind: ScopeCatalog}
}

func ProfileScope(reference string) (Scope, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return Scope{}, fmt.Errorf("governance scope.reference: must not be empty")
	}
	if len(reference) > maxScopeReferenceLength {
		return Scope{}, fmt.Errorf("governance scope.reference: is too long")
	}
	return Scope{kind: ScopeProfile, reference: reference}, nil
}

func (s Scope) Validate() error {
	switch s.kind {
	case ScopeCatalog:
		if s.reference != "" {
			return fmt.Errorf("governance scope.reference: must be empty for catalog scope")
		}
	case ScopeProfile:
		if strings.TrimSpace(s.reference) == "" {
			return fmt.Errorf("governance scope.reference: must not be empty for profile scope")
		}
		if len(s.reference) > maxScopeReferenceLength {
			return fmt.Errorf("governance scope.reference: is too long")
		}
	default:
		return fmt.Errorf("governance scope.kind: unknown value %q", s.kind)
	}
	return nil
}

func (s Scope) Kind() ScopeKind   { return s.kind }
func (s Scope) Reference() string { return s.reference }

// Context is the bounded security context shared by governed actions.
type Context struct {
	origin    Origin
	principal Principal
	scope     Scope
}

func NewContext(origin Origin, principal Principal, scope Scope) (Context, error) {
	if err := validateOrigin(origin); err != nil {
		return Context{}, err
	}
	if err := principal.Validate(); err != nil {
		return Context{}, err
	}
	if err := scope.Validate(); err != nil {
		return Context{}, err
	}
	return Context{origin: origin, principal: principal, scope: scope}, nil
}

func (c Context) Validate() error {
	_, err := NewContext(c.origin, c.principal, c.scope)
	return err
}

func (c Context) Origin() Origin       { return c.origin }
func (c Context) Principal() Principal { return c.principal }
func (c Context) Scope() Scope         { return c.scope }
