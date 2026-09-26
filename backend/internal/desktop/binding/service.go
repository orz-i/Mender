package binding

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	auditdomain "github.com/orz-i/Mender/backend/internal/audit/domain"
	"github.com/orz-i/Mender/backend/internal/bootstrap"
	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	mcpconfigstore "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/configstore"
	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

const (
	defaultToolSearchLimit = 20
	defaultAuditLimit      = 50
	timeFormat             = time.RFC3339Nano
)

// Service adapts the shared Mender Runtime to a desktop binding surface.
// It deliberately has no dependency on Wails so the backend module remains
// independently buildable and testable.
type Service struct {
	ctx     context.Context
	runtime *bootstrap.Runtime
}

func NewService(ctx context.Context, runtime *bootstrap.Runtime) *Service {
	return &Service{ctx: ctx, runtime: runtime}
}

func (s *Service) GetSystemStatus() SystemStatus {
	status := s.runtime.Status()
	return SystemStatus{
		Ready:       status.Ready,
		Policy:      policyStatus(status),
		Credentials: credentialStatus(status),
		MCP: MCPStatus{
			Stdio:          true,
			StreamableHTTP: true,
			ExposureModes:  []string{"direct", "dynamic", "hybrid"},
		},
	}
}

func (s *Service) GetPolicyStatus() PolicyStatus {
	return policyStatus(s.runtime.Status())
}

func (s *Service) GetCredentialStatus() CredentialStatus {
	return credentialStatus(s.runtime.Status())
}

func (s *Service) ListProviders() (ProviderList, error) {
	providers, err := s.runtime.Catalog.ListProviders.Execute(s.ctx)
	if err != nil {
		return ProviderList{}, err
	}
	configs, err := s.runtime.ListMCPProviderConfigs(s.ctx)
	if err != nil {
		return ProviderList{}, err
	}
	configByProvider := make(map[catalogdomain.ProviderID]mcpconfigstore.Config, len(configs))
	for _, config := range configs {
		configByProvider[config.ProviderID] = config
	}
	toolCounts, err := s.runtime.Catalog.ListProviderToolCounts.Execute(s.ctx)
	if err != nil {
		return ProviderList{}, err
	}
	items := make([]Provider, 0, len(providers))
	for _, provider := range providers {
		items = append(items, providerProjection(
			provider,
			configByProvider[provider.ID()],
			toolCounts[provider.ID()],
		))
	}
	return ProviderList{Providers: items}, nil
}

func (s *Service) CreateHTTPProvider(
	namespace string,
	displayName string,
	description string,
	endpoint string,
	headers map[string]string,
) (ProviderMutationResponse, error) {
	if strings.TrimSpace(namespace) == "" ||
		strings.TrimSpace(displayName) == "" ||
		strings.TrimSpace(endpoint) == "" {
		return ProviderMutationResponse{}, fmt.Errorf(
			"namespace, displayName, and endpoint are required",
		)
	}

	provider, err := s.runtime.RegisterMCPProvider(
		s.ctx,
		catalogapplication.RegisterProviderRequest{
			Namespace:   namespace,
			DisplayName: displayName,
			Description: description,
		},
		mcpconfigstore.Config{
			Transport: mcpconfigstore.TransportHTTP,
			Endpoint:  endpoint,
			Headers:   cloneHeaders(headers),
		},
	)
	if err != nil {
		return ProviderMutationResponse{}, err
	}
	result, err := s.runtime.SyncConfiguredMCPProvider(s.ctx, provider.ID())
	if err != nil {
		return ProviderMutationResponse{}, err
	}
	config, err := s.runtime.MCPProviderConfig(s.ctx, provider.ID())
	if err != nil {
		return ProviderMutationResponse{}, err
	}
	return ProviderMutationResponse{
		Provider: providerProjection(provider, config, result.Total),
		Sync:     syncProjection(result),
	}, nil
}

func (s *Service) CreateStdioProvider(
	namespace string,
	displayName string,
	description string,
	executable string,
	args []string,
	directory string,
) (ProviderMutationResponse, error) {
	if strings.TrimSpace(namespace) == "" ||
		strings.TrimSpace(displayName) == "" ||
		strings.TrimSpace(executable) == "" {
		return ProviderMutationResponse{}, fmt.Errorf(
			"namespace, displayName, and executable are required",
		)
	}

	provider, err := s.runtime.RegisterMCPProvider(
		s.ctx,
		catalogapplication.RegisterProviderRequest{
			Namespace:   namespace,
			DisplayName: displayName,
			Description: description,
		},
		mcpconfigstore.Config{
			Transport:  mcpconfigstore.TransportStdio,
			Executable: executable,
			Args:       append([]string(nil), args...),
			Directory:  directory,
		},
	)
	if err != nil {
		return ProviderMutationResponse{}, err
	}
	result, err := s.runtime.SyncConfiguredMCPProvider(s.ctx, provider.ID())
	if err != nil {
		return ProviderMutationResponse{}, err
	}
	config, err := s.runtime.MCPProviderConfig(s.ctx, provider.ID())
	if err != nil {
		return ProviderMutationResponse{}, err
	}
	return ProviderMutationResponse{
		Provider: providerProjection(provider, config, result.Total),
		Sync:     syncProjection(result),
	}, nil
}

