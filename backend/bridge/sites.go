package bridge

import (
	"errors"
	"fmt"
	"focus-lock/backend/blocking/hosts"
	"focus-lock/backend/watchdog"
	"sort"
	"time"
)

// GetBlockedSites returns the list of blocked websites
func (a *App) GetBlockedSites() []string {
	a.Store.Load()
	sort.Strings(a.Store.Data.BlockedSites)
	return a.Store.Data.BlockedSites
}

// AddBlockedSite adds a website to the blocked list
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

// RemoveBlockedSite removes a website from the blocked list
func (a *App) RemoveBlockedSite(url string) error {
	a.Store.Load()

	// Check if session is active - prevent removal during active session
	manualActive := !a.Store.Data.LockEndTime.IsZero() && time.Now().Before(a.Store.Data.LockEndTime)
	scheduleActive := watchdog.IsScheduleActive(a.Store.Data.Schedules)
	if manualActive || scheduleActive {
		return errors.New("cannot remove sites during an active focus session")
	}

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

// AddBlockedSites adds multiple websites to the blocked list
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

// RemoveBlockedSites removes multiple websites from the blocked list
func (a *App) RemoveBlockedSites(urls []string) error {
	a.Store.Load()

	// Check if session is active - prevent removal during active session
	manualActive := !a.Store.Data.LockEndTime.IsZero() && time.Now().Before(a.Store.Data.LockEndTime)
	scheduleActive := watchdog.IsScheduleActive(a.Store.Data.Schedules)
	if manualActive || scheduleActive {
		return errors.New("cannot remove sites during an active focus session")
	}

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

// SetBlockCommonVPN enables or disables blocking of common VPN sites
func (a *App) SetBlockCommonVPN(enabled bool) error {
	a.Store.Load()
	a.Store.Data.BlockCommonVPN = enabled
	return a.Store.Save()
}

// GetBlockCommonVPN returns whether common VPN blocking is enabled
func (a *App) GetBlockCommonVPN() bool {
	a.Store.Load()
	return a.Store.Data.BlockCommonVPN
}
