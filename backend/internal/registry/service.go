package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	installationapplication "github.com/orz-i/Mender/backend/internal/plugin/installation/application"
	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
	"github.com/orz-i/Mender/backend/internal/registry/domain"
	"github.com/orz-i/Mender/backend/internal/registry/infrastructure"
	"golang.org/x/mod/semver"
)

// AvailableUpdate describes an update available in the registry for an installed plugin.
type AvailableUpdate struct {
	InstallationID string `json:"installationId"`
	PluginID       string `json:"pluginId"`
	CurrentVersion string `json:"currentVersion"`
	TargetVersion  string `json:"targetVersion"`
	CanUpgrade     bool   `json:"canUpgrade"`
	UsageCount     int    `json:"usageCount"`
}

// ServiceConfig configures the registry service.
type ServiceConfig struct {
	DefaultRegistryID  string
	DefaultEndpointURL string
	StagingDir         string
	GOOS               string
	GOARCH             string
}

// Service orchestrates registry metadata synchronization, online package download, verification, and in-place upgrade.
type Service struct {
	config       ServiceConfig
	trustStore   *domain.TrustStore
	indexStore   *infrastructure.IndexStore
	downloader   *infrastructure.Downloader
	extractor    *infrastructure.Extractor
	verifier     *infrastructure.PackageVerifier
	repository   installationapplication.Repository
	packageStore installationapplication.PackageStore
	idGenerator  installationapplication.IDGenerator
	usageChecker installationapplication.UsageChecker
	clock        installationapplication.Clock
}

// NewService constructs a fully-wired RegistryService.
func NewService(
	cfg ServiceConfig,
	trustStore *domain.TrustStore,
	indexStore *infrastructure.IndexStore,
	downloader *infrastructure.Downloader,
	extractor *infrastructure.Extractor,
	verifier *infrastructure.PackageVerifier,
	repository installationapplication.Repository,
	packageStore installationapplication.PackageStore,
	idGenerator installationapplication.IDGenerator,
	usageChecker installationapplication.UsageChecker,
	clock installationapplication.Clock,
) *Service {
	if cfg.DefaultRegistryID == "" {
		cfg.DefaultRegistryID = "official"
	}
	if cfg.GOOS == "" {
		cfg.GOOS = runtime.GOOS
	}
	if cfg.GOARCH == "" {
		cfg.GOARCH = runtime.GOARCH
	}
	if verifier == nil {
		verifier = infrastructure.NewPackageVerifier(cfg.GOOS, cfg.GOARCH)
	}
	if extractor == nil {
		extractor = infrastructure.NewExtractor()
	}
	if downloader == nil {
		downloader = infrastructure.NewDownloader(nil)
	}

	return &Service{
		config:       cfg,
		trustStore:   trustStore,
		indexStore:   indexStore,
		downloader:   downloader,
		extractor:    extractor,
		verifier:     verifier,
		repository:   repository,
		packageStore: packageStore,
		idGenerator:  idGenerator,
		usageChecker: usageChecker,
		clock:        clock,
	}
}

// TrustStore returns the active trust store.
func (s *Service) TrustStore() *domain.TrustStore {
	return s.trustStore
}

// IndexStore returns the active SQLite index store.
func (s *Service) IndexStore() *infrastructure.IndexStore {
	return s.indexStore
}

