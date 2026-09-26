package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := application.New(application.Options{
		Name:        "Mender",
		Description: "Tools aggregation runtime and gateway for LLMs and Agents",
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      "main",
		Title:     "Mender",
		Width:     1240,
		Height:    820,
		MinWidth:  960,
		MinHeight: 640,
		URL:       "/",
	})
	window.Center()

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
