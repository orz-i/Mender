package nativekeyring

import (
	"context"
	"errors"
	"fmt"

	credentialapplication "github.com/orz-i/Mender/backend/internal/credential/application"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	"github.com/zalando/go-keyring"
)

const serviceName = "Mender"

// Store persists native secret values in the OS credential store.
type Store struct{}

func (Store) Put(
	_ context.Context,
	ref credentialdomain.Reference,
	secret []byte,
) error {
	if err := keyring.Set(serviceName, ref.String(), string(secret)); err != nil {
		return fmt.Errorf(
			"%w: native keyring write failed",
			credentialapplication.ErrSecretSourceUnavailable,
		)
	}
	return nil
}

func (Store) Get(
	_ context.Context,
	ref credentialdomain.Reference,
) ([]byte, error) {
	value, err := keyring.Get(serviceName, ref.String())
	if errors.Is(err, keyring.ErrNotFound) {
		return nil, credentialapplication.ErrSecretNotFound
	}
	if err != nil {
		return nil, fmt.Errorf(
			"%w: native keyring read failed",
			credentialapplication.ErrSecretSourceUnavailable,
		)
	}
	return []byte(value), nil
}

func (Store) Delete(
	_ context.Context,
	ref credentialdomain.Reference,
) error {
	err := keyring.Delete(serviceName, ref.String())
	if errors.Is(err, keyring.ErrNotFound) {
		return credentialapplication.ErrSecretNotFound
	}
	if err != nil {
		return fmt.Errorf(
			"%w: native keyring delete failed",
			credentialapplication.ErrSecretSourceUnavailable,
		)
	}
	return nil
}

var _ credentialapplication.SecretStore = Store{}
