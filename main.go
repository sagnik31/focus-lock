package main

import (
	"embed"
	"fmt"
	"focus-lock/backend/bridge"
	"focus-lock/backend/logging"
	"focus-lock/backend/storage"
	"focus-lock/backend/watchdog"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"golang.org/x/sys/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// The enforcer is deliberately killable. We do NOT deny PROCESS_TERMINATE:
	// if the user ends it from Task Manager, the scheduled task relaunches it
	// within a minute (with the lock state intact), which is what makes killing
	// it pointless without the app behaving like self-defending malware.

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

		// DEBUG LOG: Confirm startup (size-capped; see backend/logging)
		configDir, _ := os.UserConfigDir()
		logging.Write(filepath.Join(configDir, "FocusLock", "ghost_debug.log"),
			fmt.Sprintf("Ghost started with PID %d", os.Getpid()))

		// NOTE: The Ghost is deliberately NOT a critical process. Marking it critical
		// (RtlSetProcessIsCritical) bugchecked Windows (0xEF) whenever anything else
		// ended it: Task Scheduler on AC power loss, shutdown/restart, sign-out.

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
