package oauthflow

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	credentialapplication "github.com/orz-i/Mender/backend/internal/credential/application"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
)

func seededTokenManager(t *testing.T, endpoint string, expiry time.Time, refresh string) (*Manager, *memoryRepository, *memorySecrets, credentialdomain.Reference) {
	t.Helper()
	repo := newMemoryRepository()
	secrets := newMemorySecrets()
	ref, err := credentialdomain.NewReference("oauth", "refresh")
	if err != nil {
		t.Fatal(err)
	}
	credential, err := credentialdomain.NewCredential(ref, "Refresh fixture", credentialdomain.SourceOAuth, "", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	credential, err = credential.WithOAuthMetadata(credentialdomain.OAuthMetadata{
		Resource: "http://127.0.0.1:9282/mcp", Issuer: endpoint, ClientID: "public-client",
		TokenEndpoint: endpoint + "/token", Scopes: []string{"read"}, ExpiresAt: expiry, Status: credentialdomain.OAuthConnected,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(context.Background(), credential); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // Synthetic OAuth tokens are stored only in an in-memory test keyring.
	raw, err := json.Marshal(tokenBundle{
		TokenType: "Bearer", AccessToken: "expired-access-token", RefreshToken: refresh,
		ClientID: "public-client", Issuer: endpoint, Resource: "http://127.0.0.1:9282/mcp",
		TokenEndpoint: endpoint + "/token", ExpiresAt: expiry, Scopes: []string{"read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := secrets.Put(context.Background(), ref, raw); err != nil {
		t.Fatal(err)
	}
	mgr := NewManager(repo, secrets, nil, Options{})
	t.Cleanup(func() { _ = mgr.Close() })
	return mgr, repo, secrets, ref
}

func TestOAuthAccessTokenRefreshSerializesAndPersistsRotationAcrossRestart(t *testing.T) {
	var requests atomic.Int32
	as := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/token" || r.Method != http.MethodPost {
			http.Error(w, "unexpected request", 400)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", 400)
			return
		}
		if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "old-refresh-token" ||
			r.Form.Get("client_id") != "public-client" || r.Form.Get("resource") != "http://127.0.0.1:9282/mcp" {
			http.Error(w, "refresh form mismatch", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"new-access-token","refresh_token":"rotated-refresh-token","token_type":"Bearer","expires_in":3600,"scope":"read"}`))
	}))
	defer as.Close()
	mgr, repo, secrets, ref := seededTokenManager(t, as.URL, time.Now().Add(-time.Minute), "old-refresh-token")
	ctx := context.Background()
	const concurrency = 12
	var wg sync.WaitGroup
	wg.Add(concurrency)
	errs := make(chan error, concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			token, err := mgr.AccessToken(ctx, ref, "http://127.0.0.1:9282/mcp")
			if err != nil {
				errs <- err
				return
			}
			if token != "new-access-token" {
				errs <- errors.New("unexpected token")
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if calls := requests.Load(); calls != 1 {
		t.Fatalf("refresh calls=%d, want 1", calls)
	}
	stored, err := secrets.Get(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	var bundle tokenBundle
	if err := json.Unmarshal(stored, &bundle); err != nil {
		t.Fatal(err)
	}
	if bundle.RefreshToken != "rotated-refresh-token" || bundle.AccessToken != "new-access-token" {
		t.Fatal("refresh token rotation was not saved before reuse")
	}
	metadata, err := repo.Get(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.OAuthMetadata().ExpiresAt.Before(time.Now().Add(50 * time.Minute)) {
		t.Fatal("refresh expiry metadata was not persisted")
	}
	restarted := NewManager(repo, secrets, nil, Options{})
	defer func() { _ = restarted.Close() }()
	if token, err := restarted.AccessToken(ctx, ref, "http://127.0.0.1:9282/mcp"); err != nil || token != "new-access-token" {
		t.Fatalf("restart lost native token: %q %v", token, err)
	}
	if requests.Load() != 1 {
		t.Fatal("restart repeated an already completed refresh")
	}
}

func TestOAuthAccessTokenInvalidGrantRequiresNewAuthorization(t *testing.T) {
	as := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"sensitive upstream detail"}`))
	}))
	defer as.Close()
	mgr, repo, secrets, ref := seededTokenManager(t, as.URL, time.Now().Add(-time.Minute), "invalid-refresh")
	_, err := mgr.AccessToken(context.Background(), ref, "http://127.0.0.1:9282/mcp")
	if err == nil || strings.Contains(err.Error(), "sensitive upstream detail") {
		t.Fatalf("unexpected OAuth error: %v", err)
	}
	credential, err := repo.Get(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if credential.OAuthMetadata().Status != credentialdomain.OAuthAuthorizationRequired {
		t.Fatal("invalid_grant did not force reauthorization")
	}
	if _, err := secrets.Get(context.Background(), ref); !errors.Is(err, credentialapplication.ErrSecretNotFound) {
		t.Fatal("invalid_grant left a usable token in keychain")
	}
}

func TestOAuthAccessTokenRejectsWrongResourceAndLocalDisconnect(t *testing.T) {
	as := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "unexpected network request", 500) }))
	defer as.Close()
	mgr, repo, secrets, ref := seededTokenManager(t, as.URL, time.Now().Add(time.Hour), "old-refresh-token")
	if _, err := mgr.AccessToken(context.Background(), ref, "https://different.example/mcp"); err == nil {
		t.Fatal("OAuth token was accepted for a different resource")
	}
	metadata, err := repo.Get(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.OAuthMetadata().Status != credentialdomain.OAuthConnected {
		t.Fatal("resource mismatch changed credential")
	}
	if _, err := mgr.Disconnect(context.Background(), ref); err != nil {
		t.Fatal(err)
	}
	if _, err := secrets.Get(context.Background(), ref); !errors.Is(err, credentialapplication.ErrSecretNotFound) {
		t.Fatal("disconnect retained OAuth token")
	}
	if _, err := mgr.AccessToken(context.Background(), ref, "http://127.0.0.1:9282/mcp"); err == nil {
		t.Fatal("disconnected OAuth token was reused")
	}
}

