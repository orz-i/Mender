package application

import (
	"context"
	"time"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
)

type Repository interface {
	Create(context.Context, credentialdomain.Credential) error
	Save(context.Context, credentialdomain.Credential) error
	Get(context.Context, credentialdomain.Reference) (credentialdomain.Credential, error)
	List(context.Context) ([]credentialdomain.Credential, error)
	Delete(context.Context, credentialdomain.Reference) error
}

type SecretStore interface {
	Put(context.Context, credentialdomain.Reference, []byte) error
	Get(context.Context, credentialdomain.Reference) ([]byte, error)
	Delete(context.Context, credentialdomain.Reference) error
}

type Usage struct {
	Kind        string
	Description string
}

type UsageChecker interface {
	Usages(context.Context, credentialdomain.Reference) ([]Usage, error)
}

type Clock interface {
	Now() time.Time
}
