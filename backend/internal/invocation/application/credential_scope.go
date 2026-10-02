package application

import (
	"context"
	"fmt"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
)

type allowlistCredentialResolver struct {
	delegate CredentialResolver
	allowed  map[credentialdomain.Reference]struct{}
}

func newAllowlistCredentialResolver(
	delegate CredentialResolver,
	refs []credentialdomain.Reference,
) CredentialResolver {
	allowed := make(map[credentialdomain.Reference]struct{}, len(refs))
	for _, ref := range refs {
		allowed[ref] = struct{}{}
	}
	return allowlistCredentialResolver{
		delegate: delegate,
		allowed:  allowed,
	}
}

func (r allowlistCredentialResolver) Resolve(
	ctx context.Context,
	ref credentialdomain.Reference,
) ([]byte, error) {
	if _, ok := r.allowed[ref]; !ok {
		return nil, ErrCredentialOutsideScope
	}
	if r.delegate == nil {
		return nil, fmt.Errorf("credential resolver is unavailable")
	}
	return r.delegate.Resolve(ctx, ref)
}

func credentialResolverForScope(
	delegate CredentialResolver,
	scope governancedomain.Scope,
	governance invocationdomain.ScopedBindingGovernance,
) CredentialResolver {
	if delegate == nil {
		return nil
	}
	if scope.Kind() == governancedomain.ScopeProfile {
		return newAllowlistCredentialResolver(
			delegate,
			governance.CredentialRefs(),
		)
	}
	return delegate
}
