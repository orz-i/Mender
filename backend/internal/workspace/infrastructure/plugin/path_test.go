package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspacePathResolverRejectsTraversalAbsoluteAndSymlinks(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "safe", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "safe", "file.txt"), []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolver, err := newWorkspacePathResolver(root)
	if err != nil {
		t.Fatal(err)
	}

	for _, value := range []string{"../escape", "/absolute", "safe/../../escape"} {
		if _, _, _, err := resolver.resolveExisting(value, false); err == nil {
			t.Fatalf("resolveExisting(%q) accepted escaping path", value)
		}
	}

	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "outside-link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, _, _, err := resolver.resolveExisting("outside-link/secret.txt", false); err == nil {
		t.Fatal("resolveExisting() followed symlink component")
	}
}

func TestWorkspacePathResolverAllowsMissingCreateTargetOnly(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	resolver, err := newWorkspacePathResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	relative, path, err := resolver.resolveCreate("dir/new.txt")
	if err != nil {
		t.Fatalf("resolveCreate() error = %v", err)
	}
	if relative != "dir/new.txt" || path != filepath.Join(root, "dir", "new.txt") {
		t.Fatalf("resolveCreate() = %q %q", relative, path)
	}
	if _, _, err := resolver.resolveCreate("missing/new.txt"); err == nil {
		t.Fatal("resolveCreate() accepted missing parent directory")
	}
}
