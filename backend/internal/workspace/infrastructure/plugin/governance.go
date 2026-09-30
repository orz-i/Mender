package plugin

import (
	"context"
	"encoding/json"
	"fmt"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

func (p *Plugin) ExtractGovernanceFacts(
	_ context.Context,
	rawConfig json.RawMessage,
	tool catalogdomain.Tool,
	arguments invocationdomain.JSONValue,
) (invocationdomain.GovernanceFacts, error) {
	if p == nil {
		return invocationdomain.GovernanceFacts{}, fmt.Errorf("workspace governance extractor is nil")
	}
	if _, err := DecodeConfig(rawConfig); err != nil {
		return invocationdomain.GovernanceFacts{}, err
	}

	var effect invocationdomain.EffectFact
	var resourcePath string
	switch tool.SourceKey() {
	case sourceKeyRead:
		var input readInput
		if err := decodeStrict(arguments.Bytes(), &input, "decode workspace.read governance arguments"); err != nil {
			return invocationdomain.GovernanceFacts{}, err
		}
		normalized, err := normalizeRelativePath(input.Path, false)
		if err != nil {
			return invocationdomain.GovernanceFacts{}, err
		}
		effect = "filesystem.read"
		resourcePath = normalized

	case sourceKeySearch:
		var input searchInput
		if err := decodeStrict(arguments.Bytes(), &input, "decode workspace.search governance arguments"); err != nil {
			return invocationdomain.GovernanceFacts{}, err
		}
		if input.Path == "" {
			input.Path = "."
		}
		normalized, err := normalizeRelativePath(input.Path, true)
		if err != nil {
			return invocationdomain.GovernanceFacts{}, err
		}
		effect = "filesystem.search"
		resourcePath = normalized

	case sourceKeyEdit:
		var input editInput
		if err := decodeStrict(arguments.Bytes(), &input, "decode workspace.edit governance arguments"); err != nil {
			return invocationdomain.GovernanceFacts{}, err
		}
		normalized, err := normalizeRelativePath(input.Path, false)
		if err != nil {
			return invocationdomain.GovernanceFacts{}, err
		}
		effect = "filesystem.write"
		resourcePath = normalized

	case sourceKeyShell:
		var input shellInput
		if err := decodeStrict(arguments.Bytes(), &input, "decode workspace.shell governance arguments"); err != nil {
			return invocationdomain.GovernanceFacts{}, err
		}
		cwd := input.Cwd
		if cwd == "" {
			cwd = "."
		}
		normalized, err := normalizeRelativePath(cwd, true)
		if err != nil {
			return invocationdomain.GovernanceFacts{}, err
		}
		effect = "process.execute"
		resourcePath = normalized

	default:
		return invocationdomain.UnknownGovernanceFacts(), nil
	}

	resource, err := invocationdomain.NewResourceFact("workspace_path", resourcePath)
	if err != nil {
		return invocationdomain.GovernanceFacts{}, err
	}
	return invocationdomain.KnownGovernanceFacts(
		[]invocationdomain.EffectFact{effect},
		[]invocationdomain.ResourceFact{resource},
	)
}

var _ providerplugin.GovernanceFactProvider = (*Plugin)(nil)
