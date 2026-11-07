package bridge

import (
	"context"
	"encoding/json"
	"focus-lock/backend/blocking/hosts"
	"focus-lock/backend/clock"
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

	// Strong mode: the elevated Ghost owns the config and all enforcement.
	// The non-elevated UI must never write the config or run the enforcer
	// loop (it cannot sign). Its only job here is to make sure the Ghost is
	// running; everything else is driven through the request channel.
	if !a.Store.CanSign() {
		if !isGhostProcessRunning() && a.Store.Data.GhostExePath != "" && a.Store.Data.GhostTaskName != "" {
			_ = spawnGhost(a.Store.Data.GhostExePath, a.Store.Data.GhostTaskName)
		}
		return
	}

	manualActive := !a.Store.Data.LockEndTime.IsZero() && clock.Default.Now().Before(a.Store.Data.LockEndTime)
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
		save := false
		// Nothing is being enforced, so any enforcement warning is stale (it was
		// written during a past session). Clear it, otherwise the UI keeps showing
		// a leftover banner between sessions even though nothing is wrong now.
		if a.Store.Data.LastEnforcementError != "" {
			a.Store.Data.LastEnforcementError = ""
			save = true
		}
		if a.Store.Data.GhostTaskName != "" {
			_ = scheduler.DisablePersistence(a.Store.Data.GhostTaskName)
			a.Store.Data.GhostTaskName = ""
			a.Store.Data.GhostExePath = ""
			save = true
		}
		if save {
			a.Store.Save()
		}
	} else {
		// There is something to enforce now (an active manual lock or an active
		// schedule) or an enabled schedule that will fire later. Make sure the Ghost
		// is running. This is the immediate-recovery path when the UI is open: if the
		// Ghost was killed, we relaunch it here instead of waiting up to a minute for
		// the scheduled task's repetition trigger to do it.
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
						a.Store.Data.LastEnforcementError = ""
						a.Store.Save()
						// Only (re-)register when a correct task isn't already
						// installed: the non-elevated UI can't overwrite one, so
						// attempting it would just fail with "Access is denied".
						if !scheduler.PersistenceCurrent(ghostExe, taskName) {
							a.warnEnforcement("Reboot persistence needs a one-time admin setup (run scripts\\setup_persistence.ps1 as administrator)", scheduler.EnablePersistence(ghostExe, taskName))
						}
						a.warnEnforcement("Enforcement process failed to start", spawnGhost(ghostExe, taskName))
					}
				}
			} else {
				// Ghost was set up before (e.g., before reboot) but isn't running.
				// Re-spawn it using the existing task.
				a.warnEnforcement("Enforcement process failed to start", spawnGhost(a.Store.Data.GhostExePath, a.Store.Data.GhostTaskName))
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

// GetNow returns the enforcer's trusted time as Unix milliseconds. The frontend
// polls this and offsets its own clock by the difference, so the countdown and
// lock view reflect the same (NTP-anchored, tamper-resistant) time the Ghost
// enforces with — moving the system clock can't make the UI disagree with
// enforcement.
func (a *App) GetNow() int64 {
	return clock.Default.Now().UnixMilli()
}

// GetInsights returns the reflection data for the Insights view as a JSON
// string (same string-return pattern as ExportSettings, so it needs no
// generated model). It computes streaks with the shared, tested Go logic and
// ranks the most-killed apps, so the UI just renders.
func (a *App) GetInsights() (string, error) {
	a.Store.Load()
	st := a.Store.Data.Stats

	cur, longest := storage.StreakFromDays(st.FocusDays, clock.Default.Now())

	type appStat struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	top := make([]appStat, 0, len(st.KillCounts))
	for name, count := range st.KillCounts {
		top = append(top, appStat{Name: name, Count: count})
	}
	sort.Slice(top, func(i, j int) bool {
		if top[i].Count != top[j].Count {
			return top[i].Count > top[j].Count
		}
		return top[i].Name < top[j].Name
	})
	if len(top) > 8 {
		top = top[:8]
	}

	hours := st.KillsByHour
	if len(hours) != 24 {
		hours = make([]int, 24)
	}

	// Most recent sessions first, capped for display.
	recent := make([]storage.SessionRecord, 0, 10)
	for i := len(st.Sessions) - 1; i >= 0 && len(recent) < 10; i-- {
		recent = append(recent, st.Sessions[i])
	}

	out := map[string]interface{}{
		"current_streak":      cur,
		"longest_streak":      longest,
		"total_focus_seconds": st.TotalFocusSeconds,
		"session_count":       len(st.Sessions),
		"kills_by_hour":       hours,
		"top_killed":          top,
		"recent_sessions":     recent,
	}
	b, err := json.Marshal(out)
	return string(b), err
}

// applyOp routes a config mutation through the correct path for the current
// mode. In reduced mode (this process can sign) it validates with the shared
// policy and saves directly. In strong mode the non-elevated UI cannot sign,
// so it hands the operation to the elevated Ghost and waits for the verdict —
// the Ghost re-validates with the same policy, so a tampered or hand-crafted
// request can never weaken an active lock.
func (a *App) applyOp(op storage.Operation) error {
	if a.Store.CanSign() {
		a.Store.Load()
		if err := storage.ApplyOperation(&a.Store.Data, op, clock.Default.Now()); err != nil {
			return err
		}
		return a.Store.Save()
	}
	return a.Store.SubmitRequest(op, 3*time.Second)
}

// warnEnforcement records a user-visible warning when an enforcement setup
// step fails, so the UI can tell the user that blocking may not be active.
// A nil error is a no-op (it does not clear an existing warning; callers reset
// LastEnforcementError to "" before a fresh attempt).
func (a *App) warnEnforcement(context string, err error) {
	if err == nil {
		return
	}
	a.Store.Data.LastEnforcementError = context + ": " + err.Error()
	_ = a.Store.Save()
}
