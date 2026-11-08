package bridge

import (
	"focus-lock/backend/scheduler"
	"focus-lock/backend/storage"
	"os"
)

// GetSchedules returns all schedules
func (a *App) GetSchedules() []storage.Schedule {
	a.Store.Load()
	if a.Store.Data.Schedules == nil {
		return []storage.Schedule{}
	}
	return a.Store.Data.Schedules
}

// SaveSchedules saves schedules and spawns Ghost if needed. The lock-integrity
// rules (no deleting/disabling an active schedule during enforcement) are
// enforced by the policy — in strong mode, inside the elevated Ghost.
func (a *App) SaveSchedules(schedules []storage.Schedule) error {
	if err := a.applyOp(storage.Operation{Type: storage.OpSaveSchedules, Schedules: schedules}); err != nil {
		return err
	}

	// Check if any schedule is enabled
	hasEnabledSchedules := false
	for _, s := range schedules {
		if s.Enabled {
			hasEnabledSchedules = true
			break
		}
	}

	// Spawn Ghost if enabled schedules exist but no Ghost is set up yet.
	a.Store.Load()
	if hasEnabledSchedules && a.Store.Data.GhostTaskName == "" {
		// The enforcer runs from the installed app exe itself (no staged copy).
		currentExe, err := os.Executable()
		if err == nil {
			taskName := scheduler.EnforcerTaskName
			ghostExe := currentExe
			// Skip re-registration when a correct task already exists; the
			// non-elevated UI can't overwrite it (would be "Access is denied").
			if !scheduler.PersistenceCurrent(ghostExe, taskName) {
				a.warnEnforcement("Reboot persistence needs a one-time admin setup (run scripts\\install_setup.ps1 as administrator)", scheduler.EnablePersistence(ghostExe, taskName))
			}
			// Spawn first so a strong-mode enforcer is up to receive the next op.
			a.warnEnforcement("Enforcement process failed to start", spawnGhost(ghostExe, taskName))
			// Record where the enforcer lives (routed through the policy).
			_ = a.applyOp(storage.Operation{Type: storage.OpSetGhostInfo, GhostTaskName: taskName, GhostExePath: ghostExe})
		}
	}

	return nil
}
