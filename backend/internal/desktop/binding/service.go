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
	credentialapplication "github.com/orz-i/Mender/backend/internal/credential/application"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	mcpconfigstore "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/configstore"
	policyapplication "github.com/orz-i/Mender/backend/internal/policy/application"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
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

func (s *Service) GetSystemStatus() (SystemStatus, error) {
	status := s.runtime.Status()
	policy, err := s.runtime.Policy.Get.Execute(s.ctx)
	if err != nil {
		return SystemStatus{}, err
	}
	return SystemStatus{
		Ready:       status.Ready,
		Policy:      policyStatus(status, policy),
		Credentials: credentialStatus(status),
		MCP: MCPStatus{
			Stdio:          true,
			StreamableHTTP: true,
			ExposureModes:  []string{"direct", "dynamic", "hybrid"},
		},
	}, nil
}

func (s *Service) GetPolicyStatus() (PolicyStatus, error) {
	policy, err := s.runtime.Policy.Get.Execute(s.ctx)
	if err != nil {
		return PolicyStatus{}, err
	}
	return policyStatus(s.runtime.Status(), policy), nil
}

func (s *Service) GetPolicy() (Policy, error) {
	policy, err := s.runtime.Policy.Get.Execute(s.ctx)
	if err != nil {
		return Policy{}, err
	}
	return policyProjection(policy), nil
}

func (s *Service) SetPolicyDefault(decision string) (Policy, error) {
	policy, err := s.runtime.Policy.SetDefault.Execute(
		s.ctx,
		policydomain.DecisionKind(decision),
	)
	if err != nil {
		return Policy{}, err
	}
	return policyProjection(policy), nil
}

func (s *Service) CreatePolicyRule(input PolicyRuleInput) (PolicyRule, error) {
	ruleInput, err := parsePolicyRuleInput(input)
	if err != nil {
		return PolicyRule{}, err
	}
	rule, err := s.runtime.Policy.CreateRule.Execute(s.ctx, ruleInput)
	if err != nil {
		return PolicyRule{}, err
	}
	return policyRuleProjection(rule), nil
}

func (s *Service) UpdatePolicyRule(
	ruleID string,
	input PolicyRuleInput,
) (PolicyRule, error) {
	id, err := policydomain.ParseRuleID(ruleID)
	if err != nil {
		return PolicyRule{}, err
	}
	ruleInput, err := parsePolicyRuleInput(input)
	if err != nil {
		return PolicyRule{}, err
	}
	rule, err := s.runtime.Policy.UpdateRule.Execute(s.ctx, id, ruleInput)
	if err != nil {
		return PolicyRule{}, err
	}
	return policyRuleProjection(rule), nil
}

func (s *Service) DeletePolicyRule(ruleID string) error {
	id, err := policydomain.ParseRuleID(ruleID)
	if err != nil {
		return err
	}
	return s.runtime.Policy.DeleteRule.Execute(s.ctx, id)
}

func (s *Service) SetPolicyRuleEnabled(
	ruleID string,
	enabled bool,
) (PolicyRule, error) {
	id, err := policydomain.ParseRuleID(ruleID)
	if err != nil {
		return PolicyRule{}, err
	}
	rule, err := s.runtime.Policy.SetRuleEnabled.Execute(s.ctx, id, enabled)
	if err != nil {
		return PolicyRule{}, err
	}
	return policyRuleProjection(rule), nil
}

func (s *Service) GetCredentialStatus() CredentialStatus {
	return credentialStatus(s.runtime.Status())
}

func (s *Service) ListCredentials() (CredentialList, error) {
	items, err := s.runtime.Credentials.List.Execute(s.ctx)
	if err != nil {
		return CredentialList{}, err
	}
	result := make([]Credential, 0, len(items))
	for _, item := range items {
		result = append(result, credentialProjection(item))
	}
	return CredentialList{Credentials: result}, nil
}

