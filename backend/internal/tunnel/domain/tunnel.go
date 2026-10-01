package domain

import (
	"fmt"
	"strings"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
)

const (
	maxTunnelDisplayNameLength = 160
	maxExternalTunnelIDLength  = 256
	maxExecutableLength        = 4096
)

type Provider string

const (
	ProviderOpenAI Provider = "openai"
)

// Tunnel is persistent non-secret configuration for one external reachability adapter.
type Tunnel struct {
	id                   TunnelID
	displayName          string
	provider             Provider
	externalTunnelID     string
	runtimeCredentialRef credentialdomain.Reference
	executable           string
}

func NewTunnel(
	id TunnelID,
	displayName string,
	provider Provider,
	externalTunnelID string,
	runtimeCredentialRef credentialdomain.Reference,
	executable string,
) (Tunnel, error) {
	if _, err := ParseTunnelID(id.String()); err != nil {
		return Tunnel{}, err
	}

	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return Tunnel{}, fmt.Errorf("tunnel display_name: must not be empty")
	}
	if len(displayName) > maxTunnelDisplayNameLength {
		return Tunnel{}, fmt.Errorf("tunnel display_name: is too long")
	}

	switch provider {
	case ProviderOpenAI:
	default:
		return Tunnel{}, fmt.Errorf("tunnel provider: unsupported value %q", provider)
	}

	externalTunnelID = strings.TrimSpace(externalTunnelID)
	if externalTunnelID == "" {
		return Tunnel{}, fmt.Errorf("tunnel external_tunnel_id: must not be empty")
	}
	if len(externalTunnelID) > maxExternalTunnelIDLength {
		return Tunnel{}, fmt.Errorf("tunnel external_tunnel_id: is too long")
	}

	if _, err := credentialdomain.ParseReference(runtimeCredentialRef.String()); err != nil {
		return Tunnel{}, fmt.Errorf("tunnel runtime_credential_ref: %w", err)
	}

	executable = strings.TrimSpace(executable)
	if executable == "" {
		return Tunnel{}, fmt.Errorf("tunnel executable: must not be empty")
	}
	if len(executable) > maxExecutableLength {
		return Tunnel{}, fmt.Errorf("tunnel executable: is too long")
	}

	return Tunnel{
		id:                   id,
		displayName:          displayName,
		provider:             provider,
		externalTunnelID:     externalTunnelID,
		runtimeCredentialRef: runtimeCredentialRef,
		executable:           executable,
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

func (t Tunnel) Executable() string {
	return t.executable
}
