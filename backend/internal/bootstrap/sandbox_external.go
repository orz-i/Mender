package bootstrap

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	installationapp "github.com/orz-i/Mender/backend/internal/plugin/installation/application"
	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
	installationidentity "github.com/orz-i/Mender/backend/internal/plugin/installation/infrastructure/identity"
	pluginprocess "github.com/orz-i/Mender/backend/internal/plugin/process"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sandboxexternal "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/external"
	sandboxidentity "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/identity"
	sandboxsqlite "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sqlite"
)

// RegisterExternalSandboxBackend updates the in-memory registry from an exact
// verified installation snapshot. This operation never launches plugin code.
func (s *Sandboxes) RegisterExternalSandboxBackend(
	item installationdomain.Installation, installations sandboxexternal.InstallationLookup,
	processes *pluginprocess.Supervisor) error {
	if s == nil || s.backendPlugins == nil {
		return sandboxdomain.ErrBackendUnavailable
	}
	backend, err := sandboxexternal.NewBackend(item, installations, processes, s.store)
	if err != nil {
		return err
	}
	descriptor, _ := item.VerifiedDescriptor()
	if err := s.backendPlugins.RegisterExternal(backend, descriptor, item.ID()); err != nil {
		return err
	}
	binding := sandboxdomain.BackendBinding{PluginID: item.PluginID(), ExtensionID: item.Extension().ID, InstallationID: item.ID()}
	if _, err := s.backendPlugins.ResolveLifecycle(sandboxdomain.BackendExternal, binding); err == nil {
		return nil
	}
	return s.backendPlugins.RegisterLifecycle(sandboxdomain.BackendExternal, binding, externalLifecycle{sandboxes: s})
}

func (r *Runtime) InstallSandboxPlugin(ctx context.Context, sourcePath string) (installationdomain.Installation, error) {
	if r == nil || r.pluginInstallations == nil || r.Sandboxes == nil {
		return installationdomain.Installation{}, fmt.Errorf("install Sandbox plugin: unavailable")
	}
	usecase := installationapp.NewInstallLocal(r.pluginInstallations, r.pluginPackages, installationidentity.NewGenerator(), systemClock{})
	item, err := usecase.Execute(ctx, sourcePath)
	if err != nil {
		return installationdomain.Installation{}, err
	}
	cleanup := func(opErr error) (installationdomain.Installation, error) {
		closeErr := r.externalProcesses.CloseInstallation(item.ID())
		removeErr := installationapp.NewRemoveInstallation(r.pluginInstallations, r.pluginPackages, nil).
			Execute(context.WithoutCancel(ctx), item.ID())
		return installationdomain.Installation{}, errors.Join(opErr, closeErr, removeErr)
	}
	if item.Extension().Kind != platformplugin.SandboxBackend {
		return cleanup(fmt.Errorf("install Sandbox plugin: expected sandbox.backend, got %s", item.Extension().Kind))
	}
	// Explicit package installation is a trusted local administrative action.
	// It is not a Sandbox provision/lease; this descriptor probe receives no
	// credential and does not create a remote Sandbox.
	raw, err := r.externalProcesses.ProcessHost().Ensure(ctx, item)
	if err != nil {
		return cleanup(fmt.Errorf("install Sandbox plugin: verify executable: %w", err))
	}
	native, ok := raw.(*sandboxexternal.Process)
	if !ok {
		return cleanup(fmt.Errorf("install Sandbox plugin: invalid process extension"))
	}
	descriptor := native.Descriptor()
	if err := r.pluginInstallations.SetVerifiedDescriptor(ctx, item.ID(), descriptor); err != nil {
		return cleanup(fmt.Errorf("install Sandbox plugin: persist descriptor: %w", err))
	}
	item, err = item.WithVerifiedDescriptor(descriptor)
	if err != nil {
		return cleanup(err)
	}
	if err := r.externalProcesses.CloseInstallation(item.ID()); err != nil {
		return cleanup(err)
	}
	if err := r.Sandboxes.RegisterExternalSandboxBackend(item, r.pluginInstallations, r.externalProcesses.ProcessHost()); err != nil {
		return cleanup(fmt.Errorf("install Sandbox plugin: register typed driver: %w", err))
	}
	return item, nil
}

