package external

import (
	"context"
	"fmt"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	invocationjsonschema "github.com/orz-i/Mender/backend/internal/invocation/infrastructure/jsonschema"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sandboxsqlite "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sqlite"
	sandboxv1 "github.com/orz-i/Mender/sdk/sandboxplugin/go/protocol/v1"
)

// ValidateConfig keeps the existing schema/metadata validation separate from
// the typed Workspace execution adapter.
func (b *Backend) ValidateConfig(ctx context.Context, binding sandboxdomain.BackendBinding, config sandboxsqlite.ExternalConfig) error {
	if err := config.Validate(); err != nil {
		return err
	}
	if config.ConfigVersion != b.descriptor.ConfigVersion {
		return fmt.Errorf("sandbox plugin config version mismatch")
	}
	if len(b.descriptor.ConfigSchema) > 0 {
		schema, err := catalogdomain.NewSchema(b.descriptor.ConfigSchema)
		if err != nil {
			return err
		}
		value, err := invocationdomain.NewJSONValue(config.JSON)
		if err != nil {
			return err
		}
		valid, err := invocationjsonschema.Validator{}.Validate(ctx, schema, value)
		if err != nil {
			return err
		}
		if !valid {
			return fmt.Errorf("sandbox plugin configuration violates host-validated schema")
		}
	}
	p, err := b.process(ctx, binding)
	if err != nil {
		return err
	}
	response, err := p.Service().ValidateConfig(ctx, &sandboxv1.ValidateConfigRequest{
		ConfigVersion: config.ConfigVersion, ConfigJson: config.JSON})
	if err != nil {
		return fmt.Errorf("sandbox plugin validate config: %w", err)
	}
	if !response.GetValid() {
		return fmt.Errorf("sandbox plugin config rejected: %s", response.GetDiagnostic())
	}
	metadata, err := p.Service().InspectConfig(ctx, &sandboxv1.InspectConfigRequest{
		ConfigVersion: config.ConfigVersion, ConfigJson: config.JSON})
	if err != nil {
		return fmt.Errorf("sandbox plugin inspect config: %w", err)
	}
	if len(metadata.GetCredentialRefs()) != 0 {
		return fmt.Errorf("sandbox plugin requests unsupported credential scope")
	}
	if len(metadata.GetConnectionSummary()) > 4096 {
		return fmt.Errorf("sandbox plugin metadata too large")
	}
	return nil
}
