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
	} else if hasEnabledSchedules && a.Store.Data.GhostTaskName == "" {
		// Enabled schedule(s) exist but no Ghost is set up. Spawn one now.
		// This ensures enforcement persists even if user closes the UI before schedule activates.
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
