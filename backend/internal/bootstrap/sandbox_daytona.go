package bootstrap

import (
	"context"
	"fmt"
	"os"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	sandboxapplication "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sandboxdaytona "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/daytona"
	sandboxidentity "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/identity"
)

// DaytonaExperimentalEnvironment requires explicit host-operator opt-in for
// a remote adapter whose real live provisioning integration is NOT accepted.
// An HTTP request, Tool invocation or Provider config cannot set this flag.
const DaytonaExperimentalEnvironment = "MENDER_DAYTONA_EXPERIMENTAL"
const daytonaExperimentalOptIn = "enabled"

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
	// Keep immutable inventory available even when the unverified remote
	// runtime is quarantined; Credential deletion protection still applies.
	// Do not register the executable Backend until the operator opts in.
	s.daytonaExperimental = os.Getenv(DaytonaExperimentalEnvironment) == daytonaExperimentalOptIn
	if !s.daytonaExperimental {
		return nil, nil
	}
	return backend, nil
}

// DaytonaConfigured means explicitly opted-in experimental execution
// capability, NOT a claim that Daytona service, billing or credentials work.
func (s *Sandboxes) DaytonaConfigured() bool {
	return s != nil && s.daytonaExperimental && s.daytona != nil && s.daytonaTemplates != nil
}

// DaytonaStatus is an intentionally truthful capability label, not an SDK
// connectivity or live billing test.
func (s *Sandboxes) DaytonaStatus() string {
	if !s.DaytonaConfigured() {
		return "Experimental Daytona disabled; local operator must set MENDER_DAYTONA_EXPERIMENTAL=enabled before Runtime startup"
	}
	return "Experimental Daytona enabled by operator; credential and live provider validation required; may incur charges"
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
	// Read-only inventory remains accessible while remote execution is
	// disabled, preventing the removal of a still-referenced secret.
	if s == nil || s.daytonaTemplates == nil {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	return s.daytonaTemplates.ActiveCredentialBindings(ctx, ref)
}
