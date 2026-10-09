package binding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	auditdomain "github.com/orz-i/Mender/backend/internal/audit/domain"
	"github.com/orz-i/Mender/backend/internal/bootstrap"
	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	credentialapplication "github.com/orz-i/Mender/backend/internal/credential/application"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	mcpconfigstore "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/configstore"
	mcpplugin "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/plugin"
	policyapplication "github.com/orz-i/Mender/backend/internal/policy/application"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	installationdomain "github.com/orz-i/Mender/backend/internal/providerplugin/installation/domain"
	runtimegovernancedomain "github.com/orz-i/Mender/backend/internal/runtimegovernance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sandboxdaytona "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/daytona"
	sandboxdocker "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/dockerengine"
	skillapplication "github.com/orz-i/Mender/backend/internal/skill/application"
	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
	tunnelapplication "github.com/orz-i/Mender/backend/internal/tunnel/application"
	tunneldomain "github.com/orz-i/Mender/backend/internal/tunnel/domain"
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

func (s *Service) ListSandboxes() (SandboxList, error) {
	items, err := s.runtime.Sandboxes.List(s.ctx)
	if err != nil {
		return SandboxList{}, err
	}
	results := make([]Sandbox, 0, len(items))
	for _, item := range items {
		projected, err := s.projectSandbox(item)
		if err != nil {
			return SandboxList{}, err
		}
		results = append(results, projected)
	}
	return SandboxList{Sandboxes: results}, nil
}
func (s *Service) GetSandboxActivity(sandboxID string, limit int) (SandboxActivity, error) {
	id, err := sandboxdomain.ParseID(sandboxID)
	if err != nil {
		return SandboxActivity{}, err
	}
	activity, err := s.runtime.Sandboxes.Activity(s.ctx, id, limit)
	if err != nil {
		return SandboxActivity{}, err
	}
	projected := make([]SandboxActivityEvent, 0, len(activity.Events))
	for _, event := range activity.Events {
		projected = append(projected, SandboxActivityEvent{
			Kind: event.Kind, Action: event.Action, Status: event.Status,
			OccurredAt:    event.OccurredAt.UTC().Format(time.RFC3339Nano),
			PrincipalKind: event.PrincipalKind, Origin: event.Origin,
			PolicyDecision: event.PolicyDecision, Generation: event.Generation,
			PreviousState: event.PreviousState, NextState: event.NextState,
		})
	}
	return SandboxActivity{SandboxID: activity.SandboxID, Events: projected, Limit: activity.Limit}, nil
}

func (s *Service) GetSandbox(sandboxID string) (Sandbox, error) {
	id, err := sandboxdomain.ParseID(sandboxID)
	if err != nil {
		return Sandbox{}, err
	}
	item, err := s.runtime.Sandboxes.Get(s.ctx, id)
	if err != nil {
		return Sandbox{}, err
	}
	return s.projectSandbox(item)
}
func (s *Service) CreateLocalSandbox(input CreateLocalSandboxInput) (Sandbox, error) {
	caller, err := s.governanceContext()
	if err != nil {
		return Sandbox{}, err
	}
	item, err := s.runtime.Sandboxes.CreateLocalSandbox(s.ctx, bootstrap.CreateLocalSandboxRequest{Name: input.Name, Root: input.Root, Isolation: sandboxdomain.IsolationClass(input.Isolation)}, caller)
	if err != nil {
		return Sandbox{}, err
	}
	return s.projectSandbox(item)
}
func (s *Service) StartSandbox(sandboxID string) (Sandbox, error) {
	return s.changeLocalSandboxLifecycle(sandboxID, governancedomain.ActionSandboxStart)
}
func (s *Service) StopSandbox(sandboxID string) (Sandbox, error) {
	return s.changeLocalSandboxLifecycle(sandboxID, governancedomain.ActionSandboxStop)
}
func (s *Service) DestroySandbox(sandboxID string) (Sandbox, error) {
	return s.changeLocalSandboxLifecycle(sandboxID, governancedomain.ActionSandboxDestroy)
}
func (s *Service) changeLocalSandboxLifecycle(rawID string, action governancedomain.Action) (Sandbox, error) {
	id, err := sandboxdomain.ParseID(rawID)
	if err != nil {
		return Sandbox{}, err
	}
	caller, err := s.governanceContext()
	if err != nil {
		return Sandbox{}, err
	}
	if _, err := s.runtime.Sandboxes.ChangeLifecycle(s.ctx, id, action, caller); err != nil {
		return Sandbox{}, err
	}
	item, err := s.runtime.Sandboxes.Get(s.ctx, id)
	if err != nil {
		return Sandbox{}, err
	}
	return s.projectSandbox(item)
}

