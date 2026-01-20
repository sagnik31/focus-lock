package bridge

import (
	"context"
	"fmt"
	"focus-lock/backend/blocking/hosts"
	"focus-lock/backend/obfuscation"
	"focus-lock/backend/scheduler"
	"focus-lock/backend/storage"
	"focus-lock/backend/sysinfo"
	"focus-lock/backend/watchdog"
	"os"
	"os/exec"
	"sort"
	"strings"
	"syscall"
	"time"
)

// App struct
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

// startup is called when the app starts.
// Capitalized startup to Export it (needed if called from main package?)
// Wails usually takes a method, checking main.go...
// main.go uses app.startup. If I move it, it needs to be public?
// Actually, Wails binds the struct instance. The OnStartup lifecycle callback is passed manually.
// So yes, Startup (capital S) or just call it Startup.
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

// --- Exposed Methods ---

func (a *App) GetConfig() storage.Config {
	a.Store.Load()
	sort.Strings(a.Store.Data.BlockedApps)
	return a.Store.Data
}

func (a *App) GetInstalledApps() ([]sysinfo.AppInfo, error) {
	return sysinfo.GetInstalledApps()
}

func (a *App) AddApp(appName string) error {
	a.Store.Load()
	// Check duplicate
	for _, existing := range a.Store.Data.BlockedApps {
		if existing == appName {
			return nil
		}
	}
	a.Store.Data.BlockedApps = append(a.Store.Data.BlockedApps, appName)
	sort.Strings(a.Store.Data.BlockedApps)
	return a.Store.Save()
}

func (a *App) RemoveApp(appName string) error {
	a.Store.Load()
	newApps := []string{}
	for _, existing := range a.Store.Data.BlockedApps {
		if existing != appName {
			newApps = append(newApps, existing)
		}
	}
	a.Store.Data.BlockedApps = newApps
	return a.Store.Save()
}

// --- Website Blocking Methods ---

func (a *App) GetBlockedSites() []string {
	a.Store.Load()
	sort.Strings(a.Store.Data.BlockedSites)
	return a.Store.Data.BlockedSites
}

func (a *App) AddBlockedSite(url string) error {
	a.Store.Load()
	// Simple duplicate check
	for _, existing := range a.Store.Data.BlockedSites {
		if existing == url {
			return nil
		}
	}
	a.Store.Data.BlockedSites = append(a.Store.Data.BlockedSites, url)
	sort.Strings(a.Store.Data.BlockedSites)

	// Try to update hosts immediately (best effort)
	// If it fails (User mode), ignore it. Ghost will handle it.
	if err := hosts.Block(a.Store.Data.BlockedSites); err != nil {
		fmt.Println("Warning: Failed to block sites immediately (likely Permission Denied):", err)
	}

	return a.Store.Save()
}

func (a *App) RemoveBlockedSite(url string) error {
	a.Store.Load()
	newSites := []string{}
	for _, existing := range a.Store.Data.BlockedSites {
		if existing != url {
			newSites = append(newSites, existing)
		}
	}
	a.Store.Data.BlockedSites = newSites

	// Try to update hosts immediately (best effort)
	if err := hosts.Block(a.Store.Data.BlockedSites); err != nil {
		fmt.Println("Warning: Failed to unblock sites immediately:", err)
	}

	return a.Store.Save()
}

func (a *App) AddBlockedSites(urls []string) error {
	a.Store.Load()
	existingMap := make(map[string]bool)
	for _, s := range a.Store.Data.BlockedSites {
		existingMap[s] = true
	}

	changed := false
	for _, url := range urls {
		if !existingMap[url] {
			a.Store.Data.BlockedSites = append(a.Store.Data.BlockedSites, url)
			existingMap[url] = true
			changed = true
		}
	}

	if !changed {
		return nil
	}
	sort.Strings(a.Store.Data.BlockedSites)

	// Try to update hosts immediately (best effort)
	if err := hosts.Block(a.Store.Data.BlockedSites); err != nil {
		fmt.Println("Warning: Failed to block sites immediately:", err)
	}

	return a.Store.Save()
}

func (a *App) RemoveBlockedSites(urls []string) error {
	a.Store.Load()
	toRemove := make(map[string]bool)
	for _, url := range urls {
		toRemove[url] = true
	}

	newSites := []string{}
	for _, existing := range a.Store.Data.BlockedSites {
		if !toRemove[existing] {
			newSites = append(newSites, existing)
		}
	}

	if len(newSites) == len(a.Store.Data.BlockedSites) {
		return nil
	}

	a.Store.Data.BlockedSites = newSites
	return a.Store.Save()
}

