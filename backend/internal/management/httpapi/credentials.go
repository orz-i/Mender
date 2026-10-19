package httpapi

import (
	"net/http"

	credentialapplication "github.com/orz-i/Mender/backend/internal/credential/application"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
)

type createNativeCredentialRequest struct {
	Namespace        string `json:"namespace"`
	Name             string `json:"name"`
	DisplayName      string `json:"displayName"`
	Secret           string `json:"secret"`
	SSHKeyPassphrase string `json:"sshKeyPassphrase,omitempty"`
}

type createEnvironmentCredentialRequest struct {
	Namespace           string `json:"namespace"`
	Name                string `json:"name"`
	DisplayName         string `json:"displayName"`
	EnvironmentVariable string `json:"environmentVariable"`
}

type rotateCredentialRequest struct {
	Secret           string `json:"secret"`
	SSHKeyPassphrase string `json:"sshKeyPassphrase,omitempty"`
}

func (h *Handler) listCredentials(w http.ResponseWriter, r *http.Request) {
	items, err := h.runtime.Credentials.List.Execute(r.Context())
	if err != nil {
		writeKnownError(w, err)
		return
	}
	credentials := make([]map[string]any, 0, len(items))
	for _, item := range items {
		credentials = append(credentials, credentialProjection(item))
	}
	writeJSON(w, http.StatusOK, map[string]any{"credentials": credentials})
}

func (h *Handler) getCredential(w http.ResponseWriter, r *http.Request) {
	ref, err := credentialReferenceFromRequest(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	credential, err := h.runtime.Credentials.Get.Execute(r.Context(), ref)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, credentialProjection(credential))
}

func (h *Handler) createNativeCredential(w http.ResponseWriter, r *http.Request) {
	var input createNativeCredentialRequest
	if err := decodeJSON(r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	credential, err := h.runtime.Credentials.CreateNative.Execute(
		r.Context(),
		credentialapplication.CreateNativeRequest{
			Namespace:        input.Namespace,
			Name:             input.Name,
			DisplayName:      input.DisplayName,
			Secret:           []byte(input.Secret),
			SSHKeyPassphrase: []byte(input.SSHKeyPassphrase),
		},
	)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, credentialProjection(credential))
}

func (h *Handler) createEnvironmentCredential(w http.ResponseWriter, r *http.Request) {
	var input createEnvironmentCredentialRequest
	if err := decodeJSON(r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	credential, err := h.runtime.Credentials.CreateEnvironment.Execute(
		r.Context(),
		credentialapplication.CreateEnvironmentRequest{
			Namespace:           input.Namespace,
			Name:                input.Name,
			DisplayName:         input.DisplayName,
			EnvironmentVariable: input.EnvironmentVariable,
		},
	)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, credentialProjection(credential))
}

func (h *Handler) rotateCredential(w http.ResponseWriter, r *http.Request) {
	ref, err := credentialReferenceFromRequest(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	var input rotateCredentialRequest
	if err := decodeJSON(r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	credential, err := h.runtime.Credentials.RotateNative.Execute(
		r.Context(),
		ref,
		[]byte(input.Secret),
		[]byte(input.SSHKeyPassphrase),
	)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, credentialProjection(credential))
}

func (h *Handler) deleteCredential(w http.ResponseWriter, r *http.Request) {
	ref, err := credentialReferenceFromRequest(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	if err := h.runtime.Credentials.Delete.Execute(r.Context(), ref); err != nil {
		writeKnownError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func credentialReferenceFromRequest(r *http.Request) (credentialdomain.Reference, error) {
	return credentialdomain.NewReference(
		r.PathValue("namespace"),
		r.PathValue("name"),
	)
}

func credentialProjection(credential credentialdomain.Credential) map[string]any {
	result := map[string]any{
		"reference":           credential.Reference().String(),
		"namespace":           credential.Reference().Namespace(),
		"name":                credential.Reference().Name(),
		"displayName":         credential.DisplayName(),
		"source":              string(credential.Source()),
		"environmentVariable": credential.EnvironmentVariable(),
		"createdAt":           credential.CreatedAt(),
		"updatedAt":           credential.UpdatedAt(),
	}
	if metadata := credential.OAuthMetadata(); metadata != nil {
		result["oauth"] = metadata
	}
	return result
}
