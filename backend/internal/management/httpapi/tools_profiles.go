package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

func (h *Handler) browseTools(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "validation_error", fmt.Errorf("limit must be an integer"))
			return
		}
		limit = parsed
	}

	var providerID *catalogdomain.ProviderID
	if rawProviderID := r.URL.Query().Get("providerId"); rawProviderID != "" {
		parsed, err := catalogdomain.ParseProviderID(rawProviderID)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "validation_error", err)
			return
		}
		providerID = &parsed
	}

	results, err := h.runtime.Catalog.BrowseTools.Execute(
		r.Context(),
		catalogapplication.BrowseToolsRequest{
			Limit:      limit,
			ProviderID: providerID,
		},
	)
	if err != nil {
		writeKnownError(w, err)
		return
	}

	items := make([]map[string]any, 0, len(results))
	for index, result := range results {
		items = append(items, map[string]any{
			"rank":        index + 1,
			"toolId":      result.ToolID.String(),
			"providerId":  result.ProviderID.String(),
			"name":        result.Name,
			"title":       result.Title,
			"description": result.Description,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"tools": items})
}

func (h *Handler) searchTools(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	if query == "" {
		writeAPIError(w, http.StatusBadRequest, "validation_error", fmt.Errorf("query is required"))
		return
	}

	limit := 20
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "validation_error", fmt.Errorf("limit must be an integer"))
			return
		}
		limit = parsed
	}

	var profileSlug *profiledomain.Slug
	if rawProfile := r.URL.Query().Get("profile"); rawProfile != "" {
		slug, err := profiledomain.NewSlug(rawProfile)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "validation_error", err)
			return
		}
		profileSlug = &slug
	}

	results, err := h.runtime.SearchCatalog(r.Context(), query, limit, profileSlug)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	items := make([]map[string]any, 0, len(results))
	for _, result := range results {
		items = append(items, map[string]any{
			"rank":        result.Rank,
			"toolId":      result.ToolID.String(),
			"providerId":  result.ProviderID.String(),
			"name":        result.Name,
			"title":       result.Title,
			"description": result.Description,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"tools": items})
}

