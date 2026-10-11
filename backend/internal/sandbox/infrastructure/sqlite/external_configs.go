package sqlite

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// ExternalConfig is an immutable, bounded and non-secret plugin configuration.
type ExternalConfig struct {
	ConfigVersion uint32
	JSON          json.RawMessage
}

// Validate rejects credentials entirely in the first external driver revision.
// Future plugins must use explicit host-scoped credential handoff, never raw
// secret values in the config JSON or a general CredentialResolver.
func (c ExternalConfig) Validate() error {
	if c.ConfigVersion == 0 || len(c.JSON) == 0 || len(c.JSON) > 4096 || !json.Valid(c.JSON) {
		return fmt.Errorf("external Sandbox configuration must be bounded valid JSON")
	}
	var tree any
	if err := json.Unmarshal(c.JSON, &tree); err != nil {
		return fmt.Errorf("external Sandbox config is invalid: %w", err)
	}
	if _, ok := tree.(map[string]any); !ok {
		return fmt.Errorf("external Sandbox config must be a JSON object")
	}
	if err := rejectExternalCredentialMaterial(tree, 0); err != nil {
		return err
	}
	return nil
}

// Inspect decoded keys and nested values, rather than scanning raw JSON bytes:
// escaped Unicode sequences must not evade host-owned Credential Reference policy.
// This is a conservative filter for recognized secret-bearing shapes, NOT a
// proof that arbitrary text cannot contain a secret; native plugins are trusted.
func rejectExternalCredentialMaterial(value any, depth int) error {
	if depth > 16 {
		return fmt.Errorf("external Sandbox config nesting exceeds limit")
	}
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "", " ", "").Replace(key))
			for _, forbidden := range []string{"credential", "password", "secret", "token", "apikey", "privatekey", "bearer"} {
				if strings.Contains(normalized, forbidden) {
					return fmt.Errorf("external Sandbox config cannot contain credential or secret fields")
				}
			}
			if err := rejectExternalCredentialMaterial(child, depth+1); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := rejectExternalCredentialMaterial(child, depth+1); err != nil {
				return err
			}
		}
	case string:
		lower := strings.ToLower(typed)
		for _, forbidden := range []string{"credential://", "password", "secret", "token", "api_key", "private_key", "bearer "} {
			if strings.Contains(lower, forbidden) {
				return fmt.Errorf("external Sandbox config cannot contain secret-like values")
			}
		}
	}
	return nil
}

func (s *Store) CreateDefinedExternal(ctx context.Context, item sandboxdomain.Sandbox, config ExternalConfig) error {
	if err := item.Validate(); err != nil {
		return err
	}
	if item.Backend() != sandboxdomain.BackendExternal {
		return sandboxdomain.ErrBackendUnavailable
	}
	if err := config.Validate(); err != nil {
		return err
	}
	spec := item.Spec()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// Atomic install pin/availability admission. If the package was just
	// disabled or replaced, creation returns unavailable without a resource.
	result, err := tx.ExecContext(ctx, `INSERT INTO sandboxes(
        id,display_name,backend,backend_plugin_id,backend_extension_id,backend_installation_id,
        isolation,local_root,owner_kind,owner_ref,created_at)
        SELECT ?,?,?,?,?,?,?,?,?,?,? WHERE EXISTS(
            SELECT 1 FROM plugin_installations pi WHERE pi.installation_id=?
              AND pi.plugin_id=? AND pi.status='enabled'
              AND pi.verified_descriptor_json IS NOT NULL
              AND json_extract(pi.manifest_json,'$.extensions[0].kind')='sandbox.backend'
        )`,
		spec.ID.String(), spec.Name, string(spec.Backend), spec.Binding.PluginID.String(),
		spec.Binding.ExtensionID.String(), spec.Binding.InstallationID.String(),
		string(spec.Isolation), spec.LocalRoot, spec.OwnerKind, spec.OwnerRef, now,
		spec.Binding.InstallationID.String(), spec.Binding.PluginID.String())
	if err != nil {
		return fmt.Errorf("external Sandbox create: inventory: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("external Sandbox create: rows affected: %w", err)
	}
	if count != 1 {
		return sandboxdomain.ErrBackendUnavailable
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO sandbox_runs(sandbox_id,generation,state,external_id,updated_at) VALUES(?,1,'defined','',?)", spec.ID.String(), now); err != nil {
		return fmt.Errorf("external Sandbox create: run: %w", err)
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO sandbox_external_configs(sandbox_id,config_version,config_json,created_at) VALUES(?,?,?,?)",
		spec.ID.String(), config.ConfigVersion, string(config.JSON), now); err != nil {
		return fmt.Errorf("external Sandbox create: config: %w", err)
	}
	return tx.Commit()
}

func (s *Store) GetExternalConfig(ctx context.Context, id sandboxdomain.ID) (ExternalConfig, error) {
	if _, err := sandboxdomain.ParseID(id.String()); err != nil {
		return ExternalConfig{}, err
	}
	var version uint32
	var raw string
	err := s.db.QueryRowContext(ctx, "SELECT config_version,config_json FROM sandbox_external_configs WHERE sandbox_id=?", id.String()).Scan(&version, &raw)
	if err != nil {
		return ExternalConfig{}, fmt.Errorf("external Sandbox config unavailable: %w", err)
	}
	c := ExternalConfig{ConfigVersion: version, JSON: bytes.Clone([]byte(raw))}
	if err := c.Validate(); err != nil {
		return ExternalConfig{}, err
	}
	return c, nil
}

// ReferencedInstallation returns a bounded count of immutable external Sandbox
// references (including stopped/unknown/destroyed resources) that block package
// disable/removal. It does not need to resolve plugin processes or secrets.
func (s *Store) ReferencedInstallation(ctx context.Context, installationID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sandboxes WHERE backend='external' AND backend_installation_id=?", installationID).Scan(&count)
	return count, err
}