func (r *Runtime) SetSandboxPluginInstallationStatus(ctx context.Context, id platformplugin.InstallationID, status installationdomain.Status) (installationdomain.Installation, error) {
	item, err := r.pluginInstallations.Get(ctx, id)
	if err != nil {
		return installationdomain.Installation{}, err
	}
	if item.Extension().Kind != platformplugin.SandboxBackend {
		return installationdomain.Installation{}, fmt.Errorf("plugin installation %s is not sandbox.backend", id)
	}
	count, err := r.Sandboxes.store.ReferencedInstallation(ctx, id.String())
	if err != nil {
		return installationdomain.Installation{}, err
	}
	if count > 0 && status != installationdomain.StatusEnabled {
		return installationdomain.Installation{}, fmt.Errorf("plugin %s referenced by %d Sandbox resources: %w", id, count, installationapp.ErrInstallationInUse)
	}
	changed, err := installationapp.NewSetInstallationStatus(r.pluginInstallations).Execute(ctx, id, status)
	if err != nil {
		return installationdomain.Installation{}, err
	}
	if status != installationdomain.StatusEnabled {
		if err := r.externalProcesses.CloseInstallation(id); err != nil {
			return changed, err
		}
	} else {
		// Explicit enable never starts process nor provisions remote resources.
		if err := r.Sandboxes.RegisterExternalSandboxBackend(changed, r.pluginInstallations, r.externalProcesses.ProcessHost()); err != nil {
			// Existing already-registered, disabled-and-reenabled drivers
			// may keep their immutable in-memory descriptor; see Registry.
			if !errors.Is(err, sandboxdomain.ErrConflict) {
				return changed, err
			}
		}
	}
	return changed, nil
}

func (r *Runtime) RemoveSandboxPluginInstallation(ctx context.Context, id platformplugin.InstallationID) error {
	item, err := r.pluginInstallations.Get(ctx, id)
	if err != nil {
		return err
	}
	if item.Extension().Kind != platformplugin.SandboxBackend {
		return fmt.Errorf("installation %s is not sandbox.backend", id)
	}
	count, err := r.Sandboxes.store.ReferencedInstallation(ctx, id.String())
	if err != nil {
		return err
	}
	if count != 0 {
		return fmt.Errorf("plugin %s in use by %d Sandbox resources: %w", id, count, installationapp.ErrInstallationInUse)
	}
	if err := r.externalProcesses.CloseInstallation(id); err != nil {
		return err
	}
	return installationapp.NewRemoveInstallation(r.pluginInstallations, r.pluginPackages, nil).Execute(ctx, id)
}

type CreateExternalSandboxRequest struct {
	Name           string
	InstallationID platformplugin.InstallationID
	Config         sandboxsqlite.ExternalConfig
}

