package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	"github.com/orz-i/Mender/backend/internal/buildinfo"
	"github.com/orz-i/Mender/backend/internal/desktop/appdata"
	desktopbinding "github.com/orz-i/Mender/backend/internal/desktop/binding"
	"github.com/orz-i/Mender/backend/internal/desktop/lifecycle"
	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	databasePath, err := appdata.ResolveDatabasePath()
	if err != nil {
		return fmt.Errorf("resolve desktop database: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{
		DatabasePath: databasePath,
	})
	if err != nil {
		cancel()
		return fmt.Errorf("open desktop runtime: %w", err)
	}

	runtimeCloser := lifecycle.NewCloser(runtime.Close)
	shutdown := func() {
		cancel()
		if closeErr := runtimeCloser.Close(); closeErr != nil {
			log.Printf("mender desktop: close runtime: %v", closeErr)
		}
	}
	defer shutdown()

	service := desktopbinding.NewServiceWithDockerActivation(ctx, runtime, func(ctx context.Context) error {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return fmt.Errorf("Docker Desktop home directory unavailable")
		}
		// A fixed, operator-local UNIX socket path. No browser/Tool-provided
		// endpoint, DOCKER_HOST or context may select the Docker daemon.
		socket := "unix://" + filepath.Join(home, ".docker", "run", "docker.sock")
		return runtime.Sandboxes.ActivateDocker(ctx, socket)
	})
	info := buildinfo.Current()
	windowTitle := "Mender"
	if info.Version != "dev" {
		windowTitle = "Mender " + info.Version
	}

	app := application.New(application.Options{
		Name:        "Mender",
		Description: "Tools aggregation runtime and gateway for LLMs and Agents",
		Services: []application.Service{
			application.NewService(service),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		OnShutdown: shutdown,
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	// WKWebView may retain the previous embedded index document across local
	// rebuilds. A per-launch query keeps the document fresh while hashed Vite
	// assets continue to provide normal cache safety.
	startupURL := fmt.Sprintf("/?desktopBoot=%d", time.Now().UnixNano())

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      "main",
		Title:     windowTitle,
		Width:     1240,
		Height:    820,
		MinWidth:  960,
		MinHeight: 640,
		URL:       startupURL,
	})
	window.Center()

	if err := app.Run(); err != nil {
		return fmt.Errorf("run desktop application: %w", err)
	}
	return nil
}