// Sync pulls the remote signed metadata index from endpointURL, validates anti-freeze, anti-rollback,
// verifies Ed25519 signature against trust store, and caches it in SQLite.
func (s *Service) Sync(ctx context.Context, endpointURL string) (*domain.CatalogIndex, error) {
	if endpointURL == "" {
		endpointURL = s.config.DefaultEndpointURL
	}
	if endpointURL == "" {
		return nil, fmt.Errorf("registry sync: endpoint URL required")
	}

	raw, err := s.downloader.DownloadMetadata(ctx, endpointURL)
	if err != nil {
		return nil, fmt.Errorf("registry sync: download metadata: %w", err)
	}

	var envelope domain.SignatureEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("registry sync: decode envelope: %w", err)
	}

	// 1. Anti-freeze validation (issuedAt <= now <= expiresAt)
	now := time.Now().UTC()
	if s.clock != nil {
		now = s.clock.Now().UTC()
	}
	if err := envelope.Payload.ValidateFreshness(now); err != nil {
		return nil, fmt.Errorf("registry sync: %w", err)
	}

	// 2. Anti-rollback validation (monotonic sequence check)
	lastSeq, err := s.indexStore.GetLatestSequence(ctx, s.config.DefaultRegistryID)
	if err != nil {
		return nil, fmt.Errorf("registry sync: check previous sequence: %w", err)
	}
	if err := envelope.Payload.ValidateSequence(lastSeq); err != nil {
		return nil, fmt.Errorf("registry sync: %w", err)
	}

	// 3. Ed25519 signature verification against TrustStore (Option A)
	if err := s.trustStore.Verify(envelope); err != nil {
		return nil, fmt.Errorf("registry sync: signature verification: %w", err)
	}

	// 4. Persist validated envelope into SQLite cache
	if err := s.indexStore.SaveEnvelope(ctx, s.config.DefaultRegistryID, envelope); err != nil {
		return nil, fmt.Errorf("registry sync: persist cache: %w", err)
	}

	return &envelope.Payload, nil
}

// ListOfferings returns all plugin offerings currently available in the cached registry index.
func (s *Service) ListOfferings(ctx context.Context) ([]domain.RegistryPluginEntry, error) {
	return s.indexStore.ListPlugins(ctx, s.config.DefaultRegistryID)
}

