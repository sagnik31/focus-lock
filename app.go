package main

import (
	"context"
	"fmt"
	"focus-lock/backend/obfuscation"
	"focus-lock/backend/scheduler"
	"focus-lock/backend/storage"
	"focus-lock/backend/sysinfo"
	"os"
	"os/exec"
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
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// --- Exposed Methods ---

func (a *App) GetConfig() storage.Config {
	a.Store.Load()
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

// SetBlockedApps updates the entire list of blocked apps at once.
func (a *App) SetBlockedApps(apps []string) error {
	a.Store.Load()
	a.Store.Data.BlockedApps = apps
	return a.Store.Save()
}

func (a *App) StartFocus(minutes int) error {
	a.Store.Load()
	a.Store.Data.LockEndTime = time.Now().Add(time.Duration(minutes) * time.Minute)
	if err := a.Store.Save(); err != nil {
		return err
	}

	// 1. Setup Obfuscation
	currentExe, err := os.Executable()
	if err != nil {
		return err
	}

	taskName := obfuscation.GenerateTaskName()
	ghostExe, err := obfuscation.SetupGhostExecutable(currentExe, taskName)
	if err != nil {
		return fmt.Errorf("obfuscation setup failed: %w", err)
	}

	// 2. Persist dynamic details
	a.Store.Data.GhostTaskName = taskName
	a.Store.Data.GhostExePath = ghostExe
	if err := a.Store.Save(); err != nil {
		return err
	}

	// 3. Enable Persistence (so reboot works)
	// We ignore error here because we might not have Admin rights in dev mode,
	// but we still want to try.
	_ = scheduler.EnablePersistence(ghostExe, taskName)

	// 4. Spawn the Ghost Process immediately
	if err := spawnGhost(ghostExe); err != nil {
		return err
	}

	// 5. Quit the UI Application
	// Give a small delay for UI feedback if needed, then kill self
	go func() {
		time.Sleep(200 * time.Millisecond)
		os.Exit(0)
	}()

	return nil
}

func (a *App) StopFocus() error {
	// Only allow if time expired?
	// For V1 debug, we allow manual stop.
	a.Store.Load()

	// Cleanup Obfuscation
	taskName := a.Store.Data.GhostTaskName
	exePath := a.Store.Data.GhostExePath

	if taskName != "" {
		scheduler.DisablePersistence(taskName)
	}
	if exePath != "" {
		obfuscation.CleanupGhostExecutable(exePath)
	}

	a.Store.Data.LockEndTime = time.Time{} // Reset
	a.Store.Data.GhostTaskName = ""
	a.Store.Data.GhostExePath = ""

	a.Store.Save()
	return nil
}

func spawnGhost(exePath string) error {
	// We start the same executable with --enforce flag
	cmd := exec.Command(exePath, "--enforce")
	// Detach process so it survives parent exit
	// On Windows, Start() handles this reasonably well, but we don't wait for it.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP, // Detach strictly
	}
	return cmd.Start()
}
