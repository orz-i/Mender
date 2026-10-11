package application

import (
	"context"
	"fmt"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	tunneldomain "github.com/orz-i/Mender/backend/internal/tunnel/domain"
)

type CreateOpenAITunnelRequest struct {
	DisplayName          string
	ExternalTunnelID     string
	RuntimeCredentialRef credentialdomain.Reference
}

type CreateOpenAITunnel struct {
	repository  Repository
	ids         TunnelIDGenerator
	credentials CredentialLookup
}

func NewCreateOpenAITunnel(
	repository Repository,
	ids TunnelIDGenerator,
	credentials CredentialLookup,
) CreateOpenAITunnel {
	return CreateOpenAITunnel{
		repository:  repository,
		ids:         ids,
		credentials: credentials,
	}
}

func (uc CreateOpenAITunnel) Execute(
	ctx context.Context,
	request CreateOpenAITunnelRequest,
) (tunneldomain.Tunnel, error) {
	if uc.credentials == nil {
		return tunneldomain.Tunnel{}, fmt.Errorf("create tunnel: credential lookup is unavailable")
	}
	if _, err := uc.credentials.GetCredential(ctx, request.RuntimeCredentialRef); err != nil {
		return tunneldomain.Tunnel{}, fmt.Errorf(
			"create tunnel: runtime credential %s: %w",
			request.RuntimeCredentialRef,
			err,
		)
	}

	id, err := uc.ids.NewTunnelID()
	if err != nil {
		return tunneldomain.Tunnel{}, fmt.Errorf("create tunnel: allocate id: %w", err)
	}
	tunnel, err := tunneldomain.NewTunnel(
		id,
		request.DisplayName,
		tunneldomain.ProviderOpenAI,
		tunneldomain.OpenAIBinding(),
		request.ExternalTunnelID,
		request.RuntimeCredentialRef,
	)
	if err != nil {
		return tunneldomain.Tunnel{}, fmt.Errorf("create tunnel: %w", err)
	}
	if err := uc.repository.Create(ctx, tunnel); err != nil {
		return tunneldomain.Tunnel{}, fmt.Errorf("create tunnel: %w", err)
	}
	return tunnel, nil
}

type GetTunnel struct {
	repository Repository
}

func NewGetTunnel(repository Repository) GetTunnel {
	return GetTunnel{repository: repository}
}

func (uc GetTunnel) Execute(
	ctx context.Context,
	id tunneldomain.TunnelID,
) (tunneldomain.Tunnel, error) {
	tunnel, err := uc.repository.Get(ctx, id)
	if err != nil {
		return tunneldomain.Tunnel{}, fmt.Errorf("get tunnel: %w", err)
	}
	return tunnel, nil
}

type ListTunnels struct {
	repository Repository
}

func NewListTunnels(repository Repository) ListTunnels {
	return ListTunnels{repository: repository}
}

func (uc ListTunnels) Execute(ctx context.Context) ([]tunneldomain.Tunnel, error) {
	items, err := uc.repository.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tunnels: %w", err)
	}
	return items, nil
}

type DeleteTunnel struct {
	repository Repository
	usage      UsageChecker
}

func NewDeleteTunnel(repository Repository, usage UsageChecker) DeleteTunnel {
	return DeleteTunnel{repository: repository, usage: usage}
}

func (uc DeleteTunnel) Execute(
	ctx context.Context,
	id tunneldomain.TunnelID,
) error {
	if uc.usage != nil {
		usages, err := uc.usage.Usages(ctx, id)
		if err != nil {
			return fmt.Errorf("delete tunnel: check usage: %w", err)
		}
		if len(usages) != 0 {
			return fmt.Errorf("%w: %s", ErrTunnelInUse, usages[0].Description)
		}
	}
	if err := uc.repository.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete tunnel: %w", err)
	}
	return nil
}

type StartTunnel struct {
	repository  Repository
	credentials CredentialResolver
	runner      ProcessRunner
}

func NewStartTunnel(
	repository Repository,
	credentials CredentialResolver,
	runner ProcessRunner,
) StartTunnel {
	return StartTunnel{
		repository:  repository,
		credentials: credentials,
		runner:      runner,
	}
}

func (uc StartTunnel) Execute(
	ctx context.Context,
	id tunneldomain.TunnelID,
	endpoint string,
) (Process, tunneldomain.Tunnel, error) {
	tunnel, err := uc.repository.Get(ctx, id)
	if err != nil {
		return nil, tunneldomain.Tunnel{}, fmt.Errorf("start tunnel: load: %w", err)
	}
	if uc.credentials == nil {
		return nil, tunneldomain.Tunnel{}, fmt.Errorf("start tunnel: credential resolver is unavailable")
	}
	if uc.runner == nil {
		return nil, tunneldomain.Tunnel{}, fmt.Errorf("start tunnel: process runner is unavailable")
	}
	secret, err := uc.credentials.Resolve(ctx, tunnel.RuntimeCredentialRef())
	if err != nil {
		return nil, tunneldomain.Tunnel{}, fmt.Errorf("start tunnel: resolve runtime credential: %w", err)
	}
	defer clear(secret)

	process, err := uc.runner.Start(ctx, tunnel, endpoint, secret)
	if err != nil {
		return nil, tunneldomain.Tunnel{}, fmt.Errorf("start tunnel: %w", err)
	}
	return process, tunnel, nil
}
