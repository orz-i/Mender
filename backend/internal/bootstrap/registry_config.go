package bootstrap

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	registrydomain "github.com/orz-i/Mender/backend/internal/registry/domain"
)

const (
	// DefaultOfficialRegistryEndpoint is the default online registry index URL.
	DefaultOfficialRegistryEndpoint = "https://registry.mender.io/v1/index.json"
)

// RegistryConfigEntry defines one registry source.
type RegistryConfigEntry struct {
	ID        string `json:"id,omitempty"`
	Endpoint  string `json:"endpoint"`
	PublicKey string `json:"publicKey,omitempty"`
}

// MenderConfigFile represents the structure of ~/.mender/config.json.
type MenderConfigFile struct {
	Registry   *RegistryConfigEntry  `json:"registry,omitempty"`
	Registries []RegistryConfigEntry `json:"registries,omitempty"`
}

type loadedRegistryConfig struct {
	trustStore *registrydomain.TrustStore
	endpoint   string
}

func loadRegistryConfig(options RuntimeOptions) (loadedRegistryConfig, error) {
	// 1. Initialize trust store with official embedded root key
	officialHex := strings.TrimSpace(options.RegistryRootKeyHex)
	if officialHex == "" {
		officialHex = registrydomain.DefaultOfficialRootPublicKeyHex
	}
	officialKeyBytes, err := hex.DecodeString(officialHex)
	if err != nil || len(officialKeyBytes) != ed25519.PublicKeySize {
		return loadedRegistryConfig{}, fmt.Errorf("invalid official root public key hex %q: %w", officialHex, err)
	}
	officialKey := ed25519.PublicKey(officialKeyBytes)

	trustStore := registrydomain.NewTrustStore(officialKey, nil)

	// 2. Load ~/.mender/config.json or explicit options.ConfigFile
	configFile := strings.TrimSpace(options.ConfigFile)
	if configFile == "" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			candidate := filepath.Join(home, ".mender", "config.json")
			if _, statErr := os.Stat(candidate); statErr == nil {
				configFile = candidate
			}
		}
	}

	var parsedFile MenderConfigFile
	if configFile != "" {
		if data, err := os.ReadFile(configFile); err == nil {
			_ = json.Unmarshal(data, &parsedFile)
		}
	}

	endpoint := strings.TrimSpace(options.RegistryEndpoint)

	// 3. Option A: load enterprise private registry endpoints and keys from config.json
	if parsedFile.Registry != nil {
		if endpoint == "" && parsedFile.Registry.Endpoint != "" {
			endpoint = parsedFile.Registry.Endpoint
		}
		if parsedFile.Registry.PublicKey != "" {
			keyID := parsedFile.Registry.ID
			if keyID == "" {
				keyID = "enterprise"
			}
			_ = trustStore.AddKeyHex(keyID, parsedFile.Registry.PublicKey)
		}
	}

	for i, reg := range parsedFile.Registries {
		if endpoint == "" && reg.Endpoint != "" {
			endpoint = reg.Endpoint
		}
		if reg.PublicKey != "" {
			keyID := reg.ID
			if keyID == "" {
				keyID = fmt.Sprintf("enterprise-%d", i+1)
			}
			_ = trustStore.AddKeyHex(keyID, reg.PublicKey)
		}
	}

	// 4. RuntimeOptions enterprise keys support
	for keyID, hexKey := range options.RegistryKeys {
		if hexKey != "" {
			_ = trustStore.AddKeyHex(keyID, hexKey)
		}
	}

	// 5. Default endpoint fallback
	if endpoint == "" {
		endpoint = DefaultOfficialRegistryEndpoint
	}

	return loadedRegistryConfig{
		trustStore: trustStore,
		endpoint:   endpoint,
	}, nil
}
