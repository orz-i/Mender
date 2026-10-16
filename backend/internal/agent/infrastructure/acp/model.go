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

// ReadModelChoices accepts only the reviewed ACP v1 model select category.
// It discards all other session/connection metadata and bounds public output.
func ReadModelChoices(raw json.RawMessage) (agentapplication.ModelChoices, error) {
	var metadata sessionOptions
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return agentapplication.ModelChoices{}, agentdomain.ErrUnavailable
	}
	if len(metadata.ConfigOptions) > 32 {
		return agentapplication.ModelChoices{}, agentdomain.ErrUnavailable
	}
	result := agentapplication.ModelChoices{Models: []agentapplication.ModelChoice{}}
	seen := map[string]bool{}
	for _, option := range metadata.ConfigOptions {
		if (option.Category != "model" && option.ID != "model") || option.Type != "select" {
			continue
		}
		if len(result.Models) != 0 {
			return agentapplication.ModelChoices{}, agentdomain.ErrUnavailable
		}
		var entries []selectEntry
		if err := json.Unmarshal(option.Options, &entries); err != nil || len(entries) > 64 {
			return agentapplication.ModelChoices{}, agentdomain.ErrUnavailable
		}
		add := func(value, name string) error {
			if value == "" || seen[value] {
				return nil
			}
			if err := agentdomain.ValidateModel(value); err != nil {
				return agentdomain.ErrUnavailable
			}
			if len(result.Models) >= 64 {
				return agentdomain.ErrUnavailable
			}
			if len(name) > 120 {
				name = value
			}
			if name == "" {
				name = value
			}
			result.Models = append(result.Models, agentapplication.ModelChoice{ID: value, Name: name})
			seen[value] = true
			return nil
		}
		for _, entry := range entries {
			if err := add(entry.Value, entry.Name); err != nil {
				return agentapplication.ModelChoices{}, err
			}
			for _, nested := range entry.Options {
				if err := add(nested.Value, nested.Name); err != nil {
					return agentapplication.ModelChoices{}, err
				}
			}
		}
		result.Current = option.CurrentValue
	}
	if len(result.Models) == 0 {
		return agentapplication.ModelChoices{}, agentdomain.ErrUnavailable
	}
	if result.Current != "" && !seen[result.Current] {
		return agentapplication.ModelChoices{}, agentdomain.ErrUnavailable
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

// selectModel uses only ACP v1-reported select choices, never a host CLI flag,
// model string interpolation or speculative backend-specific parameter.
func selectModel(ctx context.Context, wire *stream, sessionID string, sessionJSON json.RawMessage, model string) error {
	if err := agentdomain.ValidateModel(model); err != nil {
		return err
	}
	if model == "" {
		return nil
	}
	var metadata sessionOptions
	if err := json.Unmarshal(sessionJSON, &metadata); err != nil {
		return fmt.Errorf("%w: invalid ACP session options", agentdomain.ErrUnavailable)
	}
	var chosen string
	for _, option := range metadata.ConfigOptions {
		if (option.Category == "model" || option.ID == "model") && option.Type == "select" && advertised(option, model) {
			if chosen != "" {
				return errors.New("ACP returned ambiguous model selectors")
			}
			chosen = option.ID
		}
	}
	if chosen == "" {
		return fmt.Errorf("%w: selected model not advertised by ACP Agent", agentdomain.ErrUnavailable)
	}
	result, err := wire.call(ctx, 4, "session/set_config_option", map[string]any{
		"sessionId": sessionID, "configId": chosen, "value": model,
	}, func(rpcMessage) error { return nil })
	if err != nil {
		return err
	}
	var confirmed sessionOptions
	if err := json.Unmarshal(result, &confirmed); err != nil {
		return errors.New("ACP rejected model configuration response")
	}
	for _, option := range confirmed.ConfigOptions {
		if option.ID == chosen && option.Type == "select" && option.CurrentValue == model {
			return nil
		}
	}
	return errors.New("ACP Agent did not confirm selected model")
}
