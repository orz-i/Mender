package domain

import "fmt"

type IsolationMode string

const (
	IsolationHost           IsolationMode = "host"
	IsolationRestrictedHost IsolationMode = "restricted_host"
)

func (m IsolationMode) Validate() error {
	switch m {
	case IsolationHost, IsolationRestrictedHost:
		return nil
	default:
		return fmt.Errorf("runtime isolation mode: unknown value %q", m)
	}
}

type IsolationSpec struct {
	WorkspaceShell   IsolationMode
	ExternalProvider IsolationMode
	TunnelClient     IsolationMode
}

type Isolation struct {
	workspaceShell   IsolationMode
	externalProvider IsolationMode
	tunnelClient     IsolationMode
}

func DefaultIsolation(restrictedHostAvailable bool) Isolation {
	workspace := IsolationHost
	if restrictedHostAvailable {
		workspace = IsolationRestrictedHost
	}
	value, err := NewIsolation(IsolationSpec{
		WorkspaceShell:   workspace,
		ExternalProvider: IsolationHost,
		TunnelClient:     workspace,
	})
	if err != nil {
		panic("runtime governance: invalid default isolation: " + err.Error())
	}
	return value
}

func NewIsolation(spec IsolationSpec) (Isolation, error) {
	if err := spec.WorkspaceShell.Validate(); err != nil {
		return Isolation{}, fmt.Errorf("workspace_shell: %w", err)
	}
	if err := spec.ExternalProvider.Validate(); err != nil {
		return Isolation{}, fmt.Errorf("external_provider: %w", err)
	}
	if err := spec.TunnelClient.Validate(); err != nil {
		return Isolation{}, fmt.Errorf("tunnel_client: %w", err)
	}
	return Isolation{
		workspaceShell:   spec.WorkspaceShell,
		externalProvider: spec.ExternalProvider,
		tunnelClient:     spec.TunnelClient,
	}, nil
}

func (i Isolation) Validate() error {
	_, err := NewIsolation(i.Spec())
	return err
}

func (i Isolation) Spec() IsolationSpec {
	return IsolationSpec{
		WorkspaceShell:   i.workspaceShell,
		ExternalProvider: i.externalProvider,
		TunnelClient:     i.tunnelClient,
	}
}

func (i Isolation) WorkspaceShell() IsolationMode   { return i.workspaceShell }
func (i Isolation) ExternalProvider() IsolationMode { return i.externalProvider }
func (i Isolation) TunnelClient() IsolationMode     { return i.tunnelClient }
