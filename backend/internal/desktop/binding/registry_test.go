package binding

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
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	registrydomain "github.com/orz-i/Mender/backend/internal/registry/domain"
	registryinfra "github.com/orz-i/Mender/backend/internal/registry/infrastructure"
)

func createDesktopTestPackageArchive(t *testing.T, pluginID, version string) ([]byte, string) {
	t.Helper()
	manifestJSON := []byte(fmt.Sprintf(`{
		"manifestVersion": 2,
		"pluginId": %q,
		"version": %q,
		"displayName": "Desktop Test Plugin",
		"description": "Desktop test desc",
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
	if err := os.WriteFile(filepath.Join(staging, "bin", "run"), []byte("bin-run"), 0o755); err != nil {
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
		Size:     int64(len("bin-run")),
		Typeflag: tar.TypeReg,
	})
	_, _ = tw.Write([]byte("bin-run"))

	_ = tw.Close()
	_ = gw.Close()

	return buf.Bytes(), treeDigest
}

func TestDesktopBindingRegistryMethods(t *testing.T) {
	t.Setenv(bootstrap.DockerSocketEnvironment, "")
	t.Setenv(bootstrap.DaytonaExperimentalEnvironment, "")

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	pluginID := "com.example.desktop-registry"
	v1Archive, v1Digest := createDesktopTestPackageArchive(t, pluginID, "1.0.0")
	v1SHA := sha256.Sum256(v1Archive)
	v1SHAHex := hex.EncodeToString(v1SHA[:])

	v2Archive, v2Digest := createDesktopTestPackageArchive(t, pluginID, "2.0.0")
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
				DisplayName: "Desktop Test Plugin",
				Description: "Desktop test desc",
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
				DisplayName: "Desktop Test Plugin",
				Description: "Desktop test desc",
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
			{KeyID: "desktop-signer", Sig: hex.EncodeToString(sig)},
		},
	}
	envelopeJSON, err = json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	rt, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{
		DatabasePath:       filepath.Join(t.TempDir(), "mender.db"),
		RegistryEndpoint:   serverURL + "/index.json",
		RegistryKeys:       map[string]string{"desktop-signer": hex.EncodeToString(pub)},
		RegistryHTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rt.Close() }()

	service := NewService(ctx, rt)

	// 1. SyncRegistry
	syncRes, err := service.SyncRegistry()
	if err != nil {
		t.Fatalf("SyncRegistry failed: %v", err)
	}
	if !syncRes.Synced || syncRes.Count != 2 {
		t.Fatalf("unexpected SyncRegistry result: %+v", syncRes)
	}

	// 2. InstallRegistryPlugin
	installed, err := service.InstallRegistryPlugin(pluginID, "1.0.0")
	if err != nil {
		t.Fatalf("InstallRegistryPlugin failed: %v", err)
	}
	if installed.PluginID != pluginID || installed.PackageVersion != "1.0.0" || installed.SourceKind != "registry" {
		t.Fatalf("unexpected installed projection: %+v", installed)
	}

	// 2b. ListRegistryUpdates (update to 2.0.0 should be available)
	updatesRes, err := service.ListRegistryUpdates()
	if err != nil {
		t.Fatalf("ListRegistryUpdates failed: %v", err)
	}
	if len(updatesRes.Updates) != 1 {
		t.Fatalf("expected 1 registry update, got %d: %+v", len(updatesRes.Updates), updatesRes.Updates)
	}
	if updatesRes.Updates[0].InstallationID != installed.InstallationID ||
		updatesRes.Updates[0].PluginID != pluginID ||
		updatesRes.Updates[0].CurrentVersion != "1.0.0" ||
		updatesRes.Updates[0].TargetVersion != "2.0.0" ||
		!updatesRes.Updates[0].CanUpgrade ||
		updatesRes.Updates[0].UsageCount != 0 {
		t.Fatalf("unexpected registry update entry: %+v", updatesRes.Updates[0])
	}

	// 3. UpgradeRegistryPlugin (in-place when usageCount == 0)
	upgraded, err := service.UpgradeRegistryPlugin(installed.InstallationID, "2.0.0")
	if err != nil {
		t.Fatalf("UpgradeRegistryPlugin failed: %v", err)
	}
	if upgraded.PackageVersion != "2.0.0" {
		t.Fatalf("expected upgraded version 2.0.0, got %s", upgraded.PackageVersion)
	}

	// 3b. ListRegistryUpdates after upgrade (no updates remaining)
	updatesAfter, err := service.ListRegistryUpdates()
	if err != nil {
		t.Fatalf("ListRegistryUpdates after upgrade failed: %v", err)
	}
	if len(updatesAfter.Updates) != 0 {
		t.Fatalf("expected 0 registry updates after upgrade, got %+v", updatesAfter.Updates)
	}

	// 4. Option B rejection when usageCount > 0
	parsedID, _ := platformplugin.ParseInstallationID(installed.InstallationID)
	if err := rt.RecordProviderUsageForTest(ctx, parsedID); err != nil {
		t.Fatalf("record provider usage: %v", err)
	}

	_, err = service.UpgradeRegistryPlugin(installed.InstallationID, "1.0.0")
	if err == nil {
		t.Fatal("expected upgrade to fail when usageCount > 0, but it succeeded")
	}
}