func (s *Service) GetSandboxBackends() (DockerBackendStatus, error) {
	return DockerBackendStatus{DockerAvailable: s.runtime.Sandboxes.DockerAvailable(), DockerStatus: s.runtime.Sandboxes.DockerStatus(), DaytonaConfigured: s.runtime.Sandboxes.DaytonaConfigured(), DaytonaStatus: "Credential Reference and live remote availability are verified during creation"}, nil
}
func (s *Service) CreateDaytonaSandbox(input CreateDaytonaSandboxInput) (Sandbox, error) {
	ref, err := credentialdomain.ParseReference(input.CredentialRef)
	if err != nil {
		return Sandbox{}, err
	}
	caller, err := s.governanceContext()
	if err != nil {
		return Sandbox{}, err
	}
	item, err := s.runtime.Sandboxes.CreateDaytonaSandbox(s.ctx, bootstrap.CreateDaytonaSandboxRequest{Name: input.Name, Template: sandboxdaytona.Template{CredentialRef: ref, ImageDigest: input.ImageDigest, Target: input.Target, CPU: input.CPU, MemoryGiB: input.MemoryGiB, DiskGiB: input.DiskGiB, TTLMinutes: input.TTLMinutes}}, caller)
	if err != nil && item.ID() == "" {
		return Sandbox{}, err
	}
	return s.projectSandbox(item)
}
func (s *Service) ReconcileDaytonaSandbox(raw string) (Sandbox, error) {
	id, err := sandboxdomain.ParseID(raw)
	if err != nil {
		return Sandbox{}, err
	}
	caller, err := s.governanceContext()
	if err != nil {
		return Sandbox{}, err
	}
	if _, err := s.runtime.Sandboxes.ReconcileDaytona(s.ctx, id, caller); err != nil {
		return Sandbox{}, err
	}
	item, err := s.runtime.Sandboxes.Get(s.ctx, id)
	if err != nil {
		return Sandbox{}, err
	}
	return s.projectSandbox(item)
}
func (s *Service) CreateDockerSandbox(input CreateDockerSandboxInput) (Sandbox, error) {
	caller, err := s.governanceContext()
	if err != nil {
		return Sandbox{}, err
	}
	item, err := s.runtime.Sandboxes.CreateDockerSandbox(s.ctx, bootstrap.CreateDockerSandboxRequest{Name: input.Name, Template: sandboxdocker.Template{ImageDigest: input.ImageDigest, UserID: input.UserID, GroupID: input.GroupID, CPUmilli: input.CPUmilli, MemoryMiB: input.MemoryMiB, WorkspaceMiB: input.WorkspaceMiB, PIDs: input.PIDs}}, caller)
	if err != nil && item.ID() == "" {
		return Sandbox{}, err
	}
	// Persisted uncertain runs are projected for operator review, with no retry.
	return s.projectSandbox(item)
}
func (s *Service) ReconcileDockerSandbox(raw string) (Sandbox, error) {
	id, err := sandboxdomain.ParseID(raw)
	if err != nil {
		return Sandbox{}, err
	}
	caller, err := s.governanceContext()
	if err != nil {
		return Sandbox{}, err
	}
	if _, err := s.runtime.Sandboxes.ReconcileDocker(s.ctx, id, caller); err != nil {
		return Sandbox{}, err
	}
	item, err := s.runtime.Sandboxes.Get(s.ctx, id)
	if err != nil {
		return Sandbox{}, err
	}
	return s.projectSandbox(item)
}

func (s *Service) projectSandbox(item sandboxdomain.Sandbox) (Sandbox, error) {
	spec := item.Spec()
	state := "defined"
	run, err := s.runtime.Sandboxes.GetRun(s.ctx, item.ID())
	if err == nil {
		state = string(run.State)
	} else if !errors.Is(err, sandboxdomain.ErrNotFound) {
		return Sandbox{}, err
	}
	active, err := s.runtime.Sandboxes.ActiveLeases(s.ctx, item.ID())
	if err != nil {
		return Sandbox{}, err
	}
	return Sandbox{SandboxID: spec.ID.String(), Name: spec.Name, Backend: string(spec.Backend), Isolation: string(spec.Isolation), State: state, LocalRoot: spec.LocalRoot, OwnerKind: spec.OwnerKind, OwnerRef: spec.OwnerRef, ActiveLeases: active, BackendAvailable: s.runtime.Sandboxes.BackendAvailable(item)}, nil
}

func (s *Service) GetRuntimeGovernance() (RuntimeGovernanceStatus, error) {
	budget, err := s.runtime.RuntimeGovernance.GetBudget.Execute(s.ctx)
	if err != nil {
		return RuntimeGovernanceStatus{}, err
	}
	isolation, err := s.runtime.RuntimeGovernance.GetIsolation.Execute(s.ctx)
	if err != nil {
		return RuntimeGovernanceStatus{}, err
	}
	return runtimeGovernanceStatus(s.runtime, budget, isolation), nil
}

func (s *Service) UpdateRuntimeBudget(input RuntimeBudget) (RuntimeGovernanceStatus, error) {
	budget, err := s.runtime.RuntimeGovernance.UpdateBudget.Execute(
		s.ctx,
		runtimegovernancedomain.BudgetSpec{
			MaxInvocations:          input.MaxInvocations,
			MaxPrincipalInvocations: input.MaxPrincipalInvocations,
			MaxProviderInvocations:  input.MaxProviderInvocations,
			MaxToolInvocations:      input.MaxToolInvocations,
			MaxProviderActivations:  input.MaxProviderActivations,
			MaxExecutionTimeoutMS:   input.MaxExecutionTimeoutMS,
			MaxArgumentBytes:        input.MaxArgumentBytes,
			MaxResultBytes:          input.MaxResultBytes,
		},
	)
	if err != nil {
		return RuntimeGovernanceStatus{}, err
	}
	isolation, err := s.runtime.RuntimeGovernance.GetIsolation.Execute(s.ctx)
	if err != nil {
		return RuntimeGovernanceStatus{}, err
	}
	return runtimeGovernanceStatus(s.runtime, budget, isolation), nil
}