func (s *Service) GetCredential(namespace string, name string) (Credential, error) {
	ref, err := credentialdomain.NewReference(namespace, name)
	if err != nil {
		return Credential{}, err
	}
	item, err := s.runtime.Credentials.Get.Execute(s.ctx, ref)
	if err != nil {
		return Credential{}, err
	}
	return credentialProjection(item), nil
}

func (s *Service) CreateNativeCredential(
	namespace string,
	name string,
	displayName string,
	secret string,
) (Credential, error) {
	item, err := s.runtime.Credentials.CreateNative.Execute(
		s.ctx,
		credentialapplication.CreateNativeRequest{
			Namespace:   namespace,
			Name:        name,
			DisplayName: displayName,
			Secret:      []byte(secret),
		},
	)
	if err != nil {
		return Credential{}, err
	}
	return credentialProjection(item), nil
}

func (s *Service) CreateEnvironmentCredential(
	namespace string,
	name string,
	displayName string,
	environmentVariable string,
) (Credential, error) {
	item, err := s.runtime.Credentials.CreateEnvironment.Execute(
		s.ctx,
		credentialapplication.CreateEnvironmentRequest{
			Namespace:           namespace,
			Name:                name,
			DisplayName:         displayName,
			EnvironmentVariable: environmentVariable,
		},
	)
	if err != nil {
		return Credential{}, err
	}
	return credentialProjection(item), nil
}

func (s *Service) RotateNativeCredential(
	namespace string,
	name string,
	secret string,
) (Credential, error) {
	ref, err := credentialdomain.NewReference(namespace, name)
	if err != nil {
		return Credential{}, err
	}
	item, err := s.runtime.Credentials.RotateNative.Execute(s.ctx, ref, []byte(secret))
	if err != nil {
		return Credential{}, err
	}
	return credentialProjection(item), nil
}

