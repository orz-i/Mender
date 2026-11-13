package bootstrap

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
	registrydomain "github.com/orz-i/Mender/backend/internal/registry/domain"
	registryinfra "github.com/orz-i/Mender/backend/internal/registry/infrastructure"
)

func TestRegistryConfigOptionA(t *testing.T) {
	// 1. Default config loads official embedded key
	cfg, err := loadRegistryConfig(RuntimeOptions{})
	if err != nil {
		t.Fatalf("load default config: %v", err)
	}
	if cfg.endpoint != DefaultOfficialRegistryEndpoint {
		t.Errorf("got endpoint %s, want %s", cfg.endpoint, DefaultOfficialRegistryEndpoint)
	}
	keys := cfg.trustStore.Keys()
	if _, ok := keys[registrydomain.OfficialRootKeyID]; !ok {
		t.Errorf("missing official root key in trust store")
	}

	// 2. Loading enterprise key and private registry endpoint from config.json
	enterprisePub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	enterpriseHex := hex.EncodeToString(enterprisePub)

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	configFileContent := fmt.Sprintf(`{
		"registry": {
			"id": "my-corp",
			"endpoint": "https://corp.internal/registry/index.json",
			"publicKey": "%s"
		}
	}`, enterpriseHex)
	if err := os.WriteFile(configPath, []byte(configFileContent), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg2, err := loadRegistryConfig(RuntimeOptions{ConfigFile: configPath})
	if err != nil {
		t.Fatalf("load enterprise config: %v", err)
	}
	if cfg2.endpoint != "https://corp.internal/registry/index.json" {
		t.Errorf("got endpoint %s, want https://corp.internal/registry/index.json", cfg2.endpoint)
	}
	keys2 := cfg2.trustStore.Keys()
	if _, ok := keys2["my-corp"]; !ok {
		t.Errorf("missing my-corp enterprise key in trust store")
	}

	// 3. Loading enterprise keys from RuntimeOptions
	corp2Pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	corp2Hex := hex.EncodeToString(corp2Pub)
	cfg3, err := loadRegistryConfig(RuntimeOptions{
		RegistryEndpoint: "https://options.internal/index.json",
		RegistryKeys:     map[string]string{"corp-2": corp2Hex},
	})
	if err != nil {
		t.Fatalf("load options config: %v", err)
	}
	if cfg3.endpoint != "https://options.internal/index.json" {
		t.Errorf("got endpoint %s, want https://options.internal/index.json", cfg3.endpoint)
	}
	keys3 := cfg3.trustStore.Keys()
	if _, ok := keys3["corp-2"]; !ok {
		t.Errorf("missing corp-2 key in trust store")
	}
}

func createTestPackageArchive(t *testing.T, pluginID, version string) ([]byte, string) {
	t.Helper()
	manifestJSON := []byte(fmt.Sprintf(`{
		"manifestVersion": 2,
		"pluginId": %q,
		"version": %q,
		"displayName": "Test Registry Plugin",
		"description": "Test description",
		"entrypoints": {
			"%s/%s": "bin/run"
		},
		"extensions": [{"id": "main", "kind": "provider.tools", "protocolVersion": 1}]
	}`, pluginID, version, runtime.GOOS, runtime.GOARCH))

	staging := t.TempDir()
	if err := os.WriteFile(filepath.Join(staging, "mender-plugin.json"), manifestJSON, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(staging, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "bin", "run"), []byte("echo binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	treeDigest, err := registryinfra.CalculateTreeDigest(context.Background(), staging)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	_ = tw.WriteHeader(&tar.Header{
		Name:     "mender-plugin.json",
		Mode:     0o644,
		Size:     int64(len(manifestJSON)),
		Typeflag: tar.TypeReg,
	})
	_, _ = tw.Write(manifestJSON)

	_ = tw.WriteHeader(&tar.Header{
		Name:     "bin/run",
		Mode:     0o755,
		Size:     int64(len("echo binary")),
		Typeflag: tar.TypeReg,
	})
	_, _ = tw.Write([]byte("echo binary"))

	_ = tw.Close()
	_ = gw.Close()

	return buf.Bytes(), treeDigest
}

func TestRegistryRuntimeSyncInstallAndCatalogue(t *testing.T) {
	t.Setenv(DockerSocketEnvironment, "")
	t.Setenv(DaytonaExperimentalEnvironment, "")

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	pluginID := "com.example.registry-test"
	v1Archive, v1Digest := createTestPackageArchive(t, pluginID, "1.0.0")
	v1SHA := sha256.Sum256(v1Archive)
	v1SHAHex := hex.EncodeToString(v1SHA[:])

	v2Archive, v2Digest := createTestPackageArchive(t, pluginID, "2.0.0")
	v2SHA := sha256.Sum256(v2Archive)
	v2SHAHex := hex.EncodeToString(v2SHA[:])

	platformKey := runtime.GOOS + "/" + runtime.GOARCH

	var serverURL string
	var envelopeJSON []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(envelopeJSON)
		case "/pkg-1.0.0.tar.gz":
			w.Header().Set("Content-Type", "application/gzip")
			_, _ = w.Write(v1Archive)
		case "/pkg-2.0.0.tar.gz":
			w.Header().Set("Content-Type", "application/gzip")
			_, _ = w.Write(v2Archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	now := time.Now().UTC()
	index := registrydomain.CatalogIndex{
		SchemaVersion: 1,
		Sequence:      10,
		IssuedAt:      now.Add(-1 * time.Minute),
		ExpiresAt:     now.Add(1 * time.Hour),
		Plugins: []registrydomain.RegistryPluginEntry{
			{
				PluginID:    pluginID,
				Version:     "1.0.0",
				DisplayName: "Registry Test Plugin",
				Description: "Registry test desc",
				Targets: map[string]registrydomain.ArtifactTarget{
					platformKey: {
						URL:        serverURL + "/pkg-1.0.0.tar.gz",
						SHA256:     v1SHAHex,
						TreeDigest: v1Digest,
						Size:       int64(len(v1Archive)),
					},
				},
			},
			{
				PluginID:    pluginID,
				Version:     "2.0.0",
				DisplayName: "Registry Test Plugin",
				Description: "Registry test desc",
				Targets: map[string]registrydomain.ArtifactTarget{
					platformKey: {
						URL:        serverURL + "/pkg-2.0.0.tar.gz",
						SHA256:     v2SHAHex,
						TreeDigest: v2Digest,
						Size:       int64(len(v2Archive)),
					},
				},
			},
		},
	}

	canonical, err := index.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	sig := ed25519.Sign(priv, canonical)
	envelope := registrydomain.SignatureEnvelope{
		Payload: index,
		Signatures: []registrydomain.Signature{
			{KeyID: "test-signer", Sig: hex.EncodeToString(sig)},
		},
	}
	envelopeJSON, err = json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{
		DatabasePath:       filepath.Join(t.TempDir(), "mender.db"),
		RegistryEndpoint:   serverURL + "/index.json",
		RegistryKeys:       map[string]string{"test-signer": hex.EncodeToString(pub)},
		RegistryHTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()

	// 1. Sync registry metadata
	synced, count, err := runtime.SyncRegistry(ctx)
	if err != nil {
		t.Fatalf("SyncRegistry failed: %v", err)
	}
	if !synced || count != 2 {
		t.Fatalf("expected synced=true, count=2, got synced=%v, count=%d", synced, count)
	}

	// 2. Install v1.0.0 directly from registry
	installation, err := runtime.InstallRegistryPlugin(ctx, pluginID, "1.0.0")
	if err != nil {
		t.Fatalf("InstallRegistryPlugin failed: %v", err)
	}
	if installation.SourceKind() != installationdomain.SourceRegistry {
		t.Errorf("got sourceKind %s, want %s", installation.SourceKind(), installationdomain.SourceRegistry)
	}
	if installation.PackageVersion() != "1.0.0" {
		t.Errorf("got packageVersion %s, want 1.0.0", installation.PackageVersion())
	}

	// 3. Verify ListPluginCatalogue reports sourceKind: registry
	catalogue, err := runtime.ListPluginCatalogue(ctx)
	if err != nil {
		t.Fatalf("ListPluginCatalogue failed: %v", err)
	}
	found := false
	for _, entry := range catalogue {
		if entry.PluginID == pluginID {
			found = true
			if entry.SourceKind != "registry" {
				t.Errorf("expected catalogue entry sourceKind to be 'registry', got %s", entry.SourceKind)
			}
			if entry.PackageVersion != "1.0.0" {
				t.Errorf("expected catalogue entry packageVersion to be '1.0.0', got %s", entry.PackageVersion)
			}
		}
	}
	if !found {
		t.Fatalf("installed registry plugin %s not found in catalogue", pluginID)
	}

	// 4. List available updates
	updates, err := runtime.ListRegistryUpdates(ctx)
	if err != nil {
		t.Fatalf("ListRegistryUpdates failed: %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updates))
	}
	if updates[0].TargetVersion != "2.0.0" {
		t.Errorf("expected targetVersion 2.0.0, got %s", updates[0].TargetVersion)
	}
	if !updates[0].CanUpgrade || updates[0].UsageCount != 0 {
		t.Errorf("expected canUpgrade=true, usageCount=0, got %v, %d", updates[0].CanUpgrade, updates[0].UsageCount)
	}

	// 5. In-place upgrade to v2.0.0 (Option B: usageCount == 0 succeeds)
	upgraded, err := runtime.UpgradeRegistryPlugin(ctx, installation.ID(), "2.0.0")
	if err != nil {
		t.Fatalf("UpgradeRegistryPlugin failed: %v", err)
	}
	if upgraded.PackageVersion() != "2.0.0" {
		t.Errorf("expected upgraded version 2.0.0, got %s", upgraded.PackageVersion())
	}

	// 6. Option B rejection when usageCount > 0
	if err := runtime.RecordProviderUsageForTest(ctx, upgraded.ID()); err != nil {
		t.Fatalf("record provider usage: %v", err)
	}

	// Attempting to upgrade while referenced must fail with ErrInstallationInUse
	_, err = runtime.UpgradeRegistryPlugin(ctx, upgraded.ID(), "1.0.0")
	if err == nil {
		t.Fatal("expected upgrade to fail when usageCount > 0, but it succeeded")
	}
	if !errors.Is(err, registrydomain.ErrInstallationInUse) {
		t.Errorf("expected ErrInstallationInUse, got %v", err)
	}
}
