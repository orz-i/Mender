package application

import "errors"

var (
	ErrCredentialNotFound      = errors.New("credential: not found")
	ErrReferenceConflict       = errors.New("credential: reference already exists")
	ErrCredentialInUse         = errors.New("credential: in use")
	ErrInvalidSecret           = errors.New("credential: invalid secret")
	ErrInvalidSource           = errors.New("credential: invalid source for operation")
	ErrSecretNotFound          = errors.New("credential secret: not found")
	ErrSecretSourceUnavailable = errors.New("credential: secret source unavailable")
	ErrEnvironmentMissing      = errors.New("credential: environment variable unavailable")
)
