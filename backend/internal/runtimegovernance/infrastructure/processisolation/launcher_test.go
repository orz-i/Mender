package processisolation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	runtimegovernancedomain "github.com/orz-i/Mender/backend/internal/runtimegovernance/domain"
)

func TestRestrictedHostUnavailableFailsClosed(t *testing.T) {
	t.Parallel()
	launcher := newLauncher("windows", "", "C:\\Users\\test", "none")
	_, err := launcher.Command(
		context.Background(),
		runtimegovernancedomain.IsolationRestrictedHost,
		CommandSpec{
			Executable:       filepath.Join(string(filepath.Separator), "bin", "echo"),
			WorkingDirectory: t.TempDir(),
			PermittedRoot:    t.TempDir(),
		},
	)
	if err == nil {
		t.Fatal("restricted_host unexpectedly fell back to host")
	}
}

func TestHostCommandUsesExactExecutable(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	launcher := newLauncher(runtime.GOOS, "", root, "none")
	executable := "/bin/echo"
	if runtime.GOOS == "windows" {
		t.Skip("unix executable assertion")
	}
	cmd, err := launcher.Command(
		context.Background(),
		runtimegovernancedomain.IsolationHost,
		CommandSpec{
			Executable:       executable,
			Arguments:        []string{"hello"},
			WorkingDirectory: root,
			PermittedRoot:    root,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Path != executable {
		t.Fatalf("command path = %q, want %q", cmd.Path, executable)
	}
}

func TestDarwinRestrictedCommandUsesCanonicalProfile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	launcher := newLauncher("darwin", "/usr/bin/sandbox-exec", "/Users/test", "seatbelt")
	cmd, err := launcher.Command(
		context.Background(),
		runtimegovernancedomain.IsolationRestrictedHost,
		CommandSpec{
			Executable:       "/bin/sh",
			Arguments:        []string{"-c", "echo ok"},
			WorkingDirectory: root,
			PermittedRoot:    root,
			RootWritable:     true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Path != "/usr/bin/sandbox-exec" {
		t.Fatalf("command path = %q", cmd.Path)
	}
	canonicalRoot, err := canonicalPath(root)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(cmd.Args, "/bin/sh") ||
		len(cmd.Args) < 3 ||
		!strings.Contains(cmd.Args[2], canonicalRoot) {
		t.Fatalf("sandbox args = %#v", cmd.Args)
	}
}

func TestRestrictedHostReadOnlyRootWhenAvailable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("restricted_host backend is unsupported on Windows")
	}
	launcher := New()
	if !launcher.Info().RestrictedHostAvailable {
		t.Skip("restricted_host backend unavailable")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(home, ".mender-seatbelt-readonly-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(root) }()
	cmd, err := launcher.Command(
		context.Background(),
		runtimegovernancedomain.IsolationRestrictedHost,
		CommandSpec{
			Executable:       "/bin/sh",
			Arguments:        []string{"-c", "echo denied > should-not-exist.txt"},
			WorkingDirectory: root,
			PermittedRoot:    root,
			RootWritable:     false,
			Environment:      []string{"PATH=/usr/bin:/bin", "TMPDIR=/tmp"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Run(); err == nil {
		t.Fatal("restricted_host read-only root allowed a write")
	}
	if _, err := os.Stat(filepath.Join(root, "should-not-exist.txt")); !os.IsNotExist(err) {
		t.Fatalf("read-only root was mutated: %v", err)
	}
}

func TestRestrictedHostReadOnlyRootInsideTempWhenAvailable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("restricted_host backend is unsupported on Windows")
	}
	launcher := New()
	if !launcher.Info().RestrictedHostAvailable {
		t.Skip("restricted_host backend unavailable")
	}
	root := t.TempDir()
	cmd, err := launcher.Command(
		context.Background(),
		runtimegovernancedomain.IsolationRestrictedHost,
		CommandSpec{
			Executable:       "/bin/sh",
			Arguments:        []string{"-c", "echo denied > should-not-exist.txt"},
			WorkingDirectory: root,
			PermittedRoot:    root,
			RootWritable:     false,
			Environment:      []string{"PATH=/usr/bin:/bin", "TMPDIR=/tmp"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Run(); err == nil {
		t.Fatal("restricted_host temp-contained read-only root allowed a write")
	}
	if _, err := os.Stat(filepath.Join(root, "should-not-exist.txt")); !os.IsNotExist(err) {
		t.Fatalf("temp-contained read-only root was mutated: %v", err)
	}
}

func TestLinuxRestrictedCommandDoesNotMountHome(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	launcher := newLauncher("linux", "/usr/bin/bwrap", "/home/test", "bubblewrap")
	cmd, err := launcher.Command(
		context.Background(),
		runtimegovernancedomain.IsolationRestrictedHost,
		CommandSpec{
			Executable:       "/bin/sh",
			Arguments:        []string{"-c", "true"},
			WorkingDirectory: root,
			PermittedRoot:    root,
			RootWritable:     false,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	canonicalRoot, err := canonicalPath(root)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(cmd.Args, "\x00")
	if !strings.Contains(joined, "--ro-bind\x00"+canonicalRoot+"\x00"+canonicalRoot) {
		t.Fatalf("bubblewrap args do not bind permitted root read-only: %#v", cmd.Args)
	}
	tmpIndex := argumentPairIndex(cmd.Args, "--tmpfs", "/tmp")
	rootIndex := argumentPairIndex(cmd.Args, "--ro-bind", canonicalRoot)
	if tmpIndex < 0 || rootIndex < 0 || tmpIndex >= rootIndex {
		t.Fatalf("bubblewrap /tmp/root mount order is unsafe: %#v", cmd.Args)
	}
	if strings.Contains(joined, "/home/test") {
		t.Fatalf("bubblewrap args unexpectedly mount host home: %#v", cmd.Args)
	}
}

func argumentPairIndex(values []string, first, second string) int {
	for index := 0; index+1 < len(values); index++ {
		if values[index] == first && values[index+1] == second {
			return index
		}
	}
	return -1
}

func TestRestrictedHostFilesystemBoundaryWhenAvailable(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("filesystem boundary assertion currently targets the Seatbelt backend")
	}
	launcher := New()
	if !launcher.Info().RestrictedHostAvailable {
		t.Skip("restricted_host backend unavailable")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	base, err := os.MkdirTemp(home, ".mender-seatbelt-boundary-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(base) }()
	root := filepath.Join(base, "workspace")
	secretRoot := filepath.Join(base, "other")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(secretRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(secretRoot, "secret.txt")
	if err := os.WriteFile(secret, []byte("TOP-SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd, err := launcher.Command(
		context.Background(),
		runtimegovernancedomain.IsolationRestrictedHost,
		CommandSpec{
			Executable: "/bin/sh",
			Arguments: []string{
				"-c",
				"echo ok > inside.txt; cat inside.txt; if cat \"" + secret + "\" >/dev/null 2>&1; then exit 77; fi",
			},
			WorkingDirectory: root,
			PermittedRoot:    root,
			RootWritable:     true,
			Environment:      []string{"PATH=/usr/bin:/bin", "HOME=" + home, "TMPDIR=/tmp"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		var exitErr interface{ ExitCode() int }
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 77 {
			t.Fatal("restricted_host could read the denied home secret")
		}
		t.Fatalf("restricted command: %v output=%s", err, output)
	}
	if string(output) != "ok\n" {
		t.Fatalf("output = %q", output)
	}
	if _, err := os.Stat(filepath.Join(root, "inside.txt")); err != nil {
		t.Fatalf("workspace write missing: %v", err)
	}
}
