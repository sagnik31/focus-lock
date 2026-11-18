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

// topAppsLimit is how many quick-add suggestions GetTopBlockedApps returns.
const topAppsLimit = 5

// GetTopBlockedApps returns the most-blocked installed apps as quick-add
// suggestions. It ranks by focus time spent blocked (recorded only when the
// UI can sign, i.e. reduced mode) and then by how often the enforcer closed
// the app (kill_counts, recorded by the Ghost in strong mode too), so the
// list is no longer always empty on an installed, strong-mode machine.
func (a *App) GetTopBlockedApps() ([]sysinfo.AppInfo, error) {
	ranked := rankTopApps(a.Store.GetBlockedDuration(), a.Store.GetKillCounts())
	if len(ranked) == 0 {
		return []sysinfo.AppInfo{}, nil // nothing to resolve; skip the scan
	}

	installed, err := a.cachedInstalledApps()
	if err != nil {
		return []sysinfo.AppInfo{}, err
	}

	// Take the first topAppsLimit ranked names that resolve to an installed
	// app (VPN clients or uninstalled apps are skipped, not counted).
	result := []sysinfo.AppInfo{}
	for _, name := range ranked {
		for _, info := range installed {
			if storage.ValidAppName(info.Exe) && strings.EqualFold(info.Exe, name) {
				result = append(result, info)
				break
			}
		}
		if len(result) == topAppsLimit {
			break
		}
	}
	return result, nil
}

// rankTopApps merges the blocked-duration and kill-count stats (keys compared
// case-insensitively) and returns the app names, most-blocked first: by
// duration, then kills, then name.
func rankTopApps(duration map[string]int64, kills map[string]int) []string {
	type score struct {
		name  string
		dur   int64
		kills int
	}
	byKey := map[string]*score{}
	get := func(name string) *score {
		k := strings.ToLower(name)
		if byKey[k] == nil {
			byKey[k] = &score{name: name}
		}
		return byKey[k]
	}
	for n, d := range duration {
		get(n).dur += d
	}
	for n, c := range kills {
		get(n).kills += c
	}
	list := make([]*score, 0, len(byKey))
	for _, s := range byKey {
		if storage.ValidAppName(s.name) && (s.dur > 0 || s.kills > 0) {
			list = append(list, s)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].dur != list[j].dur {
			return list[i].dur > list[j].dur
		}
		if list[i].kills != list[j].kills {
			return list[i].kills > list[j].kills
		}
		return strings.ToLower(list[i].name) < strings.ToLower(list[j].name)
	})
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = s.name
	}
	return out
}
