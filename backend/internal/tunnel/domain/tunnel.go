package domain

import (
	"fmt"
	"strings"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
)

const (
	maxTunnelDisplayNameLength = 160
	maxExternalTunnelIDLength  = 256
)

type Provider string

const (
	ProviderOpenAI   Provider = "openai"
	ProviderExternal Provider = "external"
)

// Tunnel is persistent non-secret configuration for one external reachability adapter.
type Tunnel struct {
	id                   TunnelID
	displayName          string
	provider             Provider
	binding              Binding
	externalTunnelID     string
	runtimeCredentialRef credentialdomain.Reference
}

func NewTunnel(
	id TunnelID,
	displayName string,
	provider Provider,
	binding Binding,
	externalTunnelID string,
	runtimeCredentialRef credentialdomain.Reference,
) (Tunnel, error) {
	if _, err := ParseTunnelID(id.String()); err != nil {
		return Tunnel{}, err
	}

	displayName = strings.TrimSpace(displayName)
	externalTunnelID = strings.TrimSpace(externalTunnelID)
	if err := ValidateNameAndExternalID(displayName, externalTunnelID); err != nil {
		return Tunnel{}, err
	}

	if err := binding.ValidateProvider(provider); err != nil {
		return Tunnel{}, err
	}

	if _, err := credentialdomain.ParseReference(runtimeCredentialRef.String()); err != nil {
		return Tunnel{}, fmt.Errorf("tunnel runtime_credential_ref: %w", err)
	}

	return Tunnel{
		id:                   id,
		displayName:          displayName,
		provider:             provider,
		binding:              binding,
		externalTunnelID:     externalTunnelID,
		runtimeCredentialRef: runtimeCredentialRef,
	}, nil
}

func (t Tunnel) ID() TunnelID {
	return t.id
}

func (t Tunnel) DisplayName() string {
	return t.displayName
}

func (t Tunnel) Provider() Provider {
	return t.provider
}

func (t Tunnel) ExternalTunnelID() string {
	return t.externalTunnelID
}

func (t Tunnel) RuntimeCredentialRef() credentialdomain.Reference {
	return t.runtimeCredentialRef
}

// Binding exposes a value copy of the exact extension identity.
func (t Tunnel) Binding() Binding { return t.binding }

// ValidateNameAndExternalID is the shared HTTP/Desktop/Domain request boundary.
// It does not allocate a resource or resolve a Credential Reference.
func ValidateNameAndExternalID(name, externalID string) error {
	name = strings.TrimSpace(name)
	externalID = strings.TrimSpace(externalID)
	if name == "" {
		return fmt.Errorf("tunnel display_name: must not be empty")
	}
	if len(name) > maxTunnelDisplayNameLength {
		return fmt.Errorf("tunnel display_name: is too long")
	}
	if externalID == "" {
		return fmt.Errorf("tunnel external_tunnel_id: must not be empty")
	}
	if len(externalID) > maxExternalTunnelIDLength {
		return fmt.Errorf("tunnel external_tunnel_id: is too long")
	}
	return nil
}
