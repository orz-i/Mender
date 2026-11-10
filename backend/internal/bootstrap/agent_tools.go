package bootstrap

import (
	"context"
	"fmt"
	"time"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	agentdelegation "github.com/orz-i/Mender/backend/internal/agent/infrastructure/delegationprovider"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	catalogidentity "github.com/orz-i/Mender/backend/internal/catalog/infrastructure/identity"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
)

// AgentTool describes one canonical Agent-owned Tool, not a Provider facade.
type AgentTool struct {
	ToolID    string `json:"toolId"`
	SourceKey string `json:"sourceKey"`
	Name      string `json:"name"`
	Title     string `json:"title"`
}

// AgentTools is a read-only projection of Agent as Tool operations.
type AgentTools struct {
	SuiteID string      `json:"suiteId"`
	Status  string      `json:"status"`
	Tools   []AgentTool `json:"tools"`
}

type ownedAgentCatalog interface {
	ListSourceTools(context.Context, catalogdomain.SourceRef) ([]catalogdomain.Tool, error)
	UpsertSourceSnapshot(context.Context, catalogdomain.SourceRef, []catalogdomain.Tool) error
}

// AgentTools resolves the exact AgentID's catalog identity; an Agent is never
// wrapped in an internal Provider to satisfy the Tool catalog.
func (r *Runtime) AgentTools(ctx context.Context, id agentdomain.ID, caller gov.Context) (AgentTools, error) {
	if _, err := r.Agents.Get(ctx, id, caller); err != nil {
		return AgentTools{}, err
	}
	result := AgentTools{SuiteID: "agent.delegation", Status: "missing", Tools: []AgentTool{}}
	store, ok := r.Catalog.store.(ownedAgentCatalog)
	if !ok {
		return result, agentdomain.ErrUnavailable
	}
	source, err := catalogdomain.AgentSource(string(id))
	if err != nil {
		return result, err
	}
	tools, err := store.ListSourceTools(ctx, source)
	if err != nil {
		return result, err
	}
	seen := make(map[string]bool)
	for _, tool := range tools {
		result.Tools = append(result.Tools, AgentTool{
			ToolID: tool.ID().String(), SourceKey: tool.SourceKey(),
			Name: tool.Name(), Title: tool.Title(),
		})
		seen[tool.SourceKey()] = true
	}
	if len(tools) == 0 {
		return result, nil
	}
	result.Status = "ready"
	for _, key := range []string{"agent.delegate", "agent.status", "agent.result", "agent.cancel", "agent.resume"} {
		if !seen[key] {
			result.Status = "incomplete"
			break
		}
	}
	if len(tools) != 5 {
		result.Status = "conflict"
	}
	return result, nil
}

// EnsureAgentTools publishes only this exact configured Agent's verified
// operation set. Neither a Provider nor any Profile grants are created.
func (r *Runtime) EnsureAgentTools(ctx context.Context, id agentdomain.ID, caller gov.Context) (AgentTools, error) {
	agent, err := r.Agents.Get(ctx, id, caller)
	if err != nil {
		return AgentTools{}, err
	}
	store, ok := r.Catalog.store.(ownedAgentCatalog)
	if !ok {
		return AgentTools{}, agentdomain.ErrUnavailable
	}
	source, err := catalogdomain.AgentSource(string(id))
	if err != nil {
		return AgentTools{}, err
	}
	existing, err := store.ListSourceTools(ctx, source)
	if err != nil {
		return AgentTools{}, err
	}
	byKey := make(map[string]catalogdomain.Tool, len(existing))
	for _, tool := range existing {
		byKey[tool.SourceKey()] = tool
	}
	definitions, err := agentdelegation.ReviewedOwnedTools(agent)
	if err != nil {
		return AgentTools{}, err
	}
	now := time.Now().UTC()
	ids := catalogidentity.NewGenerator()
	planned := make([]catalogdomain.Tool, 0, len(definitions))
	for _, def := range definitions {
		input, e := catalogdomain.NewSchema(def.InputSchema)
		if e != nil {
			return AgentTools{}, e
		}
		var output *catalogdomain.Schema
		if len(def.OutputSchema) != 0 {
			value, e := catalogdomain.NewSchema(def.OutputSchema)
			if e != nil {
				return AgentTools{}, e
			}
			output = &value
		}
		spec := catalogdomain.ToolSpec{
			SourceKey: def.SourceKey, Name: def.Name, Title: def.Title,
			Description: def.Description, InputSchema: input, OutputSchema: output,
			Tags: def.Tags,
		}
		var item catalogdomain.Tool
		if current, found := byKey[def.SourceKey]; found {
			item, e = current.RefreshMetadata(spec, now)
		} else {
			newID, idErr := ids.NewToolID()
			if idErr != nil {
				return AgentTools{}, idErr
			}
			item, e = catalogdomain.NewToolFromSource(newID, source, spec, now)
		}
		if e != nil {
			return AgentTools{}, e
		}
		planned = append(planned, item)
	}
	if err = store.UpsertSourceSnapshot(ctx, source, planned); err != nil {
		return AgentTools{}, fmt.Errorf("register Agent owned Tool IDs: %w", err)
	}
	owned, err := agentdelegation.NewOwnedExecutor(r.Agents, id)
	if err != nil {
		return AgentTools{}, err
	}
	if err = r.executors.RegisterSource(source, owned); err != nil {
		return AgentTools{}, err
	}
	return r.AgentTools(ctx, id, caller)
}

// CreateAgentWithTools is the existing public owner action; it now creates
// Agent-owned Catalog ToolIDs rather than an internal Provider facade.
func (r *Runtime) CreateAgentWithTools(ctx context.Context, request agentapplication.CreateRequest, caller gov.Context) (agentdomain.Agent, error) {
	agent, err := r.Agents.Create(ctx, request, caller)
	if err != nil {
		return agent, err
	}
	if _, err = r.EnsureAgentTools(ctx, agent.ID(), caller); err != nil {
		return agent, fmt.Errorf("agent %s created but Tool registration is incomplete: %w", agent.ID(), err)
	}
	return agent, nil
}

// DeleteAgentWithTools retires an unreferenced Agent and its canonical ToolIDs
// under one SQLite transaction; no Provider façade or blanket grant cleanup.
func (r *Runtime) DeleteAgentWithTools(ctx context.Context, id agentdomain.ID, caller gov.Context) error {
	if _, err := r.Agents.Get(ctx, id, caller); err != nil {
		return err
	}
	source, err := catalogdomain.AgentSource(string(id))
	if err != nil {
		return err
	}
	store, ok := r.Catalog.store.(interface {
		RetireOwnedAgent(context.Context, catalogdomain.SourceRef) error
	})
	if !ok {
		return agentdomain.ErrUnavailable
	}
	if err = store.RetireOwnedAgent(ctx, source); err != nil {
		return fmt.Errorf("delete Agent with canonical Tools: %w", err)
	}
	if err = r.executors.UnregisterSource(source); err != nil {
		return fmt.Errorf("delete Agent: unregister retired executor: %w", err)
	}
	return nil
}
