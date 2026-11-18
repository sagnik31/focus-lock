package bridge

import (
	"errors"
	"focus-lock/backend/storage"
	"focus-lock/backend/sysinfo"
	"sort"
	"strings"
	"time"
)

// installedAppsTTL is how long GetTopBlockedApps may reuse the last scan.
// A scan walks the registry and app folders and runs PowerShell for icons,
// so it must not run on every UI poll.
const installedAppsTTL = 10 * time.Minute

// GetInstalledApps scans the installed applications (always fresh: the app
// picker's refresh button calls it) and caches the result.
func (a *App) GetInstalledApps() ([]sysinfo.AppInfo, error) {
	apps, err := sysinfo.GetInstalledApps()
	if err == nil {
		a.appsMu.Lock()
		a.appsCache, a.appsAt = apps, time.Now()
		a.appsMu.Unlock()
	}
	return apps, err
}

// cachedInstalledApps returns the last scan if it is recent, else rescans.
func (a *App) cachedInstalledApps() ([]sysinfo.AppInfo, error) {
	a.appsMu.Lock()
	apps, at := a.appsCache, a.appsAt
	a.appsMu.Unlock()
	if apps != nil && time.Since(at) < installedAppsTTL {
		return apps, nil
	}
	return a.GetInstalledApps()
}

// AddApp adds an app to the blocked list
func (a *App) AddApp(appName string) error {
	appName = strings.TrimSpace(appName)
	if appName == "" {
		return errors.New("app name cannot be empty")
	}
	return a.applyOp(storage.Operation{Type: storage.OpAddApp, Name: appName})
}

// RemoveApp removes an app from the blocked list. The lock-integrity check
// (no removal during an active session) is enforced by the policy, which in
// strong mode runs inside the elevated Ghost.
func (a *App) RemoveApp(appName string) error {
	return a.applyOp(storage.Operation{Type: storage.OpRemoveApp, Name: appName})
}

// SetBlockedApps updates the entire list of blocked apps at once.
func (a *App) SetBlockedApps(apps []string) error {
	return a.applyOp(storage.Operation{Type: storage.OpSetApps, Apps: apps})
}

// GetTopBlockedApps returns the top 5 most blocked apps by duration
func (a *App) GetTopBlockedApps() ([]sysinfo.AppInfo, error) {
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

	if len(topApps) == 0 {
		return []sysinfo.AppInfo{}, nil // nothing to resolve; skip the scan
	}

	// Resolve AppInfo
	installed, err := a.cachedInstalledApps()
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
