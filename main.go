package main

import (
	"embed"
	"fmt"
	"focus-lock/backend/bridge"
	"focus-lock/backend/protection"
	"focus-lock/backend/storage"
	"focus-lock/backend/watchdog"
	"os"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"golang.org/x/sys/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Enable Anti-Termination Protection
	// This prevents the user from killing the process via Task Manager
	if err := protection.ProtectProcess(); err != nil {
		fmt.Printf("Warning: Failed to enable process protection: %v\n", err)
		// We initiate it but don't crash if it fails (e.g. dev environment restrictions)
	}

	// 1. Check for "--enforce" flag (Ghost Mode)
	// We check this FIRST because the Ghost process runs in the background and
	// should not be blocked by the single-instance mutex of the UI.
	if len(os.Args) > 1 && os.Args[1] == "--enforce" {
		// Headless Mode
		store, err := storage.NewStore()
		if err != nil {
			return
		}

		// Ensure only one Ghost runs (Single Instance)
		// This prevents zombie processes from piling up if the UI crashes/restarts
		mutexName, _ := windows.UTF16PtrFromString("Global\\FocusLockGhost")
		handle, err := windows.CreateMutex(nil, true, mutexName)
		if err == nil && windows.GetLastError() == windows.ERROR_ALREADY_EXISTS {
			// Another ghost is active. We can safely exit.
			// The existing ghost will pick up the new config.
			return
		}
		_ = handle // Leak the handle so it stays held until process exit

		// DEBUG LOG: Confirm startup
		configDir, _ := os.UserConfigDir()
		f, _ := os.OpenFile(configDir+"\\FocusLock\\ghost_debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		f.WriteString(fmt.Sprintf("Ghost started at %s with PID %d\n", time.Now().Format(time.RFC3339), os.Getpid()))
		f.Close()

		// Enable Critical Process Status (BSOD if killed)
		if err := protection.SetCritical(true); err != nil {
			fmt.Printf("Failed to set critical status: %v\n", err)
		} else {
			// CRITICAL: Ensure we disable it if we exit gracefully
			// This prevents BSOD when the valid timer expires and we exit.
			defer func() {
				protection.SetCritical(false)
			}()
		}

		store.Load()
		watchdog.StartEnforcer(store, true)
		return
	}

	// 2. Single Instance Lock (UI Mode Only)
	// We use a named mutex to ensure only one instance of the UI runs.
	mutexName, _ := windows.UTF16PtrFromString("Global\\FocusLockMutex")
	handle, err := windows.CreateMutex(nil, true, mutexName)
	if err != nil {
		// If error (access denied etc), we just continue, but...
		// If ERROR_ALREADY_EXISTS, it means another instance holds it.
	}
	// Check if it already existed
	if ferr := windows.GetLastError(); ferr == windows.ERROR_ALREADY_EXISTS {
		// Another instance is running.
		// If we are just launching UI, we might want to bring it to front (TODO).
		// For now, we silently exit to prevent "Multiple Windows" or config corruption.
		// Important: Close handle if we are exiting
		if handle != 0 {
			windows.CloseHandle(handle)
		}
		return
	}
	// Keep handle open until process exits
	// defer windows.CloseHandle(handle) // implied on exit

	if len(os.Args) > 1 && os.Args[1] == "--test-spawn" {
		app := bridge.NewApp()
		fmt.Println("Starting focus for 1 minute (Headless Test)...")
		if err := app.StartFocus(1); err != nil {
			fmt.Println("Error starting focus:", err)
		} else {
			fmt.Println("Focus started successfully. Check for hidden process.")
		}
		return
	}

	// 2. UI Mode
	app := bridge.NewApp()

	err = wails.Run(&options.App{
		Title:  "Focus Lock",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.Startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
