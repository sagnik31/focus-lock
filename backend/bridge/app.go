package bridge

import (
	"context"
	"focus-lock/backend/blocking/hosts"
	"focus-lock/backend/obfuscation"
	"focus-lock/backend/scheduler"
	"focus-lock/backend/storage"
	"focus-lock/backend/watchdog"
	"os"
	"sort"
	"time"
)

// App struct represents the main application
type App struct {
	ctx   context.Context
	Store *storage.Store
}

// NewApp creates a new App application struct
func NewApp() *App {
	store, _ := storage.NewStore()
	store.Load() // Ignore error, defaults are fine
	return &App{
		Store: store,
	}
}

// Startup is called when the app starts.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx

	// Startup Cleanup / Sanity Check
	a.Store.Load()
	manualActive := !a.Store.Data.LockEndTime.IsZero() && time.Now().Before(a.Store.Data.LockEndTime)
	scheduleActive := watchdog.IsScheduleActive(a.Store.Data.Schedules)

	// Check if any schedule is enabled (not just currently active)
	hasEnabledSchedules := false
	for _, s := range a.Store.Data.Schedules {
		if s.Enabled {
			hasEnabledSchedules = true
			break
		}
	}

	if !manualActive && !scheduleActive && !hasEnabledSchedules {
		// No active lock and no enabled schedules. Force cleanup.
		_ = hosts.Unblock()
		if a.Store.Data.GhostTaskName != "" {
			_ = scheduler.DisablePersistence(a.Store.Data.GhostTaskName)
			a.Store.Data.GhostTaskName = ""
			a.Store.Data.GhostExePath = ""
			a.Store.Save()
		}
	} else if hasEnabledSchedules {
		// Check if Ghost is actually running (it may have exited or never started after reboot)
		ghostRunning := isGhostProcessRunning()

		if !ghostRunning {
			// Ghost is NOT running. We need to spawn one.
			// Check if Ghost executable exists (it may have been deleted/cleaned up)
			ghostExeExists := false
			if a.Store.Data.GhostExePath != "" {
				if _, err := os.Stat(a.Store.Data.GhostExePath); err == nil {
					ghostExeExists = true
				}
			}

			if a.Store.Data.GhostTaskName == "" || !ghostExeExists {
				// No Ghost was ever set up OR the exe is missing. Create a new one.
				currentExe, err := os.Executable()
				if err == nil {
					taskName := obfuscation.GenerateTaskName()
					ghostExe, err := obfuscation.SetupGhostExecutable(currentExe, taskName)
					if err == nil {
						a.Store.Data.GhostTaskName = taskName
						a.Store.Data.GhostExePath = ghostExe
						a.Store.Save()
						_ = scheduler.EnablePersistence(ghostExe, taskName)
						_ = spawnGhost(ghostExe, taskName)
					}
				}
			} else {
				// Ghost was set up before (e.g., before reboot) but isn't running.
				// Re-spawn it using the existing task.
				_ = spawnGhost(a.Store.Data.GhostExePath, a.Store.Data.GhostTaskName)
			}
		}
	}

	// Start the Enforcer in the background of the UI process
	go watchdog.StartEnforcer(a.Store, false)
}

// GetConfig returns the current configuration
func (a *App) GetConfig() storage.Config {
	a.Store.Load()
	sort.Strings(a.Store.Data.BlockedApps)
	return a.Store.Data
}