func (s *Service) DeleteCredential(namespace string, name string) error {
	ref, err := credentialdomain.NewReference(namespace, name)
	if err != nil {
		return err
	}
	return s.runtime.Credentials.Delete.Execute(s.ctx, ref)
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
	credentialHeaders []CredentialHeader,
) (ProviderMutationResponse, error) {
	if strings.TrimSpace(namespace) == "" ||
		strings.TrimSpace(displayName) == "" ||
		strings.TrimSpace(endpoint) == "" {
		return ProviderMutationResponse{}, fmt.Errorf(
			"namespace, displayName, and endpoint are required",
		)
	}

	parsedCredentialHeaders, err := parseCredentialHeaders(credentialHeaders)
	if err != nil {
		return ProviderMutationResponse{}, err
	}
	provider, err := s.runtime.RegisterMCPProvider(
		s.ctx,
		catalogapplication.RegisterProviderRequest{
			Namespace:   namespace,
			DisplayName: displayName,
			Description: description,
		},
		mcpconfigstore.Config{
			Transport:         mcpconfigstore.TransportHTTP,
			Endpoint:          endpoint,
			Headers:           cloneHeaders(headers),
			CredentialHeaders: parsedCredentialHeaders,
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
	credentialEnvironment []CredentialEnvironment,
) (ProviderMutationResponse, error) {
	if strings.TrimSpace(namespace) == "" ||
		strings.TrimSpace(displayName) == "" ||
		strings.TrimSpace(executable) == "" {
		return ProviderMutationResponse{}, fmt.Errorf(
			"namespace, displayName, and executable are required",
		)
	}

	parsedCredentialEnvironment, err := parseCredentialEnvironment(credentialEnvironment)
	if err != nil {
		return ProviderMutationResponse{}, err
	}
	provider, err := s.runtime.RegisterMCPProvider(
		s.ctx,
		catalogapplication.RegisterProviderRequest{
			Namespace:   namespace,
			DisplayName: displayName,
			Description: description,
		},
		mcpconfigstore.Config{
			Transport:             mcpconfigstore.TransportStdio,
			Executable:            executable,
			Args:                  append([]string(nil), args...),
			Directory:             directory,
			CredentialEnvironment: parsedCredentialEnvironment,
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
	credentialHeaders []CredentialHeader,
	executable string,
	args []string,
	directory string,
	credentialEnvironment []CredentialEnvironment,
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
		parsedCredentialHeaders, parseErr := parseCredentialHeaders(credentialHeaders)
		if parseErr != nil {
			return ProviderMutationResponse{}, parseErr
		}
		config = mcpconfigstore.Config{
			Transport:         mcpconfigstore.TransportHTTP,
			Endpoint:          endpoint,
			Headers:           cloneHeaders(headers),
			CredentialHeaders: parsedCredentialHeaders,
		}
	case string(mcpconfigstore.TransportStdio):
		parsedCredentialEnvironment, parseErr := parseCredentialEnvironment(credentialEnvironment)
		if parseErr != nil {
			return ProviderMutationResponse{}, parseErr
		}
		config = mcpconfigstore.Config{
			Transport:             mcpconfigstore.TransportStdio,
			Executable:            executable,
			Args:                  append([]string(nil), args...),
			Directory:             directory,
			CredentialEnvironment: parsedCredentialEnvironment,
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

func (s *Service) ListMCPServers() MCPServerList {
	instances := s.runtime.ListMCPServers()
	items := make([]MCPServerInstance, 0, len(instances))
	for _, instance := range instances {
		items = append(items, mcpServerProjection(instance))
	}
	return MCPServerList{Servers: items}
}

func (s *Service) StartMCPServer(
	profile string,
	address string,
) (MCPServerInstance, error) {
	slug, err := profiledomain.NewSlug(profile)
	if err != nil {
		return MCPServerInstance{}, err
	}
	instance, err := s.runtime.StartMCPHTTPServer(s.ctx, slug, address)
	if err != nil {
		return MCPServerInstance{}, err
	}
	return mcpServerProjection(instance), nil
}

func (s *Service) RestartMCPServer(
	serverID string,
) (MCPServerInstance, error) {
	instance, err := s.runtime.RestartMCPServer(s.ctx, serverID)
	if err != nil {
		return MCPServerInstance{}, err
	}
	return mcpServerProjection(instance), nil
}

func (s *Service) StopMCPServer(serverID string) error {
	return s.runtime.StopMCPServer(s.ctx, serverID)
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

func (s *Service) SearchTools(
	query string,
	limit int,
	providerID string,
	profile string,
) (ToolList, error) {
	if limit == 0 {
		limit = defaultToolSearchLimit
	}
	var provider *catalogdomain.ProviderID
	if providerID != "" {
		parsed, err := catalogdomain.ParseProviderID(providerID)
		if err != nil {
			return ToolList{}, err
		}
		provider = &parsed
	}
	var profileSlug *profiledomain.Slug
	if profile != "" {
		slug, err := profiledomain.NewSlug(profile)
		if err != nil {
			return ToolList{}, err
		}
		profileSlug = &slug
	}
	results, err := s.runtime.SearchCatalog(
		s.ctx,
		query,
		limit,
		provider,
		profileSlug,
	)
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
	executionContext, err := invocationdomain.NewInvocationContext(
		invocationdomain.OriginDesktop,
		"desktop",
		invocationdomain.CatalogExecutionScope(),
	)
	if err != nil {
		return InvocationResult{}, err
	}
	response, err := s.runtime.CallConfiguredTool(s.ctx, id, payload, executionContext)
	if err != nil {
		return InvocationResult{}, err
	}
	return invocationProjection(response.InvocationID.String(), response.Outcome)
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
	tool ProfileTool,
) (Profile, error) {
	profileSlug, err := profiledomain.NewSlug(slug)
	if err != nil {
		return Profile{}, err
	}
	id, err := catalogdomain.ParseToolID(tool.ToolID)
	if err != nil {
		return Profile{}, err
	}
	credentialRefs, err := parseCredentialReferences(tool.CredentialRefs)
	if err != nil {
		return Profile{}, err
	}
	profile, err := s.runtime.Profiles.AddTool.Execute(
		s.ctx,
		profileapplication.AddToolRequest{
			ProfileSlug:           profileSlug,
			ToolID:                id,
			Direct:                tool.Direct,
			Alias:                 tool.Alias,
			TitleOverride:         tool.TitleOverride,
			DescriptionOverride:   tool.DescriptionOverride,
			PolicyConstraint:      profiledomain.PolicyConstraint(tool.PolicyConstraint),
			ConcurrencyConstraint: profiledomain.ConcurrencyConstraint(tool.ConcurrencyConstraint),
			CredentialRefs:        credentialRefs,
		},
	)
	if err != nil {
		return Profile{}, err
	}
	return profileProjection(profile), nil
}

func (s *Service) ReplaceProfileTools(
	slug string,
	tools []ProfileTool,
) (Profile, error) {
	profileSlug, err := profiledomain.NewSlug(slug)
	if err != nil {
		return Profile{}, err
	}

	bindings := make([]profileapplication.ToolBindingInput, 0, len(tools))
	for _, item := range tools {
		id, err := catalogdomain.ParseToolID(item.ToolID)
		if err != nil {
			return Profile{}, err
		}
		credentialRefs, err := parseCredentialReferences(item.CredentialRefs)
		if err != nil {
			return Profile{}, err
		}
		bindings = append(bindings, profileapplication.ToolBindingInput{
			ToolID:                id,
			Direct:                item.Direct,
			Alias:                 item.Alias,
			TitleOverride:         item.TitleOverride,
			DescriptionOverride:   item.DescriptionOverride,
			PolicyConstraint:      profiledomain.PolicyConstraint(item.PolicyConstraint),
			ConcurrencyConstraint: profiledomain.ConcurrencyConstraint(item.ConcurrencyConstraint),
			CredentialRefs:        credentialRefs,
		})
	}

	profile, err := s.runtime.Profiles.ReplaceTools.Execute(
		s.ctx,
		profileapplication.ReplaceToolsRequest{
			ProfileSlug: profileSlug,
			Tools:       bindings,
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

func policyStatus(
	status bootstrap.RuntimeStatus,
	policy policydomain.PolicySet,
) PolicyStatus {
	enabled := 0
	for _, rule := range policy.Rules() {
		if rule.Enabled() {
			enabled++
		}
	}
	return PolicyStatus{
		Mode:             status.PolicyMode,
		Editable:         true,
		Approval:         status.ApprovalMode,
		DefaultDecision:  string(policy.DefaultDecision()),
		RuleCount:        len(policy.Rules()),
		EnabledRuleCount: enabled,
	}
}

func parsePolicyRuleInput(input PolicyRuleInput) (policyapplication.RuleInput, error) {
	resources := make([]policydomain.ResourceMatcher, 0, len(input.Match.ResourcesAny))
	for _, item := range input.Match.ResourcesAny {
		resource, err := policydomain.NewResourceMatcher(
			item.Kind,
			policydomain.ResourceOperator(item.Operator),
			item.Value,
		)
		if err != nil {
			return policyapplication.RuleInput{}, err
		}
		resources = append(resources, resource)
	}

	origins := make([]policydomain.Origin, 0, len(input.Match.Origins))
	for _, value := range input.Match.Origins {
		origins = append(origins, policydomain.Origin(value))
	}
	scopeKinds := make([]policydomain.ScopeKind, 0, len(input.Match.ScopeKinds))
	for _, value := range input.Match.ScopeKinds {
		scopeKinds = append(scopeKinds, policydomain.ScopeKind(value))
	}

	return policyapplication.RuleInput{
		DisplayName: input.DisplayName,
		Enabled:     input.Enabled,
		Decision:    policydomain.DecisionKind(input.Decision),
		Match: policydomain.MatchSpec{
			Origins:       origins,
			ScopeKinds:    scopeKinds,
			ProfileRefs:   append([]string(nil), input.Match.ProfileRefs...),
			ProviderIDs:   append([]string(nil), input.Match.ProviderIDs...),
			ToolIDs:       append([]string(nil), input.Match.ToolIDs...),
			FactKnowledge: policydomain.FactKnowledge(input.Match.FactKnowledge),
			EffectsAny:    append([]string(nil), input.Match.EffectsAny...),
			ResourcesAny:  resources,
		},
	}, nil
}

func policyProjection(policy policydomain.PolicySet) Policy {
	rules := make([]PolicyRule, 0, len(policy.Rules()))
	for _, rule := range policy.Rules() {
		rules = append(rules, policyRuleProjection(rule))
	}
	return Policy{
		DefaultDecision: string(policy.DefaultDecision()),
		Rules:           rules,
	}
}

func policyRuleProjection(rule policydomain.Rule) PolicyRule {
	match := rule.Match()
	origins := make([]string, 0, len(match.Origins()))
	for _, value := range match.Origins() {
		origins = append(origins, string(value))
	}
	scopeKinds := make([]string, 0, len(match.ScopeKinds()))
	for _, value := range match.ScopeKinds() {
		scopeKinds = append(scopeKinds, string(value))
	}
	resources := make([]PolicyResourceMatcher, 0, len(match.ResourcesAny()))
	for _, resource := range match.ResourcesAny() {
		resources = append(resources, PolicyResourceMatcher{
			Kind:     resource.Kind(),
			Operator: string(resource.Operator()),
			Value:    resource.Value(),
		})
	}
	return PolicyRule{
		RuleID:      rule.ID().String(),
		DisplayName: rule.DisplayName(),
		Enabled:     rule.Enabled(),
		Decision:    string(rule.Decision()),
		Match: PolicyMatch{
			Origins:       origins,
			ScopeKinds:    scopeKinds,
			ProfileRefs:   match.ProfileRefs(),
			ProviderIDs:   match.ProviderIDs(),
			ToolIDs:       match.ToolIDs(),
			FactKnowledge: string(match.FactKnowledge()),
			EffectsAny:    match.EffectsAny(),
			ResourcesAny:  resources,
		},
		CreatedAt: rule.CreatedAt().Format(timeFormat),
		UpdatedAt: rule.UpdatedAt().Format(timeFormat),
	}
}

func credentialStatus(status bootstrap.RuntimeStatus) CredentialStatus {
	return CredentialStatus{
		Resolver:        status.CredentialResolver,
		PersistentStore: status.CredentialPersistent,
		Editable:        true,
	}
}

func credentialProjection(item credentialdomain.Credential) Credential {
	return Credential{
		Reference:           item.Reference().String(),
		Namespace:           item.Reference().Namespace(),
		Name:                item.Reference().Name(),
		DisplayName:         item.DisplayName(),
		Source:              string(item.Source()),
		EnvironmentVariable: item.EnvironmentVariable(),
		CreatedAt:           item.CreatedAt().Format(timeFormat),
		UpdatedAt:           item.UpdatedAt().Format(timeFormat),
	}
}

func parseCredentialHeaders(items []CredentialHeader) ([]mcpconfigstore.CredentialHeader, error) {
	result := make([]mcpconfigstore.CredentialHeader, 0, len(items))
	for _, item := range items {
		ref, err := credentialdomain.ParseReference(item.CredentialRef)
		if err != nil {
			return nil, err
		}
		result = append(result, mcpconfigstore.CredentialHeader{
			Name:      item.Name,
			Scheme:    item.Scheme,
			Reference: ref,
		})
	}
	return result, nil
}

func parseCredentialEnvironment(items []CredentialEnvironment) ([]mcpconfigstore.CredentialEnvironment, error) {
	result := make([]mcpconfigstore.CredentialEnvironment, 0, len(items))
	for _, item := range items {
		ref, err := credentialdomain.ParseReference(item.CredentialRef)
		if err != nil {
			return nil, err
		}
		result = append(result, mcpconfigstore.CredentialEnvironment{
			Name:      item.Name,
			Reference: ref,
		})
	}
	return result, nil
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
		result.CredentialHeaders = make([]CredentialHeader, 0, len(config.CredentialHeaders))
		for _, binding := range config.CredentialHeaders {
			result.CredentialHeaders = append(result.CredentialHeaders, CredentialHeader{
				Name:          binding.Name,
				Scheme:        binding.Scheme,
				CredentialRef: binding.Reference.String(),
			})
		}
	case mcpconfigstore.TransportStdio:
		result.Transport = "stdio"
		result.Executable = config.Executable
		result.Args = append([]string(nil), config.Args...)
		result.Directory = config.Directory
		result.CredentialEnvironment = make([]CredentialEnvironment, 0, len(config.CredentialEnvironment))
		for _, binding := range config.CredentialEnvironment {
			result.CredentialEnvironment = append(result.CredentialEnvironment, CredentialEnvironment{
				Name:          binding.Name,
				CredentialRef: binding.Reference.String(),
			})
		}
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

func mcpServerProjection(
	instance bootstrap.MCPServerInstance,
) MCPServerInstance {
	return MCPServerInstance{
		ServerID:  instance.ServerID,
		Profile:   instance.Profile,
		Transport: instance.Transport,
		Address:   instance.Address,
		Endpoint:  instance.Endpoint,
		Status:    instance.Status,
		StartedAt: instance.StartedAt.Format(timeFormat),
	}
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

func profileProjection(profile profiledomain.Profile) Profile {
	tools := make([]ProfileTool, 0, len(profile.Bindings()))
	for _, binding := range profile.Bindings() {
		tools = append(tools, ProfileTool{
			ToolID:                binding.ToolID().String(),
			Direct:                binding.Direct(),
			Alias:                 binding.Alias(),
			TitleOverride:         binding.TitleOverride(),
			DescriptionOverride:   binding.DescriptionOverride(),
			PolicyConstraint:      string(binding.PolicyConstraint()),
			ConcurrencyConstraint: string(binding.ConcurrencyConstraint()),
			CredentialRefs:        credentialReferenceStrings(binding.CredentialRefs()),
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
		InvocationID:   record.InvocationID().String(),
		ToolID:         record.ToolID().String(),
		Caller:         record.Caller(),
		StartedAt:      record.StartedAt().Format(timeFormat),
		FinishedAt:     record.FinishedAt().Format(timeFormat),
		PolicyKind:     string(record.PolicyKind()),
		PolicyReason:   record.PolicyReason(),
		Approval:       string(record.Approval()),
		Status:         string(record.Status()),
		ResultKind:     string(record.ResultKind()),
		ErrorClass:     string(record.ErrorClass()),
		ErrorSummary:   record.ErrorSummary(),
		Origin:         string(record.Origin()),
		ScopeKind:      string(record.ScopeKind()),
		ScopeReference: record.ScopeReference(),
		OutcomeKind:    string(record.OutcomeKind()),
		RejectionCode:  string(record.RejectionCode()),
		FailureSource:  string(record.FailureSource()),
		FailureCode:    string(record.FailureCode()),
	}
	if providerID := record.ProviderID(); providerID != nil {
		result.ProviderID = providerID.String()
	}
	return result
}

func invocationProjection(
	invocationID string,
	result invocationdomain.Outcome,
) (InvocationResult, error) {
	projection := InvocationResult{
		InvocationID: invocationID,
		Kind:         string(result.Kind()),
	}
	switch result.Kind() {
	case invocationdomain.OutcomeKindComplete:
		var content any
		if err := json.Unmarshal(result.Content().Bytes(), &content); err != nil {
			return InvocationResult{}, fmt.Errorf("decode invocation content: %w", err)
		}
		projection.Content = content
	case invocationdomain.OutcomeKindInputRequired:
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
	case invocationdomain.OutcomeKindTask:
		projection.TaskID = result.TaskID()
		projection.Status = result.TaskStatus()
		projection.Message = result.Message()
	case invocationdomain.OutcomeKindRejected:
		projection.Code = string(result.RejectionCode())
		projection.Message = result.Message()
	case invocationdomain.OutcomeKindFailed:
		projection.Source = string(result.FailureSource())
		projection.Code = string(result.FailureCode())
		projection.Message = result.Message()
	}
	return projection, nil
}
