package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

type sessionOption struct {
	ID           string          `json:"id"`
	Category     string          `json:"category"`
	Type         string          `json:"type"`
	CurrentValue string          `json:"currentValue"`
	Options      json.RawMessage `json:"options"`
}
type sessionOptions struct {
	ConfigOptions []sessionOption `json:"configOptions"`
}
type selectEntry struct {
	Value   string `json:"value"`
	Name    string `json:"name"`
	Options []struct {
		Value string `json:"value"`
		Name  string `json:"name"`
	} `json:"options"`
}

func selectCategory(o sessionOption, kind string) bool {
	if o.Type != "select" {
		return false
	}
	switch kind {
	case "model":
		return o.Category == "model" || o.ID == "model"
	case "reasoning_effort":
		return o.Category == "thought_level" || o.ID == "reasoning_effort"
	default:
		return false
	}
}
func choicesFor(raw json.RawMessage, kind string) (*agentapplication.OptionChoices, error) {
	var opts sessionOptions
	if err := json.Unmarshal(raw, &opts); err != nil || len(opts.ConfigOptions) > 32 {
		return nil, agentdomain.ErrUnavailable
	}
	var group *agentapplication.OptionChoices
	for _, opt := range opts.ConfigOptions {
		if !selectCategory(opt, kind) {
			continue
		}
		if group != nil {
			return nil, agentdomain.ErrUnavailable
		}
		var entries []selectEntry
		if err := json.Unmarshal(opt.Options, &entries); err != nil || len(entries) > 64 {
			return nil, agentdomain.ErrUnavailable
		}
		g := agentapplication.OptionChoices{Current: opt.CurrentValue, Choices: []agentapplication.ModelChoice{}}
		seen := map[string]bool{}
		add := func(value, name string) error {
			if value == "" || seen[value] {
				return nil
			}
			if kind == "model" {
				if err := agentdomain.ValidateModel(value); err != nil {
					return agentdomain.ErrUnavailable
				}
			} else if err := agentdomain.ValidateReasoningEffort(value); err != nil {
				return agentdomain.ErrUnavailable
			}
			if len(g.Choices) >= 64 {
				return agentdomain.ErrUnavailable
			}
			if len(name) > 120 || name == "" {
				name = value
			}
			g.Choices = append(g.Choices, agentapplication.ModelChoice{ID: value, Name: name})
			seen[value] = true
			return nil
		}
		for _, entry := range entries {
			if err := add(entry.Value, entry.Name); err != nil {
				return nil, err
			}
			for _, nested := range entry.Options {
				if err := add(nested.Value, nested.Name); err != nil {
					return nil, err
				}
			}
		}
		if len(g.Choices) == 0 {
			return nil, agentdomain.ErrUnavailable
		}
		if g.Current != "" && !seen[g.Current] {
			return nil, agentdomain.ErrUnavailable
		}
		group = &g
	}
	return group, nil
}

// ReadModelChoices projects bounded ACP v1 session configuration metadata.
// The optional thought-level group is never invented for unsupported agents.
func ReadModelChoices(raw json.RawMessage) (agentapplication.ModelChoices, error) {
	model, err := choicesFor(raw, "model")
	if err != nil {
		return agentapplication.ModelChoices{}, err
	}
	effort, err := choicesFor(raw, "reasoning_effort")
	if err != nil {
		return agentapplication.ModelChoices{}, err
	}
	if model == nil && effort == nil {
		return agentapplication.ModelChoices{}, agentdomain.ErrUnavailable
	}
	result := agentapplication.ModelChoices{Models: []agentapplication.ModelChoice{}, ReasoningEffort: effort}
	if model != nil {
		result.Current = model.Current
		result.Models = model.Choices
	}
	return result, nil
}
func (r Runner) InspectModels(ctx context.Context, agent agentdomain.Agent) (agentapplication.ModelChoices, error) {
	var result agentapplication.ModelChoices
	_, err := r.withSession(ctx, agent, agentapplication.RunConfig{WorkspaceRoot: agent.Spec().WorkspaceRoot},
		func(_ *stream, _ string, session json.RawMessage, _ <-chan error) (string, error) {
			choices, err := ReadModelChoices(session)
			if err != nil {
				return "", err
			}
			result = choices
			return "", nil
		})
	return result, err
}

func advertised(option sessionOption, requested string) bool {
	var entries []selectEntry
	if err := json.Unmarshal(option.Options, &entries); err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.Value == requested {
			return true
		}
		for _, sub := range entry.Options {
			if sub.Value == requested {
				return true
			}
		}
	}
	return false
}
func setACPChoice(ctx context.Context, wire *stream, sessionID string, raw json.RawMessage, kind, value string, rpcID int) (json.RawMessage, error) {
	if kind == "model" {
		if err := agentdomain.ValidateModel(value); err != nil {
			return nil, err
		}
	} else if err := agentdomain.ValidateReasoningEffort(value); err != nil {
		return nil, err
	}
	if value == "" {
		return raw, nil
	}
	var opts sessionOptions
	if err := json.Unmarshal(raw, &opts); err != nil {
		return nil, agentdomain.ErrUnavailable
	}
	id := ""
	for _, option := range opts.ConfigOptions {
		if selectCategory(option, kind) && advertised(option, value) {
			if id != "" {
				return nil, fmt.Errorf("%w: ambiguous ACP selector", agentdomain.ErrUnavailable)
			}
			id = option.ID
		}
	}
	if id == "" {
		return nil, fmt.Errorf("%w: selected %s not advertised", agentdomain.ErrUnavailable, kind)
	}
	result, err := wire.call(ctx, rpcID, "session/set_config_option",
		map[string]any{"sessionId": sessionID, "configId": id, "value": value}, func(rpcMessage) error { return nil })
	if err != nil {
		return nil, err
	}
	var confirmed sessionOptions
	if err := json.Unmarshal(result, &confirmed); err != nil {
		return nil, fmt.Errorf("%w: invalid ACP confirmation", agentdomain.ErrUnavailable)
	}
	for _, option := range confirmed.ConfigOptions {
		if option.ID == id && option.Type == "select" && option.CurrentValue == value {
			return result, nil
		}
	}
	return nil, errors.New("ACP Agent did not confirm selected " + kind)
}

// selectModel remains the single-option selection used by its focused tests.
func selectModel(ctx context.Context, wire *stream, sessionID string, session json.RawMessage, model string) error {
	_, err := setACPChoice(ctx, wire, sessionID, session, "model", model, 4)
	return err
}
func applySessionChoices(ctx context.Context, wire *stream, sessionID string, session json.RawMessage, model, effort string) error {
	var err error
	if model != "" {
		// ACP Agent may change its available thought levels when the model changes.
		session, err = setACPChoice(ctx, wire, sessionID, session, "model", model, 4)
		if err != nil {
			return err
		}
	}
	if effort != "" {
		_, err = setACPChoice(ctx, wire, sessionID, session, "reasoning_effort", effort, 5)
		if err != nil {
			return err
		}
	}
	return nil
}
