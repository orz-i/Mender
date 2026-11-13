package infrastructure

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCalculateTreeDigestDeterministic(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "mender-plugin.json"), []byte(`{"v":2}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bin", "echo"), []byte(`binary-data`), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	digest1, err := CalculateTreeDigest(ctx, dir)
	if err != nil {
		t.Fatalf("CalculateTreeDigest() error = %v", err)
	}
	if !strings.HasPrefix(digest1, "sha256:") {
		t.Fatalf("digest missing sha256: prefix: %s", digest1)
	}

	digest2, err := CalculateTreeDigest(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if digest1 != digest2 {
		t.Fatalf("digest not deterministic: %s vs %s", digest1, digest2)
	}

	// Verify matches
	if err := VerifyTreeDigest(ctx, dir, digest1); err != nil {
		t.Fatalf("VerifyTreeDigest() error = %v", err)
	}

	// Tamper with file
	if err := os.WriteFile(filepath.Join(dir, "bin", "echo"), []byte(`tampered-data`), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := VerifyTreeDigest(ctx, dir, digest1); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("expected digest mismatch error, got: %v", err)
	}
}
