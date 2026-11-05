package localworkspace_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/orz-i/Mender/backend/internal/agent/infrastructure/localworkspace"
)

func TestVerifierObservesDirectoryAndRegularFile(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "read.txt")
	if err := os.WriteFile(file, []byte("read preview"), 0o600); err != nil {
		t.Fatal(err)
	}
	verifier := localworkspace.New()
	ctx := context.Background()
	for _, path := range []string{root, file} {
		want, err := filepath.EvalSymlinks(path)
		if err != nil {
			t.Fatal(err)
		}
		got, err := verifier.ResolvePath(ctx, path)
		if err != nil || got != want {
			t.Fatalf("ResolvePath(%q) = %q, %v; want %q", path, got, err, want)
		}
	}
	for _, test := range []struct {
		name      string
		path      string
		directory bool
		regular   bool
	}{
		{name: "directory", path: root, directory: true},
		{name: "file", path: file, regular: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory, err := verifier.IsDirectory(ctx, test.path)
			if err != nil || directory != test.directory {
				t.Fatalf("IsDirectory = %v, %v; want %v", directory, err, test.directory)
			}
			regular, err := verifier.IsRegularFile(ctx, test.path)
			if err != nil || regular != test.regular {
				t.Fatalf("IsRegularFile = %v, %v; want %v", regular, err, test.regular)
			}
		})
	}
}

func TestVerifierPreservesMissingPathErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	verifier := localworkspace.New()
	for name, observe := range map[string]func() error{
		"resolve": func() error {
			_, err := verifier.ResolvePath(context.Background(), missing)
			return err
		},
		"directory": func() error {
			_, err := verifier.IsDirectory(context.Background(), missing)
			return err
		},
		"regular file": func() error {
			_, err := verifier.IsRegularFile(context.Background(), missing)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := observe()
			if !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("missing path error = %v; want os.ErrNotExist", err)
			}
			var pathError *os.PathError
			if !errors.As(err, &pathError) {
				t.Fatalf("missing path error lost filesystem details: %v", err)
			}
		})
	}
}

func TestVerifierObservesDirectoryReplacementWithoutAuthorizingTarget(t *testing.T) {
	root := t.TempDir()
	approved := filepath.Join(root, "approved")
	outside := filepath.Join(root, "outside")
	for _, path := range []string{approved, outside} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	file := filepath.Join(outside, "read.txt")
	if err := os.WriteFile(file, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	verifier := localworkspace.New()
	ctx := context.Background()
	original, err := verifier.ResolvePath(ctx, approved)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(approved); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, approved); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("Windows cannot create the directory symlink: %v", err)
		}
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(outside)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := verifier.ResolvePath(ctx, approved)
	if err != nil || resolved != want || resolved == original {
		t.Fatalf("replaced root = %q, %v; want external target %q, original %q", resolved, err, want, original)
	}
	if directory, err := verifier.IsDirectory(ctx, approved); err != nil || !directory {
		t.Fatalf("symlink directory = %v, %v", directory, err)
	}
	if regular, err := verifier.IsRegularFile(ctx, filepath.Join(approved, "read.txt")); err != nil || !regular {
		t.Fatalf("symlink target file = %v, %v", regular, err)
	}
	// The application must compare the observed target with the approved root;
	// the filesystem adapter does not decide whether this external target is safe.
}

func TestVerifierHonorsCanceledContextBeforeFilesystemAccess(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, stop := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer stop()
	verifier := localworkspace.New()
	for _, test := range []struct {
		name string
		ctx  context.Context
		want error
	}{
		{name: "canceled", ctx: canceled, want: context.Canceled},
		{name: "expired", ctx: expired, want: context.DeadlineExceeded},
	} {
		t.Run(test.name, func(t *testing.T) {
			if resolved, err := verifier.ResolvePath(test.ctx, missing); !errors.Is(err, test.want) || resolved != "" {
				t.Fatalf("ResolvePath = %q, %v; want context error %v", resolved, err, test.want)
			}
			if directory, err := verifier.IsDirectory(test.ctx, missing); !errors.Is(err, test.want) || directory {
				t.Fatalf("IsDirectory = %v, %v; want context error %v", directory, err, test.want)
			}
			if regular, err := verifier.IsRegularFile(test.ctx, missing); !errors.Is(err, test.want) || regular {
				t.Fatalf("IsRegularFile = %v, %v; want context error %v", regular, err, test.want)
			}
		})
	}
}
