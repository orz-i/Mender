package bootstrap

import (
	"context"
	"fmt"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	sandboxapplication "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sandboxdaytona "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/daytona"
	sandboxidentity "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/identity"
)

// configureDaytona installs only a credential-gated SDK factory. No API key is
// resolved or remote request made during Runtime startup or Provider discovery.
func (s *Sandboxes) configureDaytona(ctx context.Context, db string, resolver invocationapplication.CredentialResolver) (sandboxapplication.Backend, error) {
	if resolver == nil {
		return nil, fmt.Errorf("daytona requires Mender Credential Resolver")
	}
	factory, err := sandboxdaytona.NewSDKFactory(resolver)
	if err != nil {
		return nil, err
	}
	templates, err := sandboxdaytona.OpenTemplateStore(ctx, db)
	if err != nil {
		return nil, err
	}
	controller, err := sandboxdaytona.NewController(s.store, templates, factory, s.admission)
	if err != nil {
		_ = templates.Close()
		return nil, err
	}
	backend, err := sandboxdaytona.NewBackend(factory, templates)
	if err != nil {
		_ = templates.Close()
		return nil, err
	}
	s.daytona = controller
	s.daytonaTemplates = templates
	return backend, nil
}

// DaytonaConfigured reports adapter installation only; it does NOT imply
// Daytona API credentials, account capacity, live service or billing approval.
func (s *Sandboxes) DaytonaConfigured() bool {
	return s != nil && s.daytona != nil && s.daytonaTemplates != nil
}

type CreateDaytonaSandboxRequest struct {
	Name     string
	Template sandboxdaytona.Template
}

func (s *Sandboxes) CreateDaytonaSandbox(ctx context.Context, input CreateDaytonaSandboxRequest, caller governancedomain.Context) (sandboxdomain.Sandbox, error) {
	if !s.DaytonaConfigured() {
		return sandboxdomain.Sandbox{}, sandboxdomain.ErrBackendUnavailable
	}
	if err := caller.Validate(); err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	if err := input.Template.Validate(); err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	id, err := sandboxidentity.Generator{}.NewSandboxID()
	if err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	principal := caller.Principal()
	item, err := sandboxdomain.NewSandbox(sandboxdomain.Spec{ID: id, Name: input.Name, Backend: sandboxdomain.BackendDaytona, Isolation: sandboxdomain.IsolationRemote, OwnerKind: string(principal.Kind()), OwnerRef: principal.Reference()})
	if err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	if err := s.admission.AdmitAction(ctx, caller, item, governancedomain.ActionSandboxProvision); err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	// Check the reference BEFORE reserving one of two scarce/billable allocation
	// slots. Do not create a durable defined resource for a missing API key.
	if err := s.daytona.CheckCredential(ctx, input.Template); err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	if err := s.daytonaTemplates.CreateDefinedSandbox(ctx, item, input.Template); err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	if _, err := s.daytona.Provision(ctx, caller, id); err != nil {
		return item, fmt.Errorf("daytona provisioning outcome needs review for %s: %w", id, err)
	}
	return item, nil
}
func (s *Sandboxes) ReconcileDaytona(ctx context.Context, id sandboxdomain.ID, caller governancedomain.Context) (sandboxdomain.Run, error) {
	if !s.DaytonaConfigured() {
		return sandboxdomain.Run{}, sandboxdomain.ErrBackendUnavailable
	}
	return s.daytona.Reconcile(ctx, caller, id)
}

func (s *Sandboxes) DaytonaCredentialBindings(ctx context.Context, ref credentialdomain.Reference) ([]sandboxdomain.ID, error) {
	if !s.DaytonaConfigured() {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	return s.daytonaTemplates.ActiveCredentialBindings(ctx, ref)
}
