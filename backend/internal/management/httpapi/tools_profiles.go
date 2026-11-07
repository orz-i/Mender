package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
)

// Tool discovery accepts only canonical SourceRef filters. Silently ignoring
// an old providerId (or a typo) would turn a scoped request into an unscoped
// Catalog query, so unknown/duplicate query parameters fail closed.
func validateToolDiscoveryQuery(r *http.Request, allowed ...string) error {
	fields := make(map[string]bool, len(allowed))
	for _, name := range allowed {
		fields[name] = true
	}
	for name, values := range r.URL.Query() {
		if !fields[name] || len(values) != 1 {
			return fmt.Errorf("unsupported or repeated Tool query parameter %q", name)
		}
	}
	return nil
}

func (h *Handler) browseTools(w http.ResponseWriter, r *http.Request) {
	if err := validateToolDiscoveryQuery(r, "limit", "cursor", "sourceId"); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	limit := 100
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "validation_error", fmt.Errorf("limit must be an integer"))
			return
		}
		limit = parsed
	}

	var sourceRef *catalogdomain.SourceRef
	if raw := r.URL.Query().Get("sourceId"); raw != "" {
		parsed, err := catalogdomain.ParseSourceRef(raw)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "validation_error", err)
			return
		}
		sourceRef = &parsed
	}

	page, err := h.runtime.PageCatalog(r.Context(), catalogapplication.PageToolsRequest{
		Mode: "browse", Limit: limit, SourceRef: sourceRef, Cursor: r.URL.Query().Get("cursor"),
	}, nil)
	if err != nil {
		writeKnownError(w, err)
		return
	}

	items := make([]map[string]any, 0, len(page.Tools))
	for index, result := range page.Tools {
		item := map[string]any{
			"rank":        page.Offset + index + 1,
			"toolId":      result.ToolID.String(),
			"sourceId":    result.SourceRef.String(),
			"sourceKind":  string(result.SourceRef.Kind()),
			"name":        result.Name,
			"title":       result.Title,
			"description": result.Description,
		}
		if result.SourceRef.Kind() == catalogdomain.SourceProvider {
			item["providerId"] = result.SourceRef.ID()
		}
		items = append(items, item)
	}
	result := map[string]any{"tools": items}
	if page.NextCursor != "" {
		result["nextCursor"] = page.NextCursor
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) searchTools(w http.ResponseWriter, r *http.Request) {
	if err := validateToolDiscoveryQuery(r, "query", "limit", "cursor", "sourceId", "profile"); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
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

	var sourceRef *catalogdomain.SourceRef
	if raw := r.URL.Query().Get("sourceId"); raw != "" {
		parsed, err := catalogdomain.ParseSourceRef(raw)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "validation_error", err)
			return
		}
		sourceRef = &parsed
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

	page, err := h.runtime.PageCatalog(r.Context(), catalogapplication.PageToolsRequest{
		Mode: "search", Query: query, Limit: limit, SourceRef: sourceRef, Cursor: r.URL.Query().Get("cursor"),
	}, profileSlug)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	items := make([]map[string]any, 0, len(page.Tools))
	for index, result := range page.Tools {
		item := map[string]any{
			"rank":        page.Offset + index + 1,
			"toolId":      result.ToolID.String(),
			"sourceId":    result.SourceRef.String(),
			"sourceKind":  string(result.SourceRef.Kind()),
			"name":        result.Name,
			"title":       result.Title,
			"description": result.Description,
		}
		if result.SourceRef.Kind() == catalogdomain.SourceProvider {
			item["providerId"] = result.SourceRef.ID()
		}
		items = append(items, item)
	}
	result := map[string]any{"tools": items}
	if page.NextCursor != "" {
		result["nextCursor"] = page.NextCursor
	}
	writeJSON(w, http.StatusOK, result)
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

	executionContext, err := governancedomain.NewContext(
		governancedomain.OriginManagementHTTP,
		h.runtime.OwnerPrincipal(),
		governancedomain.CatalogScope(),
	)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "internal_error", err)
		return
	}

	response, err := h.runtime.CallConfiguredTool(
		r.Context(),
		toolID,
		arguments,
		executionContext,
	)
	if err != nil {
		if isKnownClientError(err) {
			writeKnownError(w, err)
		} else {
			writeAPIError(w, http.StatusBadGateway, "execution_error", err)
		}
		return
	}

	projection, err := invocationProjection(response.InvocationID.String(), response.Outcome)
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
	Slug        string `json:"slug"`
	DisplayName string `json:"displayName"`
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
	ToolID                string   `json:"toolId"`
	Alias                 string   `json:"alias"`
	TitleOverride         string   `json:"titleOverride"`
	DescriptionOverride   string   `json:"descriptionOverride"`
	PolicyConstraint      string   `json:"policyConstraint"`
	ConcurrencyConstraint string   `json:"concurrencyConstraint"`
	CredentialRefs        []string `json:"credentialRefs"`
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
	credentialRefs, err := parseCredentialReferences(input.CredentialRefs)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}

	profile, err := h.runtime.Profiles.AddTool.Execute(
		r.Context(),
		profileapplication.AddToolRequest{
			ProfileSlug:           slug,
			ToolID:                toolID,
			Alias:                 input.Alias,
			TitleOverride:         input.TitleOverride,
			DescriptionOverride:   input.DescriptionOverride,
			PolicyConstraint:      profiledomain.PolicyConstraint(input.PolicyConstraint),
			ConcurrencyConstraint: profiledomain.ConcurrencyConstraint(input.ConcurrencyConstraint),
			CredentialRefs:        credentialRefs,
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

	tools := make([]profileapplication.ToolBindingInput, 0, len(input.Tools))
	for _, item := range input.Tools {
		toolID, err := catalogdomain.ParseToolID(item.ToolID)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "validation_error", err)
			return
		}
		credentialRefs, err := parseCredentialReferences(item.CredentialRefs)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "validation_error", err)
			return
		}
		tools = append(tools, profileapplication.ToolBindingInput{
			ToolID:                toolID,
			Alias:                 item.Alias,
			TitleOverride:         item.TitleOverride,
			DescriptionOverride:   item.DescriptionOverride,
			PolicyConstraint:      profiledomain.PolicyConstraint(item.PolicyConstraint),
			ConcurrencyConstraint: profiledomain.ConcurrencyConstraint(item.ConcurrencyConstraint),
			CredentialRefs:        credentialRefs,
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

// replaceProfileSkills changes exact Skill membership only. No files are
// deployed and no Skill scripts run during this operation.
func (h *Handler) replaceProfileSkills(w http.ResponseWriter, r *http.Request) {
	slug, err := profiledomain.NewSlug(r.PathValue("slug"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	var input struct {
		InstallationIDs *[]string `json:"installationIds"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	if input.InstallationIDs == nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", fmt.Errorf("installationIds must be an array"))
		return
	}
	ids := make([]skilldomain.InstallationID, 0, len(*input.InstallationIDs))
	for _, rawID := range *input.InstallationIDs {
		id, err := skilldomain.ParseInstallationID(rawID)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "validation_error", err)
			return
		}
		ids = append(ids, id)
	}
	profile, err := h.runtime.Profiles.ReplaceSkills.Execute(r.Context(), profileapplication.ReplaceSkillsRequest{
		ProfileSlug: slug, InstallationIDs: ids,
	})
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
	result := map[string]any{
		"toolId":       tool.ID().String(),
		"sourceId":     tool.SourceRef().String(),
		"sourceKind":   string(tool.SourceRef().Kind()),
		"sourceKey":    tool.SourceKey(),
		"name":         tool.Name(),
		"title":        tool.Title(),
		"description":  tool.Description(),
		"inputSchema":  inputSchema,
		"outputSchema": outputSchema,
		"tags":         tool.Tags(),
	}
	if tool.SourceRef().Kind() == catalogdomain.SourceProvider {
		result["providerId"] = tool.SourceRef().ID()
	}
	return result, nil
}

func parseCredentialReferences(values []string) ([]credentialdomain.Reference, error) {
	result := make([]credentialdomain.Reference, 0, len(values))
	for _, value := range values {
		ref, err := credentialdomain.ParseReference(value)
		if err != nil {
			return nil, err
		}
		result = append(result, ref)
	}
	return result, nil
}

func credentialReferenceStrings(values []credentialdomain.Reference) []string {
	result := make([]string, 0, len(values))
	for _, ref := range values {
		result = append(result, ref.String())
	}
	return result
}

func profileSkillIDs(profile profiledomain.Profile) []string {
	ids := make([]string, 0, len(profile.Skills()))
	for _, id := range profile.Skills() {
		ids = append(ids, id.String())
	}
	return ids
}

func profileProjection(profile profiledomain.Profile) map[string]any {
	tools := make([]map[string]any, 0, len(profile.Bindings()))
	for _, binding := range profile.Bindings() {
		tools = append(tools, map[string]any{
			"toolId":                binding.ToolID().String(),
			"alias":                 binding.Alias(),
			"titleOverride":         binding.TitleOverride(),
			"descriptionOverride":   binding.DescriptionOverride(),
			"policyConstraint":      string(binding.PolicyConstraint()),
			"concurrencyConstraint": string(binding.ConcurrencyConstraint()),
			"credentialRefs":        credentialReferenceStrings(binding.CredentialRefs()),
		})
	}
	result := map[string]any{
		"profileId":   profile.ID().String(),
		"slug":        profile.Slug().String(),
		"displayName": profile.DisplayName(),
		"tools":       tools,
		"skills":      profileSkillIDs(profile),
		"revision":    profile.Revision(),
	}
	return result
}
