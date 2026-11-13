package registry

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

	installationapplication "github.com/orz-i/Mender/backend/internal/plugin/installation/application"
	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
	"github.com/orz-i/Mender/backend/internal/registry/domain"
	"github.com/orz-i/Mender/backend/internal/registry/infrastructure"
)

type testEnvironment struct {
	service      *Service
	trustStore   *domain.TrustStore
	privKey      ed25519.PrivateKey
	pubKey       ed25519.PublicKey
	repo         *memRepo
	pkgStore     *memPkgStore
	usageChecker *memUsageChecker
	indexStore   *infrastructure.IndexStore
	managedRoot  string
	server       *httptest.Server
	archiveMap   map[string][]byte
}

func setupTestEnvironment(t *testing.T) *testEnvironment {
	t.Helper()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	trustStore := domain.NewTrustStore(pub, nil)
	trustStore.AddKey("official-root", pub)

	ctx := context.Background()
	indexStore, err := infrastructure.OpenIndexStore(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}

	managedRoot := t.TempDir()
	archiveMap := make(map[string][]byte)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, ok := archiveMap[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(data)
	}))

	downloader := infrastructure.NewDownloader(server.Client())
	extractor := infrastructure.NewExtractor()
	verifier := infrastructure.NewPackageVerifier(runtime.GOOS, runtime.GOARCH)

	repo := newMemRepo()
	pkgStore := newMemPkgStore(managedRoot)
	usageChecker := &memUsageChecker{usages: make(map[string][]installationapplication.Usage)}

	svc := NewService(
		ServiceConfig{
			DefaultRegistryID:  "official",
			DefaultEndpointURL: server.URL + "/index.json",
			StagingDir:         t.TempDir(),
			GOOS:               runtime.GOOS,
			GOARCH:             runtime.GOARCH,
		},
		trustStore,
		indexStore,
		downloader,
		extractor,
		verifier,
		repo,
		pkgStore,
		&memIDGen{counter: 1},
		usageChecker,
		&memClock{now: time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)},
	)

	return &testEnvironment{
		service:      svc,
		trustStore:   trustStore,
		privKey:      priv,
		pubKey:       pub,
		repo:         repo,
		pkgStore:     pkgStore,
		usageChecker: usageChecker,
		indexStore:   indexStore,
		managedRoot:  managedRoot,
		server:       server,
		archiveMap:   archiveMap,
	}
}

func createPackageArchive(t *testing.T, pluginID, version string) ([]byte, string) {
	t.Helper()

	manifestJSON := []byte(`{
		"manifestVersion": 2,
		"pluginId": "` + pluginID + `",
		"version": "` + version + `",
		"displayName": "Echo",
		"description": "Echo description",
		"entrypoints": {
			"` + runtime.GOOS + `/` + runtime.GOARCH + `": "bin/echo"
		},
		"extensions": [{"id": "main", "kind": "provider.tools", "protocolVersion": 1}]
	}`)

	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "mender-plugin.json"), manifestJSON, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tempDir, "bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "bin", "echo"), []byte(`bin-`+version), 0o755); err != nil {
		t.Fatal(err)
	}

	digest, err := infrastructure.CalculateTreeDigest(context.Background(), tempDir)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	// Write manifest
	_ = tw.WriteHeader(&tar.Header{
		Name:     "mender-plugin.json",
		Mode:     0o644,
		Size:     int64(len(manifestJSON)),
		Typeflag: tar.TypeReg,
	})
	_, _ = tw.Write(manifestJSON)

	// Write bin/echo
	binContent := []byte(`bin-` + version)
	_ = tw.WriteHeader(&tar.Header{
		Name:     "bin/echo",
		Mode:     0o755,
		Size:     int64(len(binContent)),
		Typeflag: tar.TypeReg,
	})
	_, _ = tw.Write(binContent)

	_ = tw.Close()
	_ = gw.Close()

	return buf.Bytes(), digest
}

