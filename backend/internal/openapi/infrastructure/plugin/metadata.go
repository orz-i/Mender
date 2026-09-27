package plugin

import (
	"context"
	"encoding/json"

	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

// ValidateConfig validates OpenAPI plugin configuration without loading the
// description or resolving Credentials.
func ValidateConfig(
	_ context.Context,
	raw json.RawMessage,
) error {
	_, err := DecodeConfig(raw)
	return err
}

// InspectConfig exposes only bounded platform-governed metadata from OpenAPI
// configuration without loading the description.
func InspectConfig(
	_ context.Context,
	raw json.RawMessage,
) (providerplugin.ConfigMetadata, error) {
	config, err := DecodeConfig(raw)
	if err != nil {
		return providerplugin.ConfigMetadata{}, err
	}
	return providerplugin.NewConfigMetadata(config.RequiredCredentialRefs())
}
