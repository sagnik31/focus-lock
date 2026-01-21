package bridge

import (
	"errors"
	"focus-lock/backend/obfuscation"
	"focus-lock/backend/scheduler"
	"focus-lock/backend/storage"
	"focus-lock/backend/watchdog"
	"os"
	"time"
)

// GetSchedules returns all schedules
func (a *App) GetSchedules() []storage.Schedule {
	a.Store.Load()
	if a.Store.Data.Schedules == nil {
		return []storage.Schedule{}
	}
	return a.Store.Data.Schedules
}

// SaveSchedules saves schedules and spawns Ghost if needed
func (a *App) SaveSchedules(schedules []storage.Schedule) error {
	a.Store.Load()

	// Check for active session
	manualActive := !a.Store.Data.LockEndTime.IsZero() && time.Now().Before(a.Store.Data.LockEndTime)
	scheduleActive := watchdog.IsScheduleActive(a.Store.Data.Schedules)
	isLocked := manualActive || scheduleActive

	if isLocked {
		// Identify disabled or deleted schedules that were previously enabled
		newScheduleMap := make(map[string]storage.Schedule)
		for _, s := range schedules {
			newScheduleMap[s.ID] = s
		}

		for _, oldSch := range a.Store.Data.Schedules {
			if oldSch.Enabled {
				// Check if it exists and is still enabled
				newSch, exists := newScheduleMap[oldSch.ID]
				if !exists {
					return errors.New("cannot delete enabled schedules during an active focus session")
				}
				if !newSch.Enabled {
					return errors.New("cannot disable active schedules during an active focus session")
				}
			}
		}
	}

	a.Store.Data.Schedules = schedules
	if err := a.Store.Save(); err != nil {
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

	// Spawn Ghost if enabled schedules exist but no Ghost is running
	if hasEnabledSchedules && a.Store.Data.GhostTaskName == "" {
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

	return nil
}
