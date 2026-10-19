package application

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
)

func testOAuthMetadata() credentialdomain.OAuthMetadata {
	return credentialdomain.OAuthMetadata{
		Resource:      "https://mcp.example.test/mcp",
		Issuer:        "https://id.example.test",
		ClientID:      "desktop",
		TokenEndpoint: "https://id.example.test/token",
		Scopes:        []string{"read"},
		Status:        credentialdomain.OAuthConnected,
	}
}

func TestOAuthCredentialSaveRenewDisconnectAndDelete(t *testing.T) {
	ctx := context.Background()
	repository := newMemoryRepository()
	secrets := newMemorySecrets()
	clock := fixedClock{now: time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)}
	save := NewSaveOAuth(repository, secrets, clock)
	request := SaveOAuthRequest{Namespace: "oauth", Name: "work", DisplayName: "Work", Metadata: testOAuthMetadata(), Secret: []byte("fake-oauth-token-bundle")}
	first, err := save.Execute(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if first.Source() != credentialdomain.SourceOAuth || first.OAuthMetadata().Status != credentialdomain.OAuthConnected {
		t.Fatal("OAuth metadata not created")
	}
	if !bytes.Equal(secrets.values[first.Reference()], request.Secret) {
		t.Fatal("OAuth token not stored in SecretStore")
	}
	if _, err := NewRotateNativeSecret(repository, secrets, clock).Execute(ctx, first.Reference(), []byte("static-token"), nil); !errors.Is(err, ErrInvalidSource) {
		t.Fatalf("native rotate accepted OAuth: %v", err)
	}
	request.Secret = []byte("refreshed-oauth-token")
	_, err = save.Execute(ctx, request)
	if err != nil || !bytes.Equal(secrets.values[first.Reference()], request.Secret) {
		t.Fatalf("OAuth reauthorization: %v", err)
	}
	conflicting := request
	conflicting.Metadata.Resource = "https://different.test/mcp"
	if _, err := save.Execute(ctx, conflicting); !errors.Is(err, ErrReferenceConflict) {
		t.Fatalf("resource switch allowed: %v", err)
	}
	if !bytes.Equal(secrets.values[first.Reference()], request.Secret) {
		t.Fatal("resource collision overwrote token")
	}
	disconnected, err := NewDisconnectOAuth(repository, secrets, clock).Execute(ctx, first.Reference())
	if err != nil {
		t.Fatal(err)
	}
	if disconnected.OAuthMetadata().Status != credentialdomain.OAuthAuthorizationRequired {
		t.Fatal("disconnected account still connected")
	}
	if _, ok := secrets.values[first.Reference()]; ok {
		t.Fatal("disconnect failed to clear token")
	}
	if err := NewDeleteCredential(repository, secrets, nil).Execute(ctx, first.Reference()); err != nil {
		t.Fatal(err)
	}
}

func TestOAuthCredentialRejectsNativeCollisionBeforeTokenWrite(t *testing.T) {
	ctx := context.Background()
	repository := newMemoryRepository()
	secrets := newMemorySecrets()
	clock := fixedClock{now: time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)}
	ref := mustRef(t, "oauth", "work")
	original, _ := credentialdomain.NewCredential(ref, "Non-OAuth", credentialdomain.SourceNative, "", clock.Now())
	if err := repository.Create(ctx, original); err != nil {
		t.Fatal(err)
	}
	request := SaveOAuthRequest{Namespace: "oauth", Name: "work", DisplayName: "Work", Metadata: testOAuthMetadata(), Secret: []byte("fake-token")}
	if _, err := NewSaveOAuth(repository, secrets, clock).Execute(ctx, request); !errors.Is(err, ErrReferenceConflict) {
		t.Fatalf("OAuth overwritten non-OAuth reference: %v", err)
	}
	if secrets.putCalls != 0 {
		t.Fatal("collision wrote secret to keychain")
	}
}
