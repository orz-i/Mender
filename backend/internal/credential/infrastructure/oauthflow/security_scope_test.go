package oauthflow

import (
	"reflect"
	"testing"
)

// A server can advertise privileged scopes alongside read-only ones.
// With no explicit request, Mender must not ask for every advertised grant.
func TestOAuthScopesAreNeverBroadenedByDiscovery(t *testing.T) {
	t.Parallel()
	advertised := []string{"search:read", "search:write", "admin"}
	if actual := scopeFilterFor(nil)(advertised); len(actual) != 0 {
		t.Fatalf("implicit OAuth request asked for advertised privileges: %v", actual)
	}
	if actual := scopeFilterFor([]string{})(advertised); len(actual) != 0 {
		t.Fatalf("empty OAuth request asked for advertised privileges: %v", actual)
	}
	actual := scopeFilterFor([]string{"search:read"})(advertised)
	if !reflect.DeepEqual(actual, []string{"search:read"}) {
		t.Fatalf("requested scope was not confined: %v", actual)
	}
	if actual := scopeFilterFor([]string{"unknown"})(advertised); len(actual) != 0 {
		t.Fatalf("OAuth discovery granted an unrelated scope: %v", actual)
	}
	actual = scopeFilterFor([]string{"search:read"})(nil)
	if !reflect.DeepEqual(actual, []string{"search:read"}) {
		t.Fatalf("explicit scope lost when metadata omitted scopes: %v", actual)
	}
}
