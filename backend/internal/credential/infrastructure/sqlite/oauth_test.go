package sqlite

import (
	"context"
	"testing"
	"time"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
)

func TestOAuthCredentialMetadataRoundTripContainsNoSecrets(t *testing.T) {
	store := openTestStore(t)
	now := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	ref := mustReference(t, "oauth", "work")
	credential, err := credentialdomain.NewCredential(ref, "OAuth account", credentialdomain.SourceOAuth, "", now)
	if err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // Public OAuth client ID is a nonsecret metadata fixture.
	metadata := credentialdomain.OAuthMetadata{Resource: "https://mcp.example.test/mcp", Issuer: "https://id.example.test", ClientID: "public-desktop", TokenEndpoint: "https://id.example.test/token", Scopes: []string{"read"}, Status: credentialdomain.OAuthConnected}
	credential, err = credential.WithOAuthMetadata(metadata)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.Create(ctx, credential); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Get(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Source() != credentialdomain.SourceOAuth || loaded.OAuthMetadata() == nil || loaded.OAuthMetadata().ClientID != "public-desktop" {
		t.Fatalf("OAuth credential roundtrip failed: %+v", loaded.OAuthMetadata())
	}
	_, err = store.db.ExecContext(ctx, "UPDATE credentials SET oauth_metadata='not json' WHERE reference=?", ref.String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, ref); err == nil {
		t.Fatal("malformed OAuth state accepted")
	}
}
