package plugin

import (
	"context"
	"encoding/json"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// ExtractGovernanceFacts exposes only stable, host-reviewed operation effects
// and the exact sandbox ID from immutable Provider config. Never expose the
// command text, private task script, SSH credential, stdout or stderr to audit.
func (p *Plugin) ExtractGovernanceFacts(_ context.Context, raw json.RawMessage, tool catalogdomain.Tool, args invocationdomain.JSONValue) (invocationdomain.GovernanceFacts, error) {
	if p == nil {
		return invocationdomain.GovernanceFacts{}, sandboxdomain.ErrBackendUnavailable
	}
	cfg, err := DecodeConfig(raw)
	if err != nil {
		return invocationdomain.GovernanceFacts{}, err
	}
	var effect invocationdomain.EffectFact
	switch tool.SourceKey() {
	case SourceSubmit:
		var input struct {
			IdempotencyKey string `json:"idempotencyKey"`
			Command        string `json:"command"`
		}
		if err := strictDecode(args.Bytes(), &input); err != nil {
			return invocationdomain.GovernanceFacts{}, err
		}
		effect = "process.detached.submit"
	case SourceStatus, SourceResult:
		effect = "process.observe"
	case SourceLogs:
		effect = "process.logs.read"
	case SourceCancel:
		effect = "process.cancel"
	default:
		return invocationdomain.UnknownGovernanceFacts(), nil
	}
	resource, err := invocationdomain.NewResourceFact("sandbox_id", cfg.SandboxID)
	if err != nil {
		return invocationdomain.GovernanceFacts{}, err
	}
	return invocationdomain.KnownGovernanceFacts([]invocationdomain.EffectFact{effect}, []invocationdomain.ResourceFact{resource})
}

var _ providerplugin.GovernanceFactProvider = (*Plugin)(nil)
