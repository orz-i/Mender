package resolver

import (
	"context"
	"errors"
	"testing"
	"time"

	credentialapplication "github.com/orz-i/Mender/backend/internal/credential/application"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
)

func TestOAuthCredentialCanProveAuthorizationWithoutResolvingBearer(t *testing.T) {
	ref, _ := credentialdomain.NewReference("oauth", "work")
	//nolint:gosec // A public client ID in the synthetic OAuth metadata is nonsecret.
	metadata := credentialdomain.OAuthMetadata{
		Resource:      "https://mcp.example.test/mcp",
		Issuer:        "https://id.example.test",
		ClientID:      "native-app",
		TokenEndpoint: "https://id.example.test/token",
		Status:        credentialdomain.OAuthConnected,
	}
	item, err := credentialdomain.NewCredential(ref, "OAuth work", credentialdomain.SourceOAuth, "", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	item, err = item.WithOAuthMetadata(metadata)
	if err != nil {
		t.Fatal(err)
	}
	secretStore := fakeSecretStore{ref: []byte("opaque-access-refresh-token-bundle")}
	resolver := New(fakeRepository{ref: item}, secretStore, nil)
	if secret, err := resolver.Resolve(context.Background(), ref); !errors.Is(err, credentialapplication.ErrSecretSourceUnavailable) || secret != nil {
		t.Fatalf("OAuth token exposed by generic resolver: %v, length %d", err, len(secret))
	}
	if err := resolver.AuthorizeCredential(context.Background(), ref); err != nil {
		t.Fatalf("OAuth proof unexpectedly failed: %v", err)
	}
	disconnected := metadata
	disconnected.Status = credentialdomain.OAuthAuthorizationRequired
	disconnectedItem, err := item.WithOAuthMetadata(disconnected)
	if err != nil {
		t.Fatal(err)
	}
	blocked := New(fakeRepository{ref: disconnectedItem}, secretStore, nil)
	if err := blocked.AuthorizeCredential(context.Background(), ref); !errors.Is(err, credentialapplication.ErrSecretSourceUnavailable) {
		t.Fatalf("disconnected OAuth Credential proved authorization: %v", err)
	}
	missingToken := New(fakeRepository{ref: item}, fakeSecretStore{}, nil)
	if err := missingToken.AuthorizeCredential(context.Background(), ref); !errors.Is(err, credentialapplication.ErrSecretNotFound) {
		t.Fatalf("missing OAuth keychain token proved authorization: %v", err)
	}
}