func (h *Handler) getTool(w http.ResponseWriter, r *http.Request) {
	toolID, err := catalogdomain.ParseToolID(r.PathValue("toolId"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	tool, err := h.runtime.Catalog.DescribeTool.Execute(r.Context(), toolID)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	projection, err := toolProjection(tool)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, projection)
}

type toolCallRequest struct {
	Arguments map[string]any `json:"arguments"`
}

func (h *Handler) callTool(w http.ResponseWriter, r *http.Request) {
	toolID, err := catalogdomain.ParseToolID(r.PathValue("toolId"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}

	var input toolCallRequest
	if err := decodeJSON(r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	if input.Arguments == nil {
		input.Arguments = map[string]any{}
	}
	arguments, err := json.Marshal(input.Arguments)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}

	response, err := h.runtime.CallConfiguredTool(
		r.Context(),
		toolID,
		arguments,
		"web",
	)
	if err != nil {
		if isKnownClientError(err) {
			writeKnownError(w, err)
		} else {
			writeAPIError(w, http.StatusBadGateway, "execution_error", err)
		}
		return
	}

	projection, err := invocationProjection(response.InvocationID.String(), response.Result)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, projection)
}

func (h *Handler) listProfiles(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.runtime.Profiles.List.Execute(r.Context())
	if err != nil {
		writeKnownError(w, err)
		return
	}
	items := make([]map[string]any, 0, len(profiles))
	for _, profile := range profiles {
		items = append(items, profileProjection(profile))
	}
	writeJSON(w, http.StatusOK, map[string]any{"profiles": items})
}

type createProfileRequest struct {
	Slug        string                     `json:"slug"`
	DisplayName string                     `json:"displayName"`
	Mode        profiledomain.ExposureMode `json:"mode"`
}

func (h *Handler) createProfile(w http.ResponseWriter, r *http.Request) {
	var input createProfileRequest
	if err := decodeJSON(r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	profile, err := h.runtime.Profiles.Create.Execute(
		r.Context(),
		profileapplication.CreateProfileRequest{
			Slug:        input.Slug,
			DisplayName: input.DisplayName,
			Mode:        input.Mode,
		},
	)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, profileProjection(profile))
}

func (h *Handler) getProfile(w http.ResponseWriter, r *http.Request) {
	slug, err := profiledomain.NewSlug(r.PathValue("slug"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	profile, err := h.runtime.Profiles.Get.Execute(r.Context(), slug)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, profileProjection(profile))
}

type addProfileToolRequest struct {
	ToolID string `json:"toolId"`
	Direct bool   `json:"direct"`
}

type replaceProfileToolsRequest struct {
	Tools []addProfileToolRequest `json:"tools"`
}

func (h *Handler) addProfileTool(w http.ResponseWriter, r *http.Request) {
	slug, err := profiledomain.NewSlug(r.PathValue("slug"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	var input addProfileToolRequest
	if err := decodeJSON(r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	toolID, err := catalogdomain.ParseToolID(input.ToolID)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}

	profile, err := h.runtime.Profiles.AddTool.Execute(
		r.Context(),
		profileapplication.AddToolRequest{
			ProfileSlug: slug,
			ToolID:      toolID,
			Direct:      input.Direct,
		},
	)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, profileProjection(profile))
}

func (h *Handler) replaceProfileTools(w http.ResponseWriter, r *http.Request) {
	slug, err := profiledomain.NewSlug(r.PathValue("slug"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	var input replaceProfileToolsRequest
	if err := decodeJSON(r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}

	tools := make([]profileapplication.ReplaceToolSelection, 0, len(input.Tools))
	for _, item := range input.Tools {
		toolID, err := catalogdomain.ParseToolID(item.ToolID)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "validation_error", err)
			return
		}
		tools = append(tools, profileapplication.ReplaceToolSelection{
			ToolID: toolID,
			Direct: item.Direct,
		})
	}

	profile, err := h.runtime.Profiles.ReplaceTools.Execute(
		r.Context(),
		profileapplication.ReplaceToolsRequest{
			ProfileSlug: slug,
			Tools:       tools,
		},
	)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, profileProjection(profile))
}

func (h *Handler) removeProfileTool(w http.ResponseWriter, r *http.Request) {
	slug, err := profiledomain.NewSlug(r.PathValue("slug"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	toolID, err := catalogdomain.ParseToolID(r.PathValue("toolId"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}

	profile, err := h.runtime.Profiles.RemoveTool.Execute(
		r.Context(),
		profileapplication.RemoveToolRequest{
			ProfileSlug: slug,
			ToolID:      toolID,
		},
	)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, profileProjection(profile))
}

func toolProjection(tool catalogdomain.Tool) (map[string]any, error) {
	var inputSchema any
	if err := json.Unmarshal(tool.InputSchema().Bytes(), &inputSchema); err != nil {
		return nil, fmt.Errorf("decode input schema: %w", err)
	}
	var outputSchema any
	if schema := tool.OutputSchema(); schema != nil {
		if err := json.Unmarshal(schema.Bytes(), &outputSchema); err != nil {
			return nil, fmt.Errorf("decode output schema: %w", err)
		}
	}
	return map[string]any{
		"toolId":       tool.ID().String(),
		"providerId":   tool.ProviderID().String(),
		"sourceKey":    tool.SourceKey(),
		"name":         tool.Name(),
		"title":        tool.Title(),
		"description":  tool.Description(),
		"inputSchema":  inputSchema,
		"outputSchema": outputSchema,
		"tags":         tool.Tags(),
	}, nil
}

func profileProjection(profile profiledomain.Profile) map[string]any {
	tools := make([]map[string]any, 0, len(profile.Selections()))
	for _, selection := range profile.Selections() {
		tools = append(tools, map[string]any{
			"toolId": selection.ToolID.String(),
			"direct": selection.Direct,
		})
	}
	return map[string]any{
		"profileId":   profile.ID().String(),
		"slug":        profile.Slug().String(),
		"displayName": profile.DisplayName(),
		"mode":        string(profile.Mode()),
		"tools":       tools,
	}
}
