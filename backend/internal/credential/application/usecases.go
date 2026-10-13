package application

import (
	"context"
	"errors"
	"fmt"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
)

const maxSecretBytes = 64 * 1024

type CreateNativeRequest struct {
	Namespace        string
	Name             string
	DisplayName      string
	Secret           []byte
	SSHKeyPassphrase []byte // import-time only; never persisted
}

type CreateNative struct {
	repository Repository
	secrets    SecretStore
	clock      Clock
}

func NewCreateNative(
	repository Repository,
	secrets SecretStore,
	clock Clock,
) CreateNative {
	return CreateNative{repository: repository, secrets: secrets, clock: clock}
}

func (uc CreateNative) Execute(
	ctx context.Context,
	request CreateNativeRequest,
) (credentialdomain.Credential, error) {
	ref, err := credentialdomain.NewReference(request.Namespace, request.Name)
	if err != nil {
		return credentialdomain.Credential{}, fmt.Errorf("create native credential: %w", err)
	}
	secret, err := validateSecret(request.Secret)
	if err != nil {
		return credentialdomain.Credential{}, err
	}
	defer func() { clear(secret) }()
	if request.Namespace == "ssh" {
		normalized, err := credentialdomain.NormalizeSSHPrivateKey(secret, request.SSHKeyPassphrase)
		clear(secret)
		if err != nil {
			return credentialdomain.Credential{}, err
		}
		secret = normalized
	} else if len(request.SSHKeyPassphrase) != 0 {
		return credentialdomain.Credential{}, ErrInvalidSecret
	}
	credential, err := credentialdomain.NewCredential(
		ref,
		request.DisplayName,
		credentialdomain.SourceNative,
		"",
		uc.clock.Now(),
	)
	if err != nil {
		return credentialdomain.Credential{}, fmt.Errorf("create native credential: %w", err)
	}
	if err := ensureReferenceAvailable(ctx, uc.repository, ref); err != nil {
		return credentialdomain.Credential{}, err
	}
	if err := uc.secrets.Put(ctx, ref, secret); err != nil {
		return credentialdomain.Credential{}, fmt.Errorf("create native credential: store secret: %w", err)
	}
	if err := uc.repository.Create(ctx, credential); err != nil {
		compensateErr := uc.secrets.Delete(context.WithoutCancel(ctx), ref)
		if compensateErr != nil && !errors.Is(compensateErr, ErrSecretNotFound) {
			return credentialdomain.Credential{}, errors.Join(
				fmt.Errorf("create native credential: persist metadata: %w", err),
				fmt.Errorf("create native credential: compensate secret: %w", compensateErr),
			)
		}
		return credentialdomain.Credential{}, fmt.Errorf(
			"create native credential: persist metadata: %w",
			err,
		)
	}
	return credential, nil
}

type CreateEnvironmentRequest struct {
	Namespace           string
	Name                string
	DisplayName         string
	EnvironmentVariable string
}

type CreateEnvironment struct {
	repository Repository
	clock      Clock
}

func NewCreateEnvironment(
	repository Repository,
	clock Clock,
) CreateEnvironment {
	return CreateEnvironment{repository: repository, clock: clock}
}

func (uc CreateEnvironment) Execute(
	ctx context.Context,
	request CreateEnvironmentRequest,
) (credentialdomain.Credential, error) {
	if request.Namespace == "ssh" {
		return credentialdomain.Credential{}, fmt.Errorf("SSH private key credentials must use the native OS credential store")
	}
	ref, err := credentialdomain.NewReference(request.Namespace, request.Name)
	if err != nil {
		return credentialdomain.Credential{}, fmt.Errorf("create environment credential: %w", err)
	}
	credential, err := credentialdomain.NewCredential(
		ref,
		request.DisplayName,
		credentialdomain.SourceEnvironment,
		request.EnvironmentVariable,
		uc.clock.Now(),
	)
	if err != nil {
		return credentialdomain.Credential{}, fmt.Errorf("create environment credential: %w", err)
	}
	if err := ensureReferenceAvailable(ctx, uc.repository, ref); err != nil {
		return credentialdomain.Credential{}, err
	}
	if err := uc.repository.Create(ctx, credential); err != nil {
		return credentialdomain.Credential{}, fmt.Errorf(
			"create environment credential: persist metadata: %w",
			err,
		)
	}
	return credential, nil
}

type GetCredential struct {
	repository Repository
}

func NewGetCredential(repository Repository) GetCredential {
	return GetCredential{repository: repository}
}

