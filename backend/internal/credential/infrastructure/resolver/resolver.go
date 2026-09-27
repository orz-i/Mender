package resolver

import (
	"context"
	"fmt"
	"os"

	credentialapplication "github.com/orz-i/Mender/backend/internal/credential/application"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
)

type EnvironmentLookup func(string) (string, bool)

// Resolver resolves inventory-backed Credential References at the execution boundary.
type Resolver struct {
	repository credentialapplication.Repository
	secrets    credentialapplication.SecretStore
	lookupEnv  EnvironmentLookup
}

func New(
	repository credentialapplication.Repository,
	secrets credentialapplication.SecretStore,
	lookupEnv EnvironmentLookup,
) Resolver {
	if lookupEnv == nil {
		lookupEnv = os.LookupEnv
	}
	return Resolver{
		repository: repository,
		secrets:    secrets,
		lookupEnv:  lookupEnv,
	}
}

func (r Resolver) Resolve(
	ctx context.Context,
	ref credentialdomain.Reference,
) ([]byte, error) {
	credential, err := r.repository.Get(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("resolve credential: %w", err)
	}

	switch credential.Source() {
	case credentialdomain.SourceNative:
		secret, err := r.secrets.Get(ctx, ref)
		if err != nil {
			return nil, fmt.Errorf("resolve credential: %w", err)
		}
		return append([]byte(nil), secret...), nil

	case credentialdomain.SourceEnvironment:
		value, ok := r.lookupEnv(credential.EnvironmentVariable())
		if !ok {
			return nil, credentialapplication.ErrEnvironmentMissing
		}
		return []byte(value), nil

	default:
		return nil, credentialapplication.ErrSecretSourceUnavailable
	}
}
