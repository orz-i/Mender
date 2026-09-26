// Package unavailable provides the fail-closed credential resolver used before
// a concrete secret store is configured.
package unavailable

import (
	"context"
	"errors"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
)

var ErrUnavailable = errors.New("credential resolver unavailable")

// Resolver never returns secret material.
type Resolver struct{}

func (Resolver) Resolve(
	context.Context,
	credentialdomain.Reference,
) ([]byte, error) {
	return nil, ErrUnavailable
}
