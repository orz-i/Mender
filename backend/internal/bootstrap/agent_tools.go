package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	agentdelegation "github.com/orz-i/Mender/backend/internal/agent/infrastructure/delegationprovider"
	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
)

// AgentTool describes one canonical Tool, never an executable implementation.
type AgentTool struct {
	ToolID    string `json:"toolId"`
	SourceKey string `json:"sourceKey"`
	Name      string `json:"name"`
	Title     string `json:"title"`
}

// AgentTools is a side-effect-free description of one Agent's tool suite.
type AgentTools struct {
	SuiteID      string      `json:"suiteId"`
	Status       string      `json:"status"` // missing, ready, incomplete, legacy, conflict
	ProviderID   string      `json:"providerId,omitempty"`
	ProviderName string      `json:"providerName,omitempty"`
	Tools        []AgentTool `json:"tools"`
}

func (r *Runtime) AgentTools(ctx context.Context, id agentdomain.ID, caller gov.Context) (AgentTools, error) {
	if _, err := r.Agents.Get(ctx, id, caller); err != nil {
		return AgentTools{}, err
	}
	result := AgentTools{SuiteID: "agent.delegation", Status: "missing", Tools: []AgentTool{}}
	configs, err := r.providerConfigs.List(ctx)
	if err != nil {
		return result, err
	}
	var matches []catalogdomain.ProviderID
	for _, config := range configs {
		if config.PluginID != agentdelegation.ID || config.InstallationID != nil {
			continue
		}
		var binding struct {
			AgentID string `json:"agentId"`
		}
		if json.Unmarshal(config.Config, &binding) != nil {
			continue
		}
		if binding.AgentID == string(id) {
			matches = append(matches, config.ProviderID)
		}
	}
	pinned, err := r.agentStore.GetToolFacade(ctx, id)
	if err != nil {
		return result, err
	}
	if len(matches) > 1 || (pinned != "" && (len(matches) == 0 || pinned != matches[0].String())) {
		result.Status = "conflict"
		return result, nil
	}
	if len(matches) == 0 {
		return result, nil
	}
	providerID := matches[0]
	provider, err := r.Catalog.GetProvider.Execute(ctx, providerID)
	if err != nil {
		result.Status = "conflict"
		return result, nil
	}
	result.ProviderID = providerID.String()
	result.ProviderName = provider.DisplayName()
	tools, err := r.Catalog.store.ListProviderTools(ctx, providerID)
	if err != nil {
		return result, err
	}
	found := map[string]bool{}
	for _, tool := range tools {
		result.Tools = append(result.Tools, AgentTool{ToolID: tool.ID().String(),
			SourceKey: tool.SourceKey(), Name: tool.Name(), Title: tool.Title()})
		found[tool.SourceKey()] = true
	}
	if pinned == "" {
		result.Status = "legacy"
	} else {
		result.Status = "ready"
		for _, source := range []string{"agent.delegate", "agent.status", "agent.result", "agent.cancel"} {
			if !found[source] {
				result.Status = "incomplete"
				break
			}
		}
	}
	return result, nil
}

func agentFacadeNamespace(name string, id agentdomain.ID) string {
	var slug strings.Builder
	previousDash := false
	for _, ch := range strings.ToLower(name) {
		switch {
		case ch >= 'a' && ch <= 'z', ch >= '0' && ch <= '9':
			slug.WriteRune(ch)
			previousDash = false
		case (unicode.IsSpace(ch) || ch == '-' || ch == '_') && slug.Len() > 0 && !previousDash:
			slug.WriteByte('-')
			previousDash = true
		}
		if slug.Len() >= 20 {
			break
		}
	}
	prefix := strings.Trim(slug.String(), "-")
	if prefix == "" {
		prefix = "agent"
	}
	hex := strings.TrimPrefix(string(id), "agent_")
	return "agent-" + prefix + "-" + hex[len(hex)-16:]
}

