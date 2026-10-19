package httpapi

import (
	"errors"
	"net/http"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	oauthflow "github.com/orz-i/Mender/backend/internal/credential/infrastructure/oauthflow"
)

// OAuth flow endpoints are exclusively local management operations. Tool
// execution and Provider activation have no path to trigger browser consent.
type startOAuthCredentialRequest struct {
	Namespace           string   `json:"namespace"`
	Name                string   `json:"name"`
	DisplayName         string   `json:"displayName"`
	MCPEndpoint         string   `json:"mcpEndpoint"`
	ClientID            string   `json:"clientId"`
	ClientIDMetadataURL string   `json:"clientIdMetadataUrl,omitempty"`
	Scopes              []string `json:"scopes,omitempty"`
}

func (h *Handler) startOAuthCredential(w http.ResponseWriter, r *http.Request) {
	var input startOAuthCredentialRequest
	if err := decodeJSON(r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	result, err := h.runtime.Credentials.OAuthFlow.Start(r.Context(), oauthflow.StartRequest{
		Namespace: input.Namespace, Name: input.Name, DisplayName: input.DisplayName,
		MCPEndpoint: input.MCPEndpoint, ClientID: input.ClientID,
		ClientIDMetadataURL: input.ClientIDMetadataURL, Scopes: input.Scopes,
	})
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "oauth_authorization_failed", err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h *Handler) getOAuthFlow(w http.ResponseWriter, r *http.Request) {
	view, err := h.runtime.Credentials.OAuthFlow.Status(r.Context(), r.PathValue("flowId"))
	if errors.Is(err, oauthflow.ErrFlowNotFound) {
		writeAPIError(w, http.StatusNotFound, "not_found", err)
		return
	}
	if err != nil {
		writeKnownError(w, err)
		return
	}
	result := map[string]any{
		"flowId":    view.FlowID,
		"status":    view.Status,
		"createdAt": view.CreatedAt,
		"expiresAt": view.ExpiresAt,
	}
	if view.Message != "" {
		result["message"] = view.Message
	}
	if view.Credential != nil {
		ref, parseErr := credentialdomain.ParseReference(view.Credential.Reference)
		if parseErr != nil {
			writeKnownError(w, parseErr)
			return
		}
		credential, getErr := h.runtime.Credentials.Get.Execute(r.Context(), ref)
		if getErr != nil {
			writeKnownError(w, getErr)
			return
		}
		result["credential"] = credentialProjection(credential)
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) cancelOAuthFlow(w http.ResponseWriter, r *http.Request) {
	err := h.runtime.Credentials.OAuthFlow.Cancel(r.Context(), r.PathValue("flowId"))
	switch {
	case errors.Is(err, oauthflow.ErrFlowNotFound):
		writeAPIError(w, http.StatusNotFound, "not_found", err)
	case errors.Is(err, oauthflow.ErrFlowNotPending):
		writeAPIError(w, http.StatusConflict, "conflict", err)
	case err != nil:
		writeKnownError(w, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