func (s *Sandboxes) externalBackend(binding sandboxdomain.BackendBinding) (*sandboxexternal.Backend, error) {
	if s == nil || s.backendPlugins == nil {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	adapter, err := s.backendPlugins.ResolveExternal(binding)
	if err != nil {
		return nil, err
	}
	backend, ok := adapter.(*sandboxexternal.Backend)
	if !ok {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	return backend, nil
}

func sandboxOperationID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return "sbop_" + hex.EncodeToString(value[:]), nil
}

func (s *Sandboxes) CreateExternalSandbox(ctx context.Context, request CreateExternalSandboxRequest,
	caller governancedomain.Context) (sandboxdomain.Sandbox, error) {
	if s == nil || s.externalInstallationLookup == nil || s.store == nil {
		return sandboxdomain.Sandbox{}, sandboxdomain.ErrBackendUnavailable
	}
	if err := caller.Validate(); err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	// Service Principals (including external Profile/MCP callers) may only
	// acquire capabilities explicitly granted to a managed Profile. They must
	// never install/provision third-party native execution resources.
	if caller.Principal().Kind() != governancedomain.PrincipalLocalUser ||
		caller.Origin() == governancedomain.OriginMCP {
		return sandboxdomain.Sandbox{}, sandboxdomain.ErrNotAuthorized
	}
	if err := request.Config.Validate(); err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	installation, err := s.externalInstallationLookup.Get(ctx, request.InstallationID)
	if err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	if installation.Status() != installationdomain.StatusEnabled || installation.Extension().Kind != platformplugin.SandboxBackend {
		return sandboxdomain.Sandbox{}, sandboxdomain.ErrBackendUnavailable
	}
	descriptor, ok := installation.VerifiedDescriptor()
	if !ok || descriptor.ConfigVersion != request.Config.ConfigVersion {
		return sandboxdomain.Sandbox{}, sandboxdomain.ErrBackendUnavailable
	}
	id, err := sandboxidentity.Generator{}.NewSandboxID()
	if err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	owner := caller.Principal()
	binding := sandboxdomain.BackendBinding{PluginID: installation.PluginID(),
		ExtensionID: installation.Extension().ID, InstallationID: installation.ID()}
	item, err := sandboxdomain.NewSandbox(sandboxdomain.Spec{ID: id, Name: request.Name,
		Backend: sandboxdomain.BackendExternal, Binding: binding, Isolation: sandboxdomain.IsolationRemote,
		OwnerKind: string(owner.Kind()), OwnerRef: owner.Reference()})
	if err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	// This is the authority boundary. No third-party executable can start or
	// validate config through RPC until the owner's Policy/Audit allowed it.
	if err := s.admission.AdmitAction(ctx, caller, item, governancedomain.ActionSandboxProvision); err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	backend, err := s.externalBackend(binding)
	if err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	if err := backend.ValidateConfig(ctx, binding, request.Config); err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	if err := s.store.CreateDefinedExternal(ctx, item, request.Config); err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	current, err := s.store.GetRun(ctx, id)
	if err != nil {
		return item, err
	}
	inFlight, err := s.store.TransitionExternalRun(ctx, current, sandboxdomain.StateProvisioning, "",
		governancedomain.ActionSandboxProvision, caller, false)
	if err != nil {
		return item, err
	}
	opID, err := sandboxOperationID()
	if err != nil {
		return item, s.finishExternalUncertain(ctx, inFlight, item, caller, governancedomain.ActionSandboxProvision, err)
	}
	if err := s.store.BeginExternalOperation(ctx, inFlight, governancedomain.ActionSandboxProvision, caller, opID); err != nil {
		return item, s.finishExternalUncertain(ctx, inFlight, item, caller, governancedomain.ActionSandboxProvision, err)
	}
	observed, remoteErr := backend.Provision(ctx, item, inFlight, opID)
	if remoteErr != nil {
		return item, s.finishExternalUncertain(ctx, inFlight, item, caller, governancedomain.ActionSandboxProvision, remoteErr, opID)
	}
	if observed.Run.State != sandboxdomain.StateReady && observed.Run.State != sandboxdomain.StateUnknown &&
		observed.Run.State != sandboxdomain.StateFailed {
		return item, s.finishExternalUncertain(ctx, inFlight, item, caller, governancedomain.ActionSandboxProvision,
			fmt.Errorf("sandbox plugin reported invalid provision state %s", observed.Run.State), opID)
	}
	_, err = s.transitionExternalObserved(ctx, inFlight, observed.Run, governancedomain.ActionSandboxProvision, caller, opID)
	if err != nil {
		return item, err
	}
	if observed.Run.State != sandboxdomain.StateReady {
		return item, fmt.Errorf("sandbox %s provision requires explicit reconciliation: %s", id, observed.Run.State)
	}
	return item, nil
}

// State uncertainty must survive client cancellation. The host never retries
// remote provisioning or destruction. A separate admitted reconcile is required.
func (s *Sandboxes) finishExternalUncertain(ctx context.Context, inFlight sandboxdomain.Run,
	item sandboxdomain.Sandbox, caller governancedomain.Context, action governancedomain.Action,
	reason error, operationIDs ...string) error {
	recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_, stateErr := s.store.TransitionExternalRun(recovery, inFlight, sandboxdomain.StateUnknown,
		inFlight.ExternalID, action, caller, true)
	var journalErr error
	if len(operationIDs) != 0 {
		journalErr = s.store.FinishExternalOperation(recovery, operationIDs[0], true)
	}
	return errors.Join(fmt.Errorf("sandbox %s remote outcome uncertain (reconcile required): %w", item.ID(), reason), stateErr, journalErr)
}
func (s *Sandboxes) transitionExternalObserved(ctx context.Context, previous, observed sandboxdomain.Run,
	action governancedomain.Action, caller governancedomain.Context, operationIDs ...string) (sandboxdomain.Run, error) {
	if previous.SandboxID != observed.SandboxID || previous.Generation != observed.Generation {
		return sandboxdomain.Run{}, sandboxdomain.ErrConflict
	}
	recovered, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	next, err := s.store.TransitionExternalRun(recovered, previous, observed.State, observed.ExternalID, action, caller, true)
	if err != nil {
		return next, err
	}
	if len(operationIDs) != 0 {
		if err := s.store.FinishExternalOperation(recovered, operationIDs[0],
			observed.State == sandboxdomain.StateUnknown || observed.State == sandboxdomain.StateOrphaned); err != nil {
			return next, fmt.Errorf("record sandbox operation result: %w", err)
		}
	}
	return next, nil
}

// ChangeExternalLifecycle is invoked only after the owner has explicitly
// selected this exact Sandbox resource and lifecycle operation.
func (s *Sandboxes) ChangeExternalLifecycle(ctx context.Context, id sandboxdomain.ID,
	action governancedomain.Action, caller governancedomain.Context) (sandboxdomain.Run, error) {
	item, err := s.store.Get(ctx, id)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	if item.Backend() != sandboxdomain.BackendExternal {
		return sandboxdomain.Run{}, sandboxdomain.ErrBackendUnavailable
	}
	if action != governancedomain.ActionSandboxStart && action != governancedomain.ActionSandboxStop &&
		action != governancedomain.ActionSandboxDestroy {
		return sandboxdomain.Run{}, fmt.Errorf("unsupported external lifecycle action %s", action)
	}
	current, err := s.store.GetRun(ctx, id)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	var staging, success sandboxdomain.State
	var method string
	switch action {
	case governancedomain.ActionSandboxStart:
		if current.State != sandboxdomain.StateStopped {
			return sandboxdomain.Run{}, sandboxdomain.ErrConflict
		}
		staging, success, method = sandboxdomain.StateProvisioning, sandboxdomain.StateReady, "start"
	case governancedomain.ActionSandboxStop:
		if current.State != sandboxdomain.StateReady {
			return sandboxdomain.Run{}, sandboxdomain.ErrConflict
		}
		staging, success, method = sandboxdomain.StateStopping, sandboxdomain.StateStopped, "stop"
	case governancedomain.ActionSandboxDestroy:
		if current.State != sandboxdomain.StateReady && current.State != sandboxdomain.StateStopped &&
			current.State != sandboxdomain.StateUnknown && current.State != sandboxdomain.StateOrphaned {
			return sandboxdomain.Run{}, sandboxdomain.ErrConflict
		}
		staging, success, method = sandboxdomain.StateDestroying, sandboxdomain.StateDestroyed, "destroy"
	}
	if err := s.admission.AdmitAction(ctx, caller, item, action); err != nil {
		return sandboxdomain.Run{}, err
	}
	backend, err := s.externalBackend(item.Spec().Binding)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	staged, err := s.store.TransitionExternalRun(ctx, current, staging, current.ExternalID, action, caller, false)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	opID, err := sandboxOperationID()
	if err != nil {
		return staged, s.finishExternalUncertain(ctx, staged, item, caller, action, err)
	}
	// A restart increments generation, while the external run ID still refers
	// to its previous stopped instance. The plugin sees the new fenced token.
	remoteRun := staged
	if action == governancedomain.ActionSandboxStart {
		remoteRun.ExternalID = current.ExternalID
	}
	if err := s.store.BeginExternalOperation(ctx, staged, action, caller, opID); err != nil {
		return staged, s.finishExternalUncertain(ctx, staged, item, caller, action, err)
	}
	observed, remoteErr := backend.Change(ctx, item, remoteRun, opID, method)
	if remoteErr != nil {
		return staged, s.finishExternalUncertain(ctx, staged, item, caller, action, remoteErr, opID)
	}
	if observed.Run.State != success && observed.Run.State != sandboxdomain.StateUnknown &&
		observed.Run.State != sandboxdomain.StateFailed {
		return staged, s.finishExternalUncertain(ctx, staged, item, caller, action,
			fmt.Errorf("unexpected external lifecycle state %s", observed.Run.State), opID)
	}
	next, err := s.transitionExternalObserved(ctx, staged, observed.Run, action, caller, opID)
	if err != nil {
		return staged, err
	}
	if next.State != success {
		return next, fmt.Errorf("sandbox %s lifecycle requires explicit reconciliation", id)
	}
	return next, nil
}

func (s *Sandboxes) ReconcileExternalSandbox(ctx context.Context, id sandboxdomain.ID,
	caller governancedomain.Context) (sandboxdomain.Run, error) {
	item, err := s.store.Get(ctx, id)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	if item.Backend() != sandboxdomain.BackendExternal {
		return sandboxdomain.Run{}, sandboxdomain.ErrBackendUnavailable
	}
	current, err := s.store.GetRun(ctx, id)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	// A Runtime crash between the durable in-progress CAS and the outbound
	// RPC can leave provisioning/stopping/destroying without an operation
	// response. Explicit owner-admitted reconcile must recover those states.
	switch current.State {
	case sandboxdomain.StateUnknown, sandboxdomain.StateOrphaned,
		sandboxdomain.StateProvisioning, sandboxdomain.StateStopping, sandboxdomain.StateDestroying:
	default:
		return sandboxdomain.Run{}, sandboxdomain.ErrConflict
	}
	if err := s.admission.AdmitAction(ctx, caller, item, governancedomain.ActionSandboxReconcile); err != nil {
		return sandboxdomain.Run{}, err
	}
	backend, err := s.externalBackend(item.Spec().Binding)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	if current.State != sandboxdomain.StateUnknown && current.State != sandboxdomain.StateOrphaned {
		// Mark the uncertain pre-crash stage visibly UNKNOWN before remote
		// observation. No side-effecting action is replayed.
		current, err = s.store.TransitionExternalRun(ctx, current, sandboxdomain.StateUnknown,
			current.ExternalID, governancedomain.ActionSandboxReconcile, caller, true)
		if err != nil {
			return sandboxdomain.Run{}, err
		}
	}
	opID, err := sandboxOperationID()
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	if err := s.store.BeginExternalOperation(ctx, current, governancedomain.ActionSandboxReconcile, caller, opID); err != nil {
		return current, err
	}
	observed, err := backend.Change(ctx, item, current, opID, "reconcile")
	if err != nil {
		recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		journalErr := s.store.FinishExternalOperation(recovery, opID, true)
		return current, errors.Join(fmt.Errorf("sandbox %s reconciliation still uncertain: %w", id, err), journalErr)
	}
	if observed.Run.State == sandboxdomain.StateUnknown {
		recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		return current, s.store.FinishExternalOperation(recovery, opID, true)
	}
	return s.transitionExternalObserved(ctx, current, observed.Run, governancedomain.ActionSandboxReconcile, caller, opID)
}
