package main

import (
	"context"
	"focus-lock/backend/scheduler"
	"focus-lock/backend/storage"
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

func (a *App) StartFocus(minutes int) error {
	a.Store.Load()
	a.Store.Data.LockEndTime = time.Now().Add(time.Duration(minutes) * time.Minute)
	if err := a.Store.Save(); err != nil {
		return err
	}

	// 1. Enable Persistence (so reboot works)
	// We ignore error here because we might not have Admin rights in dev mode,
	// but we still want to try.
	_ = scheduler.EnablePersistence()

	// 2. Spawn the Ghost Process immediately
	return spawnGhost()
}

func (a *App) StopFocus() error {
	// Only allow if time expired?
	// For V1 debug, we allow manual stop.
	a.Store.Load()
	a.Store.Data.LockEndTime = time.Time{} // Reset
	a.Store.Save()
	scheduler.DisablePersistence()
	return nil
}

func spawnGhost() error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}

	// We start the same executable with --enforce flag
	cmd := exec.Command(exePath, "--enforce")
	// Detach process so it survives parent exit
	// On Windows, Start() handles this reasonably well, but we don't wait for it.
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Start()
}
