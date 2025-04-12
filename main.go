package main

import (
	"embed"
	"fmt"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

var appName = "TiDB Desktop"
var version = "0.1.0"

func main() {
	// Create an instance of the app structure
	app := NewApp()

	err := wails.Run(&options.App{
		Title:  appName,
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 255, G: 255, B: 255, A: 0},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []any{
			app,
		},
		Mac: &mac.Options{
			About: &mac.AboutInfo{
				Title:   fmt.Sprintf("%s %s", appName, version),
				Message: "A modern lightweight TiDB desktop client.\n\nCopyright © 2025",
			},
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