func TestSyncAndInstallFlow(t *testing.T) {
	t.Parallel()

	env := setupTestEnvironment(t)
	defer env.server.Close()
	defer func() { _ = env.indexStore.Close() }()

	v1Archive, v1Digest := createPackageArchive(t, "dev.example.echo", "1.0.0")
	v1SHA256 := sha256.Sum256(v1Archive)
	v1SHAHex := hex.EncodeToString(v1SHA256[:])

	env.archiveMap["/echo-1.0.0.tar.gz"] = v1Archive

	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	index := domain.CatalogIndex{
		SchemaVersion: 1,
		Sequence:      1,
		IssuedAt:      now.Add(-1 * time.Hour),
		ExpiresAt:     now.Add(24 * time.Hour),
		Plugins: []domain.RegistryPluginEntry{
			{
				PluginID:    "dev.example.echo",
				Version:     "1.0.0",
				DisplayName: "Echo",
				Description: "Echo plugin",
				Targets: map[string]domain.ArtifactTarget{
					runtime.GOOS + "/" + runtime.GOARCH: {
						URL:        env.server.URL + "/echo-1.0.0.tar.gz",
						SHA256:     "sha256:" + v1SHAHex,
						TreeDigest: v1Digest,
						Size:       int64(len(v1Archive)),
					},
				},
			},
		},
	}

	envelope, err := domain.SignIndex(index, "official-root", env.privKey)
	if err != nil {
		t.Fatal(err)
	}

	envelopeJSON, _ := json.Marshal(envelope)
	env.archiveMap["/index.json"] = envelopeJSON

	// 1. Sync
	ctx := context.Background()
	synced, err := env.service.Sync(ctx, env.server.URL+"/index.json")
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if synced.Sequence != 1 {
		t.Fatalf("synced sequence = %d, want 1", synced.Sequence)
	}

	// 2. Install
	inst, err := env.service.Install(ctx, "dev.example.echo", "1.0.0")
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if inst.SourceKind() != installationdomain.SourceRegistry {
		t.Fatalf("got sourceKind %q, want %q", inst.SourceKind(), installationdomain.SourceRegistry)
	}
	if inst.PackageVersion() != "1.0.0" {
		t.Fatalf("got version %q, want 1.0.0", inst.PackageVersion())
	}
	if inst.Digest() != v1Digest {
		t.Fatalf("got digest %q, want %q", inst.Digest(), v1Digest)
	}
}

