package application

import (
	"context"
	"errors"
	"fmt"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
)

// SaveOAuthRequest contains an opaque token bundle intended for the OS secret
// store. Its bytes are never projected into the Credential inventory.
type SaveOAuthRequest struct {
	Namespace   string
	Name        string
	DisplayName string
	Metadata    credentialdomain.OAuthMetadata
	Secret      []byte
}

// SaveOAuth creates or renews the same OAuth Credential Reference.
// A reference cannot be repurposed for another resource or source.
type SaveOAuth struct {
	repository Repository
	secrets    SecretStore
	clock      Clock
}

func NewSaveOAuth(repository Repository, secrets SecretStore, clock Clock) SaveOAuth {
	return SaveOAuth{repository: repository, secrets: secrets, clock: clock}
}

func (uc SaveOAuth) Execute(ctx context.Context, request SaveOAuthRequest) (credentialdomain.Credential, error) {
	ref, err := credentialdomain.NewReference(request.Namespace, request.Name)
	if err != nil {
		return credentialdomain.Credential{}, fmt.Errorf("save OAuth credential: %w", err)
	}
	if ref.Namespace() == "ssh" {
		return credentialdomain.Credential{}, fmt.Errorf("OAuth credentials cannot use SSH namespace")
	}
	secret, err := validateSecret(request.Secret)
	if err != nil {
		return credentialdomain.Credential{}, err
	}
	defer clear(secret)
	if request.Metadata.Status != credentialdomain.OAuthConnected {
		return credentialdomain.Credential{}, fmt.Errorf("save OAuth credential: connected token is required")
	}

	existing, err := uc.repository.Get(ctx, ref)
	creating := errors.Is(err, ErrCredentialNotFound)
	if err != nil && !creating {
		return credentialdomain.Credential{}, err
	}
	var item credentialdomain.Credential
	if creating {
		item, err = credentialdomain.NewCredential(ref, request.DisplayName, credentialdomain.SourceOAuth, "", uc.clock.Now())
		if err != nil {
			return credentialdomain.Credential{}, err
		}
	} else {
		old := existing.OAuthMetadata()
		if existing.Source() != credentialdomain.SourceOAuth || old == nil || old.Resource != request.Metadata.Resource ||
			(old.MCPEndpoint != "" && old.MCPEndpoint != request.Metadata.MCPEndpoint) {
			return credentialdomain.Credential{}, ErrReferenceConflict
		}
		item, err = existing.Touch(uc.clock.Now())
		if err != nil {
			return credentialdomain.Credential{}, err
		}
	}
	item, err = item.WithOAuthMetadata(request.Metadata)
	if err != nil {
		return credentialdomain.Credential{}, err
	}
	if err := uc.secrets.Put(ctx, ref, secret); err != nil {
		return credentialdomain.Credential{}, fmt.Errorf("save OAuth credential: store token: %w", err)
	}
	if creating {
		if err := uc.repository.Create(ctx, item); err != nil {
			if deleteErr := uc.secrets.Delete(context.WithoutCancel(ctx), ref); deleteErr != nil && !errors.Is(deleteErr, ErrSecretNotFound) {
				return credentialdomain.Credential{}, errors.Join(err, deleteErr)
			}
			return credentialdomain.Credential{}, fmt.Errorf("save OAuth credential: persist metadata: %w", err)
		}
	} else {
		if err := uc.repository.Save(ctx, item); err != nil {
			return credentialdomain.Credential{}, fmt.Errorf("save OAuth credential: update metadata: %w", err)
		}
	}
	return item, nil
}

// DisconnectOAuth removes the stored token but retains the reference so
// existing Provider and Profile bindings remain auditable and fail closed.
type DisconnectOAuth struct {
	repository Repository
	secrets    SecretStore
	clock      Clock
}

func NewDisconnectOAuth(repository Repository, secrets SecretStore, clock Clock) DisconnectOAuth {
	return DisconnectOAuth{repository: repository, secrets: secrets, clock: clock}
}

func (uc DisconnectOAuth) Execute(ctx context.Context, ref credentialdomain.Reference) (credentialdomain.Credential, error) {
	item, err := uc.repository.Get(ctx, ref)
	if err != nil {
		return credentialdomain.Credential{}, err
	}
	if item.Source() != credentialdomain.SourceOAuth {
		return credentialdomain.Credential{}, ErrInvalidSource
	}
	if err := uc.secrets.Delete(ctx, ref); err != nil && !errors.Is(err, ErrSecretNotFound) {
		return credentialdomain.Credential{}, fmt.Errorf("disconnect OAuth credential: %w", err)
	}
	meta := *item.OAuthMetadata()
	meta.Status = credentialdomain.OAuthAuthorizationRequired
	meta.ExpiresAt = meta.ExpiresAt.UTC()
	item, err = item.Touch(uc.clock.Now())
	if err != nil {
		return credentialdomain.Credential{}, err
	}
	item, err = item.WithOAuthMetadata(meta)
	if err != nil {
		return credentialdomain.Credential{}, err
	}
	if err := uc.repository.Save(ctx, item); err != nil {
		return credentialdomain.Credential{}, err
	}
	return item, nil
}
