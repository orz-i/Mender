package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	auditdomain "github.com/orz-i/Mender/backend/internal/audit/domain"
	"github.com/orz-i/Mender/backend/internal/bootstrap"
	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
)

func (h *Handler) getSystemStatus(w http.ResponseWriter, _ *http.Request) {
	status := h.runtime.Status()
	writeJSON(w, http.StatusOK, map[string]any{
		"ready":  status.Ready,
		"policy": policyStatusProjection(status.PolicyMode, status.ApprovalMode),
		"credentials": credentialStatusProjection(
			status.CredentialResolver,
			status.CredentialPersistent,
		),
		"mcp": map[string]any{
			"stdio":          true,
			"streamableHttp": true,
			"exposureModes":  []string{"direct", "dynamic", "hybrid"},
		},
	})
}

func (h *Handler) getPolicyStatus(w http.ResponseWriter, _ *http.Request) {
	status := h.runtime.Status()
	writeJSON(w, http.StatusOK, policyStatusProjection(status.PolicyMode, status.ApprovalMode))
}

func (h *Handler) getCredentialStatus(w http.ResponseWriter, _ *http.Request) {
	status := h.runtime.Status()
	writeJSON(w, http.StatusOK, credentialStatusProjection(
		status.CredentialResolver,
		status.CredentialPersistent,
	))
}

func policyStatusProjection(mode, approval string) map[string]any {
	return map[string]any{
		"mode":     mode,
		"editable": false,
		"approval": approval,
	}
}

func credentialStatusProjection(resolver string, persistent bool) map[string]any {
	return map[string]any{
		"resolver":        resolver,
		"persistentStore": persistent,
		"editable":        false,
	}
}

func (h *Handler) listAudit(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "validation_error", fmt.Errorf("limit must be an integer"))
			return
		}
		limit = parsed
	}

	records, err := h.runtime.ListAudit.Execute(r.Context(), limit)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	items := make([]map[string]any, 0, len(records))
	for _, record := range records {
		items = append(items, auditProjection(record))
	}
	writeJSON(w, http.StatusOK, map[string]any{"records": items})
}

func auditProjection(record auditdomain.Record) map[string]any {
	result := map[string]any{
		"invocationId": record.InvocationID().String(),
		"toolId":       record.ToolID().String(),
		"caller":       record.Caller(),
		"startedAt":    record.StartedAt(),
		"finishedAt":   record.FinishedAt(),
		"policyKind":   string(record.PolicyKind()),
		"policyReason": record.PolicyReason(),
		"approval":     string(record.Approval()),
		"status":       string(record.Status()),
		"resultKind":   string(record.ResultKind()),
		"errorClass":   string(record.ErrorClass()),
		"errorSummary": record.ErrorSummary(),
	}
	if providerID := record.ProviderID(); providerID != nil {
		result["providerId"] = providerID.String()
	}
	return result
}

func invocationProjection(
	invocationID string,
	result invocationdomain.Result,
) (map[string]any, error) {
	projection := map[string]any{
		"invocationId": invocationID,
		"kind":         string(result.Kind()),
	}

	switch result.Kind() {
	case invocationdomain.ResultKindComplete:
		var content any
		if err := json.Unmarshal(result.Content().Bytes(), &content); err != nil {
			return nil, fmt.Errorf("decode invocation content: %w", err)
		}
		projection["content"] = content

	case invocationdomain.ResultKindInputRequired:
		projection["requestId"] = result.RequestID()
		projection["message"] = result.Message()
		if schema := result.InputSchema(); len(schema) != 0 {
			var inputSchema any
			if err := json.Unmarshal(schema, &inputSchema); err != nil {
				return nil, fmt.Errorf("decode input-required schema: %w", err)
			}
			projection["inputSchema"] = inputSchema
		}

	case invocationdomain.ResultKindTask:
		projection["taskId"] = result.TaskID()
		projection["status"] = result.TaskStatus()
		projection["message"] = result.Message()
	}

	return projection, nil
}

func isKnownClientError(err error) bool {
	return errors.Is(err, catalogapplication.ErrProviderNotFound) ||
		errors.Is(err, catalogapplication.ErrToolNotFound) ||
		errors.Is(err, profileapplication.ErrProfileNotFound) ||
		errors.Is(err, bootstrap.ErrInvalidMCPServerAddress) ||
		errors.Is(err, bootstrap.ErrMCPServerNotFound) ||
		errors.Is(err, invocationapplication.ErrPolicyDenied) ||
		errors.Is(err, invocationapplication.ErrApprovalDenied)
}
