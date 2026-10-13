package bootstrap

import (
	"context"
	"fmt"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sandboxidentity "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/identity"
	sshremote "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sshremote"
)

type CreateSSHRemoteSandboxRequest struct {
	Name     string
	Template sshremote.Template
}

// SSH Remote is a connection-only remote account binding; there is no host
// installation or SSH server provisioning. Admission precedes all I/O.
func (s *Sandboxes) CreateSSHRemoteSandbox(ctx context.Context, input CreateSSHRemoteSandboxRequest, caller governancedomain.Context) (sandboxdomain.Sandbox, error) {
	if s == nil || s.store == nil || s.sshRemote == nil {
		return sandboxdomain.Sandbox{}, sandboxdomain.ErrBackendUnavailable
	}
	if err := caller.Validate(); err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	// Only an actual local operator may configure a remote login. A Profile
	// service, MCP client or model Tool cannot make Mender contact an
	// arbitrary SSH host even if it happens to possess Sandbox grants.
	if caller.Principal().Kind() != governancedomain.PrincipalLocalUser ||
		caller.Origin() == governancedomain.OriginMCP {
		return sandboxdomain.Sandbox{}, sandboxdomain.ErrNotAuthorized
	}
	if err := input.Template.Validate(); err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	id, err := sandboxidentity.Generator{}.NewSandboxID()
	if err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	owner := caller.Principal()
	item, err := sandboxdomain.NewSandbox(sandboxdomain.Spec{
		ID: id, Name: input.Name, Backend: sandboxdomain.BackendSSHRemote,
		Binding:   sandboxdomain.BuiltinBinding(sandboxdomain.BackendSSHRemote),
		Isolation: sandboxdomain.IsolationRemote,
		OwnerKind: string(owner.Kind()), OwnerRef: owner.Reference(),
	})
	if err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	if err := s.admission.AdmitAction(ctx, caller, item, governancedomain.ActionSandboxProvision); err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	conn, err := s.sshRemote.Dial.Connect(ctx, input.Template)
	if err != nil {
		return sandboxdomain.Sandbox{}, fmt.Errorf("SSH remote connection failed: %w", err)
	}
	defer func() { _ = conn.Close() }()
	run, err := sandboxdomain.NewRun(id, 1, sandboxdomain.StateReady, "ssh_remote:"+id.String())
	if err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	if err := s.store.CreateReadySSH(ctx, item, run, input.Template); err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	return item, nil
}
func (s *Sandboxes) SSHCredentialBindings(ctx context.Context, ref credentialdomain.Reference) ([]sandboxdomain.ID, error) {
	if s == nil || s.store == nil {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	return s.store.SSHCredentialBindings(ctx, ref)
}
func (s *Sandboxes) ChangeSSHRemoteLifecycle(ctx context.Context, id sandboxdomain.ID, action governancedomain.Action, caller governancedomain.Context) (sandboxdomain.Run, error) {
	if s == nil || s.store == nil || s.sshRemote == nil {
		return sandboxdomain.Run{}, sandboxdomain.ErrBackendUnavailable
	}
	if err := caller.Validate(); err != nil {
		return sandboxdomain.Run{}, err
	}
	item, err := s.store.Get(ctx, id)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	if item.Backend() != sandboxdomain.BackendSSHRemote {
		return sandboxdomain.Run{}, sandboxdomain.ErrBackendUnavailable
	}
	if action != governancedomain.ActionSandboxStart && action != governancedomain.ActionSandboxStop && action != governancedomain.ActionSandboxDestroy {
		return sandboxdomain.Run{}, sandboxdomain.ErrCapabilityUnavailable
	}
	run, err := s.store.GetRun(ctx, id)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	if err := s.admission.AdmitAction(ctx, caller, item, action); err != nil {
		return sandboxdomain.Run{}, err
	}
	if action == governancedomain.ActionSandboxStart {
		// Re-verify the pinned identity before re-enabling new execution leases.
		t, err := s.store.GetSSHRemoteTemplate(ctx, id)
		if err != nil {
			return sandboxdomain.Run{}, err
		}
		conn, err := s.sshRemote.Dial.Connect(ctx, t)
		if err != nil {
			return sandboxdomain.Run{}, sandboxdomain.ErrBackendUnavailable
		}
		_ = conn.Close()
	}
	return s.store.ChangeSSHRemoteLifecycle(ctx, run, action, caller)
}
