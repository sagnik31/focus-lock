package main

import (
	"embed"
	"fmt"
	"focus-lock/backend/storage"
	"focus-lock/backend/watchdog"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// 1. Check for "--enforce" flag
	if len(os.Args) > 1 && os.Args[1] == "--enforce" {
		// Headless Mode
		store, err := storage.NewStore()
		if err != nil {
			return
		}
		store.Load()
		watchdog.StartEnforcer(store)
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "--test-spawn" {
		app := NewApp()
		fmt.Println("Starting focus for 1 minute (Headless Test)...")
		if err := app.StartFocus(1); err != nil {
			fmt.Println("Error starting focus:", err)
		} else {
			fmt.Println("Focus started successfully. Check for hidden process.")
		}
		// Sleep briefly to allow spawn to authenticate/detach if needed
		// time.Sleep(2 * time.Second)
		return
	}

	// 2. UI Mode
	app := NewApp()

	err := wails.Run(&options.App{
		Title:  "Focus Lock",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