func TestOAuthRefreshPreservesRegisteredClientSecretBasicAuthentication(t *testing.T) {
	var calls atomic.Int32
	as := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		id, secret, ok := r.BasicAuth()
		if !ok || id != "registered-client" || secret != "private-client-secret" {
			http.Error(w, "missing registered Basic credentials", http.StatusUnauthorized)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad body", 400)
			return
		}
		if r.PostForm.Get("client_secret") != "" || r.PostForm.Get("client_id") != "" ||
			r.PostForm.Get("resource") != "http://127.0.0.1:9282/mcp" {
			http.Error(w, "invalid Basic client token request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"basic-refreshed-access","refresh_token":"new-basic-refresh","token_type":"Bearer","expires_in":3600}`))
	}))
	defer as.Close()
	manager, repo, secrets, ref := seededTokenManager(t, as.URL, time.Now().Add(-time.Minute), "basic-refresh")
	ctx := context.Background()
	credential, _ := repo.Get(ctx, ref)
	meta := *credential.OAuthMetadata()
	meta.ClientID = "registered-client"
	credential, _ = credential.WithOAuthMetadata(meta)
	if err := repo.Save(ctx, credential); err != nil {
		t.Fatal(err)
	}
	raw, err := secrets.Get(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	var bundle tokenBundle
	if err := json.Unmarshal(raw, &bundle); err != nil {
		t.Fatal(err)
	}
	bundle.ClientID = "registered-client"
	bundle.ClientSecret = "private-client-secret"
	bundle.TokenAuthStyle = "basic"
	//nolint:gosec // Test-only token bundle is stored solely in an in-memory keyring.
	updated, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if err := secrets.Put(ctx, ref, updated); err != nil {
		t.Fatal(err)
	}
	value, err := manager.AccessToken(ctx, ref, "http://127.0.0.1:9282/mcp")
	if err != nil || value != "basic-refreshed-access" {
		t.Fatalf("Basic refresh=%q %v", value, err)
	}
	if calls.Load() != 1 {
		t.Fatal("unexpected Basic token endpoint calls")
	}
}
