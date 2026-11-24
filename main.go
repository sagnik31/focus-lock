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
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// The enforcer is deliberately killable. We do NOT deny PROCESS_TERMINATE:
	// if the user ends it from Task Manager, the scheduled task relaunches it
	// within a minute (with the lock state intact), which is what makes killing
	// it pointless without the app behaving like self-defending malware.

	// 0. "--session-active": used by the installer to refuse an install/update
	// mid-session. Exits 3 if a manual lock or schedule window is active, 0 if
	// not. Never starts the UI or takes either mutex.
	if len(os.Args) > 1 && os.Args[1] == "--session-active" {
		os.Exit(sessionActiveExitCode())
	}

	// 0b. "--apply-update": run elevated by the "FocusLock Updater" task to
	// install a newer, signed release without a UAC prompt (apply_update.go).
	// Never starts the UI or takes either mutex.
	if len(os.Args) > 1 && os.Args[1] == "--apply-update" {
		os.Exit(applyUpdateExitCode())
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
		if watchdog.AcquireEnforcer(store.SecureDir(), store.StrongMode(), store.CanSign()) {
			// Another ghost is active. We can safely exit.
			// The existing ghost will pick up the new config.
			return
		}

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
	if watchdog.AcquireInstance(watchdog.UIMutex) {
		// Another instance is running.
		// If we are just launching UI, we might want to bring it to front (TODO).
		// For now, we silently exit to prevent "Multiple Windows" or config corruption.
		return
	}

	// 2. UI Mode
	app := bridge.NewApp()

	err := wails.Run(&options.App{
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
