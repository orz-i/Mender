package application

import (
	"context"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	tunneldomain "github.com/orz-i/Mender/backend/internal/tunnel/domain"
)

type Repository interface {
	Create(context.Context, tunneldomain.Tunnel) error
	Get(context.Context, tunneldomain.TunnelID) (tunneldomain.Tunnel, error)
	List(context.Context) ([]tunneldomain.Tunnel, error)
	Delete(context.Context, tunneldomain.TunnelID) error
}

type TunnelIDGenerator interface {
	NewTunnelID() (tunneldomain.TunnelID, error)
}

type CredentialLookup interface {
	GetCredential(
		context.Context,
		credentialdomain.Reference,
	) (credentialdomain.Credential, error)
}

type CredentialResolver interface {
	Resolve(context.Context, credentialdomain.Reference) ([]byte, error)
}

type Usage struct {
	Kind        string
	Description string
}

type UsageChecker interface {
	Usages(context.Context, tunneldomain.TunnelID) ([]Usage, error)
}

type Process interface {
	Stop(context.Context) error
	Done() <-chan error
}

// ProcessRunner is a typed transport dispatcher. Preflight must reject a
// disabled/wrong-kind/missing driver BEFORE StartTunnel resolves credentials.
type ProcessRunner interface {
	Preflight(context.Context, tunneldomain.Tunnel) error
	Start(context.Context, tunneldomain.Tunnel, string, []byte) (Process, error)
}

// HealthReporter provides cached evidence. Management catalogue reads never
// initiate plugin RPCs or infer "ready" merely because a child started.
type HealthReporter interface{ TunnelHealth() string }
