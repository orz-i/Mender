package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	goRuntime "runtime"
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

//go:embed build/appicon.png
var appIconPNG []byte

//go:embed build/windows/icon.ico
var appIconICO []byte

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

	// Only this trusted native host starts Docker auto-discovery. The
	// worker never creates containers and does not delay desktop startup.
	if err := runtime.Sandboxes.StartDockerAutoDiscovery(ctx); err != nil {
		log.Printf("mender desktop: Docker Engine auto-discovery unavailable: %v", err)
	}
	service := desktopbinding.NewService(ctx, runtime)
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
		Frameless: true,
		Windows: application.WindowsWindow{
			NonClientRegionSupport:            true,
			DisableFramelessWindowDecorations: false,
		},
		Mac: application.MacWindow{
			TitleBar: application.MacTitleBar{
				FullSizeContent: true,
				Hide:            true,
			},
		},
	})
	window.Center()

	tray := app.SystemTray.New()
	icon := appIconPNG
	if goRuntime.GOOS == "windows" && len(appIconICO) > 0 {
		icon = appIconICO
	}
	if len(icon) > 0 {
		tray.SetIcon(icon)
	}
	tray.SetTooltip(windowTitle)
	trayMenu := app.NewMenu()
	trayMenu.Add("Show Mender").OnClick(func(ctx *application.Context) {
		window.Show()
		window.Focus()
	})
	trayMenu.AddSeparator()
	trayMenu.Add("Quit").OnClick(func(ctx *application.Context) {
		app.Quit()
	})
	tray.SetMenu(trayMenu)
	tray.OnClick(func() {
		if window.IsVisible() {
			window.Focus()
		} else {
			window.Show()
			window.Focus()
		}
	})

	if err := app.Run(); err != nil {
		return fmt.Errorf("run desktop application: %w", err)
	}
	return nil
}
