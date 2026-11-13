package infrastructure

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/orz-i/Mender/backend/internal/plugin/manifest"
)

// PackageVerifier inspects and validates an extracted package staging directory.
type PackageVerifier struct {
	goos   string
	goarch string
}

// NewPackageVerifier creates a PackageVerifier configured for the host or target platform.
func NewPackageVerifier(goos, goarch string) *PackageVerifier {
	return &PackageVerifier{
		goos:   strings.TrimSpace(goos),
		goarch: strings.TrimSpace(goarch),
	}
}

// VerifyStagedPackage verifies the presence and schema of manifest v2 and platform entrypoint.
func (v *PackageVerifier) VerifyStagedPackage(
	ctx context.Context,
	stagingDir string,
	expectedPluginID string,
	expectedVersion string,
) (manifest.Manifest, []byte, error) {
	manifestPath := filepath.Join(stagingDir, manifest.FileName)
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		return manifest.Manifest{}, nil, fmt.Errorf("verifier: read %s: %w", manifest.FileName, err)
	}

	parsed, err := manifest.Parse(manifestBytes)
	if err != nil {
		return manifest.Manifest{}, nil, fmt.Errorf("verifier: parse manifest: %w", err)
	}

	if parsed.PluginID != expectedPluginID {
		return manifest.Manifest{}, nil, fmt.Errorf("verifier: manifest pluginId %q does not match expected %q", parsed.PluginID, expectedPluginID)
	}
	if parsed.Version != expectedVersion {
		return manifest.Manifest{}, nil, fmt.Errorf("verifier: manifest version %q does not match expected %q", parsed.Version, expectedVersion)
	}

	// Verify platform entrypoint
	entrypoint, err := parsed.Entrypoint(v.goos, v.goarch)
	if err != nil {
		return manifest.Manifest{}, nil, fmt.Errorf("verifier: platform entrypoint for %s/%s: %w", v.goos, v.goarch, err)
	}

	entrypointPath := filepath.Join(stagingDir, filepath.FromSlash(entrypoint))
	info, err := os.Lstat(entrypointPath)
	if err != nil {
		return manifest.Manifest{}, nil, fmt.Errorf("verifier: entrypoint %s: %w", entrypoint, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return manifest.Manifest{}, nil, fmt.Errorf("verifier: entrypoint %s must be a regular file", entrypoint)
	}

	if v.goos != "windows" && info.Mode().Perm()&0o111 == 0 {
		return manifest.Manifest{}, nil, fmt.Errorf("verifier: entrypoint %s is not executable", entrypoint)
	}

	return parsed, manifestBytes, nil
}
