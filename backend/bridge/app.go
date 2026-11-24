package bridge

import (
	"context"
	"encoding/json"
	"focus-lock/backend/blocking/hosts"
	"focus-lock/backend/clock"
	"focus-lock/backend/scheduler"
	"focus-lock/backend/storage"
	"focus-lock/backend/sysinfo"
	"focus-lock/backend/watchdog"
	"os"
	"sort"
	"sync"
	"time"
)

// App struct represents the main application
type App struct {
	ctx   context.Context
	Store *storage.Store

	updOnce sync.Once
	upd     *updateManager // in-app updates (update.go); use a.updates()

	// Last installed-apps scan (apps.go), reused by GetTopBlockedApps.
	appsMu    sync.Mutex
	appsCache []sysinfo.AppInfo
	appsAt    time.Time

	// UI-local enforcement warning for strong mode (warning.go).
	warnMu       sync.Mutex
	localWarning string
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
		a.clearLocalWarning()
		startClock() // StartEnforcer does this in reduced mode
		// Always ask the task to run: it ignores the request while the
		// enforcer is alive, and any process could hold a mutex that only
		// looks like the enforcer's.
		currentExe, err := os.Executable()
		if err == nil {
			err = spawnEnforcer(currentExe, scheduler.EnforcerTaskName)
		}
		a.warnEnforcement("Enforcement process failed to start", err)
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
		clearWarning, clearTask := false, false
		// Nothing is being enforced, so any enforcement warning is stale (it was
		// written during a past session). Clear it, otherwise the UI keeps showing
		// a leftover banner between sessions even though nothing is wrong now.
		if a.Store.Data.LastEnforcementError != "" {
			clearWarning = true
		}
		if a.Store.Data.GhostTaskName != "" {
			_ = scheduler.DisablePersistence(a.Store.Data.GhostTaskName)
			clearTask = true
		}
		if clearWarning || clearTask {
			_ = a.Store.UpdateAtomic(func(cfg *storage.Config) {
				if clearWarning {
					cfg.LastEnforcementError = ""
				}
				if clearTask {
					cfg.GhostTaskName = ""
					cfg.GhostExePath = ""
				}
			})
		}
	} else {
		// There is something to enforce now (an active manual lock or an active
		// schedule) or an enabled schedule that will fire later. Make sure the Ghost
		// is running. This is the immediate-recovery path when the UI is open: if the
		// Ghost was killed, we relaunch it here instead of waiting up to a minute for
		// the scheduled task's repetition trigger to do it.
		if !ghostRunning() {
			// The enforcer runs from the installed app exe itself (no staged
			// copy). Spawn it and make sure reboot persistence is in place.
			currentExe, err := os.Executable()
			if err == nil {
				taskName := scheduler.EnforcerTaskName
				_ = a.Store.UpdateAtomic(func(cfg *storage.Config) {
					cfg.GhostTaskName = taskName
					cfg.GhostExePath = currentExe
					cfg.LastEnforcementError = ""
				})
				// Only (re-)register when a correct task isn't already installed:
				// the non-elevated UI can't overwrite one, so attempting it would
				// just fail with "Access is denied".
				if !scheduler.PersistenceCurrent(currentExe, taskName) {
					a.warnEnforcement("Reboot persistence needs a one-time admin setup (run scripts\\install_setup.ps1 as administrator)", scheduler.EnablePersistence(currentExe, taskName))
				}
				a.warnEnforcement("Enforcement process failed to start", spawnEnforcer(currentExe, taskName))
			}
		}
	}

	// Start the Enforcer in the background of the UI process
	go watchdog.StartEnforcer(a.Store, false)
}

// GetConfig returns the current configuration
func (a *App) GetConfig() storage.Config {
	a.Store.Load()
	cfg := a.withLocalWarning(a.Store.Snapshot())
	sort.Strings(cfg.BlockedApps)
	return cfg
}

// Seams for tests.
var (
	spawnEnforcer = spawnGhost
	ghostRunning  = isGhostProcessRunning
	startClock    = func() { clock.Default.Start(5*time.Minute, nil) }
)

// GetNow returns the enforcer's trusted time as Unix milliseconds. The frontend
// polls this and offsets its own clock by the difference, so the countdown and
// lock view reflect the same (NTP-anchored, tamper-resistant) time the Ghost
// enforces with — moving the system clock can't make the UI disagree with
// enforcement.
func (a *App) GetNow() int64 {
	return clock.Default.Now().UnixMilli()
}

// GetLockState returns the lock in force now as a JSON string:
// {"kind": "manual"|"schedule"|"none", "end_time": RFC 3339 or ""}. The end
// time is whichever of the manual lock and the active schedule windows ends
// later, computed with the same schedule logic the enforcer uses (overnight
// windows included), so the lock screen's countdown can't drift from it.
func (a *App) GetLockState() (string, error) {
	a.Store.Load()
	cfg := a.Store.Snapshot()
	return lockStateJSON(storage.LockStateAt(&cfg, clock.Default.Now()))
}

func lockStateJSON(st storage.LockState) (string, error) {
	end := ""
	if !st.EndTime.IsZero() {
		end = st.EndTime.Format(time.RFC3339Nano)
	}
	b, err := json.Marshal(map[string]string{"kind": st.Kind, "end_time": end})
	return string(b), err
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
		// ApplyRequest reloads, runs the policy and saves under the store
		// lock. A separate Load / mutate / Save here raced the in-process
		// enforcer's own read-modify-writes and could lose the change.
		return a.Store.ApplyRequest(op, clock.Default.Now())
	}
	return a.Store.SubmitRequest(op, 3*time.Second)
}
