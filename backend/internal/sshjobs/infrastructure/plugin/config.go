package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

const ID providerplugin.ID = "io.mender.provider.sandbox-jobs"
const ConfigVersion = 1

type Config struct {
	SandboxID string `json:"sandboxId"`
}

func DecodeConfig(raw json.RawMessage) (Config, error) {
	var cfg Config
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("SSH Jobs config: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return Config{}, fmt.Errorf("SSH Jobs config: exactly one object required")
	}
	if _, err := sandboxdomain.ParseID(cfg.SandboxID); err != nil {
		return Config{}, fmt.Errorf("SSH Jobs config: invalid Sandbox identity: %w", err)
	}
	return cfg, nil
}
func Descriptor() providerplugin.Descriptor {
	return providerplugin.Descriptor{ID: ID, Version: "1.0.0", DisplayName: "SSH Durable Jobs", ConfigVersion: ConfigVersion,
		Description:  "Five governed operations for one explicitly bound SSH Sandbox. Requires root systemd manager; task state remains backend-owned.",
		ConfigSchema: append(json.RawMessage(nil), facadeConfigSchema...),
		Capabilities: []string{"jobs.submit", "jobs.status", "jobs.logs", "jobs.result", "jobs.cancel"}}
}
func InspectConfig(_ context.Context, raw json.RawMessage) (providerplugin.ConfigMetadata, error) {
	cfg, err := DecodeConfig(raw)
	if err != nil {
		return providerplugin.ConfigMetadata{}, err
	}
	return providerplugin.NewConfigMetadataWithSummary(nil, "sandbox:"+cfg.SandboxID)
}
