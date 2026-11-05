package application

import (
	"encoding/json"
	"fmt"

	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

func (s Service) resolveProviderPlugin(
	pluginID providerplugin.ID,
) (providerplugin.Plugin, providerplugin.Descriptor, error) {
	plugin, err := s.deps.Builtins.Lookup(pluginID)
	if err != nil {
		return nil, providerplugin.Descriptor{}, err
	}
	descriptor := plugin.Descriptor()
	if descriptor.ID != pluginID {
		return nil, providerplugin.Descriptor{}, fmt.Errorf(
			"provider plugin descriptor id %s does not match registry id %s",
			descriptor.ID,
			pluginID,
		)
	}
	return plugin, descriptor, nil
}

func cloneRawJSON(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}

func wrapOptionalError(message string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}
