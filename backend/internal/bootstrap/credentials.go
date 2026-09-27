package bootstrap

import (
	"context"
	"fmt"

	credentialapplication "github.com/orz-i/Mender/backend/internal/credential/application"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	credentialkeyring "github.com/orz-i/Mender/backend/internal/credential/infrastructure/nativekeyring"
	credentialresolver "github.com/orz-i/Mender/backend/internal/credential/infrastructure/resolver"
	credentialsqlite "github.com/orz-i/Mender/backend/internal/credential/infrastructure/sqlite"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
)

// Credentials composes non-secret inventory use cases and the runtime secret resolver.
type Credentials struct {
	CreateNative      credentialapplication.CreateNative
	CreateEnvironment credentialapplication.CreateEnvironment
	Get               credentialapplication.GetCredential
	List              credentialapplication.ListCredentials
	RotateNative      credentialapplication.RotateNativeSecret
	Resolver          invocationapplication.CredentialResolver

	store   *credentialsqlite.Store
	secrets credentialapplication.SecretStore
}

func NewCredentials(
	ctx context.Context,
	databasePath string,
) (*Credentials, error) {
	store, err := credentialsqlite.Open(ctx, databasePath)
	if err != nil {
		return nil, fmt.Errorf("compose credentials: %w", err)
	}
	secrets := credentialkeyring.Store{}
	clock := systemClock{}
	resolver := credentialresolver.New(store, secrets, nil)

	return &Credentials{
		CreateNative:      credentialapplication.NewCreateNative(store, secrets, clock),
		CreateEnvironment: credentialapplication.NewCreateEnvironment(store, clock),
		Get:               credentialapplication.NewGetCredential(store),
		List:              credentialapplication.NewListCredentials(store),
		RotateNative:      credentialapplication.NewRotateNativeSecret(store, secrets, clock),
		Resolver:          resolver,
		store:             store,
		secrets:           secrets,
	}, nil
}

func (c *Credentials) Close() error {
	if c == nil || c.store == nil {
		return nil
	}
	return c.store.Close()
}

type profileCredentialLookup struct {
	get credentialapplication.GetCredential
}

func (a profileCredentialLookup) GetCredential(
	ctx context.Context,
	ref credentialdomain.Reference,
) (credentialdomain.Credential, error) {
	return a.get.Execute(ctx, ref)
}
