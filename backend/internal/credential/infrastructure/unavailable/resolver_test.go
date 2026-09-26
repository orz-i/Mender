package unavailable

import (
	"context"
	"errors"
	"testing"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
)

func TestResolverFailsClosed(t *testing.T) {
	t.Parallel()

	ref, _ := credentialdomain.ParseReference("credential://github/work")
	secret, err := (Resolver{}).Resolve(context.Background(), ref)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Resolve() error = %v", err)
	}
	if secret != nil {
		t.Fatalf("Resolve() secret = %q, want nil", secret)
	}
}
