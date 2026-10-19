package domain

import (
	"testing"
	"time"
)

func TestOAuthCredentialMetadataValidationAndIsolation(t *testing.T) {
	ref, err := NewReference("oauth", "work")
	if err != nil {
		t.Fatal(err)
	}
	cred, err := NewCredential(ref, "Work Account", SourceOAuth, "", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	meta := OAuthMetadata{
		Resource:      "https://mcp.example.test/mcp",
		Issuer:        "https://accounts.example.test",
		ClientID:      "desktop-client",
		TokenEndpoint: "https://accounts.example.test/token",
		Scopes:        []string{"search:read"}, Status: OAuthConnected,
	}
	cred, err = cred.WithOAuthMetadata(meta)
	if err != nil {
		t.Fatal(err)
	}
	meta.Scopes[0] = "mutated"
	returned := cred.OAuthMetadata()
	if returned.Scopes[0] != "search:read" {
		t.Fatal("OAuth metadata aliased input")
	}
	returned.Scopes[0] = "mutated again"
	if cred.OAuthMetadata().Scopes[0] != "search:read" {
		t.Fatal("OAuth metadata exposed mutable state")
	}
	updated, err := cred.Touch(time.Now().UTC().Add(time.Minute))
	if err != nil || updated.OAuthMetadata() == nil || updated.OAuthMetadata().Resource != "https://mcp.example.test/mcp" {
		t.Fatalf("Touch() lost OAuth metadata: %v %v", updated.OAuthMetadata(), err)
	}
	if _, err := NewCredential(ref, "Wrong", SourceOAuth, "TOKEN", time.Now().UTC()); err == nil {
		t.Fatal("OAuth Credential allowed environment variable")
	}
	for _, raw := range []OAuthMetadata{
		{Resource: "http://evil.test/mcp", Issuer: meta.Issuer, TokenEndpoint: meta.TokenEndpoint, ClientID: meta.ClientID, Status: OAuthConnected},
		{Resource: meta.Resource, Issuer: "http://evil.test", TokenEndpoint: meta.TokenEndpoint, ClientID: meta.ClientID, Status: OAuthConnected},
		{Resource: meta.Resource, Issuer: meta.Issuer, TokenEndpoint: "http://evil.test/token", ClientID: meta.ClientID, Status: OAuthConnected},
		{Resource: meta.Resource, Issuer: meta.Issuer, TokenEndpoint: meta.TokenEndpoint, ClientID: meta.ClientID, Status: "unknown"},
	} {
		if _, err := cred.WithOAuthMetadata(raw); err == nil {
			t.Fatalf("accepted insecure OAuth metadata %+v", raw)
		}
	}
}