func (a *App) SetBlockCommonVPN(enabled bool) error {
	a.Store.Load()
	a.Store.Data.BlockCommonVPN = enabled
	return a.Store.Save()
}

func (a *App) GetBlockCommonVPN() bool {
	a.Store.Load()
	return a.Store.Data.BlockCommonVPN
}

// --- Schedule Methods ---

func (a *App) GetSchedules() []storage.Schedule {
	a.Store.Load()
	if a.Store.Data.Schedules == nil {
		return []storage.Schedule{}
	}
	return a.Store.Data.Schedules
}

func (a *App) SaveSchedules(schedules []storage.Schedule) error {
	a.Store.Load()
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

// SetBlockedApps updates the entire list of blocked apps at once.
func (a *App) SetBlockedApps(apps []string) error {
	a.Store.Load()
	sort.Strings(apps)
	a.Store.Data.BlockedApps = apps
	return a.Store.Save()
}

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

func (a *App) StopFocus() error {
	// Only allow if time expired?
	// For V1 debug, we allow manual stop.
	a.Store.Load()

	// Cleanup Obfuscation
	taskName := a.Store.Data.GhostTaskName
	exePath := a.Store.Data.GhostExePath

	// Unblock sites and apps
	// Ignore errors (User mode)
	_ = hosts.Unblock()

	// Disable persistence immediately
	if taskName != "" {
		_ = scheduler.DisablePersistence(taskName)
	}
	if exePath != "" {
		obfuscation.CleanupGhostExecutable(exePath)
	}

	a.Store.Data.LockEndTime = time.Time{} // Reset
	a.Store.Data.RemainingDuration = 0
	a.Store.Data.GhostTaskName = ""
	a.Store.Data.GhostExePath = ""

	hosts.Unblock()

	if err := a.Store.Save(); err != nil {
		return err
	}
	return nil
}

func (a *App) EmergencyUnlock() error {
	a.Store.Load()

	if a.Store.Data.EmergencyUnlocksUsed >= 2 {
		return fmt.Errorf("emergency unlock limit reached (2/2)")
	}

	a.Store.Data.PausedUntil = time.Now().Add(1 * time.Minute)
	a.Store.Data.EmergencyUnlocksUsed++

	return a.Store.Save()
}

func (a *App) GetTopBlockedApps() ([]sysinfo.AppInfo, error) {
	// a.Store.Load() // Not needed if we use safe getter, but good for refresh
	durationMap := a.Store.GetBlockedDuration()

	type kv struct {
		Key      string
		Duration int64
	}

	var ss []kv
	for k, v := range durationMap {
		ss = append(ss, kv{k, v})
	}

	sort.Slice(ss, func(i, j int) bool {
		if ss[i].Duration != ss[j].Duration {
			return ss[i].Duration > ss[j].Duration // Duration Descending
		}
		return strings.ToLower(ss[i].Key) < strings.ToLower(ss[j].Key) // Name Ascending (Alphabetical)
	})

	// Get top 5
	topCount := 5
	if len(ss) < topCount {
		topCount = len(ss)
	}

	topApps := []string{}
	for i := 0; i < topCount; i++ {
		topApps = append(topApps, ss[i].Key)
	}

	// Resolve AppInfo
	installed, err := sysinfo.GetInstalledApps()
	if err != nil {
		return []sysinfo.AppInfo{}, err // Return empty slice on error
	}

	result := []sysinfo.AppInfo{} // Initialize as empty slice
	for _, appName := range topApps {
		for _, info := range installed {
			if strings.EqualFold(info.Exe, appName) {
				result = append(result, info)
				break
			}
		}
	}

	return result, nil
}

func spawnGhost(exePath, taskName string) error {
	// 1. Try to launch via Scheduled Task (for Admin privileges without UAC)
	// This only works if the task was created previously (e.g. by installer or Admin setup).
	// We use "schtasks /run" which doesn't trigger UAC if the task is already set up.
	if err := exec.Command("schtasks", "/run", "/tn", taskName).Run(); err == nil {
		fmt.Println("Ghost spawned via Scheduled Task (Admin Mode).")
		return nil
	}

	// 2. Fallback: Direct spawn (User Mode)
	// This won't be able to block websites, but will handle other logic or fail gracefully.
	fmt.Println("Warning: Failed to run Scheduled Task (falling back to User Mode spawn). Blocking may fail.")
	cmd := exec.Command(exePath, "--enforce")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | 0x00000008 | 0x01000000,
	}
	return cmd.Start()
}
