package bridge

import (
	"fmt"
	"focus-lock/backend/blocking/hosts"
	"focus-lock/backend/obfuscation"
	"focus-lock/backend/scheduler"
	"focus-lock/backend/storage"
	"os"
	"time"
)

// StartFocus starts a focus session for the given duration.
//
// The side effects (set up the obfuscated Ghost exe, ensure the scheduled task
// exists, spawn the Ghost) are the UI's job and need no secret. The actual lock
// state is committed through applyOp: in strong mode that means the just-spawned
// elevated Ghost writes and signs it; in reduced mode the UI signs it directly.
func (a *App) StartFocus(seconds int) error {
	a.Store.Load()

	var taskName, ghostExe string

	// Reuse an existing Ghost (e.g. from a schedule) or set one up.
	if a.Store.Data.GhostTaskName != "" && a.Store.Data.GhostExePath != "" {
		taskName = a.Store.Data.GhostTaskName
		ghostExe = a.Store.Data.GhostExePath
	} else {
		currentExe, err := os.Executable()
		if err != nil {
			return err
		}
		taskName = obfuscation.GenerateTaskName()
		ghostExe, err = obfuscation.SetupGhostExecutable(currentExe, taskName)
		if err != nil {
			return fmt.Errorf("obfuscation setup failed: %w", err)
		}
	}

	// Enable reboot persistence if a correct task isn't already installed.
	if !scheduler.PersistenceCurrent(ghostExe, taskName) {
		a.warnEnforcement("Reboot persistence needs a one-time admin setup (run scripts\\setup_persistence.ps1 as administrator)", scheduler.EnablePersistence(ghostExe, taskName))
	}

	// Spawn the Ghost first so it is running to receive the request (strong
	// mode) and to enforce. schtasks /run is idempotent (single-instance mutex).
	if err := spawnGhost(ghostExe, taskName); err != nil {
		a.warnEnforcement("Enforcement process failed to start", err)
		return err
	}

	// Commit the lock through the policy. In strong mode this hands the
	// operation to the Ghost we just spawned (the 3s timeout covers its start).
	op := storage.Operation{
		Type:          storage.OpStartFocus,
		Seconds:       seconds,
		GhostTaskName: taskName,
		GhostExePath:  ghostExe,
	}
	if err := a.applyOp(op); err != nil {
		return err
	}

	// Record usage stats (reduced mode only — the UI can't sign in strong mode;
	// stats are not security-critical, so we simply skip them there).
	if a.Store.CanSign() {
		a.Store.UpdateBlockedStats(a.Store.Data.BlockedApps, seconds)
	}

	// Give the Ghost a moment to settle before the UI switches screens.
	time.Sleep(500 * time.Millisecond)
	return nil
}

// StopFocus ends the current focus session. The policy refuses this while a
// manual lock is still running (only emergency unlock can pause an active
// lock); it succeeds once the lock has expired, as legitimate cleanup.
func (a *App) StopFocus() error {
	a.Store.Load()

	// Best-effort unblock; in strong mode the Ghost re-manages the hosts file.
	_ = hosts.Unblock()

	if err := a.applyOp(storage.Operation{Type: storage.OpStopFocus}); err != nil {
		return err
	}

	// Reduced mode only: we own the config and the task, so tear the Ghost down
	// when no schedule still needs it. In strong mode the task is admin-owned
	// and stays resident (an idle Ghost is cheap).
	if a.Store.CanSign() {
		a.Store.Load()
		hasEnabledSchedules := false
		for _, s := range a.Store.Data.Schedules {
			if s.Enabled {
				hasEnabledSchedules = true
				break
			}
		}
		if !hasEnabledSchedules {
			if a.Store.Data.GhostTaskName != "" {
				_ = scheduler.DisablePersistence(a.Store.Data.GhostTaskName)
			}
			if a.Store.Data.GhostExePath != "" {
				obfuscation.CleanupGhostExecutable(a.Store.Data.GhostExePath)
			}
			a.Store.Data.GhostTaskName = ""
			a.Store.Data.GhostExePath = ""
			_ = a.Store.Save()
		}
	}
	return nil
}

// EmergencyUnlock temporarily pauses enforcement (limited uses per session).
// The 2-use limit and the counter live in the signed config, so the policy
// enforces them authoritatively in the Ghost.
func (a *App) EmergencyUnlock() error {
	return a.applyOp(storage.Operation{Type: storage.OpEmergencyUnlock})
}