func (s *Service) UpdateProvider(
	providerID string,
	displayName string,
	description string,
	transport string,
	endpoint string,
	headers map[string]string,
	executable string,
	args []string,
	directory string,
) (ProviderMutationResponse, error) {
	id, err := catalogdomain.ParseProviderID(providerID)
	if err != nil {
		return ProviderMutationResponse{}, err
	}
	if strings.TrimSpace(displayName) == "" {
		return ProviderMutationResponse{}, fmt.Errorf("displayName is required")
	}

	var config mcpconfigstore.Config
	switch transport {
	case string(mcpconfigstore.TransportHTTP):
		config = mcpconfigstore.Config{
			Transport: mcpconfigstore.TransportHTTP,
			Endpoint:  endpoint,
			Headers:   cloneHeaders(headers),
		}
	case string(mcpconfigstore.TransportStdio):
		config = mcpconfigstore.Config{
			Transport:  mcpconfigstore.TransportStdio,
			Executable: executable,
			Args:       append([]string(nil), args...),
			Directory:  directory,
		}
	default:
		return ProviderMutationResponse{}, fmt.Errorf("transport must be http or stdio")
	}

	provider, err := s.runtime.UpdateMCPProvider(
		s.ctx,
		id,
		catalogapplication.UpdateProviderRequest{
			DisplayName: displayName,
			Description: description,
		},
		config,
	)
	if err != nil {
		return ProviderMutationResponse{}, err
	}
	result, err := s.runtime.SyncConfiguredMCPProvider(s.ctx, id)
	if err != nil {
		return ProviderMutationResponse{}, err
	}

	return ProviderMutationResponse{
		Provider: providerProjection(provider, config, result.Total),
		Sync:     syncProjection(result),
	}, nil
}

func (s *Service) SyncProvider(providerID string) (SyncResult, error) {
	id, err := catalogdomain.ParseProviderID(providerID)
	if err != nil {
		return SyncResult{}, err
	}
	result, err := s.runtime.SyncConfiguredMCPProvider(s.ctx, id)
	if err != nil {
		return SyncResult{}, err
	}
	return syncProjection(result), nil
}

func (s *Service) DeleteProvider(providerID string) error {
	id, err := catalogdomain.ParseProviderID(providerID)
	if err != nil {
		return err
	}
	return s.runtime.DeleteMCPProvider(s.ctx, id)
}

func (s *Service) BrowseTools(limit int, providerID string) (ToolList, error) {
	if limit == 0 {
		limit = 100
	}
	var id *catalogdomain.ProviderID
	if providerID != "" {
		parsed, err := catalogdomain.ParseProviderID(providerID)
		if err != nil {
			return ToolList{}, err
		}
		id = &parsed
	}
	results, err := s.runtime.Catalog.BrowseTools.Execute(
		s.ctx,
		catalogapplication.BrowseToolsRequest{
			Limit:      limit,
			ProviderID: id,
		},
	)
	if err != nil {
		return ToolList{}, err
	}
	items := make([]ToolSummary, 0, len(results))
	for index, result := range results {
		items = append(items, ToolSummary{
			Rank:        index + 1,
			ToolID:      result.ToolID.String(),
			ProviderID:  result.ProviderID.String(),
			Name:        result.Name,
			Title:       result.Title,
			Description: result.Description,
		})
	}
	return ToolList{Tools: items}, nil
}

func (s *Service) SearchTools(query string, limit int, profile string) (ToolList, error) {
	if limit == 0 {
		limit = defaultToolSearchLimit
	}
	var profileSlug *profiledomain.Slug
	if profile != "" {
		slug, err := profiledomain.NewSlug(profile)
		if err != nil {
			return ToolList{}, err
		}
		profileSlug = &slug
	}
	results, err := s.runtime.SearchCatalog(s.ctx, query, limit, profileSlug)
	if err != nil {
		return ToolList{}, err
	}
	items := make([]ToolSummary, 0, len(results))
	for _, result := range results {
		items = append(items, ToolSummary{
			Rank:        result.Rank,
			ToolID:      result.ToolID.String(),
			ProviderID:  result.ProviderID.String(),
			Name:        result.Name,
			Title:       result.Title,
			Description: result.Description,
		})
	}
	return ToolList{Tools: items}, nil
}

