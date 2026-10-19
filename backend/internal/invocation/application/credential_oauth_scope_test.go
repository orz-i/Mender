package application

import (
	"context"
	"errors"
	"testing"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
)

type oauthAuthorizingResolver struct{ authCalls, resolveCalls int }

func (r *oauthAuthorizingResolver) Resolve(context.Context, credentialdomain.Reference) ([]byte, error) {
	r.resolveCalls++
	return nil, errors.New("OAuth tokens are not generic Credential bytes")
}
func (r *oauthAuthorizingResolver) AuthorizeCredential(context.Context, credentialdomain.Reference) error {
	r.authCalls++
	return nil
}

func TestOAuthAuthorizerRespectsCredentialAllowlistWithoutResolvingTokens(t *testing.T) {
	allowed, _ := credentialdomain.NewReference("oauth", "one")
	denied, _ := credentialdomain.NewReference("oauth", "two")
	delegate := &oauthAuthorizingResolver{}
	wrapper := newAllowlistCredentialResolver(delegate, []credentialdomain.Reference{allowed})
	scoped, ok := wrapper.(CredentialAuthorizer)
	if !ok {
		t.Fatal("scoped resolver lost OAuth authorization capability")
	}
	if err := scoped.AuthorizeCredential(context.Background(), denied); !errors.Is(err, ErrCredentialOutsideScope) {
		t.Fatalf("unapproved OAuth Credential authorized: %v", err)
	}
	if delegate.authCalls != 0 {
		t.Fatal("denied ref reached global Credential resolver")
	}
	if err := scoped.AuthorizeCredential(context.Background(), allowed); err != nil {
		t.Fatalf("approved OAuth ref failed: %v", err)
	}
	if delegate.authCalls != 1 || delegate.resolveCalls != 0 {
		t.Fatalf("OAuth access invoked Resolve or bypassed proof: %#v", delegate)
	}
}

type staticOnlyCredentialResolver struct{}

func (staticOnlyCredentialResolver) Resolve(context.Context, credentialdomain.Reference) ([]byte, error) {
	return []byte("secret"), nil
}

func TestOAuthAuthorizerFailsClosedWhenDelegateLacksCapability(t *testing.T) {
	ref, _ := credentialdomain.NewReference("oauth", "one")
	wrapper := newAllowlistCredentialResolver(staticOnlyCredentialResolver{}, []credentialdomain.Reference{ref})
	authorizer := wrapper.(CredentialAuthorizer)
	if err := authorizer.AuthorizeCredential(context.Background(), ref); err == nil {
		t.Fatal("OAuth authorization accepted a resolver without proof capability")
	}
}
