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
	mcpconfigstore "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/configstore"
)

// Credentials composes non-secret inventory use cases and the runtime secret resolver.
type Credentials struct {
	CreateNative      credentialapplication.CreateNative
	CreateEnvironment credentialapplication.CreateEnvironment
	Get               credentialapplication.GetCredential
	List              credentialapplication.ListCredentials
	RotateNative      credentialapplication.RotateNativeSecret
	Delete            credentialapplication.DeleteCredential
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

type runtimeCredentialUsageChecker struct {
	profiles *Profiles
	configs  interface {
		List(context.Context) ([]mcpconfigstore.Config, error)
	}
}

func (a runtimeCredentialUsageChecker) Usages(
	ctx context.Context,
	ref credentialdomain.Reference,
) ([]credentialapplication.Usage, error) {
	usages := make([]credentialapplication.Usage, 0)
	profiles, err := a.profiles.List.Execute(ctx)
	if err != nil {
		return nil, fmt.Errorf("credential usage: list profiles: %w", err)
	}
	for _, profile := range profiles {
		for _, binding := range profile.Bindings() {
			for _, bindingRef := range binding.CredentialRefs() {
				if bindingRef == ref {
					usages = append(usages, credentialapplication.Usage{
						Kind: "profile",
						Description: fmt.Sprintf(
							"profile %s tool %s",
							profile.Slug(),
							binding.ToolID(),
						),
					})
				}
			}
		}
	}
	configs, err := a.configs.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("credential usage: list provider configs: %w", err)
	}
	for _, config := range configs {
		for _, configRef := range config.RequiredCredentialRefs() {
			if configRef == ref {
				usages = append(usages, credentialapplication.Usage{
					Kind:        "provider",
					Description: fmt.Sprintf("provider %s", config.ProviderID),
				})
			}
		}
	}
	return usages, nil
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
