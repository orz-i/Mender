package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	credentialapplication "github.com/orz-i/Mender/backend/internal/credential/application"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	credentialkeyring "github.com/orz-i/Mender/backend/internal/credential/infrastructure/nativekeyring"
	credentialresolver "github.com/orz-i/Mender/backend/internal/credential/infrastructure/resolver"
	credentialsqlite "github.com/orz-i/Mender/backend/internal/credential/infrastructure/sqlite"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
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
	profiles  *Profiles
	tunnels   *Tunnels
	sandboxes *Sandboxes
	configs   interface {
		List(context.Context) ([]providerplugin.Configuration, error)
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
	if a.tunnels != nil {
		tunnels, err := a.tunnels.List.Execute(ctx)
		if err != nil {
			return nil, fmt.Errorf("credential usage: list tunnels: %w", err)
		}
		for _, tunnel := range tunnels {
			if tunnel.RuntimeCredentialRef() == ref {
				usages = append(usages, credentialapplication.Usage{
					Kind:        "tunnel",
					Description: fmt.Sprintf("tunnel %s", tunnel.DisplayName()),
				})
			}
		}
	}
	configs, err := a.configs.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("credential usage: list provider configs: %w", err)
	}
	for _, config := range configs {
		metadata, ok := config.ConfigMetadata()
		if ok {
			if metadataReferencesCredential(metadata, ref) {
				usages = append(usages, credentialapplication.Usage{
					Kind:        "provider",
					Description: fmt.Sprintf("provider %s", config.ProviderID),
				})
			}
			continue
		}

		// Forward-migrated rows from before normalized metadata are inspected
		// locally and conservatively. Protocol v1 external plugins already require
		// declared Credential References to occur as exact config strings; treating
		// any exact match as usage can only over-block deletion, never release a
		// referenced secret because a plugin process is unavailable.
		contains, err := providerConfigContainsExactString(config.Config, ref.String())
		if err != nil {
			return nil, fmt.Errorf(
				"credential usage: inspect legacy provider config %s: %w",
				config.ProviderID,
				err,
			)
		}
		if contains {
			usages = append(usages, credentialapplication.Usage{
				Kind:        "provider",
				Description: fmt.Sprintf("provider %s", config.ProviderID),
			})
		}
	}
	if a.sandboxes != nil {
		sandboxIDs, err := a.sandboxes.DaytonaCredentialBindings(ctx, ref)
		if err != nil {
			return nil, fmt.Errorf("credential usage: Daytona inventory unavailable: %w", err)
		}
		for _, id := range sandboxIDs {
			usages = append(usages, credentialapplication.Usage{Kind: "sandbox", Description: fmt.Sprintf("Daytona Sandbox %s", id)})
		}
	}
	return usages, nil
}

func metadataReferencesCredential(
	metadata providerplugin.ConfigMetadata,
	ref credentialdomain.Reference,
) bool {
	for _, configRef := range metadata.CredentialRefs {
		if configRef == ref {
			return true
		}
	}
	return false
}

func providerConfigContainsExactString(raw []byte, target string) (bool, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return false, err
	}
	return jsonValueContainsExactString(value, target), nil
}

func jsonValueContainsExactString(value any, target string) bool {
	switch typed := value.(type) {
	case string:
		return typed == target
	case []any:
		for _, item := range typed {
			if jsonValueContainsExactString(item, target) {
				return true
			}
		}
	case map[string]any:
		for _, item := range typed {
			if jsonValueContainsExactString(item, target) {
				return true
			}
		}
	}
	return false
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
