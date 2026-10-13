package bootstrap

import (
	"context"
	"fmt"
	"time"

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

// InspectSSHHostKey is an operator-only pre-auth handshake. Admission precedes
// outbound network access; it does not create a Sandbox or resolve a key.
func (s *Sandboxes) InspectSSHHostKey(ctx context.Context, host string, port uint16, caller governancedomain.Context) (sshremote.HostIdentity, error) {
	if s == nil || s.sshRemote == nil || s.sshRemote.Dial.Trust == nil {
		return sshremote.HostIdentity{}, sandboxdomain.ErrBackendUnavailable
	}
	if err := caller.Validate(); err != nil {
		return sshremote.HostIdentity{}, err
	}
	if caller.Principal().Kind() != governancedomain.PrincipalLocalUser ||
		caller.Origin() == governancedomain.OriginMCP {
		return sshremote.HostIdentity{}, sandboxdomain.ErrNotAuthorized
	}
	id, err := sandboxidentity.Generator{}.NewSandboxID()
	if err != nil {
		return sshremote.HostIdentity{}, err
	}
	principal := caller.Principal()
	pending, err := sandboxdomain.NewSandbox(sandboxdomain.Spec{
		ID: id, Name: "SSH host identity inspection",
		Backend:   sandboxdomain.BackendSSHRemote,
		Binding:   sandboxdomain.BuiltinBinding(sandboxdomain.BackendSSHRemote),
		Isolation: sandboxdomain.IsolationRemote,
		OwnerKind: string(principal.Kind()), OwnerRef: principal.Reference(),
	})
	if err != nil {
		return sshremote.HostIdentity{}, err
	}
	if err := s.admission.AdmitAction(ctx, caller, pending, governancedomain.ActionSandboxProvision); err != nil {
		return sshremote.HostIdentity{}, err
	}
	return s.sshRemote.Dial.Trust.Inspect(ctx, host, port)
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
	// Persist only the host key proven by the exact pinned SSH handshake.
	// Mender maintains its own known_hosts; the user's SSH files stay untouched.
	if err := s.sshRemote.Dial.Trust.RememberPinned(input.Template.Host, input.Template.Port, input.Template.HostPublicKey); err != nil {
		return sandboxdomain.Sandbox{}, err
	}
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
	// Only SSH resources that have actually admitted managed Job submission
	// require a remote no-active/unknown-Jobs proof. Existing unenrolled SSH
	// resources retain their passive offline Stop/Destroy behavior. A concurrent
	// new submit still cannot escape: the Store rechecks the marker and lease
	// inside its atomic state transition.
	managed, err := s.store.SSHJobManaged(ctx, id)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	if managed && action != governancedomain.ActionSandboxStart {
		// The exact exclusive execution lease blocks new job submissions and
		// Workspace operations during remote reconciliation. An unknown/partial
		// remote outcome must keep the Sandbox Ready/Stopped, not invent success.
		token, e := s.store.ReserveSSHJobLifecycle(ctx, run, caller)
		if e != nil {
			return sandboxdomain.Run{}, e
		}
		consumed := false
		defer func() {
			if !consumed {
				cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
				defer cancel()
				_ = s.store.ReleaseExecution(cleanup, id, token)
			}
		}()
		if err := s.sshRemote.QuiescentJobs(ctx, id); err != nil {
			return sandboxdomain.Run{}, fmt.Errorf("SSH jobs have unknown or active execution; lifecycle refused: %w", err)
		}
		next, e := s.store.ChangeSSHRemoteLifecycleFenced(ctx, run, action, caller, token)
		if e == nil {
			consumed = true
		}
		return next, e
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
