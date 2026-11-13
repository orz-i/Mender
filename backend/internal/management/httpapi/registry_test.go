package httpapi

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

func createHTTPTestPackageArchive(t *testing.T, pluginID, version string) ([]byte, string) {
	t.Helper()
	manifestJSON := []byte(fmt.Sprintf(`{
		"manifestVersion": 2,
		"pluginId": %q,
		"version": %q,
		"displayName": "HTTP Test Plugin",
		"description": "HTTP test desc",
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
	if err := os.WriteFile(filepath.Join(staging, "bin", "run"), []byte("bin-body"), 0o755); err != nil {
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
		Size:     int64(len("bin-body")),
		Typeflag: tar.TypeReg,
	})
	_, _ = tw.Write([]byte("bin-body"))

	_ = tw.Close()
	_ = gw.Close()

	return buf.Bytes(), treeDigest
}

func setupRegistryServerAndRuntime(t *testing.T, pluginID string) (*httptest.Server, *bootstrap.Runtime, *httptest.Server) {
	t.Helper()
	t.Setenv(bootstrap.DockerSocketEnvironment, "")
	t.Setenv(bootstrap.DaytonaExperimentalEnvironment, "")

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	v1Archive, v1Digest := createHTTPTestPackageArchive(t, pluginID, "1.0.0")
	v1SHA := sha256.Sum256(v1Archive)
	v1SHAHex := hex.EncodeToString(v1SHA[:])

	v2Archive, v2Digest := createHTTPTestPackageArchive(t, pluginID, "2.0.0")
	v2SHA := sha256.Sum256(v2Archive)
	v2SHAHex := hex.EncodeToString(v2SHA[:])

	platformKey := runtime.GOOS + "/" + runtime.GOARCH

	var registryServerURL string
	var envelopeJSON []byte

	registryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	registryServerURL = registryServer.URL

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
				DisplayName: "HTTP Registry Plugin",
				Description: "HTTP registry desc",
				Targets: map[string]registrydomain.ArtifactTarget{
					platformKey: {
						URL:        registryServerURL + "/pkg-1.0.0.tar.gz",
						SHA256:     v1SHAHex,
						TreeDigest: v1Digest,
						Size:       int64(len(v1Archive)),
					},
				},
			},
			{
				PluginID:    pluginID,
				Version:     "2.0.0",
				DisplayName: "HTTP Registry Plugin",
				Description: "HTTP registry desc",
				Targets: map[string]registrydomain.ArtifactTarget{
					platformKey: {
						URL:        registryServerURL + "/pkg-2.0.0.tar.gz",
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
			{KeyID: "http-signer", Sig: hex.EncodeToString(sig)},
		},
	}
	envelopeJSON, err = json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	rt, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{
		DatabasePath:       filepath.Join(t.TempDir(), "mender.db"),
		RegistryEndpoint:   registryServerURL + "/index.json",
		RegistryKeys:       map[string]string{"http-signer": hex.EncodeToString(pub)},
		RegistryHTTPClient: registryServer.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}

	apiServer := httptest.NewServer(New(rt))
	return registryServer, rt, apiServer
}

func TestRegistryHTTPAPIEndpoints(t *testing.T) {
	pluginID := "com.example.http-registry"
	regServer, rt, apiServer := setupRegistryServerAndRuntime(t, pluginID)
	defer regServer.Close()
	defer apiServer.Close()
	defer func() { _ = rt.Close() }()

	client := apiServer.Client()

	// 1. POST /api/v1/registry/sync
	syncRes := requestJSON(t, client, http.MethodPost, apiServer.URL+"/api/v1/registry/sync", nil, http.StatusOK)
	if syncRes["synced"] != true || syncRes["count"] != float64(2) {
		t.Fatalf("unexpected sync response: %+v", syncRes)
	}

	// 2. POST /api/v1/registry/plugins/install - validation failure
	_ = requestJSON(t, client, http.MethodPost, apiServer.URL+"/api/v1/registry/plugins/install", map[string]any{}, http.StatusBadRequest)

	// 3. POST /api/v1/registry/plugins/install - unknown plugin
	_ = requestJSON(t, client, http.MethodPost, apiServer.URL+"/api/v1/registry/plugins/install", map[string]any{
		"pluginId": "com.example.unknown",
		"version":  "1.0.0",
	}, http.StatusNotFound)

	// 4. POST /api/v1/registry/plugins/install - success (1-click install without extra friction)
	installRes := requestJSON(t, client, http.MethodPost, apiServer.URL+"/api/v1/registry/plugins/install", map[string]any{
		"pluginId": pluginID,
		"version":  "1.0.0",
	}, http.StatusOK)
	instObj, ok := installRes["installation"].(map[string]any)
	if !ok {
		t.Fatalf("expected installation in response: %+v", installRes)
	}
	if instObj["pluginId"] != pluginID || instObj["packageVersion"] != "1.0.0" || instObj["sourceKind"] != "registry" {
		t.Fatalf("unexpected installation response: %+v", instObj)
	}
	installationID := instObj["installationId"].(string)

	// 5. GET /api/v1/registry/updates
	updatesRes := requestJSON(t, client, http.MethodGet, apiServer.URL+"/api/v1/registry/updates", nil, http.StatusOK)
	updates, ok := updatesRes["updates"].([]any)
	if !ok || len(updates) != 1 {
		t.Fatalf("expected 1 update, got: %+v", updatesRes)
	}
	updateItem := updates[0].(map[string]any)
	if updateItem["targetVersion"] != "2.0.0" || updateItem["canUpgrade"] != true {
		t.Fatalf("unexpected update item: %+v", updateItem)
	}

	// 6. POST /api/v1/registry/plugins/upgrade - validation failure
	_ = requestJSON(t, client, http.MethodPost, apiServer.URL+"/api/v1/registry/plugins/upgrade", map[string]any{}, http.StatusBadRequest)

	// 7. POST /api/v1/registry/plugins/upgrade - success (in-place upgrade when usageCount == 0)
	upgradeRes := requestJSON(t, client, http.MethodPost, apiServer.URL+"/api/v1/registry/plugins/upgrade", map[string]any{
		"installationId": installationID,
		"version":        "2.0.0",
	}, http.StatusOK)
	upgradedObj := upgradeRes["installation"].(map[string]any)
	if upgradedObj["packageVersion"] != "2.0.0" {
		t.Fatalf("expected upgraded version 2.0.0, got: %+v", upgradedObj)
	}

	// 8. Option B enforcement: when usageCount > 0, upgrade returns 409 Conflict
	ctx := context.Background()
	parsedInstID, _ := platformplugin.ParseInstallationID(installationID)
	if err := rt.RecordProviderUsageForTest(ctx, parsedInstID); err != nil {
		t.Fatalf("record provider usage: %v", err)
	}

	// Attempt upgrade when usageCount > 0 must return 409 Conflict
	_ = requestJSON(t, client, http.MethodPost, apiServer.URL+"/api/v1/registry/plugins/upgrade", map[string]any{
		"installationId": installationID,
		"version":        "1.0.0",
	}, http.StatusConflict)
}