func TestOptionBUpgradePolicy(t *testing.T) {
	t.Parallel()

	env := setupTestEnvironment(t)
	defer env.server.Close()
	defer func() { _ = env.indexStore.Close() }()

	v1Archive, v1Digest := createPackageArchive(t, "dev.example.echo", "1.0.0")
	v1SHA := hex.EncodeToString(func() []byte { h := sha256.Sum256(v1Archive); return h[:] }())
	env.archiveMap["/echo-1.0.0.tar.gz"] = v1Archive

	v2Archive, v2Digest := createPackageArchive(t, "dev.example.echo", "2.0.0")
	v2SHA := hex.EncodeToString(func() []byte { h := sha256.Sum256(v2Archive); return h[:] }())
	env.archiveMap["/echo-2.0.0.tar.gz"] = v2Archive

	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	index := domain.CatalogIndex{
		SchemaVersion: 1,
		Sequence:      2,
		IssuedAt:      now.Add(-1 * time.Hour),
		ExpiresAt:     now.Add(24 * time.Hour),
		Plugins: []domain.RegistryPluginEntry{
			{
				PluginID:    "dev.example.echo",
				Version:     "1.0.0",
				DisplayName: "Echo",
				Targets: map[string]domain.ArtifactTarget{
					runtime.GOOS + "/" + runtime.GOARCH: {
						URL:        env.server.URL + "/echo-1.0.0.tar.gz",
						SHA256:     v1SHA,
						TreeDigest: v1Digest,
						Size:       int64(len(v1Archive)),
					},
				},
			},
			{
				PluginID:    "dev.example.echo",
				Version:     "2.0.0",
				DisplayName: "Echo",
				Targets: map[string]domain.ArtifactTarget{
					runtime.GOOS + "/" + runtime.GOARCH: {
						URL:        env.server.URL + "/echo-2.0.0.tar.gz",
						SHA256:     v2SHA,
						TreeDigest: v2Digest,
						Size:       int64(len(v2Archive)),
					},
				},
			},
		},
	}

	envelope, _ := domain.SignIndex(index, "official-root", env.privKey)
	envelopeJSON, _ := json.Marshal(envelope)
	env.archiveMap["/index.json"] = envelopeJSON

	ctx := context.Background()
	if _, err := env.service.Sync(ctx, env.server.URL+"/index.json"); err != nil {
		t.Fatal(err)
	}

	// Install v1.0.0
	inst, err := env.service.Install(ctx, "dev.example.echo", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}

	// Case 1: usageCount > 0 -> in-place upgrade MUST be blocked (Option B)
	env.usageChecker.usages[inst.ID().String()] = []installationapplication.Usage{
		{Kind: "provider", Description: "Used by active Provider 'echo-prov'"},
	}

	// Check updates reports CanUpgrade = false
	updates, err := env.service.CheckUpdates(ctx, []installationdomain.Installation{inst})
	if err != nil {
		t.Fatalf("CheckUpdates() error = %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("got %d updates, want 1", len(updates))
	}
	if updates[0].CanUpgrade {
		t.Fatal("expected CanUpgrade to be false when usageCount > 0")
	}
	if updates[0].UsageCount != 1 {
		t.Fatalf("got UsageCount = %d, want 1", updates[0].UsageCount)
	}

	// Attempting Upgrade when usageCount > 0 must return error
	_, err = env.service.Upgrade(ctx, inst.ID(), "2.0.0")
	if err == nil || !errors.Is(err, domain.ErrInstallationInUse) {
		t.Fatalf("expected ErrInstallationInUse, got: %v", err)
	}

	// Case 2: usageCount == 0 -> in-place upgrade allowed!
	delete(env.usageChecker.usages, inst.ID().String())

	updates, err = env.service.CheckUpdates(ctx, []installationdomain.Installation{inst})
	if err != nil {
		t.Fatal(err)
	}
	if !updates[0].CanUpgrade {
		t.Fatal("expected CanUpgrade to be true when usageCount == 0")
	}

	upgraded, err := env.service.Upgrade(ctx, inst.ID(), "2.0.0")
	if err != nil {
		t.Fatalf("Upgrade() error = %v", err)
	}
	if upgraded.PackageVersion() != "2.0.0" {
		t.Fatalf("upgraded version = %q, want 2.0.0", upgraded.PackageVersion())
	}
	if upgraded.Digest() != v2Digest {
		t.Fatalf("upgraded digest = %q, want %q", upgraded.Digest(), v2Digest)
	}
	if upgraded.ID() != inst.ID() {
		t.Fatalf("installation ID changed during in-place upgrade")
	}
}

// In-memory mocks for test environment

type memRepo struct {
	items map[installationdomain.InstallationID]installationdomain.Installation
}

func newMemRepo() *memRepo {
	return &memRepo{items: make(map[installationdomain.InstallationID]installationdomain.Installation)}
}

func (r *memRepo) Create(_ context.Context, i installationdomain.Installation) error {
	r.items[i.ID()] = i
	return nil
}

func (r *memRepo) Get(_ context.Context, id installationdomain.InstallationID) (installationdomain.Installation, error) {
	it, ok := r.items[id]
	if !ok {
		return installationdomain.Installation{}, installationapplication.ErrInstallationNotFound
	}
	return it, nil
}

func (r *memRepo) GetByDigest(_ context.Context, digest string) (installationdomain.Installation, error) {
	for _, it := range r.items {
		if it.Digest() == digest {
			return it, nil
		}
	}
	return installationdomain.Installation{}, installationapplication.ErrInstallationNotFound
}

func (r *memRepo) List(_ context.Context) ([]installationdomain.Installation, error) {
	list := make([]installationdomain.Installation, 0, len(r.items))
	for _, it := range r.items {
		list = append(list, it)
	}
	return list, nil
}

func (r *memRepo) SetStatus(_ context.Context, id installationdomain.InstallationID, st installationdomain.Status) error {
	it, ok := r.items[id]
	if !ok {
		return installationapplication.ErrInstallationNotFound
	}
	updated, err := it.WithStatus(st)
	if err != nil {
		return err
	}
	r.items[id] = updated
	return nil
}

func (r *memRepo) Delete(_ context.Context, id installationdomain.InstallationID) error {
	delete(r.items, id)
	return nil
}

func (r *memRepo) UpdatePackage(_ context.Context, i installationdomain.Installation) error {
	if _, ok := r.items[i.ID()]; !ok {
		return installationapplication.ErrInstallationNotFound
	}
	r.items[i.ID()] = i
	return nil
}

type memPkgStore struct {
	root string
}

func newMemPkgStore(root string) *memPkgStore {
	return &memPkgStore{root: root}
}

func (p *memPkgStore) InstallLocal(ctx context.Context, source string, id installationdomain.InstallationID) (installationapplication.ManagedPackage, error) {
	dest := filepath.Join(p.root, id.String())
	_ = os.MkdirAll(dest, 0o700)
	digest, err := infrastructure.CalculateTreeDigest(ctx, source)
	if err != nil {
		return installationapplication.ManagedPackage{}, err
	}
	verifier := infrastructure.NewPackageVerifier(runtime.GOOS, runtime.GOARCH)
	parsed, raw, _ := verifier.VerifyStagedPackage(ctx, source, "dev.example.echo", "1.0.0")
	return installationapplication.ManagedPackage{
		Manifest:        parsed,
		ManifestJSON:    raw,
		Digest:          digest,
		SourceReference: source,
	}, nil
}

func (p *memPkgStore) UpgradeLocal(ctx context.Context, source string, id installationdomain.InstallationID) (installationapplication.ManagedPackage, error) {
	digest, err := infrastructure.CalculateTreeDigest(ctx, source)
	if err != nil {
		return installationapplication.ManagedPackage{}, err
	}
	verifier := infrastructure.NewPackageVerifier(runtime.GOOS, runtime.GOARCH)
	parsed, raw, _ := verifier.VerifyStagedPackage(ctx, source, "dev.example.echo", "2.0.0")
	return installationapplication.ManagedPackage{
		Manifest:        parsed,
		ManifestJSON:    raw,
		Digest:          digest,
		SourceReference: source,
	}, nil
}

func (p *memPkgStore) Resolve(_ context.Context, _ installationdomain.Installation) (installationapplication.ResolvedPackage, error) {
	return installationapplication.ResolvedPackage{}, nil
}

func (p *memPkgStore) Remove(_ context.Context, id installationdomain.InstallationID) error {
	_ = os.RemoveAll(filepath.Join(p.root, id.String()))
	return nil
}

type memIDGen struct {
	counter int
}

func (g *memIDGen) NewInstallationID() (installationdomain.InstallationID, error) {
	g.counter++
	return installationdomain.ParseInstallationID(fmt.Sprintf("pinst_000000000000000000000000000000%02d", g.counter))
}

type memUsageChecker struct {
	usages map[string][]installationapplication.Usage
}

func (c *memUsageChecker) Usages(_ context.Context, id installationdomain.InstallationID) ([]installationapplication.Usage, error) {
	return c.usages[id.String()], nil
}

type memClock struct {
	now time.Time
}

func (c *memClock) Now() time.Time {
	return c.now
}