func (s *Service) GetTool(toolID string) (ToolDetail, error) {
	id, err := catalogdomain.ParseToolID(toolID)
	if err != nil {
		return ToolDetail{}, err
	}
	tool, err := s.runtime.Catalog.DescribeTool.Execute(s.ctx, id)
	if err != nil {
		return ToolDetail{}, err
	}
	return toolProjection(tool)
}

func (s *Service) CallTool(
	toolID string,
	arguments map[string]any,
) (InvocationResult, error) {
	id, err := catalogdomain.ParseToolID(toolID)
	if err != nil {
		return InvocationResult{}, err
	}
	if arguments == nil {
		arguments = map[string]any{}
	}
	payload, err := json.Marshal(arguments)
	if err != nil {
		return InvocationResult{}, fmt.Errorf("encode tool arguments: %w", err)
	}
	response, err := s.runtime.CallConfiguredTool(s.ctx, id, payload, "desktop")
	if err != nil {
		return InvocationResult{}, err
	}
	return invocationProjection(response.InvocationID.String(), response.Result)
}

func (s *Service) ListProfiles() (ProfileList, error) {
	profiles, err := s.runtime.Profiles.List.Execute(s.ctx)
	if err != nil {
		return ProfileList{}, err
	}
	items := make([]Profile, 0, len(profiles))
	for _, profile := range profiles {
		items = append(items, profileProjection(profile))
	}
	return ProfileList{Profiles: items}, nil
}

func (s *Service) CreateProfile(slug string, displayName string, mode string) (Profile, error) {
	profile, err := s.runtime.Profiles.Create.Execute(
		s.ctx,
		profileapplication.CreateProfileRequest{
			Slug:        slug,
			DisplayName: displayName,
			Mode:        profiledomain.ExposureMode(mode),
		},
	)
	if err != nil {
		return Profile{}, err
	}
	return profileProjection(profile), nil
}

func (s *Service) GetProfile(slug string) (Profile, error) {
	id, err := profiledomain.NewSlug(slug)
	if err != nil {
		return Profile{}, err
	}
	profile, err := s.runtime.Profiles.Get.Execute(s.ctx, id)
	if err != nil {
		return Profile{}, err
	}
	return profileProjection(profile), nil
}

func (s *Service) AddProfileTool(
	slug string,
	toolID string,
	direct bool,
) (Profile, error) {
	profileSlug, err := profiledomain.NewSlug(slug)
	if err != nil {
		return Profile{}, err
	}
	id, err := catalogdomain.ParseToolID(toolID)
	if err != nil {
		return Profile{}, err
	}
	profile, err := s.runtime.Profiles.AddTool.Execute(
		s.ctx,
		profileapplication.AddToolRequest{
			ProfileSlug: profileSlug,
			ToolID:      id,
			Direct:      direct,
		},
	)
	if err != nil {
		return Profile{}, err
	}
	return profileProjection(profile), nil
}

func (s *Service) RemoveProfileTool(slug string, toolID string) (Profile, error) {
	profileSlug, err := profiledomain.NewSlug(slug)
	if err != nil {
		return Profile{}, err
	}
	id, err := catalogdomain.ParseToolID(toolID)
	if err != nil {
		return Profile{}, err
	}
	profile, err := s.runtime.Profiles.RemoveTool.Execute(
		s.ctx,
		profileapplication.RemoveToolRequest{
			ProfileSlug: profileSlug,
			ToolID:      id,
		},
	)
	if err != nil {
		return Profile{}, err
	}
	return profileProjection(profile), nil
}

func (s *Service) ListAudit(limit int) (AuditList, error) {
	if limit == 0 {
		limit = defaultAuditLimit
	}
	records, err := s.runtime.ListAudit.Execute(s.ctx, limit)
	if err != nil {
		return AuditList{}, err
	}
	items := make([]AuditRecord, 0, len(records))
	for _, record := range records {
		items = append(items, auditProjection(record))
	}
	return AuditList{Records: items}, nil
}

func policyStatus(status bootstrap.RuntimeStatus) PolicyStatus {
	return PolicyStatus{
		Mode:     status.PolicyMode,
		Editable: false,
		Approval: status.ApprovalMode,
	}
}

func credentialStatus(status bootstrap.RuntimeStatus) CredentialStatus {
	return CredentialStatus{
		Resolver:        status.CredentialResolver,
		PersistentStore: status.CredentialPersistent,
		Editable:        false,
	}
}

