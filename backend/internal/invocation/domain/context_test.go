package domain

import (
	"strings"
	"testing"
)

func TestInvocationContextRequiresExplicitOriginAndScope(t *testing.T) {
	t.Parallel()

	if err := (InvocationContext{}).Validate(); err == nil {
		t.Fatal("zero InvocationContext Validate() expected error")
	}

	if _, err := NewInvocationContext(
		Origin("unknown"),
		"",
		CatalogExecutionScope(),
	); err == nil {
		t.Fatal("NewInvocationContext() accepted unknown origin")
	}

	if _, err := ProfileExecutionScope(""); err == nil {
		t.Fatal("ProfileExecutionScope() accepted empty reference")
	}

	longCaller := strings.Repeat("x", maxInvocationCallerLength+1)
	if _, err := NewInvocationContext(
		OriginCLI,
		longCaller,
		CatalogExecutionScope(),
	); err == nil {
		t.Fatal("NewInvocationContext() accepted caller longer than limit")
	}
}

func TestInvocationContextCatalogScope(t *testing.T) {
	t.Parallel()

	ctx, err := NewInvocationContext(
		OriginCLI,
		"  local-cli  ",
		CatalogExecutionScope(),
	)
	if err != nil {
		t.Fatalf("NewInvocationContext() error = %v", err)
	}
	if ctx.Origin() != OriginCLI {
		t.Fatalf("Origin() = %q", ctx.Origin())
	}
	if ctx.Caller() != "local-cli" {
		t.Fatalf("Caller() = %q", ctx.Caller())
	}
	if ctx.Scope().Kind() != ScopeCatalog || ctx.Scope().Reference() != "" {
		t.Fatalf("Scope() = kind %q reference %q", ctx.Scope().Kind(), ctx.Scope().Reference())
	}
}

func TestInvocationContextProfileScope(t *testing.T) {
	t.Parallel()

	scope, err := ProfileExecutionScope("prof_00000000000000000000000000000001")
	if err != nil {
		t.Fatalf("ProfileExecutionScope() error = %v", err)
	}
	ctx, err := NewInvocationContext(OriginMCP, "mcp:default", scope)
	if err != nil {
		t.Fatalf("NewInvocationContext() error = %v", err)
	}
	if ctx.Scope().Kind() != ScopeProfile {
		t.Fatalf("Scope kind = %q", ctx.Scope().Kind())
	}
	if got := ctx.Scope().Reference(); got != "prof_00000000000000000000000000000001" {
		t.Fatalf("Scope reference = %q", got)
	}
}
