package plugin

import (
	"context"
	"encoding/json"
	"errors"

	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// resolveSelection projects one reviewed workspace alias onto the exact
// Provider's configured Sandbox, without letting callers name a raw SandboxID.
// The alias never reaches file/exec backends; they receive their existing v2
// strict argument schema unchanged.
func resolveSelection(ctx context.Context, lookup TargetLookup, providerID string, pinned sandboxdomain.ID,
	args invocationdomain.JSONValue) (sandboxdomain.ID, invocationdomain.JSONValue, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(args.Bytes(), &obj); err != nil || obj == nil {
		return "", invocationdomain.JSONValue{}, errors.New("workspace arguments must be an object")
	}
	raw, selected := obj["workspace"]
	if !selected {
		return pinned, args, nil
	}
	var key string
	if err := decodeStrict(raw, &key, "decode workspace target"); err != nil {
		return "", invocationdomain.JSONValue{}, err
	}
	if err := validateTargetKey(key); err != nil {
		return "", invocationdomain.JSONValue{}, err
	}
	if lookup == nil || providerID == "" {
		return "", invocationdomain.JSONValue{}, sandboxdomain.ErrNotAuthorized
	}
	target, err := lookup.ResolveWorkspaceTarget(ctx, providerID, key)
	if err != nil {
		return "", invocationdomain.JSONValue{}, err
	}
	delete(obj, "workspace")
	stripped, err := json.Marshal(obj)
	if err != nil {
		return "", invocationdomain.JSONValue{}, err
	}
	value, err := invocationdomain.NewJSONValue(stripped)
	return target, value, err
}

// admittedSandboxTarget requires one unambiguous, host-reviewed target fact.
// A missing or stale fact must never authorize a new selected Sandbox.
func admittedSandboxTarget(facts invocationdomain.GovernanceFacts, target sandboxdomain.ID) bool {
	if facts.Knowledge() != invocationdomain.FactKnowledgeKnown {
		return false
	}
	matches := 0
	for _, resource := range facts.Resources() {
		if resource.Kind() != "sandbox_id" {
			continue
		}
		if resource.Value() != target.String() {
			return false
		}
		matches++
	}
	return matches == 1
}

func validateTargetKey(key string) error {
	if len(key) == 0 || len(key) > 64 || key == "default" {
		return sandboxdomain.ErrNotAuthorized
	}
	for _, r := range key {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' && r != '_' {
			return sandboxdomain.ErrNotAuthorized
		}
	}
	return nil
}
