// Command menderd runs the local Mender management control plane.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	"github.com/orz-i/Mender/backend/internal/buildinfo"
	"github.com/orz-i/Mender/backend/internal/management/httpapi"
	managementserver "github.com/orz-i/Mender/backend/internal/management/server"
)

func main() {
	var databasePath string
	var listenAddress string

	flag.StringVar(&databasePath, "db", "", "path to Mender SQLite database")
	flag.StringVar(&listenAddress, "listen", "127.0.0.1:8080", "loopback management listen address")
	flag.Parse()

	path, err := resolveDatabasePath(databasePath)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "menderd: %v\n", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: path})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "menderd: open runtime: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if err := runtime.Close(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "menderd: close runtime: %v\n", err)
		}
	}()

	handler := httpapi.New(runtime)
	info := buildinfo.Current()
	_, _ = fmt.Fprintf(
		os.Stderr,
		"menderd: %s commit=%s built=%s\n",
		info.Version,
		info.Commit,
		info.BuildDate,
	)
	_, _ = fmt.Fprintf(os.Stderr, "menderd: management API listening on http://%s\n", listenAddress)
	if err := managementserver.Run(ctx, listenAddress, handler); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "menderd: %v\n", err)
		os.Exit(1)
	}
}

func resolveDatabasePath(explicit string) (string, error) {
	path := explicit
	if path == "" {
		path = os.Getenv("MENDER_DB")
	}
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		path = filepath.Join(home, ".mender", "mender.db")
	}

	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve database path: %w", err)
	}
	// #nosec G703 -- --db/MENDER_DB are explicit trusted local operator settings;
	// filepath.Abs normalizes the directory, and no remote request selects it.
	if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
		return "", fmt.Errorf("create database directory: %w", err)
	}
	return absolute, nil
}
