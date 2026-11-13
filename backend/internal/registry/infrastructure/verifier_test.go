package infrastructure

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestVerifyStagedPackageSuccess(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	manifestJSON := []byte(`{
		"manifestVersion": 2,
		"pluginId": "dev.example.echo",
		"version": "1.2.3",
		"displayName": "Echo",
		"description": "Echoes messages",
		"entrypoints": {
			"` + runtime.GOOS + `/` + runtime.GOARCH + `": "bin/echo"
		},
		"extensions": [{"id": "main", "kind": "provider.tools", "protocolVersion": 1}]
	}`)

	if err := os.WriteFile(filepath.Join(dir, "mender-plugin.json"), manifestJSON, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bin", "echo"), []byte(`binary`), 0o755); err != nil {
		t.Fatal(err)
	}

	verifier := NewPackageVerifier(runtime.GOOS, runtime.GOARCH)
	parsed, raw, err := verifier.VerifyStagedPackage(context.Background(), dir, "dev.example.echo", "1.2.3")
	if err != nil {
		t.Fatalf("VerifyStagedPackage() error = %v", err)
	}
	if parsed.PluginID != "dev.example.echo" || parsed.Version != "1.2.3" {
		t.Fatalf("unexpected parsed manifest: %+v", parsed)
	}
	if len(raw) == 0 {
		t.Fatal("empty manifest bytes returned")
	}

	// Mismatched plugin ID
	if _, _, err := verifier.VerifyStagedPackage(context.Background(), dir, "other.plugin", "1.2.3"); err == nil || !strings.Contains(err.Error(), "does not match expected") {
		t.Fatalf("expected pluginId mismatch error, got: %v", err)
	}

	// Mismatched version
	if _, _, err := verifier.VerifyStagedPackage(context.Background(), dir, "dev.example.echo", "2.0.0"); err == nil || !strings.Contains(err.Error(), "does not match expected") {
		t.Fatalf("expected version mismatch error, got: %v", err)
	}
}

func TestVerifyStagedPackageMissingEntrypoint(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	manifestJSON := []byte(`{
		"manifestVersion": 2,
		"pluginId": "dev.example.echo",
		"version": "1.2.3",
		"displayName": "Echo",
		"entrypoints": {
			"` + runtime.GOOS + `/` + runtime.GOARCH + `": "bin/missing"
		},
		"extensions": [{"id": "main", "kind": "provider.tools", "protocolVersion": 1}]
	}`)

	if err := os.WriteFile(filepath.Join(dir, "mender-plugin.json"), manifestJSON, 0o644); err != nil {
		t.Fatal(err)
	}

	verifier := NewPackageVerifier(runtime.GOOS, runtime.GOARCH)
	if _, _, err := verifier.VerifyStagedPackage(context.Background(), dir, "dev.example.echo", "1.2.3"); err == nil {
		t.Fatal("expected error for missing entrypoint file")
	}
}