// EnsureAgentTools explicitly creates or adopts one resource-owned facade.
// No Profile membership, AgentGrant or Policy rule is mutated.
func (r *Runtime) EnsureAgentTools(ctx context.Context, id agentdomain.ID, caller gov.Context) (AgentTools, error) {
	agent, err := r.Agents.Get(ctx, id, caller)
	if err != nil {
		return AgentTools{}, err
	}
	state, err := r.AgentTools(ctx, id, caller)
	if err != nil {
		return AgentTools{}, err
	}
	if state.Status == "conflict" {
		return state, fmt.Errorf("%w: Agent has multiple or mismatched delegation Providers", agentdomain.ErrConflict)
	}
	if state.Status == "ready" {
		// Explicit owner action also refreshes schema of an already-ready suite.
		// Phase 40 reasoningEffort is a new optional field on the same ToolID,
		// so the existing Profile ToolBindings and AgentGrant must remain stable.
		providerID, err := catalogdomain.ParseProviderID(state.ProviderID)
		if err != nil {
			return state, err
		}
		if _, err = r.syncConfiguredProviderForOwner(ctx, providerID, caller, true); err != nil {
			return state, fmt.Errorf("refresh managed Agent tool schema: %w", err)
		}
		return r.AgentTools(ctx, id, caller)
	}
	if state.ProviderID == "" {
		raw, err := json.Marshal(map[string]string{"agentId": string(id)})
		if err != nil {
			return state, err
		}
		provider, err := r.registerConfiguredProvider(ctx, catalogapplication.RegisterProviderRequest{
			Namespace:   agentFacadeNamespace(agent.Spec().Name, id),
			DisplayName: agent.Spec().Name,
			Description: "Managed Agent Delegation tools for " + agent.Spec().Name,
		}, agentdelegation.ID, raw, caller, true)
		if err != nil {
			// A concurrent create may have won the unique namespace. Retry
			// only the metadata lookup, never the side-effecting create.
			current, lookupErr := r.AgentTools(ctx, id, caller)
			if lookupErr == nil && current.ProviderID != "" && current.Status != "conflict" {
				state = current
			} else {
				return state, fmt.Errorf("create Agent tool facade: %w", err)
			}
		} else {
			state.ProviderID = provider.ID().String()
		}
	}
	providerID, err := catalogdomain.ParseProviderID(state.ProviderID)
	if err != nil {
		return state, err
	}
	if _, err = r.syncConfiguredProviderForOwner(ctx, providerID, caller, true); err != nil {
		return state, fmt.Errorf("synchronize Agent tool facade: %w", err)
	}
	if err = r.agentStore.ClaimToolFacade(ctx, id, providerID.String()); err != nil {
		return state, fmt.Errorf("claim Agent tool facade: %w", err)
	}
	return r.AgentTools(ctx, id, caller)
}

// CreateAgentWithTools is the single user-facing explicit creation path for
// HTTP, Wails and CLI. Tool activation failure is recoverable and does not
// conceal an Agent record already persisted.
func (r *Runtime) CreateAgentWithTools(ctx context.Context, req agentapplication.CreateRequest, caller gov.Context) (agentdomain.Agent, error) {
	agent, err := r.Agents.Create(ctx, req, caller)
	if err != nil {
		return agent, err
	}
	if _, err = r.EnsureAgentTools(ctx, agent.ID(), caller); err != nil {
		return agent, fmt.Errorf("agent %s created but tools are not ready; repair from agent details: %w", agent.ID(), err)
	}
	return agent, nil
}

// DeleteAgentWithTools refuses to delete an in-use Provider/Agent. The hosted
// facade must be explicitly retired; no Profile bindings are silently removed.
func (r *Runtime) DeleteAgentWithTools(ctx context.Context, id agentdomain.ID, caller gov.Context) error {
	if _, err := r.Agents.Get(ctx, id, caller); err != nil {
		return err
	}
	state, err := r.AgentTools(ctx, id, caller)
	if err != nil {
		return err
	}
	if state.Status == "conflict" || state.Status == "legacy" {
		return fmt.Errorf("%w: Agent tool associations need explicit resolution", agentdomain.ErrConflict)
	}
	tasks, err := r.Agents.Tasks(ctx, id, caller)
	if err != nil {
		return err
	}
	if len(tasks) > 0 {
		return fmt.Errorf("%w: Agent has task history", agentdomain.ErrConflict)
	}
	if state.ProviderID != "" {
		providerID, parseErr := catalogdomain.ParseProviderID(state.ProviderID)
		if parseErr != nil {
			return parseErr
		}
		if err = r.ensureProviderUnused(ctx, providerID); err != nil {
			return err
		}
		if err = r.agentStore.ReleaseToolFacade(ctx, id, state.ProviderID); err != nil {
			return err
		}
		if err = r.DeleteConfiguredProvider(ctx, providerID); err != nil {
			restoreErr := r.agentStore.ClaimToolFacade(context.WithoutCancel(ctx), id, state.ProviderID)
			return errors.Join(err, restoreErr)
		}
	}
	return r.Agents.Delete(ctx, id, caller)
}