// Install downloads, validates, and installs a plugin directly from the registry index.
func (s *Service) Install(ctx context.Context, pluginID, version string) (installationdomain.Installation, error) {
	parsedPluginID, err := platformplugin.ParseID(pluginID)
	if err != nil {
		return installationdomain.Installation{}, fmt.Errorf("registry install: %w", err)
	}

	entry, err := s.indexStore.GetPluginVersion(ctx, s.config.DefaultRegistryID, pluginID, version)
	if err != nil {
		return installationdomain.Installation{}, fmt.Errorf("registry install: lookup plugin %s@%s: %w", pluginID, version, err)
	}

	target, err := entry.Target(s.config.GOOS, s.config.GOARCH)
	if err != nil {
		return installationdomain.Installation{}, fmt.Errorf("registry install: target: %w", err)
	}

	// Create temporary staging directory for download & verification
	stagingParent := s.config.StagingDir
	if stagingParent == "" {
		stagingParent = os.TempDir()
	}
	_ = os.MkdirAll(stagingParent, 0o700)
	stagingDir, err := os.MkdirTemp(stagingParent, "mender-registry-stage-")
	if err != nil {
		return installationdomain.Installation{}, fmt.Errorf("registry install: create staging dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(stagingDir) }()

	// 1. Streaming download with 256MB cutoff and SHA-256 verification
	archivePath, err := s.downloader.DownloadArchive(ctx, target.URL, target.SHA256, stagingDir)
	if err != nil {
		return installationdomain.Installation{}, fmt.Errorf("registry install: download: %w", err)
	}
	defer func() { _ = os.Remove(archivePath) }()

	// 2. Zip-Slip & path-traversal proof extraction into extracted/ subdir
	extractedDir := filepath.Join(stagingDir, "package")
	if err := os.MkdirAll(extractedDir, 0o700); err != nil {
		return installationdomain.Installation{}, fmt.Errorf("registry install: create package dir: %w", err)
	}
	if err := s.extractor.ExtractTarGz(ctx, archivePath, extractedDir); err != nil {
		return installationdomain.Installation{}, fmt.Errorf("registry install: extract: %w", err)
	}

	// 3. Tree digest calculation & verification with "mender-plugin-package-tree-v2\x00" preamble
	if err := infrastructure.VerifyTreeDigest(ctx, extractedDir, target.TreeDigest); err != nil {
		return installationdomain.Installation{}, fmt.Errorf("registry install: %w", err)
	}

	// 4. Manifest v2 parsing & platform entrypoint validation
	parsedManifest, manifestBytes, err := s.verifier.VerifyStagedPackage(ctx, extractedDir, pluginID, version)
	if err != nil {
		return installationdomain.Installation{}, fmt.Errorf("registry install: %w", err)
	}

	// 5. Allocate stable installation ID
	id, err := s.idGenerator.NewInstallationID()
	if err != nil {
		return installationdomain.Installation{}, fmt.Errorf("registry install: allocate id: %w", err)
	}

	// 6. Install into managed root
	managedPkg, err := s.packageStore.InstallLocal(ctx, extractedDir, id)
	if err != nil {
		return installationdomain.Installation{}, fmt.Errorf("registry install: copy package: %w", err)
	}

	now := time.Now().UTC()
	if s.clock != nil {
		now = s.clock.Now().UTC()
	}

	installation, err := installationdomain.NewInstallation(
		id,
		parsedPluginID,
		parsedManifest.Version,
		installationdomain.SourceRegistry,
		target.URL,
		managedPkg.Digest,
		manifestBytes,
		installationdomain.StatusEnabled,
		now,
	)
	if err != nil {
		_ = s.packageStore.Remove(context.WithoutCancel(ctx), id)
		return installationdomain.Installation{}, fmt.Errorf("registry install: metadata: %w", err)
	}

	if err := s.repository.Create(ctx, installation); err != nil {
		_ = s.packageStore.Remove(context.WithoutCancel(ctx), id)
		return installationdomain.Installation{}, fmt.Errorf("registry install: persist metadata: %w", err)
	}

	return installation, nil
}

// Upgrade performs an in-place upgrade of an existing installation (Option B).
// Enforces usageCount == 0 per user decision 2.
func (s *Service) Upgrade(
	ctx context.Context,
	id installationdomain.InstallationID,
	targetVersion string,
) (installationdomain.Installation, error) {
	// Option B check: Enforce usageCount == 0
	if s.usageChecker != nil {
		usages, err := s.usageChecker.Usages(ctx, id)
		if err != nil {
			return installationdomain.Installation{}, fmt.Errorf("registry upgrade: check usage: %w", err)
		}
		if len(usages) != 0 {
			return installationdomain.Installation{}, fmt.Errorf(
				"%w: %s (in-place upgrade requires usageCount == 0)",
				domain.ErrInstallationInUse,
				usages[0].Description,
			)
		}
	}

	existing, err := s.repository.Get(ctx, id)
	if err != nil {
		return installationdomain.Installation{}, fmt.Errorf("registry upgrade: load installation: %w", err)
	}

	if existing.PackageVersion() == targetVersion {
		return installationdomain.Installation{}, fmt.Errorf("registry upgrade: target version %s is already installed", targetVersion)
	}

	entry, err := s.indexStore.GetPluginVersion(ctx, s.config.DefaultRegistryID, existing.PluginID().String(), targetVersion)
	if err != nil {
		return installationdomain.Installation{}, fmt.Errorf("registry upgrade: lookup plugin %s@%s: %w", existing.PluginID(), targetVersion, err)
	}

	target, err := entry.Target(s.config.GOOS, s.config.GOARCH)
	if err != nil {
		return installationdomain.Installation{}, fmt.Errorf("registry upgrade: target: %w", err)
	}

	stagingParent := s.config.StagingDir
	if stagingParent == "" {
		stagingParent = os.TempDir()
	}
	_ = os.MkdirAll(stagingParent, 0o700)
	stagingDir, err := os.MkdirTemp(stagingParent, "mender-registry-upgrade-")
	if err != nil {
		return installationdomain.Installation{}, fmt.Errorf("registry upgrade: create staging dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(stagingDir) }()

	// 1. Download target archive
	archivePath, err := s.downloader.DownloadArchive(ctx, target.URL, target.SHA256, stagingDir)
	if err != nil {
		return installationdomain.Installation{}, fmt.Errorf("registry upgrade: download: %w", err)
	}
	defer func() { _ = os.Remove(archivePath) }()

	// 2. Extract into staging package dir
	extractedDir := filepath.Join(stagingDir, "package")
	if err := os.MkdirAll(extractedDir, 0o700); err != nil {
		return installationdomain.Installation{}, fmt.Errorf("registry upgrade: create package dir: %w", err)
	}
	if err := s.extractor.ExtractTarGz(ctx, archivePath, extractedDir); err != nil {
		return installationdomain.Installation{}, fmt.Errorf("registry upgrade: extract: %w", err)
	}

	// 3. Tree digest validation
	if err := infrastructure.VerifyTreeDigest(ctx, extractedDir, target.TreeDigest); err != nil {
		return installationdomain.Installation{}, fmt.Errorf("registry upgrade: %w", err)
	}

	// 4. Manifest v2 & entrypoint verification
	_, manifestBytes, err := s.verifier.VerifyStagedPackage(ctx, extractedDir, existing.PluginID().String(), targetVersion)
	if err != nil {
		return installationdomain.Installation{}, fmt.Errorf("registry upgrade: %w", err)
	}

	// 5. Atomic in-place replacement of package files
	managedPkg, err := s.packageStore.UpgradeLocal(ctx, extractedDir, id)
	if err != nil {
		return installationdomain.Installation{}, fmt.Errorf("registry upgrade: replace package files: %w", err)
	}

	// 6. Update installation metadata record
	upgraded, err := existing.WithUpgradedPackage(targetVersion, managedPkg.Digest, manifestBytes)
	if err != nil {
		return installationdomain.Installation{}, fmt.Errorf("registry upgrade: update metadata: %w", err)
	}

	if err := s.repository.UpdatePackage(ctx, upgraded); err != nil {
		return installationdomain.Installation{}, fmt.Errorf("registry upgrade: persist updated metadata: %w", err)
	}

	return upgraded, nil
}

// CheckUpdates inspects a slice of installations and returns all available upgrades found in the registry cache.
func (s *Service) CheckUpdates(
	ctx context.Context,
	installations []installationdomain.Installation,
) ([]AvailableUpdate, error) {
	updates := make([]AvailableUpdate, 0)

	for _, inst := range installations {
		latest, err := s.indexStore.GetPlugin(ctx, s.config.DefaultRegistryID, inst.PluginID().String())
		if err != nil {
			if errors.Is(err, domain.ErrPluginNotFound) {
				continue
			}
			return nil, fmt.Errorf("check updates for %s: %w", inst.PluginID(), err)
		}

		currentV := inst.PackageVersion()
		targetV := latest.Version
		if !strings.HasPrefix(currentV, "v") {
			currentV = "v" + currentV
		}
		if !strings.HasPrefix(targetV, "v") {
			targetV = "v" + targetV
		}

		if semver.Compare(targetV, currentV) > 0 {
			usageCount := 0
			if s.usageChecker != nil {
				usages, err := s.usageChecker.Usages(ctx, inst.ID())
				if err == nil {
					usageCount = len(usages)
				}
			}

			updates = append(updates, AvailableUpdate{
				InstallationID: inst.ID().String(),
				PluginID:       inst.PluginID().String(),
				CurrentVersion: inst.PackageVersion(),
				TargetVersion:  latest.Version,
				CanUpgrade:     usageCount == 0,
				UsageCount:     usageCount,
			})
		}
	}

	return updates, nil
}
