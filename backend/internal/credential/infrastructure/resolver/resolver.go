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

// AuthorizeCredential proves that an OAuth reference is a connected inventory
// entry with OS-backed secret material, without returning a bearer token.
// OAuth token consumption remains private to the MCP OAuth transport.
func (r Resolver) AuthorizeCredential(ctx context.Context, ref credentialdomain.Reference) error {
	credential, err := r.repository.Get(ctx, ref)
	if err != nil {
		return fmt.Errorf("authorize credential: %w", err)
	}
	metadata := credential.OAuthMetadata()
	if credential.Source() != credentialdomain.SourceOAuth || metadata == nil || metadata.Status != credentialdomain.OAuthConnected {
		return credentialapplication.ErrSecretSourceUnavailable
	}
	secret, err := r.secrets.Get(ctx, ref)
	if err != nil {
		return fmt.Errorf("authorize credential: %w", err)
	}
	clear(secret)
	return nil
}

func (r Resolver) Resolve(
	ctx context.Context,
	ref credentialdomain.Reference,
) ([]byte, error) {
	credential, err := r.repository.Get(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("resolve credential: %w", err)
	}
	// SSH private keys must come from the OS keyring even for historical
	// credential rows created before the dedicated SSH namespace contract.
	// Never read them from an environment variable or another fallback.
	if ref.Namespace() == "ssh" && credential.Source() != credentialdomain.SourceNative {
		return nil, credentialapplication.ErrSecretSourceUnavailable
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
