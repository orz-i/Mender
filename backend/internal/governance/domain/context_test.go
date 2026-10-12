package domain

import "testing"

func TestPrincipalAndContextValidation(t *testing.T) {
	t.Parallel()

	if _, err := NewPrincipal("", "x"); err == nil {
		t.Fatal("NewPrincipal accepted unknown kind")
	}
	if _, err := NewPrincipal(PrincipalLocalUser, ""); err == nil {
		t.Fatal("NewPrincipal accepted empty reference")
	}

	principal, err := NewPrincipal(PrincipalLocalUser, "os:501")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := ProfileScope("prof_00000000000000000000000000000001")
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := NewContext(OriginCLI, principal, scope)
	if err != nil {
		t.Fatal(err)
	}
	if ctx.Principal().Kind() != PrincipalLocalUser ||
		ctx.Principal().Reference() != "os:501" ||
		ctx.Scope().Kind() != ScopeProfile {
		t.Fatalf("context = %#v", ctx)
	}
}

func TestContextRejectsZeroPrincipal(t *testing.T) {
	t.Parallel()
	if _, err := NewContext(OriginCLI, Principal{}, CatalogScope()); err == nil {
		t.Fatal("NewContext accepted zero Principal")
	}
}

func TestActionValidation(t *testing.T) {
	t.Parallel()
	for _, action := range []Action{ActionToolInvoke, ActionProviderActivate, ActionSandboxAcquire, ActionSandboxProvision, ActionSandboxStart, ActionSandboxStop, ActionSandboxDestroy, ActionSandboxReconcile, ActionAgentActivate, ActionAgentTaskSubmit, ActionAgentTaskCancel} {
		if err := action.Validate(); err != nil {
			t.Fatalf("%s: %v", action, err)
		}
	}
	if err := Action("future").Validate(); err == nil {
		t.Fatal("Action accepted unknown value")
	}
}
