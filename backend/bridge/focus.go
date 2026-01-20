package bridge

import (
	"fmt"
	"focus-lock/backend/blocking/hosts"
	"focus-lock/backend/obfuscation"
	"focus-lock/backend/scheduler"
	"os"
	"time"
)

// StartFocus starts a focus session for the given duration
func (a *App) StartFocus(seconds int) error {
	a.Store.Load()

	var taskName, ghostExe string

	// Check if a Ghost already exists (e.g., from a schedule)
	if a.Store.Data.GhostTaskName != "" && a.Store.Data.GhostExePath != "" {
		// Reuse existing Ghost - just update the lock time
		taskName = a.Store.Data.GhostTaskName
		ghostExe = a.Store.Data.GhostExePath
	} else {
		// 1. Setup Obfuscation (Copy executable first so path is known)
		currentExe, err := os.Executable()
		if err != nil {
			return err
		}

		taskName = obfuscation.GenerateTaskName() // Returns "FocusLockGhost"
		ghostExe, err = obfuscation.SetupGhostExecutable(currentExe, taskName)
		if err != nil {
			return fmt.Errorf("obfuscation setup failed: %w", err)
		}
	}

	// 2. Update ALL config fields BEFORE spawning Ghost
	a.Store.Data.LockEndTime = time.Now().Add(time.Duration(seconds) * time.Second)
	a.Store.Data.RemainingDuration = time.Duration(seconds) * time.Second
	a.Store.Data.EmergencyUnlocksUsed = 0
	a.Store.Data.GhostTaskName = taskName
	a.Store.Data.GhostExePath = ghostExe
	a.Store.UpdateBlockedStats(a.Store.Data.BlockedApps, seconds)

	// **CRITICAL**: Save BEFORE spawning Ghost so it sees the correct LockEndTime
	if err := a.Store.Save(); err != nil {
		return err
	}

	// 3. Enable Persistence (so reboot works)
	_ = scheduler.EnablePersistence(ghostExe, taskName)

	// 4. Spawn the Ghost Process immediately (if not already running, schtasks /run is idempotent)
	if err := spawnGhost(ghostExe, taskName); err != nil {
		return err
	}

	// Give it a moment to start
	time.Sleep(500 * time.Millisecond)

	// 5. App remains open to show "Focus Active" screen
	return nil
}

// StopFocus ends the current focus session
func (a *App) StopFocus() error {
	// Only allow if time expired?
	// For V1 debug, we allow manual stop.
	a.Store.Load()

	// Check if any schedule is enabled - we'll preserve Ghost if so
	hasEnabledSchedules := false
	for _, s := range a.Store.Data.Schedules {
		if s.Enabled {
			hasEnabledSchedules = true
			break
		}
	}

	// Unblock sites (only for manual lock end, schedules will re-block)
	_ = hosts.Unblock()

	// Only cleanup Ghost if NO enabled schedules exist
	// This preserves the scheduled task for future schedule activations
	if !hasEnabledSchedules {
		taskName := a.Store.Data.GhostTaskName
		exePath := a.Store.Data.GhostExePath

		if taskName != "" {
			_ = scheduler.DisablePersistence(taskName)
		}
		if exePath != "" {
			obfuscation.CleanupGhostExecutable(exePath)
		}

		a.Store.Data.GhostTaskName = ""
		a.Store.Data.GhostExePath = ""
	}
	// If schedules exist, keep GhostTaskName and GhostExePath so Ghost continues running

	a.Store.Data.LockEndTime = time.Time{} // Reset manual lock
	a.Store.Data.RemainingDuration = 0

	if err := a.Store.Save(); err != nil {
		return err
	}
	return nil
}

// EmergencyUnlock temporarily pauses enforcement (limited uses per session)
func (a *App) EmergencyUnlock() error {
	a.Store.Load()

	if a.Store.Data.EmergencyUnlocksUsed >= 2 {
		return fmt.Errorf("emergency unlock limit reached (2/2)")
	}

	a.Store.Data.PausedUntil = time.Now().Add(1 * time.Minute)
	a.Store.Data.EmergencyUnlocksUsed++

	return a.Store.Save()
}