func providerProjection(
	provider catalogdomain.Provider,
	config mcpconfigstore.Config,
	toolCount int,
) Provider {
	result := Provider{
		ProviderID:  provider.ID().String(),
		Namespace:   provider.Namespace().String(),
		DisplayName: provider.DisplayName(),
		Description: provider.Description(),
		CreatedAt:   provider.CreatedAt().Format(timeFormat),
		UpdatedAt:   provider.UpdatedAt().Format(timeFormat),
		Transport:   "unconfigured",
		ToolCount:   toolCount,
	}
	switch config.Transport {
	case mcpconfigstore.TransportHTTP:
		result.Transport = "http"
		result.Endpoint = config.Endpoint
		result.Headers = cloneHeaders(config.Headers)
	case mcpconfigstore.TransportStdio:
		result.Transport = "stdio"
		result.Executable = config.Executable
		result.Args = append([]string(nil), config.Args...)
		result.Directory = config.Directory
	}
	return result
}

func cloneHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return map[string]string{}
	}
	result := make(map[string]string, len(headers))
	for name, value := range headers {
		result[name] = value
	}
	return result
}

func syncProjection(result catalogapplication.SyncProviderToolsResult) SyncResult {
	return SyncResult{
		Created: result.Created,
		Updated: result.Updated,
		Removed: result.Removed,
		Total:   result.Total,
	}
}

func toolProjection(tool catalogdomain.Tool) (ToolDetail, error) {
	var inputSchema any
	if err := json.Unmarshal(tool.InputSchema().Bytes(), &inputSchema); err != nil {
		return ToolDetail{}, fmt.Errorf("decode input schema: %w", err)
	}
	var outputSchema any
	if schema := tool.OutputSchema(); schema != nil {
		if err := json.Unmarshal(schema.Bytes(), &outputSchema); err != nil {
			return ToolDetail{}, fmt.Errorf("decode output schema: %w", err)
		}
	}
	return ToolDetail{
		ToolID:       tool.ID().String(),
		ProviderID:   tool.ProviderID().String(),
		SourceKey:    tool.SourceKey(),
		Name:         tool.Name(),
		Title:        tool.Title(),
		Description:  tool.Description(),
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
		Tags:         tool.Tags(),
	}, nil
}

func profileProjection(profile profiledomain.Profile) Profile {
	tools := make([]ProfileTool, 0, len(profile.Selections()))
	for _, selection := range profile.Selections() {
		tools = append(tools, ProfileTool{
			ToolID: selection.ToolID.String(),
			Direct: selection.Direct,
		})
	}
	return Profile{
		ProfileID:   profile.ID().String(),
		Slug:        profile.Slug().String(),
		DisplayName: profile.DisplayName(),
		Mode:        string(profile.Mode()),
		Tools:       tools,
	}
}

func auditProjection(record auditdomain.Record) AuditRecord {
	result := AuditRecord{
		InvocationID: record.InvocationID().String(),
		ToolID:       record.ToolID().String(),
		Caller:       record.Caller(),
		StartedAt:    record.StartedAt().Format(timeFormat),
		FinishedAt:   record.FinishedAt().Format(timeFormat),
		PolicyKind:   string(record.PolicyKind()),
		PolicyReason: record.PolicyReason(),
		Approval:     string(record.Approval()),
		Status:       string(record.Status()),
		ResultKind:   string(record.ResultKind()),
		ErrorClass:   string(record.ErrorClass()),
	}
	if providerID := record.ProviderID(); providerID != nil {
		result.ProviderID = providerID.String()
	}
	return result
}

func invocationProjection(
	invocationID string,
	result invocationdomain.Result,
) (InvocationResult, error) {
	projection := InvocationResult{
		InvocationID: invocationID,
		Kind:         string(result.Kind()),
	}
	switch result.Kind() {
	case invocationdomain.ResultKindComplete:
		var content any
		if err := json.Unmarshal(result.Content().Bytes(), &content); err != nil {
			return InvocationResult{}, fmt.Errorf("decode invocation content: %w", err)
		}
		projection.Content = content
	case invocationdomain.ResultKindInputRequired:
		projection.RequestID = result.RequestID()
		projection.Message = result.Message()
		if schema := result.InputSchema(); len(schema) != 0 {
			var inputSchema any
			if err := json.Unmarshal(schema, &inputSchema); err != nil {
				return InvocationResult{}, fmt.Errorf(
					"decode input-required schema: %w",
					err,
				)
			}
			projection.InputSchema = inputSchema
		}
	case invocationdomain.ResultKindTask:
		projection.TaskID = result.TaskID()
		projection.Status = result.TaskStatus()
		projection.Message = result.Message()
	}
	return projection, nil
}