func (s *Service) UpdateRuntimeIsolation(input RuntimeIsolation) (RuntimeGovernanceStatus, error) {
	isolation, err := s.runtime.RuntimeGovernance.UpdateIsolation(
		s.ctx,
		runtimegovernancedomain.IsolationSpec{
			WorkspaceShell:   runtimegovernancedomain.IsolationMode(input.WorkspaceShell),
			ExternalProvider: runtimegovernancedomain.IsolationMode(input.ExternalProvider),
			TunnelClient:     runtimegovernancedomain.IsolationMode(input.TunnelClient),
		},
	)
	if err != nil {
		return RuntimeGovernanceStatus{}, err
	}
	budget, err := s.runtime.RuntimeGovernance.GetBudget.Execute(s.ctx)
	if err != nil {
		return RuntimeGovernanceStatus{}, err
	}
	return runtimeGovernanceStatus(s.runtime, budget, isolation), nil
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

func (s *Service) ListProviderPlugins() (ProviderPluginList, error) {
	catalog, err := s.runtime.ListAvailableProviderPluginCatalog(s.ctx)
	if err != nil {
		return ProviderPluginList{}, err
	}
	items := make([]ProviderPlugin, 0, len(catalog.Plugins))
	for _, item := range catalog.Plugins {
		projected, projectionErr := availableProviderPluginProjection(item)
		if projectionErr != nil {
			return ProviderPluginList{}, projectionErr
		}
		items = append(items, projected)
	}
	issues := make([]ProviderPluginIssue, 0, len(catalog.Issues))
	for _, issue := range catalog.Issues {
		issues = append(issues, ProviderPluginIssue{
			InstallationID: issue.InstallationID.String(),
			Code:           issue.Code,
			Message:        issue.Message,
		})
	}
	return ProviderPluginList{Plugins: items, Issues: issues}, nil
}

func (s *Service) ListProviderPluginInstallations() (
	ProviderPluginInstallationList,
	error,
) {
	items, err := s.runtime.ListProviderPluginInstallations(s.ctx)
	if err != nil {
		return ProviderPluginInstallationList{}, err
	}
	result := make([]ProviderPluginInstallation, 0, len(items))
	for _, item := range items {
		result = append(result, providerPluginInstallationProjection(item))
	}
	return ProviderPluginInstallationList{Installations: result}, nil
}

func (s *Service) InstallProviderPlugin(
	sourcePath string,
) (ProviderPluginInstallation, error) {
	item, err := s.runtime.InstallProviderPlugin(s.ctx, sourcePath)
	if err != nil {
		return ProviderPluginInstallation{}, err
	}
	return providerPluginInstallationProjection(item), nil
}

func (s *Service) EnableProviderPluginInstallation(
	installationID string,
) (ProviderPluginInstallation, error) {
	return s.setProviderPluginInstallationStatus(
		installationID,
		installationdomain.StatusEnabled,
	)
}

func (s *Service) DisableProviderPluginInstallation(
	installationID string,
) (ProviderPluginInstallation, error) {
	return s.setProviderPluginInstallationStatus(
		installationID,
		installationdomain.StatusDisabled,
	)
}

func (s *Service) setProviderPluginInstallationStatus(
	rawID string,
	status installationdomain.Status,
) (ProviderPluginInstallation, error) {
	id, err := providerplugin.ParseInstallationID(rawID)
	if err != nil {
		return ProviderPluginInstallation{}, err
	}
	item, err := s.runtime.SetProviderPluginInstallationStatus(
		s.ctx,
		id,
		status,
	)
	if err != nil {
		return ProviderPluginInstallation{}, err
	}
	return providerPluginInstallationProjection(item), nil
}

func (s *Service) DeleteProviderPluginInstallation(
	installationID string,
) error {
	id, err := providerplugin.ParseInstallationID(installationID)
	if err != nil {
		return err
	}
	return s.runtime.RemoveProviderPluginInstallation(s.ctx, id)
}

func (s *Service) ListSkills() (SkillInstallationList, error) {
	items, err := s.runtime.Skills.List(s.ctx)
	if err != nil {
		return SkillInstallationList{}, err
	}
	result := make([]SkillInstallation, 0, len(items))
	for _, item := range items {
		result = append(result, skillInstallationProjection(item))
	}
	return SkillInstallationList{Installations: result}, nil
}

func (s *Service) GetSkillInstallation(
	installationID string,
) (SkillInspection, error) {
	id, err := skilldomain.ParseInstallationID(installationID)
	if err != nil {
		return SkillInspection{}, err
	}
	item, err := s.runtime.Skills.Inspect(s.ctx, id)
	if err != nil {
		return SkillInspection{}, err
	}
	return skillInspectionProjection(item), nil
}

func (s *Service) InstallSkill(
	sourcePath string,
) (SkillInstallation, error) {
	item, err := s.runtime.Skills.InstallLocal(s.ctx, sourcePath)
	if err != nil {
		return SkillInstallation{}, err
	}
	return managedSkillInstallationProjection(item), nil
}

func (s *Service) TrustSkillInstallation(
	installationID string,
) (SkillInstallation, error) {
	return s.setSkillTrust(installationID, true)
}

func (s *Service) UntrustSkillInstallation(
	installationID string,
) (SkillInstallation, error) {
	return s.setSkillTrust(installationID, false)
}

func (s *Service) setSkillTrust(
	rawID string,
	trusted bool,
) (SkillInstallation, error) {
	id, err := skilldomain.ParseInstallationID(rawID)
	if err != nil {
		return SkillInstallation{}, err
	}
	item, err := s.runtime.Skills.SetTrust(s.ctx, id, trusted)
	if err != nil {
		return SkillInstallation{}, err
	}
	return managedSkillInstallationProjection(item), nil
}

func (s *Service) EnableSkillInstallation(
	installationID string,
) (SkillInstallation, error) {
	return s.setSkillEnabled(installationID, true)
}

func (s *Service) DisableSkillInstallation(
	installationID string,
) (SkillInstallation, error) {
	return s.setSkillEnabled(installationID, false)
}

func (s *Service) setSkillEnabled(
	rawID string,
	enabled bool,
) (SkillInstallation, error) {
	id, err := skilldomain.ParseInstallationID(rawID)
	if err != nil {
		return SkillInstallation{}, err
	}
	item, err := s.runtime.Skills.SetEnabled(s.ctx, id, enabled)
	if err != nil {
		return SkillInstallation{}, err
	}
	return managedSkillInstallationProjection(item), nil
}

func (s *Service) DeleteSkillInstallation(
	installationID string,
) error {
	id, err := skilldomain.ParseInstallationID(installationID)
	if err != nil {
		return err
	}
	return s.runtime.Skills.Remove(s.ctx, id)
}

func (s *Service) ListSkillDeployments() (SkillDeploymentList, error) {
	items, err := s.runtime.Skills.ListDeployments(s.ctx)
	if err != nil {
		return SkillDeploymentList{}, err
	}
	result := make([]SkillDeployment, 0, len(items))
	for _, item := range items {
		result = append(result, skillDeploymentProjection(item))
	}
	return SkillDeploymentList{Deployments: result}, nil
}

func (s *Service) DeploySkill(
	installationID string,
	targetValue string,
	projectRoot string,
) (SkillDeployment, error) {
	id, err := skilldomain.ParseInstallationID(installationID)
	if err != nil {
		return SkillDeployment{}, err
	}
	target := skilldomain.DeploymentTarget(strings.TrimSpace(targetValue))
	if err := target.Validate(); err != nil {
		return SkillDeployment{}, err
	}

	root := strings.TrimSpace(projectRoot)
	if target.ProjectScoped() {
		if root == "" {
			return SkillDeployment{}, fmt.Errorf("project root is required for target %s", target)
		}
		root, err = filepath.Abs(root)
		if err != nil {
			return SkillDeployment{}, fmt.Errorf("resolve project root: %w", err)
		}
	} else {
		if root != "" {
			return SkillDeployment{}, fmt.Errorf("project root is only valid for project-scoped Skill targets")
		}
		root, err = os.UserHomeDir()
		if err != nil {
			return SkillDeployment{}, fmt.Errorf("resolve user home: %w", err)
		}
		root, err = filepath.Abs(root)
		if err != nil {
			return SkillDeployment{}, fmt.Errorf("resolve user home path: %w", err)
		}
	}

	item, err := s.runtime.Skills.Deploy(s.ctx, id, target, root)
	if err != nil {
		return SkillDeployment{}, err
	}
	return skillDeploymentProjection(skillapplication.DeploymentInspection{
		Deployment:    item,
		Drift:         skilldomain.DeploymentCurrent,
		CurrentDigest: item.Digest(),
	}), nil
}

func (s *Service) UndeploySkill(deploymentID string) error {
	id, err := skilldomain.ParseDeploymentID(deploymentID)
	if err != nil {
		return err
	}
	return s.runtime.Skills.Undeploy(s.ctx, id)
}

func (s *Service) ListProviders() (ProviderList, error) {
	providers, err := s.runtime.Catalog.ListProviders.Execute(s.ctx)
	if err != nil {
		return ProviderList{}, err
	}
	configs, err := s.runtime.ListProviderConfigs(s.ctx)
	if err != nil {
		return ProviderList{}, err
	}
	configByProvider := make(
		map[catalogdomain.ProviderID]providerplugin.Configuration,
		len(configs),
	)
	for _, config := range configs {
		configByProvider[config.ProviderID] = config
	}
	toolCounts, err := s.runtime.Catalog.ListProviderToolCounts.Execute(s.ctx)
	if err != nil {
		return ProviderList{}, err
	}
	items := make([]Provider, 0, len(providers))
	for _, provider := range providers {
		config, configured := configByProvider[provider.ID()]
		var configPtr *providerplugin.Configuration
		var descriptorPtr *providerplugin.Descriptor
		if configured {
			configCopy := config
			configPtr = &configCopy
			descriptorPtr = desktopDescriptorForProviderConfig(
				s.ctx,
				s.runtime,
				config,
			)
		}
		items = append(items, providerProjection(
			provider,
			configPtr,
			descriptorPtr,
			toolCounts[provider.ID()],
		))
	}
	return ProviderList{Providers: items}, nil
}

func (s *Service) CreateProvider(
	namespace string,
	displayName string,
	description string,
	pluginID string,
	installationID string,
	config map[string]any,
) (ProviderMutationResponse, error) {
	id, err := providerplugin.ParseID(pluginID)
	if err != nil {
		return ProviderMutationResponse{}, err
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return ProviderMutationResponse{}, fmt.Errorf("encode provider config: %w", err)
	}
	registerRequest := catalogapplication.RegisterProviderRequest{
		Namespace:   namespace,
		DisplayName: displayName,
		Description: description,
	}
	var provider catalogdomain.Provider
	if strings.TrimSpace(installationID) == "" {
		provider, err = s.registerConfiguredProvider(
			s.ctx,
			registerRequest,
			id,
			raw,
		)
	} else {
		parsedInstallationID, parseErr := providerplugin.ParseInstallationID(
			installationID,
		)
		if parseErr != nil {
			return ProviderMutationResponse{}, parseErr
		}
		provider, err = s.registerInstalledProvider(
			s.ctx,
			registerRequest,
			id,
			parsedInstallationID,
			raw,
		)
	}
	if err != nil {
		return ProviderMutationResponse{}, err
	}
	result, err := s.syncConfiguredProvider(s.ctx, provider.ID())
	if err != nil {
		return ProviderMutationResponse{}, err
	}
	stored, err := s.runtime.ProviderConfig(s.ctx, provider.ID())
	if err != nil {
		return ProviderMutationResponse{}, err
	}
	descriptor := desktopDescriptorForProviderConfig(s.ctx, s.runtime, stored)
	return ProviderMutationResponse{
		Provider: providerProjection(provider, &stored, descriptor, result.Total),
		Sync:     syncProjection(result),
	}, nil
}

func (s *Service) RebindProvider(
	providerID string,
	installationID string,
	config map[string]any,
) (ProviderMutationResponse, error) {
	id, err := catalogdomain.ParseProviderID(providerID)
	if err != nil {
		return ProviderMutationResponse{}, err
	}
	parsedInstallationID, err := providerplugin.ParseInstallationID(installationID)
	if err != nil {
		return ProviderMutationResponse{}, err
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return ProviderMutationResponse{}, fmt.Errorf("encode provider config: %w", err)
	}
	result, err := s.rebindProviderInstallation(
		s.ctx,
		id,
		parsedInstallationID,
		raw,
	)
	if err != nil {
		return ProviderMutationResponse{}, err
	}
	provider, err := s.runtime.Catalog.GetProvider.Execute(s.ctx, id)
	if err != nil {
		return ProviderMutationResponse{}, err
	}
	stored, err := s.runtime.ProviderConfig(s.ctx, id)
	if err != nil {
		return ProviderMutationResponse{}, err
	}
	descriptor := desktopDescriptorForProviderConfig(s.ctx, s.runtime, stored)
	return ProviderMutationResponse{
		Provider: providerProjection(provider, &stored, descriptor, result.Total),
		Sync:     syncProjection(result),
	}, nil
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
	result, err := s.syncConfiguredProvider(s.ctx, provider.ID())
	if err != nil {
		return ProviderMutationResponse{}, err
	}
	config, err := s.runtime.ProviderConfig(s.ctx, provider.ID())
	if err != nil {
		return ProviderMutationResponse{}, err
	}
	descriptor := desktopDescriptorForProviderConfig(s.ctx, s.runtime, config)
	return ProviderMutationResponse{
		Provider: providerProjection(provider, &config, descriptor, result.Total),
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
	result, err := s.syncConfiguredProvider(s.ctx, provider.ID())
	if err != nil {
		return ProviderMutationResponse{}, err
	}
	config, err := s.runtime.ProviderConfig(s.ctx, provider.ID())
	if err != nil {
		return ProviderMutationResponse{}, err
	}
	descriptor := desktopDescriptorForProviderConfig(s.ctx, s.runtime, config)
	return ProviderMutationResponse{
		Provider: providerProjection(provider, &config, descriptor, result.Total),
		Sync:     syncProjection(result),
	}, nil
}

func (s *Service) UpdateConfiguredProvider(
	providerID string,
	displayName string,
	description string,
	config map[string]any,
) (ProviderMutationResponse, error) {
	id, err := catalogdomain.ParseProviderID(providerID)
	if err != nil {
		return ProviderMutationResponse{}, err
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return ProviderMutationResponse{}, fmt.Errorf("encode provider config: %w", err)
	}
	provider, err := s.updateConfiguredProvider(
		s.ctx,
		id,
		catalogapplication.UpdateProviderRequest{
			DisplayName: displayName,
			Description: description,
		},
		raw,
	)
	if err != nil {
		return ProviderMutationResponse{}, err
	}
	result, err := s.syncConfiguredProvider(s.ctx, id)
	if err != nil {
		return ProviderMutationResponse{}, err
	}
	stored, err := s.runtime.ProviderConfig(s.ctx, id)
	if err != nil {
		return ProviderMutationResponse{}, err
	}
	descriptor := desktopDescriptorForProviderConfig(s.ctx, s.runtime, stored)
	return ProviderMutationResponse{
		Provider: providerProjection(provider, &stored, descriptor, result.Total),
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
	result, err := s.syncConfiguredProvider(s.ctx, id)
	if err != nil {
		return ProviderMutationResponse{}, err
	}
	stored, err := s.runtime.ProviderConfig(s.ctx, id)
	if err != nil {
		return ProviderMutationResponse{}, err
	}
	descriptor := desktopDescriptorForProviderConfig(s.ctx, s.runtime, stored)

	return ProviderMutationResponse{
		Provider: providerProjection(provider, &stored, descriptor, result.Total),
		Sync:     syncProjection(result),
	}, nil
}

func (s *Service) SyncProvider(providerID string) (SyncResult, error) {
	id, err := catalogdomain.ParseProviderID(providerID)
	if err != nil {
		return SyncResult{}, err
	}
	result, err := s.syncConfiguredProvider(s.ctx, id)
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
	instance, err := s.startMCPHTTPServer(s.ctx, slug, address)
	if err != nil {
		return MCPServerInstance{}, err
	}
	return mcpServerProjection(instance), nil
}

func (s *Service) RestartMCPServer(
	serverID string,
) (MCPServerInstance, error) {
	instance, err := s.restartMCPServer(s.ctx, serverID)
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
	executionContext, err := governancedomain.NewContext(
		governancedomain.OriginDesktop,
		s.runtime.OwnerPrincipal(),
		governancedomain.CatalogScope(),
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

func (s *Service) SetProfileTunnel(slug string, tunnelID string) (Profile, error) {
	profileSlug, err := profiledomain.NewSlug(slug)
	if err != nil {
		return Profile{}, err
	}
	var id tunneldomain.TunnelID
	if tunnelID != "" {
		id, err = tunneldomain.ParseTunnelID(tunnelID)
		if err != nil {
			return Profile{}, err
		}
	}
	profile, err := s.runtime.Profiles.SetTunnel.Execute(
		s.ctx,
		profileapplication.SetTunnelRequest{
			ProfileSlug: profileSlug,
			TunnelID:    id,
		},
	)
	if err != nil {
		return Profile{}, err
	}
	return profileProjection(profile), nil
}

func (s *Service) ListTunnels() (TunnelList, error) {
	items, err := s.runtime.Tunnels.List.Execute(s.ctx)
	if err != nil {
		return TunnelList{}, err
	}
	tunnels := make([]Tunnel, 0, len(items))
	for _, item := range items {
		tunnels = append(tunnels, tunnelProjection(item))
	}
	return TunnelList{Tunnels: tunnels}, nil
}

func (s *Service) CreateOpenAITunnel(
	displayName string,
	externalTunnelID string,
	runtimeCredentialRef string,
) (Tunnel, error) {
	ref, err := credentialdomain.ParseReference(runtimeCredentialRef)
	if err != nil {
		return Tunnel{}, err
	}
	item, err := s.runtime.Tunnels.Create.Execute(
		s.ctx,
		tunnelapplication.CreateOpenAITunnelRequest{
			DisplayName:          displayName,
			ExternalTunnelID:     externalTunnelID,
			RuntimeCredentialRef: ref,
		},
	)
	if err != nil {
		return Tunnel{}, err
	}
	return tunnelProjection(item), nil
}

func (s *Service) DeleteTunnel(tunnelID string) error {
	id, err := tunneldomain.ParseTunnelID(tunnelID)
	if err != nil {
		return err
	}
	return s.runtime.Tunnels.Delete.Execute(s.ctx, id)
}

func (s *Service) ListProviderActivationAudit(limit int) (ProviderActivationAuditList, error) {
	if limit == 0 {
		limit = defaultAuditLimit
	}
	records, err := s.runtime.ListProviderActivationAudit.Execute(s.ctx, limit)
	if err != nil {
		return ProviderActivationAuditList{}, err
	}
	items := make([]ProviderActivationAuditRecord, 0, len(records))
	for _, record := range records {
		items = append(items, providerActivationAuditProjection(record))
	}
	return ProviderActivationAuditList{Records: items}, nil
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

	actions := make([]governancedomain.Action, 0, len(input.Match.Actions))
	for _, value := range input.Match.Actions {
		actions = append(actions, governancedomain.Action(value))
	}
	origins := make([]governancedomain.Origin, 0, len(input.Match.Origins))
	for _, value := range input.Match.Origins {
		origins = append(origins, governancedomain.Origin(value))
	}
	principalKinds := make([]governancedomain.PrincipalKind, 0, len(input.Match.PrincipalKinds))
	for _, value := range input.Match.PrincipalKinds {
		principalKinds = append(principalKinds, governancedomain.PrincipalKind(value))
	}
	scopeKinds := make([]governancedomain.ScopeKind, 0, len(input.Match.ScopeKinds))
	for _, value := range input.Match.ScopeKinds {
		scopeKinds = append(scopeKinds, governancedomain.ScopeKind(value))
	}

	return policyapplication.RuleInput{
		DisplayName: input.DisplayName,
		Enabled:     input.Enabled,
		Decision:    policydomain.DecisionKind(input.Decision),
		Match: policydomain.MatchSpec{
			Actions:        actions,
			Origins:        origins,
			PrincipalKinds: principalKinds,
			PrincipalRefs:  append([]string(nil), input.Match.PrincipalRefs...),
			ScopeKinds:     scopeKinds,
			ProfileRefs:    append([]string(nil), input.Match.ProfileRefs...),
			ProviderIDs:    append([]string(nil), input.Match.ProviderIDs...),
			ToolIDs:        append([]string(nil), input.Match.ToolIDs...),
			FactKnowledge:  policydomain.FactKnowledge(input.Match.FactKnowledge),
			EffectsAny:     append([]string(nil), input.Match.EffectsAny...),
			ResourcesAny:   resources,
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
	actions := make([]string, 0, len(match.Actions()))
	for _, value := range match.Actions() {
		actions = append(actions, string(value))
	}
	origins := make([]string, 0, len(match.Origins()))
	for _, value := range match.Origins() {
		origins = append(origins, string(value))
	}
	principalKinds := make([]string, 0, len(match.PrincipalKinds()))
	for _, value := range match.PrincipalKinds() {
		principalKinds = append(principalKinds, string(value))
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
			Actions:        actions,
			Origins:        origins,
			PrincipalKinds: principalKinds,
			PrincipalRefs:  match.PrincipalRefs(),
			ScopeKinds:     scopeKinds,
			ProfileRefs:    match.ProfileRefs(),
			ProviderIDs:    match.ProviderIDs(),
			ToolIDs:        match.ToolIDs(),
			FactKnowledge:  string(match.FactKnowledge()),
			EffectsAny:     match.EffectsAny(),
			ResourcesAny:   resources,
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

func availableProviderPluginProjection(
	item bootstrap.AvailableProviderPlugin,
) (ProviderPlugin, error) {
	descriptor := item.Descriptor
	result := ProviderPlugin{
		PluginID:        descriptor.ID.String(),
		Version:         descriptor.Version,
		PackageVersion:  descriptor.Version,
		DisplayName:     descriptor.DisplayName,
		Description:     descriptor.Description,
		ConfigVersion:   descriptor.ConfigVersion,
		SourceKind:      item.SourceKind,
		ProtocolVersion: descriptor.ProtocolVersion,
		Capabilities:    append([]string{}, descriptor.Capabilities...),
		Enabled:         item.Enabled,
	}
	if item.InstallationID != nil {
		result.InstallationID = item.InstallationID.String()
	}
	if len(descriptor.ConfigSchema) != 0 {
		if err := json.Unmarshal(descriptor.ConfigSchema, &result.ConfigSchema); err != nil {
			return ProviderPlugin{}, fmt.Errorf(
				"project Provider plugin %s config schema: %w",
				descriptor.ID,
				err,
			)
		}
	}
	if len(descriptor.UISchema) != 0 {
		if err := json.Unmarshal(descriptor.UISchema, &result.UISchema); err != nil {
			return ProviderPlugin{}, fmt.Errorf(
				"project Provider plugin %s UI schema: %w",
				descriptor.ID,
				err,
			)
		}
	}
	return result, nil
}

func providerPluginInstallationProjection(
	item installationdomain.Installation,
) ProviderPluginInstallation {
	result := ProviderPluginInstallation{
		InstallationID: item.ID().String(),
		PluginID:       item.PluginID().String(),
		PackageVersion: item.PackageVersion(),
		SourceKind:     string(item.SourceKind()),
		Digest:         item.Digest(),
		Status:         string(item.Status()),
		InstalledAt:    item.InstalledAt().Format(timeFormat),
	}
	if descriptor, ok := item.VerifiedDescriptor(); ok {
		result.DisplayName = descriptor.DisplayName
		result.Description = descriptor.Description
		result.ConfigVersion = descriptor.ConfigVersion
		result.ProtocolVersion = descriptor.ProtocolVersion
		result.Capabilities = append([]string(nil), descriptor.Capabilities...)
	}
	return result
}

func skillInstallationProjection(
	item bootstrap.SkillInstallation,
) SkillInstallation {
	result := SkillInstallation{
		SkillID:         item.SkillID.String(),
		InstallationID:  item.InstallationID.String(),
		Name:            item.Name,
		Description:     item.Description,
		SourceKind:      item.SourceKind,
		SourceReference: item.SourceReference,
		Digest:          item.Digest,
		VersionHint:     item.VersionHint,
		Trusted:         item.Trusted,
		Enabled:         item.Enabled,
		BuiltIn:         item.BuiltIn,
	}
	if item.InstalledAt != nil {
		result.InstalledAt = item.InstalledAt.Format(timeFormat)
	}
	return result
}

func managedSkillInstallationProjection(
	item skilldomain.Installation,
) SkillInstallation {
	return SkillInstallation{
		SkillID:         item.SkillID().String(),
		InstallationID:  item.ID().String(),
		Name:            item.Name(),
		Description:     item.Description(),
		SourceKind:      string(item.SourceKind()),
		SourceReference: item.SourceReference(),
		Digest:          item.Digest(),
		VersionHint:     item.VersionHint(),
		Trusted:         item.Trusted(),
		Enabled:         item.Enabled(),
		BuiltIn:         false,
		InstalledAt:     item.InstalledAt().Format(timeFormat),
	}
}

func skillInspectionProjection(
	item bootstrap.SkillInspection,
) SkillInspection {
	resources := make([]SkillResource, 0, len(item.Resources))
	for _, resource := range item.Resources {
		resources = append(resources, SkillResource{
			Path:       resource.Path,
			Kind:       string(resource.Kind),
			Size:       resource.Size,
			SHA256:     resource.SHA256,
			Executable: resource.Executable,
		})
	}
	return SkillInspection{
		Installation: skillInstallationProjection(item.Installation),
		Metadata: SkillMetadata{
			Name:          item.Metadata.Name,
			Description:   item.Metadata.Description,
			License:       item.Metadata.License,
			Compatibility: item.Metadata.Compatibility,
			Metadata:      item.Metadata.Values,
			AllowedTools:  item.Metadata.AllowedTools,
		},
		Resources:  resources,
		TotalBytes: item.TotalBytes,
	}
}

func skillDeploymentProjection(
	item skillapplication.DeploymentInspection,
) SkillDeployment {
	deployment := item.Deployment
	return SkillDeployment{
		DeploymentID:   deployment.ID().String(),
		InstallationID: deployment.InstallationID().String(),
		Target:         string(deployment.Target()),
		Root:           deployment.Root(),
		Destination:    deployment.Destination(),
		Digest:         deployment.Digest(),
		DeployedAt:     deployment.DeployedAt().Format(timeFormat),
		Drift:          string(item.Drift),
		CurrentDigest:  item.CurrentDigest,
	}
}

func providerProjection(
	provider catalogdomain.Provider,
	config *providerplugin.Configuration,
	descriptor *providerplugin.Descriptor,
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
	if config == nil {
		return result
	}

	result.PluginID = config.PluginID.String()
	if config.InstallationID != nil {
		result.InstallationID = config.InstallationID.String()
	}
	result.ConfigVersion = config.ConfigVersion
	if metadata, ok := config.ConfigMetadata(); ok {
		result.ConnectionSummary = metadata.ConnectionSummary
	}
	if descriptor != nil {
		result.PluginVersion = descriptor.Version
		result.PluginDisplayName = descriptor.DisplayName
	}
	if safeConfig, err := providerplugin.ManagementSafeConfig(descriptor, config.Config); err == nil {
		if object, ok := safeConfig.(map[string]any); ok {
			result.Config = object
		}
	}

	if config.PluginID != mcpplugin.ID {
		return result
	}
	mcpConfig, err := mcpplugin.DecodeConfig(config.Config)
	if err != nil {
		return result
	}
	switch mcpConfig.Transport.Kind {
	case mcpplugin.TransportStreamableHTTP:
		result.Transport = "http"
		result.Endpoint = mcpConfig.Transport.Endpoint
		result.Headers = cloneHeaders(mcpConfig.Transport.Headers)
		result.CredentialHeaders = make(
			[]CredentialHeader,
			0,
			len(mcpConfig.Transport.CredentialHeaders),
		)
		for _, binding := range mcpConfig.Transport.CredentialHeaders {
			result.CredentialHeaders = append(
				result.CredentialHeaders,
				CredentialHeader{
					Name:          binding.Name,
					Scheme:        binding.Scheme,
					CredentialRef: binding.Reference.String(),
				},
			)
		}
	case mcpplugin.TransportStdio:
		result.Transport = "stdio"
		result.Executable = mcpConfig.Transport.Executable
		result.Args = append([]string(nil), mcpConfig.Transport.Args...)
		result.Directory = mcpConfig.Transport.Directory
		result.CredentialEnvironment = make(
			[]CredentialEnvironment,
			0,
			len(mcpConfig.Transport.CredentialEnvironment),
		)
		for _, binding := range mcpConfig.Transport.CredentialEnvironment {
			result.CredentialEnvironment = append(
				result.CredentialEnvironment,
				CredentialEnvironment{
					Name:          binding.Name,
					CredentialRef: binding.Reference.String(),
				},
			)
		}
	}
	return result
}

func desktopDescriptorForProviderConfig(
	ctx context.Context,
	runtime *bootstrap.Runtime,
	config providerplugin.Configuration,
) *providerplugin.Descriptor {
	descriptor, ok, err := runtime.ProviderPluginDescriptorForConfig(ctx, config)
	if err != nil || !ok {
		return nil
	}
	return &descriptor
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
		ServerID:          instance.ServerID,
		Profile:           instance.Profile,
		Transport:         instance.Transport,
		Address:           instance.Address,
		Endpoint:          instance.Endpoint,
		Status:            instance.Status,
		StartedAt:         instance.StartedAt.Format(timeFormat),
		TunnelID:          instance.TunnelID,
		TunnelDisplayName: instance.TunnelDisplayName,
		TunnelProvider:    instance.TunnelProvider,
		TunnelExternalID:  instance.TunnelExternalID,
		TunnelStatus:      instance.TunnelStatus,
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
	result := Profile{
		ProfileID:   profile.ID().String(),
		Slug:        profile.Slug().String(),
		DisplayName: profile.DisplayName(),
		Mode:        string(profile.Mode()),
		Tools:       tools,
	}
	if tunnelID, ok := profile.TunnelID(); ok {
		result.TunnelID = tunnelID.String()
	}
	return result
}

func runtimeGovernanceStatus(
	runtime *bootstrap.Runtime,
	budget runtimegovernancedomain.Budget,
	isolation runtimegovernancedomain.Isolation,
) RuntimeGovernanceStatus {
	spec := budget.Spec()
	backend := runtime.RuntimeGovernance.IsolationBackend()
	usage := runtime.RuntimeGovernance.BudgetUsage()
	return RuntimeGovernanceStatus{
		Budget: RuntimeBudget{
			MaxInvocations:          spec.MaxInvocations,
			MaxPrincipalInvocations: spec.MaxPrincipalInvocations,
			MaxProviderInvocations:  spec.MaxProviderInvocations,
			MaxToolInvocations:      spec.MaxToolInvocations,
			MaxProviderActivations:  spec.MaxProviderActivations,
			MaxExecutionTimeoutMS:   spec.MaxExecutionTimeoutMS,
			MaxArgumentBytes:        spec.MaxArgumentBytes,
			MaxResultBytes:          spec.MaxResultBytes,
		},
		Isolation: RuntimeIsolation{
			WorkspaceShell:   string(isolation.WorkspaceShell()),
			ExternalProvider: string(isolation.ExternalProvider()),
			TunnelClient:     string(isolation.TunnelClient()),
		},
		Backend: RuntimeBackend{
			Name:                    backend.Name,
			RestrictedHostAvailable: backend.RestrictedHostAvailable,
		},
		Usage: RuntimeUsage{
			Invocations: usage.Invocations,
			Activations: usage.Activations,
		},
	}
}

func tunnelProjection(item tunneldomain.Tunnel) Tunnel {
	return Tunnel{
		TunnelID:             item.ID().String(),
		DisplayName:          item.DisplayName(),
		Provider:             string(item.Provider()),
		ExternalTunnelID:     item.ExternalTunnelID(),
		RuntimeCredentialRef: item.RuntimeCredentialRef().String(),
	}
}

func providerActivationAuditProjection(
	record auditdomain.ProviderActivationRecord,
) ProviderActivationAuditRecord {
	return ProviderActivationAuditRecord{
		ActivationID:   record.ActivationID(),
		ProviderID:     record.ProviderID().String(),
		Action:         "provider.activate",
		PrincipalKind:  record.PrincipalKind(),
		PrincipalRef:   record.PrincipalRef(),
		Origin:         string(record.Origin()),
		ScopeKind:      string(record.ScopeKind()),
		ScopeReference: record.ScopeReference(),
		StartedAt:      record.StartedAt().Format(timeFormat),
		FinishedAt:     record.FinishedAt().Format(timeFormat),
		PolicyKind:     string(record.PolicyKind()),
		PolicyReason:   record.PolicyReason(),
		Approval:       string(record.Approval()),
		Status:         string(record.Status()),
		ErrorClass:     string(record.ErrorClass()),
		ErrorSummary:   record.ErrorSummary(),
	}
}

func auditProjection(record auditdomain.Record) AuditRecord {
	result := AuditRecord{
		InvocationID:   record.InvocationID().String(),
		ToolID:         record.ToolID().String(),
		PrincipalKind:  record.PrincipalKind(),
		PrincipalRef:   record.PrincipalRef(),
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