func (uc GetCredential) Execute(
	ctx context.Context,
	ref credentialdomain.Reference,
) (credentialdomain.Credential, error) {
	credential, err := uc.repository.Get(ctx, ref)
	if err != nil {
		return credentialdomain.Credential{}, fmt.Errorf("get credential: %w", err)
	}
	return credential, nil
}

type ListCredentials struct {
	repository Repository
}

func NewListCredentials(repository Repository) ListCredentials {
	return ListCredentials{repository: repository}
}

func (uc ListCredentials) Execute(
	ctx context.Context,
) ([]credentialdomain.Credential, error) {
	items, err := uc.repository.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list credentials: %w", err)
	}
	return items, nil
}

type RotateNativeSecret struct {
	repository Repository
	secrets    SecretStore
	clock      Clock
}

func NewRotateNativeSecret(
	repository Repository,
	secrets SecretStore,
	clock Clock,
) RotateNativeSecret {
	return RotateNativeSecret{
		repository: repository,
		secrets:    secrets,
		clock:      clock,
	}
}

func (uc RotateNativeSecret) Execute(
	ctx context.Context,
	ref credentialdomain.Reference,
	rawSecret []byte,
	sshKeyPassphrase []byte,
) (credentialdomain.Credential, error) {
	credential, err := uc.repository.Get(ctx, ref)
	if err != nil {
		return credentialdomain.Credential{}, fmt.Errorf("rotate credential secret: %w", err)
	}
	if credential.Source() != credentialdomain.SourceNative {
		return credentialdomain.Credential{}, ErrInvalidSource
	}
	secret, err := validateSecret(rawSecret)
	if err != nil {
		return credentialdomain.Credential{}, err
	}
	defer func() { clear(secret) }()
	if ref.Namespace() == "ssh" {
		normalized, err := credentialdomain.NormalizeSSHPrivateKey(secret, sshKeyPassphrase)
		clear(secret)
		if err != nil {
			return credentialdomain.Credential{}, err
		}
		secret = normalized
	} else if len(sshKeyPassphrase) != 0 {
		return credentialdomain.Credential{}, ErrInvalidSecret
	}
	if err := uc.secrets.Put(ctx, ref, secret); err != nil {
		return credentialdomain.Credential{}, fmt.Errorf("rotate credential secret: %w", err)
	}
	updated, err := credential.Touch(uc.clock.Now())
	if err != nil {
		return credentialdomain.Credential{}, fmt.Errorf("rotate credential secret: %w", err)
	}
	if err := uc.repository.Save(ctx, updated); err != nil {
		return credentialdomain.Credential{}, fmt.Errorf(
			"rotate credential secret: update metadata: %w",
			err,
		)
	}
	return updated, nil
}

type DeleteCredential struct {
	repository Repository
	secrets    SecretStore
	usage      UsageChecker
}

func NewDeleteCredential(
	repository Repository,
	secrets SecretStore,
	usage UsageChecker,
) DeleteCredential {
	return DeleteCredential{repository: repository, secrets: secrets, usage: usage}
}

func (uc DeleteCredential) Execute(
	ctx context.Context,
	ref credentialdomain.Reference,
) error {
	credential, err := uc.repository.Get(ctx, ref)
	if err != nil {
		return fmt.Errorf("delete credential: %w", err)
	}
	if uc.usage != nil {
		usages, err := uc.usage.Usages(ctx, ref)
		if err != nil {
			return fmt.Errorf("delete credential: check usage: %w", err)
		}
		if len(usages) != 0 {
			return fmt.Errorf("%w: %s", ErrCredentialInUse, usages[0].Description)
		}
	}
	if credential.Source() == credentialdomain.SourceNative {
		if err := uc.secrets.Delete(ctx, ref); err != nil &&
			!errors.Is(err, ErrSecretNotFound) {
			return fmt.Errorf("delete credential: delete secret: %w", err)
		}
	}
	if err := uc.repository.Delete(ctx, ref); err != nil {
		return fmt.Errorf("delete credential: delete metadata: %w", err)
	}
	return nil
}

func validateSecret(raw []byte) ([]byte, error) {
	if len(raw) == 0 || len(raw) > maxSecretBytes {
		return nil, ErrInvalidSecret
	}
	return append([]byte(nil), raw...), nil
}

func ensureReferenceAvailable(
	ctx context.Context,
	repository Repository,
	ref credentialdomain.Reference,
) error {
	_, err := repository.Get(ctx, ref)
	switch {
	case err == nil:
		return ErrReferenceConflict
	case errors.Is(err, ErrCredentialNotFound):
		return nil
	default:
		return fmt.Errorf("check credential reference: %w", err)
	}
}
