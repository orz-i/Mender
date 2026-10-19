package oauthflow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	credentialapplication "github.com/orz-i/Mender/backend/internal/credential/application"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
)

// AccessToken provides an OAuth bearer only to the MCP HTTP transport. Unlike
// the generic CredentialResolver, it never makes token bytes available to
// Tool arguments, Policy, Profile grants, or management responses.
//
// Refresh and token rotation are serialized with local disconnection.
// Client-visible errors intentionally never include OAuth endpoint responses.
func (m *Manager) AccessToken(ctx context.Context, ref credentialdomain.Reference, resource string) (string, error) {
	if m == nil {
		return "", errors.New("OAuth token manager is unavailable")
	}
	m.tokenMu.Lock()
	defer m.tokenMu.Unlock()

	credential, err := m.repository.Get(ctx, ref)
	if err != nil {
		return "", fmt.Errorf("OAuth credential is unavailable: %w", err)
	}
	metadata := credential.OAuthMetadata()
	endpoint := ""
	if metadata != nil {
		endpoint = metadata.MCPEndpoint
		if endpoint == "" {
			endpoint = metadata.Resource
		}
	}
	if credential.Source() != credentialdomain.SourceOAuth || metadata == nil ||
		metadata.Status != credentialdomain.OAuthConnected || endpoint != resource {
		return "", errors.New("OAuth credential is not authorized for this MCP resource")
	}
	raw, err := m.secrets.Get(ctx, ref)
	if err != nil {
		return "", errors.New("OAuth credential has no native secret material")
	}
	defer clear(raw)
	var stored tokenBundle
	if err := json.Unmarshal(raw, &stored); err != nil {
		return "", errors.New("OAuth stored token is invalid")
	}
	if stored.Resource != metadata.Resource ||
		(stored.MCPEndpoint != "" && stored.MCPEndpoint != resource) ||
		stored.Issuer != metadata.Issuer ||
		stored.ClientID != metadata.ClientID ||
		stored.TokenEndpoint != metadata.TokenEndpoint ||
		stored.AccessToken == "" || !strings.EqualFold(stored.TokenType, "Bearer") {
		return "", errors.New("OAuth stored token does not match its Credential identity")
	}
	if stored.ExpiresAt.IsZero() || stored.ExpiresAt.After(m.clock.Now().Add(45*time.Second)) {
		return stored.AccessToken, nil
	}
	if stored.RefreshToken == "" {
		expired, touchErr := credential.Touch(m.clock.Now())
		if touchErr == nil {
			next := *metadata
			next.Status = credentialdomain.OAuthExpired
			if expired, touchErr = expired.WithOAuthMetadata(next); touchErr == nil {
				_ = m.repository.Save(ctx, expired)
			}
		}
		return "", errors.New("OAuth access token expired and no refresh token is available")
	}
	return m.refreshToken(ctx, ref, credential, &stored)
}

// Disconnect performs a local OAuth disconnect without racing token refresh.
func (m *Manager) Disconnect(ctx context.Context, ref credentialdomain.Reference) (credentialdomain.Credential, error) {
	if m == nil {
		return credentialdomain.Credential{}, errors.New("OAuth token manager is unavailable")
	}
	m.tokenMu.Lock()
	defer m.tokenMu.Unlock()
	return credentialapplication.NewDisconnectOAuth(m.repository, m.secrets, m.clock).Execute(ctx, ref)
}

type refreshTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope"`
	Error        string `json:"error"`
}

func (m *Manager) refreshToken(
	ctx context.Context,
	ref credentialdomain.Reference,
	credential credentialdomain.Credential,
	stored *tokenBundle,
) (string, error) {
	metadata := credential.OAuthMetadata()
	endpoint, err := url.Parse(metadata.TokenEndpoint)
	if err != nil || checkRequestURL(endpoint) != nil {
		return "", errors.New("OAuth refresh endpoint is unsafe")
	}
	values := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {stored.RefreshToken},
		"resource":      {metadata.Resource},
	}
	if stored.TokenAuthStyle == "basic" && stored.ClientSecret != "" {
		// RFC 6749 client_secret_basic authenticates with HTTP Basic, not
		// a second client_secret parameter in the request body.
	} else {
		values.Set("client_id", stored.ClientID)
		if stored.ClientSecret != "" {
			values.Set("client_secret", stored.ClientSecret)
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, metadata.TokenEndpoint, bytes.NewBufferString(values.Encode()))
	if err != nil {
		return "", errors.New("OAuth refresh request could not be constructed")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if stored.TokenAuthStyle == "basic" && stored.ClientSecret != "" {
		req.SetBasicAuth(stored.ClientID, stored.ClientSecret)
	}
	resp, err := m.options.HTTPClient.Do(req)
	if err != nil {
		return "", errors.New("OAuth refresh request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	var result refreshTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", errors.New("OAuth refresh response is invalid")
	}
	if resp.StatusCode == http.StatusBadRequest && result.Error == "invalid_grant" {
		if _, err := credentialapplication.NewDisconnectOAuth(m.repository, m.secrets, m.clock).Execute(ctx, ref); err != nil {
			return "", errors.New("OAuth refresh was rejected; credential disconnect failed")
		}
		return "", errors.New("OAuth authorization has expired; reauthorization required")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 ||
		strings.TrimSpace(result.AccessToken) == "" ||
		!strings.EqualFold(result.TokenType, "Bearer") ||
		result.ExpiresIn <= 0 || result.ExpiresIn > 365*24*3600 {
		return "", errors.New("OAuth authorization server rejected token refresh")
	}
	next := *stored
	next.TokenType = "Bearer"
	next.AccessToken = result.AccessToken
	if result.RefreshToken != "" {
		next.RefreshToken = result.RefreshToken
	}
	next.ExpiresAt = m.clock.Now().Add(time.Duration(result.ExpiresIn) * time.Second)
	next.CapturedAt = m.clock.Now()
	if result.Scope != "" {
		next.Scopes = grantedScopeTokens(result.Scope)
	}
	//nolint:gosec // Rotated token bytes are serialized only for native OS keyring replacement.
	nextRaw, err := json.Marshal(next)
	if err != nil {
		return "", errors.New("OAuth refresh token could not be encoded")
	}
	defer clear(nextRaw)
	if err := m.secrets.Put(ctx, ref, nextRaw); err != nil {
		return "", errors.New("OAuth rotated token could not be persisted")
	}
	updated, err := credential.Touch(m.clock.Now())
	if err != nil {
		return "", errors.New("OAuth refresh metadata is invalid")
	}
	newMeta := *metadata
	newMeta.ExpiresAt = next.ExpiresAt
	newMeta.Scopes = append([]string(nil), next.Scopes...)
	newMeta.Status = credentialdomain.OAuthConnected
	updated, err = updated.WithOAuthMetadata(newMeta)
	if err != nil {
		return "", errors.New("OAuth refresh metadata could not be updated")
	}
	if err := m.repository.Save(ctx, updated); err != nil {
		return "", errors.New("OAuth refreshed metadata could not be persisted")
	}
	return next.AccessToken, nil
}
